package event

import (
	"encoding/json"
	"sort"

	"github.com/banana-pixel/quakealert/server/internal/consensus"
)

// Snapshot adalah potret satu transisi, diambil DI DALAM lock dan dipakai di luar
// lock oleh emitter maupun penulisan durable. Nilai, bukan pointer ke Event: apa
// yang diterima klien harus menggambarkan state yang benar-benar dipegang server
// pada saat itu, bukan state yang sudah berubah lagi sebelum frame terkirim.
type Snapshot struct {
	EventID  string
	From     State
	To       State
	Reason   string
	Revision int

	OriginTS       int64
	OriginTSSource string
	DecidedAt      int64

	// CreatedAt adalah jam server saat event ini dibuat. Dibawa terpisah dari
	// OriginTS karena keduanya menjawab pertanyaan berbeda dan pernah
	// dikonflasikan (F5): started_at berarti kapan barisnya lahir, origin_ts
	// berarti kapan tanahnya bergerak.
	CreatedAt int64

	CentroidLat    float64
	CentroidLon    float64
	LocationName   string
	PeakPGA        float64
	MMIScale       string
	IntensityLabel string

	NodeCount        int
	IndependentCells int

	// EverConfirmed menentukan hak FCM pada RESOLVED/CANCELLED (§8.1).
	EverConfirmed bool

	Evidence EvidenceSummary
}

// EvidenceSummary adalah potret ringkas bukti pada saat transisi, disimpan sebagai
// JSONB di event_state_log.
//
// Potret, BUKAN join: observasi penyumbangnya dapat dibuang antrean ledger yang
// berbatas (D17), jadi transisi yang menyebut mereka lewat referensi bisa menjadi
// tak dapat dijelaskan. Ukurannya dibatasi oleh besar fleet, dan itu satu-satunya
// alasan kolom JSONB dapat diterima di sini.
type EvidenceSummary struct {
	Contributors     []ContributorEvidence `json:"contributors"`
	IndependentCells int                   `json:"independent_cells"`
	CellIDs          []CellID              `json:"cell_ids"`
	OriginTSSource   string                `json:"origin_ts_source"`

	// MixedProvenance benar bila kontributor event ini tidak sepakat asal onsetnya
	// — sebagian terukur sensor, sebagian batas dari waktu publish. Dibawa
	// eksplisit supaya tidak ada pembaca lampau yang perlu menebak apakah jangkar
	// event campuran adalah pengukuran.
	MixedProvenance bool `json:"mixed_provenance"`
}

// ContributorEvidence adalah satu node di dalam potret bukti.
type ContributorEvidence struct {
	NodeID      string  `json:"node_id"`
	PeakPGA     float64 `json:"peak_pga"`
	Phase       string  `json:"phase"`
	OnsetTS     int64   `json:"onset_ts"`
	OnsetSource string  `json:"onset_source"`
	ObsSeq      *int64  `json:"obs_seq,omitempty"`
	Cell        CellID  `json:"cell"`
}

// CellID adalah sel independensi dalam bentuk yang dapat diserialkan. Pasangan
// bilangan bulat, bukan string terformat — sama seperti kunci in-memory-nya (A17).
type CellID struct {
	X int32 `json:"x"`
	Y int32 `json:"y"`
}

// snapshot membangun potret transisi from -> e.State dengan alasan reason.
// Dipanggil dengan lock Tracker dipegang.
func (e *Event) snapshot(from State, reason string) Snapshot {
	c := e.centroid()
	peak := e.peakPGA()
	mmi, label := consensus.Intensity(peak)

	name := ""
	if rs := e.readings(); len(rs) > 0 {
		name = consensus.NearestToCentroid(rs, c).LocationName
	} else if e.anchor != nil {
		name = e.anchor.LocationName
	}

	return Snapshot{
		EventID:          e.ID,
		From:             from,
		To:               e.State,
		Reason:           reason,
		Revision:         e.Revision,
		OriginTS:         e.OriginTS,
		OriginTSSource:   e.OriginTSSource,
		DecidedAt:        e.DecidedAt,
		CreatedAt:        e.CreatedAt,
		CentroidLat:      c.Lat,
		CentroidLon:      c.Lon,
		LocationName:     name,
		PeakPGA:          peak,
		MMIScale:         mmi,
		IntensityLabel:   label,
		NodeCount:        e.nodeCount(),
		IndependentCells: e.independentCells(),
		EverConfirmed:    e.EverConfirmed,
		Evidence:         e.evidence(),
	}
}

// evidence membangun potret bukti dengan urutan node_id yang STABIL: baris
// event_state_log dibandingkan antar-revisi oleh manusia, dan urutan iterasi map
// Go akan membuat perbandingan itu tidak mungkin.
func (e *Event) evidence() EvidenceSummary {
	ids := make([]string, 0, len(e.Contributors))
	for id := range e.Contributors {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := EvidenceSummary{
		Contributors:   make([]ContributorEvidence, 0, len(ids)),
		OriginTSSource: e.OriginTSSource,
	}
	cells := make(map[cellKey]struct{}, len(ids))
	sources := make(map[string]struct{}, 2)

	for _, id := range ids {
		c := e.Contributors[id]
		out.Contributors = append(out.Contributors, ContributorEvidence{
			NodeID:      c.NodeID,
			PeakPGA:     c.PeakPGA,
			Phase:       c.Phase,
			OnsetTS:     c.OnsetTS,
			OnsetSource: c.OnsetSource,
			ObsSeq:      c.ObsSeq,
			Cell:        CellID{X: c.Cell.X, Y: c.Cell.Y},
		})
		cells[c.Cell] = struct{}{}
		sources[c.OnsetSource] = struct{}{}
	}

	keys := make([]cellKey, 0, len(cells))
	for k := range cells {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].X != keys[j].X {
			return keys[i].X < keys[j].X
		}
		return keys[i].Y < keys[j].Y
	})
	out.CellIDs = make([]CellID, 0, len(keys))
	for _, k := range keys {
		out.CellIDs = append(out.CellIDs, CellID{X: k.X, Y: k.Y})
	}
	out.IndependentCells = len(keys)
	out.MixedProvenance = len(sources) > 1

	return out
}

// JSON menyerialkan potret bukti untuk kolom evidence_summary. Kegagalan
// serialisasi tidak mungkin untuk struct ini (tidak ada channel, func, atau NaN
// yang dapat masuk), jadi galat diringkas menjadi objek JSON kosong alih-alih
// diteruskan: penulisan durable tidak boleh punya jalur galat kedua yang harus
// ditangani jalur peringatan.
func (s EvidenceSummary) JSON() []byte {
	b, err := json.Marshal(s)
	if err != nil {
		return []byte(`{}`)
	}
	return b
}
