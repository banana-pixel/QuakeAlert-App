// Command quakealert adalah entrypoint server backend QuakeAlert.
// Bertanggung jawab: bootstrap config, pool pgx, cipher AES-GCM, client MQTT
// (TLS di produksi), subscriber ingest, dan graceful shutdown (SIGTERM/SIGINT).
package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/banana-pixel/quakealert/server/internal/api"
	"github.com/banana-pixel/quakealert/server/internal/config"
	"github.com/banana-pixel/quakealert/server/internal/consensus"
	"github.com/banana-pixel/quakealert/server/internal/crypto"
	"github.com/banana-pixel/quakealert/server/internal/dispatch"
	"github.com/banana-pixel/quakealert/server/internal/event"
	"github.com/banana-pixel/quakealert/server/internal/ingest"
	"github.com/banana-pixel/quakealert/server/internal/ledger"
	"github.com/banana-pixel/quakealert/server/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// Peringatan konfigurasi dicatat DI SINI dan tidak di paket config: config
	// tidak punya logger, dan sebuah fallback nama env yang usang yang tidak
	// pernah tercetak adalah cara termudah menjalankan produksi dengan jendela
	// korelasi yang salah.
	for _, w := range cfg.Warnings {
		log.Warn("config: " + w)
	}

	// Context bootstrap dengan timeout IO untuk koneksi awal.
	bootCtx, cancel := context.WithTimeout(context.Background(), cfg.IOTimeout*3)
	defer cancel()

	st, err := store.New(bootCtx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()
	log.Info("terhubung ke postgres")

	cipher, err := crypto.New(cfg.MasterKey)
	if err != nil {
		return err
	}

	// --- Observation ledger (migrasi 000006) ---
	// Dikonstruksi SEBELUM verifier dan dispatcher karena keduanya memegangnya.
	// Satu goroutine drain; produsennya (jalur verifikasi trigger dan jalur
	// keputusan dispatch) hanya mengantre dan tidak pernah menunggu.
	//
	// Nonaktif = pointer nil, bukan writer kosong: metode *ledger.Writer aman
	// pada penerima nil, jadi penonaktifan tidak memerlukan cabang if di
	// pemanggil mana pun.
	var ledgerWriter *ledger.Writer
	if cfg.ObservationLedgerEnabled {
		ledgerWriter = ledger.NewWriter(st, cfg.ObservationLedgerQueueSize, log)

		// Context terpisah dari bootCtx (yang punya timeout) dan dibatalkan lewat
		// defer, mengikuti pola purgeChatLoop / purgeAbandonedPendingLoop.
		ledgerCtx, stopLedger := context.WithCancel(context.Background())
		defer stopLedger()
		go ledgerWriter.Run(ledgerCtx)

		// Stop() menunggu sisa antrean ditulis. Dijalankan lewat defer agar
		// baris yang sudah diantre pada milidetik terakhir tetap sampai ke DB —
		// pool pgx baru ditutup setelahnya (defer st.Close() terdaftar lebih awal,
		// jadi berjalan lebih akhir).
		defer ledgerWriter.Stop()

		log.Info("observation ledger aktif", "queue_size", cfg.ObservationLedgerQueueSize)
	} else {
		log.Warn("observation ledger NONAKTIF — tidak ada rekaman masukan sensor maupun keputusan dispatch")
	}

	verifier := ingest.NewVerifier(st, cipher, log)
	if ledgerWriter != nil {
		// Dipasang hanya bila benar-benar ada. Menyimpan pointer nil ke dalam
		// interface akan membuat pemeriksaan `!= nil` di pemanggil bernilai true
		// dan tiap trigger membangun baris yang kemudian dibuang.
		verifier.WithLedger(ledgerWriter)
	}

	// --- Dispatch tier: WebSocket Hub + FCM (opsional) ---
	hub := dispatch.NewHub(log, wsOriginChecker(cfg.WSAllowedOrigins, log))

	var fcm dispatch.FCMSender
	if cfg.FCMProjectID != "" && cfg.FCMCredentialsFile != "" {
		saJSON, rerr := os.ReadFile(cfg.FCMCredentialsFile)
		if rerr != nil {
			return rerr
		}
		fcm, err = dispatch.NewHTTPV1Sender(bootCtx, cfg.FCMProjectID, saJSON, log)
		if err != nil {
			return err
		}
		log.Info("FCM sender aktif", "project_id", cfg.FCMProjectID)
	} else {
		log.Warn("FCM tidak dikonfigurasi — delivery background nonaktif (hanya WebSocket)")
	}

	dispatcher := dispatch.NewDispatcher(st, hub, fcm, cfg.CooldownDuration, log)
	if ledgerWriter != nil {
		dispatcher.SetLedger(ledgerWriter)
	}
	dispatcher.SetSingleNodeGeoTopicGuard(cfg.SingleNodeGeoTopicGuard)
	if !cfg.SingleNodeGeoTopicGuard {
		log.Warn("guard satu-node NONAKTIF — kluster satu sensor dapat menyiarkan ke topik FCM nasional")
	}

	// --- Event tier: korelasi + siklus hidup (§11.2) ---
	// Dua jalur, dipilih EVENT_TRACKER_ENABLED, dan tepat satu yang hidup dalam
	// satu proses. Jalur lama SENGAJA masih dapat dieksekusi selama tepat satu
	// rilis (§11.3, §17): rollback Fase 3 harus berupa satu variabel environment
	// dan sebuah restart, bukan sebuah redeploy.
	var handler func(context.Context, *ingest.Trigger)

	// tracker hidup di luar cabang HANYA supaya rute admin dapat memasangnya
	// sebagai pencabut bukti setelah apiSrv dibangun (§11.4). Nil pada jalur
	// Fase 2, dan setter di sana memang tidak dipanggil.
	var tracker *event.Tracker

	if cfg.EventTrackerEnabled {
		tracker = event.NewTracker(st, eventOptions(cfg), log)

		// Emitter dipasang lewat setter, bukan konstruktor: dispatcher dan tracker
		// dibangun pada titik yang berbeda, dan Bridge-lah yang menerjemahkan
		// transisi menjadi frame (§8.3) supaya arah impornya tetap event ->
		// dispatch saja.
		tracker.SetEmitter(event.NewBridge(dispatcher))

		if ledgerWriter != nil {
			// Persistensi event menempuh antrean yang SAMA dengan observasi:
			// asinkron, berbatas, boleh membuang. Observer-nya adalah tracker
			// sendiri, supaya kegagalan tulis menjadi counter alih-alih nilai
			// kembalian yang tidak ada pemanggilnya (§9.5).
			tracker.SetLedger(ledgerWriter)
			ledgerWriter.SetEventObserver(tracker)
		}

		trackerCtx, stopTracker := context.WithCancel(context.Background())
		defer stopTracker()
		go tracker.Run(trackerCtx)

		// §15.3 — rekonsiliasi restart. Timeout sendiri (bukan bootCtx, yang juga
		// dipakai koneksi awal) karena ia membaca setiap event terbuka plus
		// koordinat setiap kontributornya.
		recCtx, recCancel := context.WithTimeout(context.Background(), cfg.IOTimeout*3)
		if rerr := tracker.Reconcile(recCtx); rerr != nil {
			log.Warn("event: rekonsiliasi awal gagal", "err", rerr)
			// §15.3 langkah 5: kegagalan dicatat, tidak fatal, dan sapuan lama
			// tetap menjadi jaring yang sudah ada — tanpa itu baris HAPPENING yang
			// gagal dibaca akan menggantung selamanya. Hanya di cabang ini: sapuan
			// itu memakai started_at, jadi menjalankannya setelah rekonsiliasi yang
			// BERHASIL akan menandai RESOLVED justru event yang baru saja diangkat
			// kembali sebagai masih hidup.
			resolveStaleEventsAtStartup(st, cfg, log)
		}
		tracker.CheckFleetIndependence(recCtx) // §7.3 pemeriksaan-diri saat boot
		recCancel()

		// Handler trigger yang lolos verifikasi -> masukkan ke tracker event.
		// Trigger TIDAK lagi dipotong menjadi tiga field: onset, fase dan obs_seq
		// adalah yang menyetir korelasi (§6.1).
		handler = func(ctx context.Context, t *ingest.Trigger) {
			tracker.Ingest(ctx, event.ObservationFrom(t))
		}

		log.Info("event tracker aktif (Fase 3)",
			"correlation_window", cfg.CorrelationWindow,
			"attach_radius_km", cfg.AttachRadiusKm,
			"independence_cell_km", cfg.IndependenceCellKm,
			"min_independent_cells", cfg.MinIndependentCells,
			"resolve_after", cfg.EventResolveAfter,
			"sweep_interval", cfg.EventSweepInterval)
	} else {
		// --- Consensus tier: spatial engine (jalur Fase 2) ---
		// Cooldown = jeda antar-emisi + waktu menuju EVENT_RESOLVED (state machine).
		engine := consensus.NewEngine(cfg.ConsensusWindow, cfg.CooldownDuration, st, dispatcher.Dispatch, log)

		// Rekonsiliasi startup: event HAPPENING yang lebih tua dari cooldown
		// ditandai RESOLVED. State machine resolusi dispatcher hanya in-memory —
		// tanpa ini, event yang sedang berlangsung saat proses restart akan
		// selamanya menggantung sebagai HAPPENING (tanpa EVENT_RESOLVED).
		resolveStaleEventsAtStartup(st, cfg, log)

		// Handler trigger yang lolos verifikasi -> masukkan ke consensus engine.
		handler = func(ctx context.Context, t *ingest.Trigger) {
			engine.Ingest(ctx, t.NodeID, t.PGA, t.TS)
		}

		log.Info("event tracker NONAKTIF — memakai consensus engine Fase 2 (EVENT_TRACKER_ENABLED=false)")
	}

	// Handler heartbeat -> perbarui telemetri liveness node (RSSI, latency,
	// last_heartbeat) yang dibaca endpoint /sensors.
	hbValidator := ingest.NewHeartbeatValidator(log)
	hbHandler := func(ctx context.Context, h *ingest.Heartbeat, latencyMs *int) {
		known, uerr := st.UpdateHeartbeat(ctx, h.ID, h.RSSI, latencyMs)
		if uerr != nil {
			log.Error("gagal update heartbeat", "station_id", h.ID, "err", uerr)
			return
		}
		if !known {
			log.Warn("heartbeat dari node tak dikenal", "station_id", h.ID)
		}
	}

	client, err := newMQTTClient(cfg, log)
	if err != nil {
		return err
	}

	sub := ingest.NewSubscriber(client, verifier, handler, log, cfg.IOTimeout).
		WithHeartbeat(hbValidator, hbHandler)
	if err := sub.Start(); err != nil {
		return err
	}

	// --- REST API tier: rate limiter (Redis bila tersedia, fallback memory) ---
	var limiter api.RateLimiter
	if cfg.RedisURL != "" {
		rl, rerr := api.NewRedisRateLimiter(cfg.RedisURL)
		if rerr != nil {
			return rerr
		}
		if perr := rl.Ping(bootCtx); perr != nil {
			log.Warn("Redis tidak tersedia — fallback rate limiter in-memory", "err", perr)
			_ = rl.Close()
			limiter = api.NewMemoryRateLimiter()
		} else {
			defer rl.Close()
			limiter = rl
			log.Info("rate limiter Redis aktif")
		}
	} else {
		limiter = api.NewMemoryRateLimiter()
	}

	apiSrv := api.NewServer(st, cipher, limiter, api.MQTTPublic{
		Broker: cfg.MQTTPublicBroker,
		Port:   cfg.MQTTPublicPort,
		TLS:    cfg.MQTTPublicTLS,
	}, api.AuthConfig{
		JWTSecret: cfg.JWTSecret,
		TokenTTL:  cfg.JWTTokenTTL,
	}, log)

	// --- Chat: jembatan antara api dan dispatch ---
	// api dan dispatch tidak saling impor, dan itu disengaja. main yang
	// menjembatani: sebuah adapter mengubah api.ChatEvent menjadi
	// dispatch.ChatMessage, dan sebuah resolver menjawab keanggotaan kanal
	// sebuah koneksi WS tanpa dispatch perlu tahu soal auth atau basis data.
	apiSrv.SetChatFanout(chatFanout{hub: hub})

	// --- Siaran operator: jembatan yang sama, arah yang sama ---
	// Kunci dipasang di sini, bukan dibaca Router: Router hanya melihat ada atau
	// tidaknya kunci, sehingga instalasi tanpa ADMIN_API_KEY tidak punya rute
	// admin untuk ditembus sama sekali.
	apiSrv.SetAdminAPIKey(cfg.AdminAPIKey)
	if tracker != nil {
		// §7.5/§11.4: rute unverify operator mencabut bukti node dari setiap event
		// terbuka. Satu-satunya pemanggil InvalidateContributor di Fase 3 — tidak
		// ada pemanggil otomatis, dengan sengaja.
		apiSrv.SetEvidenceInvalidator(tracker)
	}
	apiSrv.SetBroadcastFanout(broadcastFanout{dispatcher: dispatcher})
	apiSrv.SetTestAlertFanout(testAlertFanout{dispatcher: dispatcher})
	apiSrv.SetMQTTHealthCheck(func() bool { return client.IsConnected() })
	hub.SetChannelResolver(func(r *http.Request) []string {
		userID, ok := api.UserIDFromContext(r.Context())
		if !ok {
			return nil
		}
		channels, err := st.ListChatChannels(r.Context(), userID)
		if err != nil {
			log.Warn("gagal membaca kanal chat untuk klien ws", "err", err)
			// Kanal global tetap diberikan: kegagalan katalog tidak boleh
			// membuat satu-satunya ruang yang selalu ada ikut hilang.
			return []string{store.GlobalChannelID}
		}
		ids := make([]string, 0, len(channels))
		for _, c := range channels {
			ids = append(ids, c.ChannelID)
		}
		return ids
	})
	// Retensi chat: goroutine terjadwal, bukan pg_cron — ekstensi itu belum tentu
	// ada di host produksi, dan retensi yang bergantung pada ekstensi opsional
	// adalah retensi yang diam-diam tidak berjalan. Dibatalkan saat shutdown.
	chatCtx, stopChatPurge := context.WithCancel(context.Background())
	defer stopChatPurge()
	go purgeChatLoop(chatCtx, st, log)

	// Jaring pembersihan node pending terlantar: alasan keberadaannya sama dengan
	// purge chat di atas — retensi tidak boleh bergantung ekstensi opsional — dan
	// goroutine yang sama polanya: sweep awal + ticker + cancel saat shutdown.
	pendingCtx, stopPendingSweep := context.WithCancel(context.Background())
	defer stopPendingSweep()
	go purgeAbandonedPendingLoop(pendingCtx, st, log)

	// --- HTTP server (WebSocket WSS via reverse proxy TLS di produksi) ---
	// Router chi: /api/v1/auth/anonymous publik, /api/v1/events auth opsional,
	// sisanya + /ws wajib Bearer JWT (HS256).
	handlerHTTP := apiSrv.Router(hub.ServeWS, log)
	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handlerHTTP,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	// Catatan WebSocket: gorilla/websocket menghapus deadline HTTP setelah
	// hijack (server.go:254 netConn.SetDeadline(time.Time{})), jadi timeout di
	// atas tidak memutus koneksi WS yang idle; liveness WS ditangani ping/pong.

	// Jalankan HTTP server di goroutine. Bila ListenAndServe gagal SEBELUM
	// menerima sinyal shutdown (mis. port sudah terpakai), error langsung
	// dikembalikan agar proses mati cepat (fail-fast) alih-alih diam-diam
	// berjalan tanpa API.
	serveErr := make(chan error, 1)
	go func() {
		log.Info("http server listen", "addr", cfg.HTTPAddr)
		serveErr <- httpSrv.ListenAndServe()
	}()

	// --- Graceful shutdown (Aturan Server #4) ---
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)

	select {
	case err := <-serveErr:
		return fmt.Errorf("http server gagal start: %w", err)
	case <-stop:
	}
	log.Info("sinyal shutdown diterima, drain koneksi")

	// Putuskan MQTT dengan quiesce 250ms.
	client.Disconnect(250)

	// Tutup HTTP server (drain WS) dengan timeout.
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	if err := httpSrv.Shutdown(shutCtx); err != nil {
		log.Warn("http shutdown error", "err", err)
	}

	// Pool pgx ditutup via defer st.Close().
	log.Info("shutdown selesai")
	return nil
}

