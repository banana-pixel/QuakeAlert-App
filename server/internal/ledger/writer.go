// Package ledger menulis observation ledger (sensor_observations,
// alert_emissions) DI LUAR jalur peringatan.
//
// Alasan paket ini ada, dan mengapa ia asinkron:
//
//	consensus.Engine.Ingest dan dispatch.Dispatcher.Dispatch keduanya berjalan
//	inline pada goroutine callback pesan MQTT (ingest/subscriber.go onMessage).
//	INSERT sinkron di jalur itu akan menaruh latensi basis data di depan
//	konsensus untuk setiap observasi yang masuk — sebuah pool yang lambat atau
//	kehabisan koneksi akan memperlambat, atau menghentikan, deteksi gempa demi
//	pencatatan. Karena itu kontraknya dibalik: pencatatan boleh gagal, jalur
//	peringatan tidak.
//
// Konsekuensi kontrak tersebut, dan sifat yang harus dipertahankan:
//
//	Enqueue TIDAK PERNAH memblokir dan TIDAK PERNAH mengembalikan error yang
//	dapat ditindaklanjuti pemanggil. Antrean berbatas; saat penuh, baris TERTUA
//	dibuang (yang terbaru lebih berguna untuk diagnosis insiden yang sedang
//	berjalan) dan ledger_drops_total naik. Angka yang hilang selalu terlihat di
//	counter — tidak pernah hilang diam-diam.
package ledger

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// AlgoVer menandai versi algoritma keputusan yang menghasilkan baris
// alert_emissions. SENGAJA konstanta compile-time, bukan variabel lingkungan:
// nilainya harus mengikuti biner yang benar-benar membuat keputusan. Bila dapat
// dikonfigurasi saat runtime, operator dapat memberi label salah pada keputusan
// lampau, dan justru barisan lampau itulah satu-satunya alasan kolom ini ada.
//
// Naikkan bersama setiap perubahan pada ambang, clustering, jendela, atau
// cooldown di internal/consensus.
const AlgoVer = "phase1-1.0"

// Nilai audience yang diakui (kolom alert_emissions.audience).
const (
	AudienceTokensRadius = "TOKENS_RADIUS_200KM"
	AudienceGeoTopicAll  = "GEO_TOPIC_ALL"
	AudienceNone         = "NONE"
)

// Nilai tetap kolom sensor_observations.
//
// PhasePrelim dan OnsetSourceSensor hanya dapat berasal dari payload v2:
// observasi v1 selalu FINAL (ia dipublish saat event sudah selesai) dan
// onset-nya selalu sebuah BATAS, bukan pengukuran.
const (
	SourceClassFixedESP32 = "FIXED_ESP32"
	PhasePrelim           = "PRELIM"
	PhaseFinal            = "FINAL"
	OnsetSourcePublish    = "PUBLISH_BOUND"
	OnsetSourceSensor     = "SENSOR"
	VerifyResultOK        = "OK"
)

// rejectionInterval membatasi baris penolakan menjadi satu per node per selang
// ini. Kredensial broker berlaku untuk SELURUH fleet (deploy/mosquitto/aclfile:
// satu user quakealert-node dengan hak tulis sensor/+/trigger), jadi siapa pun
// yang memegangnya dapat memublikasikan payload tak tertanda tangan sebanyak
// yang ia mau. Tanpa batas ini, publish yang GAGAL verifikasi tetap menjadi
// penulisan durable tanpa batas — sebuah amplifikasi penulisan yang dipicu oleh
// pihak yang tidak terotentikasi.
const rejectionInterval = time.Minute

// writeTimeout membatasi satu INSERT ledger. Cukup longgar untuk pool yang
// sibuk, cukup ketat agar satu pernyataan yang menggantung tidak menghentikan
// drain selamanya.
const writeTimeout = 5 * time.Second

