package consensus

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// Konstanta konsensus (default; window dikonfigurasi via config).
const (
	// ClusterRadiusKm: dua node dianggap satu kluster bila jarak <= 50 km.
	ClusterRadiusKm = 50.0
	// MinNodesConfirmed: >= 3 node unik terverifikasi -> CONFIRMED.
	MinNodesConfirmed = 3
	// MinPGAGal: ambang PGA minimum (gal) agar satu kluster dianggap peristiwa
	// gempa yang layak di-alert. Sama dengan batas bawah label "light" pada
	// Intensity(). Di bawah ini dianggap noise: firmware STA/LTA sudah meng-gate
	// di sisi sensor, ini pertahanan server kedua terhadap node yang salah
	// konfigurasi (mis. PGA=0 ikut menambah hitungan node).
	MinPGAGal = 16.6
)

// Status hasil evaluasi kluster.
type Status string

const (
	StatusConfirmed Status = "CONFIRMED" // >= 3 node unik dalam kluster
	StatusAdvisory  Status = "ADVISORY"  // 1-2 node (belum cukup konsensus)
)

// Event adalah hasil evaluasi konsensus yang siap didispatch/persistensi.
type Event struct {
	Status         Status
	Centroid       Centroid
	MaxPGA         float64 // gal
	MMIScale       string
	IntensityLabel string
	NodeCount      int
	Readings       []Reading
	LocationName   string // label dari node terdekat centroid
	CreatedAtMs    int64  // ms epoch UTC
}

// locator mengambil koordinat node (diabstraksi agar engine dapat diuji tanpa DB).
type locator interface {
	GetNodeLocation(ctx context.Context, stationID string) (*store.NodeLocation, error)
}

// EventSink dipanggil saat konsensus menghasilkan event (CONFIRMED/ADVISORY).
type EventSink func(ctx context.Context, ev *Event)

// Engine adalah Spatial Consensus Engine dengan sliding window in-memory.
// Aman untuk akses konkuren (subscriber MQTT memanggil Ingest dari goroutine
// callback paho). Retensi window = windowMs; reading kadaluarsa dipangkas.
//
// Cooldown diterapkan PER-SEL spasial (~1° grid, lihat cellKey): engine hanya
// mengemisi event baru untuk sebuah sel bila cooldown sel itu sudah lewat.
// Dalam cooldown hanya eskalasi ADVISORY -> CONFIRMED yang diizinkan. Ini
// menghasilkan event_id yang stabil untuk satu gempa (dispatcher hanya dipanggil
// sekali per gempa, tanpa spam re-emisi) SEKALIGUS tidak menekan gempa nyata di
// wilayah lain yang terpisah (false-negative multi-region).
type Engine struct {
	mu       sync.Mutex
	window   time.Duration
	cooldown time.Duration
	readings map[string]Reading // key = node_id (dedup: reading terbaru per node)

	loc  locator
	sink EventSink
	log  *slog.Logger
	now  func() time.Time

	emit map[string]emitState // cooldown per sel spasial (bukan global)
}

// emitState melacak emisi terakhir per sel spasial.
type emitState struct {
	lastEmitMs int64
	status     Status
}

// NewEngine membuat engine. window biasanya dari cfg.ConsensusWindow (8000ms),
// cooldown dari cfg.CooldownDuration (default 90s).
func NewEngine(window, cooldown time.Duration, loc locator, sink EventSink, log *slog.Logger) *Engine {
	if cooldown <= 0 {
		cooldown = 90 * time.Second
	}
	return &Engine{
		window:   window,
		cooldown: cooldown,
		readings: make(map[string]Reading, 16),
		emit:     make(map[string]emitState),
		loc:      loc,
		sink:     sink,
		log:      log,
		now:      time.Now,
	}
}

// Ingest menambahkan satu trigger terverifikasi ke window lalu mengevaluasi
// konsensus. Koordinat node diambil via locator (PostGIS). Hot path: kunci
// dipegang singkat; IO DB dilakukan sebelum mengunci.
func (e *Engine) Ingest(ctx context.Context, nodeID string, pga float64, ts int64) {
	nl, err := e.loc.GetNodeLocation(ctx, nodeID)
	if err != nil {
		e.log.Warn("konsensus: lokasi node tak ditemukan, trigger diabaikan",
			"node_id", nodeID, "err", err)
		return
	}

	r := Reading{NodeID: nodeID, Lat: nl.Lat, Lon: nl.Lon, PGA: pga, TS: ts, LocationName: nl.LocationName}

	e.mu.Lock()
	nowMs := e.now().UnixMilli()
	e.pruneLocked(nowMs)
	// Dedup per node: simpan reading dengan PGA tertinggi (eskalasi) untuk window.
	if prev, ok := e.readings[nodeID]; !ok || r.PGA >= prev.PGA {
		e.readings[nodeID] = r
	}
	snapshot := e.snapshotLocked()
	e.mu.Unlock()

	ev := Evaluate(snapshot, nowMs)
	if ev == nil {
		return
	}

	// Cooldown dievaluasi di bawah kunci untuk serialisasi antar-goroutine.
	e.mu.Lock()
	allowed := e.allowEmitLocked(ev, nowMs)
	e.mu.Unlock()
	if !allowed {
		e.log.Debug("konsensus: emisi ditekan cooldown", "status", ev.Status, "now_ms", nowMs)
		return
	}

	// Dispatcher menangani persistensi (hanya CONFIRMED), broadcast WS, dan FCM.
	e.sink(ctx, ev)
}