// eventOptions menyalin parameter korelasi dari config ke event.Options.
//
// Ada di main dan bukan sebagai metode config.Config karena paket config
// SENGAJA tidak mengimpor apa pun dari dalam server (lihat maxAcceptedTriggerAge
// di config.go): sebuah metode yang mengembalikan event.Options akan membalik
// arah itu demi kenyamanan satu pemanggil. Nilainya disalin sekali saat boot —
// event.Options memang tidak dibaca ulang, supaya sebuah ambang tidak berubah di
// tengah hidup sebuah event.
func eventOptions(cfg *config.Config) event.Options {
	return event.Options{
		CorrelationWindowMs: cfg.CorrelationWindow.Milliseconds(),
		AttachRadiusKm:      cfg.AttachRadiusKm,
		IndependenceCellKm:  cfg.IndependenceCellKm,
		MinIndependentCells: cfg.MinIndependentCells,
		MaxEventDiameterKm:  cfg.MaxEventDiameterKm,
		ResolveAfterMs:      cfg.EventResolveAfter.Milliseconds(),
		SweepIntervalMs:     cfg.EventSweepInterval.Milliseconds(),
		MaxOpen:             cfg.EventTrackerMaxOpen,
		TerminalRetentionMs: cfg.TerminalRetention.Milliseconds(),
		MaxTombstones:       cfg.EventTrackerMaxTombstones,
	}
}