// counterLogInterval mengatur seberapa sering counter dilaporkan. Counter yang
// tidak pernah dicetak sama dengan counter yang tidak ada: Fase 1 belum punya
// endpoint /metrics, jadi log adalah satu-satunya jalan keluarnya.
const counterLogInterval = 5 * time.Minute

// DefaultQueueSize dipakai bila ukuran yang diberikan tidak masuk akal (<= 0).
const DefaultQueueSize = 1024

// Observation dan Emission adalah alias tipe baris store, bukan salinannya.
// Alias, bukan struct terpisah: satu definisi field berarti tidak ada dua
// tempat yang bisa saling menyimpang, dan pemanggil (ingest, dispatch) tetap
// tidak perlu mengimpor store hanya untuk mencatat.
type (
	Observation = store.Observation
	Emission    = store.AlertEmission
)

// ledgerStore adalah bagian store yang dibutuhkan Writer. Interface, bukan
// *store.Store konkret, mengikuti pola ingest.nodeSource dan api.Repo agar
// writer dapat diuji tanpa Postgres.
type ledgerStore interface {
	InsertObservation(ctx context.Context, o *store.Observation) error
	InsertAlertEmission(ctx context.Context, e *store.AlertEmission) error
	GetNodeLocation(ctx context.Context, stationID string) (*store.NodeLocation, error)
}

// item adalah satu satuan kerja dalam antrean: tepat satu dari kedua field
// terisi.
type item struct {
	obs  *Observation
	emis *Emission
}

// Writer menerima baris ledger dari jalur peringatan dan menuliskannya dari satu
// goroutine drain.
//
// Writer bernilai nil adalah writer yang dinonaktifkan: setiap metodenya aman
// dipanggil dan tidak melakukan apa pun. Itu yang membuat penonaktifan lewat
// OBSERVATION_LEDGER_ENABLED tidak memerlukan satu pun cabang if di pemanggil.
type Writer struct {
	store ledgerStore
	log   *slog.Logger
	queue chan item
	now   func() time.Time

	stopped  atomic.Bool
	stopOnce sync.Once
	stop     chan struct{}
	drained  chan struct{}

	// mu melindungi bookkeeping pembatasan penolakan.
	mu            sync.Mutex
	lastRejection map[string]time.Time
	suppressed    map[string]int

	drops             atomic.Int64
	unknownRejections atomic.Int64
	writeFailures     atomic.Int64
	written           atomic.Int64
}

// NewWriter membuat writer dengan antrean berkapasitas queueSize.
func NewWriter(st ledgerStore, queueSize int, log *slog.Logger) *Writer {
	if queueSize <= 0 {
		queueSize = DefaultQueueSize
	}
	return &Writer{
		store:         st,
		log:           log,
		queue:         make(chan item, queueSize),
		now:           time.Now,
		stop:          make(chan struct{}),
		drained:       make(chan struct{}),
		lastRejection: make(map[string]time.Time),
		suppressed:    make(map[string]int),
	}
}

// RecordObservation memasukkan satu observasi ke antrean.
//
// Tidak mengembalikan error, dan itu disengaja: pemanggilnya adalah jalur
// verifikasi trigger, yang tidak punya tindakan benar apa pun untuk merespons
// kegagalan pencatatan. Satu-satunya perilaku yang boleh terjadi di sini adalah
// kembali dengan cepat.
func (w *Writer) RecordObservation(o *Observation) {
	if w == nil || o == nil || w.stopped.Load() {
		return
	}

	// Baris penolakan dibatasi lajunya SEBELUM masuk antrean, bukan saat drain:
	// banjir penolakan yang sudah berada di dalam antrean berbatas akan menggusur
	// observasi asli, jadi penyaringan harus terjadi sebelum ia menempati slot.
	if o.VerifyResult != VerifyResultOK {
		count, admit := w.admitRejection(o.NodeID)
		if !admit {
			return
		}
		o.SuppressedRejections = count
	}

	w.enqueue(item{obs: o})
}

