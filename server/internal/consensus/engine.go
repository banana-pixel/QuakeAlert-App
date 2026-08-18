package consensus

import (
	"context"
	"log/slog"
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
	CreatedAtMs    int64 // ms epoch UTC
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
type Engine struct {
	mu       sync.Mutex
	window   time.Duration
	readings map[string]Reading // key = node_id (dedup: reading terbaru per node)

	loc  locator
	sink EventSink
	log  *slog.Logger
	now  func() time.Time

	lastEmitMs int64 // cooldown sederhana agar tidak spam event identik
}

// NewEngine membuat engine. window biasanya dari cfg.ConsensusWindow (8000ms).
func NewEngine(window time.Duration, loc locator, sink EventSink, log *slog.Logger) *Engine {
	return &Engine{
		window:   window,
		readings: make(map[string]Reading, 16),
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

	r := Reading{NodeID: nodeID, Lat: nl.Lat, Lon: nl.Lon, PGA: pga, TS: ts}

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

	// Hanya CONFIRMED yang dipersistensi & dibroadcast penuh; ADVISORY tetap
	// diteruskan agar dispatch dapat mengirim silent yellow banner.
	e.sink(ctx, ev)
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
// tidak ada reading.
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
	mmi, label := Intensity(maxPGA)
	ev := &Event{
		Centroid:       WeightedCentroid(cluster),
		MaxPGA:         maxPGA,
		MMIScale:       mmi,
		IntensityLabel: label,
		NodeCount:      len(cluster),
		Readings:       cluster,
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