// resolveStaleEventsAtStartup menandai RESOLVED setiap baris HAPPENING yang mulai
// sebelum satu cooldown lalu.
//
// Diekstrak menjadi fungsi karena kini dipanggil dari dua tempat dengan dua peran
// yang berbeda: satu-satunya rekonsiliasi pada jalur Fase 2, dan JARING saat
// rekonsiliasi Fase 3 gagal membaca (§15.3 langkah 5). Kegagalannya dicatat dan
// tidak fatal di kedua peran: sebuah baris yang menggantung sebagai HAPPENING
// tidak sebanding dengan menolak menyalakan server.
func resolveStaleEventsAtStartup(st *store.Store, cfg *config.Config, log *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.IOTimeout)
	defer cancel()

	n, err := st.ResolveStaleEvents(ctx, time.Now().Add(-cfg.CooldownDuration))
	if err != nil {
		log.Warn("gagal rekonsiliasi event stale saat startup", "err", err)
		return
	}
	if n > 0 {
		log.Info("event stale ditandai RESOLVED saat startup", "count", n)
	}
}

// newMQTTClient membuat client MQTT. Untuk skema tls:///ssl:// mengaktifkan
// TLS dengan verifikasi CA sistem (ADR-0003: TLS everywhere, plaintext dilarang
// di produksi). Skema tcp:// hanya untuk pengembangan lokal.
func newMQTTClient(cfg *config.Config, log *slog.Logger) (mqtt.Client, error) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.MQTTBroker)
	opts.SetClientID(cfg.MQTTClientID)
	opts.SetUsername(cfg.MQTTUser)
	opts.SetPassword(cfg.MQTTPassword)
	opts.SetCleanSession(false) // pertahankan langganan QoS 1 antar-reconnect
	opts.SetOrderMatters(false)
	opts.SetKeepAlive(30 * time.Second)
	opts.SetConnectTimeout(cfg.IOTimeout)
	opts.SetAutoReconnect(true)
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		log.Warn("koneksi mqtt terputus", "err", err)
	})

	if isTLS(cfg.MQTTBroker) {
		// Verifikasi CA sistem; JANGAN InsecureSkipVerify di produksi.
		opts.SetTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12})
	} else {
		log.Warn("MQTT tanpa TLS — hanya untuk pengembangan lokal (ADR-0003)", "broker", cfg.MQTTBroker)
	}

	client := mqtt.NewClient(opts)
	token := client.Connect()
	token.Wait()
	if err := token.Error(); err != nil {
		return nil, err
	}
	log.Info("terhubung ke broker mqtt", "broker", cfg.MQTTBroker)
	return client, nil
}