// RecordEmission memasukkan satu baris keputusan dispatch ke antrean. Sama
// seperti RecordObservation: tanpa error, tanpa blocking.
func (w *Writer) RecordEmission(e *Emission) {
	if w == nil || e == nil || w.stopped.Load() {
		return
	}
	w.enqueue(item{emis: e})
}

// admitRejection melaporkan apakah penolakan untuk nodeID boleh menjadi satu
// baris sekarang, beserta jumlah penolakan yang ditekan sejak baris terakhir.
//
// Jumlah yang ditekan dibawa oleh baris berikutnya yang diterima, sehingga
// pembatasan laju kehilangan BARIS tetapi tidak pernah kehilangan ANGKA.
func (w *Writer) admitRejection(nodeID string) (suppressed int, admit bool) {
	now := w.now()

	w.mu.Lock()
	defer w.mu.Unlock()

	last, seen := w.lastRejection[nodeID]
	if seen && now.Sub(last) < rejectionInterval {
		w.suppressed[nodeID]++
		return 0, false
	}
	w.lastRejection[nodeID] = now
	suppressed = w.suppressed[nodeID]
	delete(w.suppressed, nodeID)
	return suppressed, true
}

// returnSuppressed mengembalikan jumlah yang ditekan ke bookkeeping ketika baris
// pembawanya dibuang karena antrean penuh — tanpa ini, tekanan antrean akan
// menghapus angka yang justru dirancang untuk selamat dari pembatasan laju.
func (w *Writer) returnSuppressed(nodeID string, count int) {
	if count <= 0 {
		return
	}
	w.mu.Lock()
	w.suppressed[nodeID] += count
	w.mu.Unlock()
}

// enqueue menaruh it di antrean tanpa memblokir. Saat penuh: buang yang TERTUA,
// naikkan counter, lalu coba lagi sekali.
func (w *Writer) enqueue(it item) {
	select {
	case w.queue <- it:
		return
	default:
	}

	// Antrean penuh. Buang satu item tertua agar yang terbaru tetap masuk.
	select {
	case dropped := <-w.queue:
		w.drops.Add(1)
		if dropped.obs != nil {
			w.returnSuppressed(dropped.obs.NodeID, dropped.obs.SuppressedRejections)
		}
	default:
		// Drain sudah mengosongkannya lebih dulu; tidak ada yang perlu dibuang.
	}

	select {
	case w.queue <- it:
	default:
		// Produsen lain mengisi slot itu lebih dulu. Buang milik kita sendiri,
		// jangan berputar: memblokir di sini akan mengalahkan seluruh maksud paket.
		w.drops.Add(1)
		if it.obs != nil {
			w.returnSuppressed(it.obs.NodeID, it.obs.SuppressedRejections)
		}
	}
}

// Run menjalankan satu goroutine drain sampai ctx dibatalkan atau Stop dipanggil.
// Dimaksudkan dijalankan sebagai `go w.Run(ctx)`.
func (w *Writer) Run(ctx context.Context) {
	defer close(w.drained)

	ticker := time.NewTicker(counterLogInterval)
	defer ticker.Stop()

	for {
		select {
		case it := <-w.queue:
			w.write(ctx, it)
		case <-ticker.C:
			w.logCounters("periodik")
		case <-w.stop:
			w.finalDrain()
			return
		case <-ctx.Done():
			w.finalDrain()
			return
		}
	}
}

// finalDrain menuliskan apa yang masih tersisa di antrean saat shutdown, dengan
// context terpisah: context milik server sudah dibatalkan pada titik ini, dan
// memakainya berarti setiap penulisan sisa gagal seketika.
func (w *Writer) finalDrain() {
	w.stopped.Store(true)

	ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	defer cancel()

	for {
		select {
		case it := <-w.queue:
			w.write(ctx, it)
			if ctx.Err() != nil {
				w.logCounters("shutdown (drain terpotong)")
				return
			}
		default:
			w.logCounters("shutdown")
			return
		}
	}
}