// allowEmitLocked memutuskan apakah event boleh diemisi untuk sel spasialnya.
// Wajib dipanggil dengan e.mu terkunci.
//
// Aturan (per sel):
//   - Belum pernah emisi di sel itu (lastEmitMs == 0)  -> izinkan.
//   - Cooldown sel sudah lewat dari emisi terakhir     -> izinkan (gempa baru).
//   - Dalam cooldown, eskalasi ADVISORY -> CONFIRMED   -> izinkan (satu node
//     lagi menguatkan konsensus; life-safety menuntut eskalasi).
//   - Selainnya                                        -> tekan (dedup event_id).
func (e *Engine) allowEmitLocked(ev *Event, nowMs int64) bool {
	cell := cellKey(ev.Centroid)
	st := e.emit[cell]
	cooldownMs := e.cooldown.Milliseconds()
	if st.lastEmitMs == 0 || nowMs >= st.lastEmitMs+cooldownMs {
		e.emit[cell] = emitState{lastEmitMs: nowMs, status: ev.Status}
		return true
	}
	if ev.Status == StatusConfirmed && st.status == StatusAdvisory {
		e.emit[cell] = emitState{lastEmitMs: nowMs, status: StatusConfirmed}
		return true
	}
	return false
}

// cellKey memetakan centroid ke sel grid ~1° (≈111 km). Cooldown per-sel
// memastikan gempa di wilayah berbeda tidak saling menekan, sementara gempa
// susulan/duplikat di sel yang sama tetap di-dedup.
func cellKey(c Centroid) string {
	return fmt.Sprintf("%.0f:%.0f", math.Round(c.Lat), math.Round(c.Lon))
}

// pruneLocked membuang reading yang lebih tua dari window. Harus dipanggil
// dengan mu terkunci.
func (e *Engine) pruneLocked(nowMs int64) {
	cutoff := nowMs - e.window.Milliseconds()
	for id, r := range e.readings {
		if r.TS < cutoff {
			delete(e.readings, id)
		}
	}
}

// snapshotLocked menyalin reading aktif ke slice baru (mu terkunci).
func (e *Engine) snapshotLocked() []Reading {
	out := make([]Reading, 0, len(e.readings))
	for _, r := range e.readings {
		out = append(out, r)
	}
	return out
}

// Evaluate mengelompokkan reading secara spasial (radius 50 km) lalu memilih
// kluster terbesar. Bila kluster terbesar >= 3 node -> CONFIRMED; 1-2 node ->
// ADVISORY. nowMs dipakai sebagai CreatedAtMs event. Mengembalikan nil bila
// tidak ada reading ATAU kluster tidak memenuhi ambang PGA minimum (MinPGAGal)
// — kluster dengan PGA di bawah ambang dianggap noise, bukan gempa.
//
// Fungsi murni (tanpa side-effect) agar mudah diuji.
func Evaluate(readings []Reading, nowMs int64) *Event {
	if len(readings) == 0 {
		return nil
	}
	cluster := largestCluster(readings)
	if len(cluster) == 0 {
		return nil
	}

	maxPGA := MaxPGA(cluster)
	if maxPGA < MinPGAGal {
		return nil
	}
	mmi, label := Intensity(maxPGA)
	centroid := WeightedCentroid(cluster)
	ev := &Event{
		Centroid:       centroid,
		MaxPGA:         maxPGA,
		MMIScale:       mmi,
		IntensityLabel: label,
		NodeCount:      len(cluster),
		Readings:       cluster,
		LocationName:   nearestToCentroid(cluster, centroid).LocationName,
		CreatedAtMs:    nowMs,
	}
	if len(cluster) >= MinNodesConfirmed {
		ev.Status = StatusConfirmed
	} else {
		ev.Status = StatusAdvisory
	}
	return ev
}

// largestCluster mengelompokkan node berbasis kedekatan (single-linkage:
// node bergabung ke kluster bila berjarak <= 50 km dari SALAH SATU anggota).
// Mengembalikan anggota kluster terbesar. Kompleksitas O(n^2) — memadai karena
// n (node aktif dalam window 8s) kecil, dan tanpa alokasi berlebih.
func largestCluster(readings []Reading) []Reading {
	n := len(readings)
	if n == 1 {
		return readings
	}

	// Union-Find sederhana untuk single-linkage clustering.
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}

	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			d := haversineKm(readings[i].Lat, readings[i].Lon, readings[j].Lat, readings[j].Lon)
			if d <= ClusterRadiusKm {
				union(i, j)
			}
		}
	}

	groups := make(map[int][]Reading)
	for i := 0; i < n; i++ {
		root := find(i)
		groups[root] = append(groups[root], readings[i])
	}

	// Pilih kluster terbesar; tie-break deterministik: PGA max tertinggi.
	var best []Reading
	for _, g := range groups {
		if len(g) > len(best) || (len(g) == len(best) && MaxPGA(g) > MaxPGA(best)) {
			best = g
		}
	}

	// Urutkan anggota agar output deterministik (memudahkan test & log).
	sort.Slice(best, func(a, b int) bool { return best[a].NodeID < best[b].NodeID })
	return best
}