func isTLS(broker string) bool {
	b := strings.ToLower(broker)
	return strings.HasPrefix(b, "tls://") || strings.HasPrefix(b, "ssl://") || strings.HasPrefix(b, "mqtts://")
}

// wsOriginChecker membangun gorilla CheckOrigin dari allowlist konfigurasi.
//
// Hanya browser yang mengirim header Origin, dan hanya browser yang perlu
// dilindungi dari cross-site WebSocket hijacking — di sana kredensial ada di
// cookie yang dikirim otomatis. Klien native (OkHttp pada aplikasi Android)
// tidak mengirim Origin dan membawa Bearer token secara eksplisit, jadi upgrade
// tanpa Origin diizinkan; tanpa ini setiap handshake dari aplikasi ditolak 403
// selama WS_ALLOWED_ORIGINS belum diisi — jalur realtime life-safety mati pada
// konfigurasi default.
//
// Origin yang ADA tetap diperiksa terhadap allowlist. Allowlist kosong berarti
// "belum ada origin browser yang dipercaya", bukan "semua".
func wsOriginChecker(allowed []string, log *slog.Logger) func(*http.Request) bool {
	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		for _, o := range allowed {
			if o == origin || o == "*" {
				return true
			}
		}
		log.Warn("upgrade websocket ditolak", "origin", origin)
		return false
	}
}

