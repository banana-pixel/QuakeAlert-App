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
	"github.com/banana-pixel/quakealert/server/internal/ingest"
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

	verifier := ingest.NewVerifier(st, cipher, log)

	// --- Dispatch tier: WebSocket Hub + FCM (opsional) ---
	// CheckOrigin: produksi harus membatasi origin; default menolak lintas-origin.
	hub := dispatch.NewHub(log, func(r *http.Request) bool {
		if len(cfg.WSAllowedOrigins) == 0 {
			return false
		}
		origin := r.Header.Get("Origin")
		for _, o := range cfg.WSAllowedOrigins {
			if o == origin || o == "*" {
				return true
			}
		}
		return false
	})

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

	// --- Consensus tier: spatial engine ---
	// Cooldown = jeda antar-emisi + waktu menuju EVENT_RESOLVED (state machine).
	engine := consensus.NewEngine(cfg.ConsensusWindow, cfg.CooldownDuration, st, dispatcher.Dispatch, log)

	// Rekonsiliasi startup: event HAPPENING yang lebih tua dari cooldown
	// ditandai RESOLVED. State machine resolusi dispatcher hanya in-memory —
	// tanpa ini, event yang sedang berlangsung saat proses restart akan
	// selamanya menggantung sebagai HAPPENING (tanpa EVENT_RESOLVED).
	{
		recCtx, cancel := context.WithTimeout(context.Background(), cfg.IOTimeout)
		n, rerr := st.ResolveStaleEvents(recCtx, time.Now().Add(-cfg.CooldownDuration))
		cancel()
		if rerr != nil {
			log.Warn("gagal rekonsiliasi event stale saat startup", "err", rerr)
		} else if n > 0 {
			log.Info("event stale ditandai RESOLVED saat startup", "count", n)
		}
	}

	// Handler trigger yang lolos verifikasi -> masukkan ke consensus engine.
	handler := func(ctx context.Context, t *ingest.Trigger) {
		engine.Ingest(ctx, t.NodeID, t.PGA, t.TS)
	}

	// Handler heartbeat -> perbarui telemetri liveness node (RSSI, latency,
	// last_heartbeat) yang dibaca endpoint /sensors.
	hbValidator := ingest.NewHeartbeatValidator(log)
	hbHandler := func(ctx context.Context, h *ingest.Heartbeat, latencyMs int) {
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
