package event

import (
	"context"
	"strconv"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// algoVerBase menandai versi algoritma keputusan Fase 3. SENGAJA konstanta
// compile-time, dengan alasan yang sama seperti ledger.AlgoVer: nilainya harus
// mengikuti biner yang benar-benar membuat keputusan, dan operator yang dapat
// mengubahnya saat runtime dapat memberi label salah pada keputusan lampau.
const algoVerBase = "phase3-1.0"

// persister menerima satuan persistensi event. Implementasi: *ledger.Writer.
// Interface, dan bukan tipe konkret, dengan alasan yang sama seperti
// dispatch.emissionWriter: Tracker tidak boleh punya CARA untuk menulis ke basis
// data secara sinkron — bukan sekadar tidak melakukannya (§9.5).
type persister interface {
	RecordEventUnit(u *store.EventUnit)
}

// eventStore adalah seluruh yang dibutuhkan Tracker dari basis data: koordinat
// node pada jalur masuk, dan event terbuka pada saat boot (§15.3). Interface
// sempit, mengikuti pola ingest.nodeSource dan ledger.ledgerStore.
type eventStore interface {
	nodeSource
	LoadOpenEvents(ctx context.Context) ([]*store.EarthquakeEvent, error)
	ListActiveNodeLocations(ctx context.Context) ([]store.NodeLocation, error)
}

// algoVer adalah label algoritma yang ikut pada setiap baris yang ditulis Tracker.
// IndependenceCellKm masuk ke dalamnya karena ia dapat dikonfigurasi (§7.3): sebuah
// keputusan hanya dapat ditafsirkan bersama parameter yang menghasilkannya, dan
// "CONFIRMED" pada 5 km bukan pernyataan yang sama dengan "CONFIRMED" pada 50 km.
func (t *Tracker) algoVer() string {
	return algoVerBase + "/ic=" + strconv.FormatFloat(t.opt.IndependenceCellKm, 'f', -1, 64)
}

// statusFor memproyeksikan state siklus hidup ke kolom status dua nilai yang sudah
// dipublikasikan openapi.yaml. Proyeksi, bukan pengganti: mengganti status akan
// merusak kontrak REST tanpa imbalan apa pun (§9.1).
func statusFor(s State) string {
	if isTerminal(s) {
		return "RESOLVED"
	}
	return "HAPPENING"
}

// unitFor membangun satuan persistensi untuk satu transisi: baris induk dan baris
// riwayatnya, dalam satu satuan (§9.5).
//
// from_state DITULIS meski state itu sendiri tidak pernah menjadi baris. Sebuah
// event pertama kali menjadi durable pada transisi PUBLIK pertamanya, jadi
// DETECTED tidak punya baris — tetapi riwayat transisinya tetap lengkap, dan itu
// yang membuat persistensi lazy tidak kehilangan apa pun yang akan dibaca orang.
func (t *Tracker) unitFor(s Snapshot) *store.EventUnit {
	peak := s.PeakPGA
	from := string(s.From)

	return &store.EventUnit{
		Event: &store.EarthquakeEvent{
			EventID:              s.EventID,
			Status:               statusFor(s.To),
			CentroidLat:          s.CentroidLat,
			CentroidLon:          s.CentroidLon,
			LocationName:         s.LocationName,
			MMIScale:             s.MMIScale,
			IntensityLabel:       s.IntensityLabel,
			MaxPGA:               s.PeakPGA,
			TriggeredNodes:       s.NodeCount,
			StartedAtMs:          s.CreatedAt,
			EventState:           string(s.To),
			Revision:             s.Revision,
			OriginTS:             s.OriginTS,
			OriginTSSource:       s.OriginTSSource,
			IndependentCellCount: s.IndependentCells,
			AlgoVer:              t.algoVer(),
		},
		Log: &store.EventStateLog{
			EventID:          s.EventID,
			Revision:         s.Revision,
			FromState:        &from,
			ToState:          string(s.To),
			Reason:           s.Reason,
			DecidedAt:        s.DecidedAt,
			NodeCount:        s.NodeCount,
			IndependentCells: s.IndependentCells,
			PeakPGA:          &peak,
			EvidenceSummary:  s.Evidence.JSON(),
			AlgoVer:          t.algoVer(),
		},
	}
}