// chatFanout mengadaptasi api.ChatFanout ke hub WebSocket. Ada di main karena
// api dan dispatch tidak saling impor; satu-satunya tempat keduanya bertemu.
type chatFanout struct{ hub *dispatch.Hub }

func (f chatFanout) BroadcastChat(e api.ChatEvent) {
	f.hub.BroadcastChat(&dispatch.ChatMessage{
		Type:            "CHAT_MESSAGE",
		MessageID:       e.MessageID,
		ChannelID:       e.ChannelID,
		SenderID:        e.SenderID,
		SenderPseudonym: e.SenderPseudonym,
		SenderLocation:  e.LocationTag,
		Message:         e.Body,
		IsAdmin:         e.IsAdmin,
		Timestamp:       e.CreatedAt.UnixMilli(),
	})
}

// broadcastFanout mengadaptasi api.BroadcastFanout ke dispatcher, yang
// mengurus WebSocket dan FCM sekaligus. Lewat dispatcher (bukan langsung ke
// hub) karena pengumuman harus sampai juga ke perangkat yang aplikasinya
// tertutup — di situlah hampir semua pembacanya berada.
type broadcastFanout struct{ dispatcher *dispatch.Dispatcher }

func (f broadcastFanout) BroadcastAdmin(b api.AdminBroadcast) {
	f.dispatcher.DispatchBroadcast(&dispatch.BroadcastMessage{
		BroadcastID: b.ID,
		Title:       b.Title,
		Body:        b.Body,
		RegionCode:  b.RegionCode,
		Timestamp:   b.CreatedAt.UnixMilli(),
	})
}