// write menuliskan satu item. Kegagalan hanya dicatat: tidak ada pemanggil yang
// menunggunya, dan retry akan menahan antrean di belakang basis data yang sedang
// bermasalah.
func (w *Writer) write(ctx context.Context, it item) {
	switch {
	case it.obs != nil:
		w.writeObservation(ctx, it.obs)
	case it.emis != nil:
		w.writeEmission(ctx, it.emis)
	}
}

func (w *Writer) writeObservation(ctx context.Context, o *Observation) {
	wctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	// Snapshot lokasi diambil DI SINI, di luar jalur peringatan: satu query
	// tambahan per observasi tidak boleh berada di depan konsensus.
	//
	// Pencarian ini sekaligus menjadi uji "node dikenal" untuk baris penolakan.
	// iot_nodes.location adalah NOT NULL (migrasi 000001), jadi pencarian yang
	// berhasil setara dengan node yang ada. node_id tak dikenal tidak menjadi
	// baris sama sekali — hanya counter — karena kredensial broker yang berlaku
	// fleet-wide berarti nama node yang sembarang pun dapat mencapai server.
	loc, err := w.store.GetNodeLocation(wctx, o.NodeID)
	switch {
	case err == nil:
		lat, lon := loc.Lat, loc.Lon
		o.Lat, o.Lon = &lat, &lon
	case errors.Is(err, store.ErrNodeNotFound):
		if o.VerifyResult != VerifyResultOK {
			w.unknownRejections.Add(1)
			w.log.Warn("ledger: penolakan dari node tak dikenal tidak dicatat",
				"node_id", o.NodeID, "verify_result", o.VerifyResult)
			return
		}
		// verify_result 'OK' dengan node_location NULL: observasi terotentikasi
		// yang tidak dapat dipakai konsensus karena tidak punya koordinat. Justru
		// kasus ini yang harus terlihat.
	default:
		w.log.Warn("ledger: pencarian lokasi node gagal", "node_id", o.NodeID, "err", err)
	}

	if err := w.store.InsertObservation(wctx, o); err != nil {
		w.writeFailures.Add(1)
		w.log.Error("ledger: insert observation gagal", "node_id", o.NodeID, "err", err)
		return
	}
	w.written.Add(1)
}

func (w *Writer) writeEmission(ctx context.Context, e *Emission) {
	wctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	if err := w.store.InsertAlertEmission(wctx, e); err != nil {
		w.writeFailures.Add(1)
		w.log.Error("ledger: insert emission gagal",
			"alert_type", e.AlertType, "audience", e.Audience, "err", err)
		return
	}
	w.written.Add(1)
}

// Stop menghentikan drain dan menunggunya menuliskan sisa antrean.
// Idempoten dan aman dipanggil berkali-kali.
func (w *Writer) Stop() {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() { close(w.stop) })
	<-w.drained
}

func (w *Writer) logCounters(reason string) {
	w.log.Info("ledger: counters",
		"reason", reason,
		"ledger_rows_written_total", w.written.Load(),
		"ledger_drops_total", w.drops.Load(),
		"ledger_unknown_node_rejections_total", w.unknownRejections.Load(),
		"ledger_write_failures_total", w.writeFailures.Load(),
		"queue_depth", len(w.queue),
	)
}

// Drops mengembalikan ledger_drops_total.
func (w *Writer) Drops() int64 {
	if w == nil {
		return 0
	}
	return w.drops.Load()
}

// UnknownNodeRejections mengembalikan ledger_unknown_node_rejections_total.
func (w *Writer) UnknownNodeRejections() int64 {
	if w == nil {
		return 0
	}
	return w.unknownRejections.Load()
}

// WriteFailures mengembalikan jumlah INSERT ledger yang gagal.
func (w *Writer) WriteFailures() int64 {
	if w == nil {
		return 0
	}
	return w.writeFailures.Load()
}

// Written mengembalikan jumlah baris ledger yang berhasil ditulis.
func (w *Writer) Written() int64 {
	if w == nil {
		return 0
	}
	return w.written.Load()
}