// testAlertFanout mengadaptasi api.TestAlertFanout ke dispatcher. Jalur yang
// berbeda dari broadcastFanout meski keduanya berakhir di dispatcher yang sama:
// drill memakai envelope alert (agar layar peringatan yang diuji adalah yang
// sesungguhnya) tetapi topic FCM sendiri, dan tidak menyentuh persistensi.
type testAlertFanout struct{ dispatcher *dispatch.Dispatcher }

func (f testAlertFanout) DispatchTestAlert(t api.TestAlert) {
	f.dispatcher.DispatchTestAlert(&dispatch.AlertMessage{
		Type:           dispatch.TypeAlert,
		EventID:        t.EventID,
		MMI:            t.MMI,
		IntensityLabel: t.IntensityLabel,
		PGAGal:         t.PGAGal,
		CentroidLat:    t.Latitude,
		CentroidLon:    t.Longitude,
		LocationName:   t.LocationName,
		Timestamp:      t.Timestamp.UnixMilli(),
		NodeCount:      t.NodeCount,
	})
}

// Retensi chat: 7 hari, diperiksa setiap jam.
const (
	chatRetention     = 7 * 24 * time.Hour
	chatPurgeInterval = time.Hour
)

// purgeChatLoop menghapus pesan chat yang melewati masa retensi.
//
// Berjalan sekali di awal lalu setiap chatPurgeInterval: sebuah proses yang baru
// dimulai setelah lama mati akan menemukan tumpukan pesan basi, dan menunggu satu
// jam untuk membersihkannya berarti menyajikan riwayat yang seharusnya sudah
// tidak ada.
func purgeChatLoop(ctx context.Context, st *store.Store, log *slog.Logger) {
	purge := func() {
		opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		deleted, err := st.PurgeChatMessages(opCtx, chatRetention)
		if err != nil {
			log.Warn("gagal membersihkan pesan chat basi", "err", err)
			return
		}
		if deleted > 0 {
			log.Info("pesan chat basi dibersihkan", "rows", deleted)
		}
	}

	purge()
	ticker := time.NewTicker(chatPurgeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			purge()
		}
	}
}

// Retensi node pending yang ditinggalkan: 14 hari, diperiksa sekali sehari.
//
// Wizard mencetak baris iot_nodes di langkah LOCATION; sesi yang mati sebelum
// ESP32 terkonfigurasi (proses dibunuh, respons provisioning hilang di jalan,
// pengguna keluar tanpa sempat revoke) meninggalkan baris verified=FALSE yang
// tidak pernah berdenyut. Jalur client-driven (POST /nodes/revoke) tidak bisa
// menjangkau kasus itu — ponsel kehilangan capability-nya bersama prosesnya —
// jadi pembersihan behavioral inilah jaring pengamannya.
const (
	pendingNodeRetention     = 14 * 24 * time.Hour
	pendingNodeSweepInterval = 24 * time.Hour
)

// purgeAbandonedPendingLoop menghapus node pending yang tidak pernah heartbeat
// melewati masa retensi. Pola sama dengan purgeChatLoop: goroutine terjadwal
// (bukan pg_cron), berjalan sekali di awal untuk rekonsiliasi startup lalu
// per interval, dengan timeout operasi per-tick dan pembatalan saat shutdown.
//
// Loop ini aman terhadap node sah: predikat penghapusan menuntut verified=FALSE
// DAN ketiadaan heartbeat (lihat PurgeAbandonedPendingNodes), sehingga instalasi
// nyata yang sedang menunggu konfirmasi operator tidak akan tersentuh berapa
// pun lamanya menunggu.
func purgeAbandonedPendingLoop(ctx context.Context, st *store.Store, log *slog.Logger) {
	sweep := func() {
		opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		deleted, err := st.PurgeAbandonedPendingNodes(opCtx, pendingNodeRetention)
		if err != nil {
			log.Warn("gagal membersihkan node pending terlantar", "err", err)
			return
		}
		if deleted > 0 {
			log.Info("node pending terlantar dibersihkan", "rows", deleted)
		}
	}

	sweep()
	ticker := time.NewTicker(pendingNodeSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweep()
		}
	}
}
