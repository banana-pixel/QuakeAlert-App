package event

// Rekonsiliasi saat boot (§15.3) dan pemeriksaan-diri fleet (§7.3, §6.3.1).
//
// Keduanya ada di satu file karena keduanya adalah jalur STARTUP: dijalankan
// sekali, sebelum subscriber menyala, dan keduanya gagal dengan cara yang sama —
// dengan sebuah baris log, bukan dengan menghentikan server. Sebuah server yang
// menolak menyala karena tidak dapat membaca event lampau adalah server yang
// tidak dapat memperingatkan gempa yang sedang berlangsung.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// reloadAnchor adalah agregat baris earthquake_events untuk event yang dimuat
// ulang tanpa bukti. Lihat Event.anchor untuk alasannya ada.
type reloadAnchor struct {
	Lat          float64
	Lon          float64
	LocationName string
	PeakPGA      float64
	NodeCount    int
	Cells        int
}

// Reconcile membangun ulang setiap event yang masih HAPPENING dari basis data
// (§15.3). Dipanggil SEKALI saat boot, sebelum subscriber menyala, sehingga
// observasi pertama setelah restart menempel pada event_id sebelum restart alih-alih
// membentuk event kedua.
//
// Galat DIKEMBALIKAN tetapi tidak fatal bagi pemanggil (§15.3 langkah 5): main.go
// mencatatnya dan tetap menyala, dan sweeper ResolveStaleEvents lama tetap menjadi
// jaring terakhir yang sudah ada.
func (t *Tracker) Reconcile(ctx context.Context) error {
	// Catatan near-confirmation durable dimuat LEBIH DULU, dan lewat assertion
	// tipenya SENDIRI (P4-M2′, D-012).
	//
	// Lebih dulu, karena sebuah event yang menyeberangi restart dapat langsung
	// mengubah entri near-confirmed-nya pada transisi rekonsiliasi di bawah — dan
	// bila tabelnya dimuat SESUDAH itu, entri yang baru saja berubah akan menang
	// atas barisnya sendiri dan riwayat sebelum restart akan hilang tepat pada
	// event yang paling menarik.
	//
	// Assertion sendiri, karena kedua kemampuan itu tidak datang berpasangan:
	// sebuah toko yang dapat memuat event terbuka tetapi belum menjalankan migrasi
	// 000009 tetap sah, dan uji paket ini justru dibangun begitu.
	if nr, ok := t.loc.(nearConfirmedReader); ok {
		t.LoadNearConfirmed(ctx, nr)
	}

	st, ok := t.loc.(eventStore)
	if !ok {
		// Bukan galat: Tracker yang dipasangi sumber lokasi saja tetap sah, dan uji
		// paket ini justru dibangun begitu.
		t.log.Info("event: rekonsiliasi dilewati, sumber lokasi tidak dapat membaca event terbuka")
		return nil
	}

	rows, err := st.LoadOpenEvents(ctx)
	if err != nil {
		return fmt.Errorf("muat event terbuka: %w", err)
	}

	now := t.now().UnixMilli()
	var out []Snapshot

	for _, row := range rows {
		// Pencarian koordinat kontributor terjadi DI LUAR kunci, disiplin yang sama
		// dengan Ingest: rekonsiliasi melakukan satu query per node, dan melakukannya
		// di bawah kunci Tracker akan menahan seluruh jalur masuk selama boot.
		e := t.rebuild(ctx, st, row)
		if e == nil {
			continue
		}

		t.mu.Lock()
		s, kept := t.adoptLocked(e, now)
		t.mu.Unlock()

		if !kept {
			continue
		}
		if s != nil {
			out = append(out, *s)
		}
	}

	// Emisi (dan persistensi resolusi) menyusul di luar kunci, lewat jalur yang sama
	// dengan setiap transisi lain: klien yang menyeberangi restart sambil menampilkan
	// alert mendapat all-clear-nya, dan barisnya mendapat state terminalnya.
	t.publish(ctx, out)

	t.log.Info("event: rekonsiliasi selesai",
		"baris", len(rows), "direkonsiliasi", t.Reconciled(), "diselesaikan", len(out))
	return nil
}

// adoptLocked memasukkan event yang sudah dibangun ulang ke dalam map dan indeks,
// lalu menyelesaikannya bila buktinya sudah kedaluwarsa. Mengembalikan transisi
// yang perlu diemisikan, dan apakah event benar-benar diadopsi.
func (t *Tracker) adoptLocked(e *Event, now int64) (*Snapshot, bool) {
	if _, dup := t.events[e.ID]; dup {
		// Sebuah event dengan id yang sama sudah hidup di memori. Hanya mungkin bila
		// Reconcile dipanggil dua kali; memori adalah otoritas (§9.5), jadi yang di
		// memori yang menang.
		return nil, false
	}

	stale := now-e.LastEvidenceTS > t.opt.ResolveAfterMs

	if stale && !isPublic(e.State) {
		// DETECTED tidak pernah publik dan D->RESOLVED ilegal (§5.2): tidak ada yang
		// dapat ditutup dan tidak ada yang perlu diberi tahu. Baris seperti ini tidak
		// seharusnya ada — DETECTED tidak pernah dipersistensi — jadi ia diserahkan
		// ke jaring ResolveStaleEvents alih-alih dipaksa melewati mesin keadaan.
		t.log.Warn("event: baris HAPPENING non-publik kedaluwarsa dilewati",
			"event_id", e.ID, "event_state", string(e.State))
		return nil, false
	}

	// Langit-langit §15.4 berlaku untuk jalur ini juga. Melebihinya berarti basis
	// data memuat lebih banyak event terbuka daripada yang Tracker boleh pegang, dan
	// yang benar adalah berhenti memuat — bukan mendorong keluar event yang baru saja
	// dimuat oleh event berikutnya dalam daftar yang sama.
	if len(t.openLocked()) >= t.opt.MaxOpen {
		t.log.Warn("event: langit-langit event terbuka tercapai saat rekonsiliasi, sisanya dilewati",
			"event_id", e.ID, "max_open", t.opt.MaxOpen)
		return nil, false
	}

	t.events[e.ID] = e
	t.indexLocked(e, t.keyOf(e))
	t.counters.reconciled++

	if !stale {
		return nil, true
	}

	// Diselesaikan, TETAP DILACAK: event yang baru saja menjadi terminal adalah
	// tombstone (§6.8), dan tombstone itulah yang mencegah bukti yang terlambat
	// membuat alert publik kedua untuk gempa yang sama.
	return t.forceTransitionLocked(e, StateResolved, now, ReasonNoNewEvidence), true
}

// rebuild membangun satu Event dari baris + evidence_summary revisi tertingginya.
// Mengembalikan nil bila baris tidak dapat ditafsirkan.
func (t *Tracker) rebuild(ctx context.Context, src nodeSource, row *store.EarthquakeEvent) *Event {
	state, ok := reloadState(row.EventState)
	if !ok {
		// status='HAPPENING' dengan event_state terminal adalah baris yang saling
		// membantah. Tidak ditebak: proyeksi §9.1 hanya berlaku satu arah, dan
		// mengadopsi baris seperti itu berarti memilih salah satu kolom sebagai
		// pembohong tanpa bukti.
		t.log.Warn("event: baris terbuka dengan event_state tak konsisten dilewati",
			"event_id", row.EventID, "event_state", row.EventState, "status", row.Status)
		return nil
	}

	// Waktu bukti terakhir: decided_at transisi terakhir, dengan started_at sebagai
	// cadangan. Cadangannya konservatif ke arah yang benar — sebuah event tanpa
	// baris log terlihat LEBIH TUA daripada mungkin sebenarnya, jadi ia diselesaikan
	// lebih awal alih-alih menggantung tanpa batas.
	lastEvidence := row.LatestDecidedAt
	if lastEvidence == 0 {
		lastEvidence = row.StartedAtMs
	}
	origin := row.OriginTS
	if origin == 0 {
		origin = row.StartedAtMs
	}

	e := &Event{
		ID:             row.EventID,
		State:          state,
		Revision:       row.Revision,
		OriginTS:       origin,
		OriginTSSource: row.OriginTSSource,
		DecidedAt:      lastEvidence,
		LastEvidenceTS: lastEvidence,
		CreatedAt:      row.StartedAtMs,
		Contributors:   make(map[string]*Contributor, 4),
		// EverConfirmed diturunkan dari state yang tersimpan, bukan dari riwayat:
		// hak FCM pada all-clear (§8.1) milik audiens yang pernah menerima alarm, dan
		// satu-satunya state tersimpan yang pernah mengirim alarm adalah CONFIRMED.
		EverConfirmed: state == StateConfirmed,
		minCells:      t.opt.MinIndependentCells,
		minSepKm:      t.opt.IndependenceCellKm,
		anchor: &reloadAnchor{
			Lat:          row.CentroidLat,
			Lon:          row.CentroidLon,
			LocationName: row.LocationName,
			PeakPGA:      row.MaxPGA,
			NodeCount:    row.TriggeredNodes,
			Cells:        row.IndependentCellCount,
		},
	}

	t.restoreContributors(ctx, src, e, row)
	return e
}

// restoreContributors mengisi kontributor dari potret bukti. Koordinat DICARI
// ULANG lewat nodeSource karena potret tidak menyimpannya: ContributorEvidence
// sengaja tidak membawa lat/lon, dan menyimpannya di sana akan membuat sebuah node
// yang dipindahkan punya dua posisi yang berbeda di dalam satu basis data.
//
// Sel independensi dihitung ULANG dari koordinat sekarang, dengan
// IndependenceCellKm yang berlaku SEKARANG — bukan yang dibaca dari algo_ver baris
// itu. Event yang hidup harus dinilai dengan parameter biner yang menilainya, dan
// algo_ver ada untuk menjelaskan keputusan LAMPAU, bukan untuk menghidupkannya
// kembali.
func (t *Tracker) restoreContributors(ctx context.Context, src nodeSource, e *Event, row *store.EarthquakeEvent) {
	if len(row.LatestEvidence) == 0 {
		return
	}

	var ev EvidenceSummary
	if err := json.Unmarshal(row.LatestEvidence, &ev); err != nil {
		t.log.Warn("event: evidence_summary tidak dapat dibaca, event dimuat tanpa kontributor",
			"event_id", row.EventID, "err", err)
		return
	}

	for _, c := range ev.Contributors {
		nl, err := src.GetNodeLocation(ctx, c.NodeID)
		if err != nil {
			// Gagal tertutup, sama seperti Ingest: kontributor tanpa koordinat tidak
			// dapat menyumbang ke geometri apa pun. Event tetap dimuat — kehilangan satu
			// suara jauh lebih ringan daripada kehilangan identitas event.
			t.log.Warn("event: lokasi kontributor tak ditemukan saat rekonsiliasi",
				"event_id", row.EventID, "node_id", c.NodeID, "err", err)
			continue
		}
		obsSeq := c.ObsSeq
		e.Contributors[c.NodeID] = &Contributor{
			NodeID:        c.NodeID,
			Lat:           nl.Lat,
			Lon:           nl.Lon,
			LocationName:  nl.LocationName,
			Cell:          independenceCell(nl.Lat, nl.Lon, t.opt.IndependenceCellKm),
			ObsSeq:        obsSeq,
			Phase:         c.Phase,
			PeakPGA:       c.PeakPGA,
			OnsetTS:       c.OnsetTS,
			OnsetSource:   c.OnsetSource,
			LastPublishTS: c.OnsetTS,
			Revisions:     1,
		}
	}
}

// reloadState memetakan kolom event_state ke State. String kosong adalah baris
// pra-Fase-3: ia punya status tetapi tidak punya state, dan justru baris itulah
// yang paling mungkin menggantung setelah restart.
//
// Kosong dibaca sebagai UNCONFIRMED, bukan CONFIRMED, dan itu keputusan yang
// disengaja: satu-satunya konsekuensi state yang dipulihkan adalah hak FCM pada
// all-clear (§8.1), dan sebuah boot yang menemukan sepuluh baris pra-Fase-3
// menggantung tidak boleh mengirim sepuluh push ke seluruh fleet. Frame WS tetap
// terkirim, jadi klien yang benar-benar menampilkan alert tetap dibersihkan.
func reloadState(col string) (State, bool) {
	switch col {
	case string(StateConfirmed):
		return StateConfirmed, true
	case string(StateUnconfirmed), "":
		return StateUnconfirmed, true
	case string(StateDetected):
		return StateDetected, true
	default:
		return "", false
	}
}

// CheckFleetIndependence adalah pemeriksaan-diri saat boot (§7.3): ia mengeluh
// SEKARANG tentang fleet yang tidak akan pernah dapat mencapai CONFIRMED, alih-alih
// membiarkan operator menyimpulkannya setelah gempa dari alert yang tidak pernah
// datang.
//
// Server tetap menyala, dan ambangnya TIDAK diturunkan sendiri. Menurunkan ambang
// keselamatan agar sebuah peringatan startup berhenti muncul adalah cara sebuah
// sistem berhenti bekerja tanpa ada yang memutuskannya.
func (t *Tracker) CheckFleetIndependence(ctx context.Context) {
	st, ok := t.loc.(eventStore)
	if !ok {
		return
	}

	nodes, err := st.ListActiveNodeLocations(ctx)
	if err != nil {
		t.log.Warn("event: pemeriksaan independensi fleet gagal", "err", err)
		return
	}
	if len(nodes) == 0 {
		t.log.Warn("event: tidak ada node aktif terverifikasi — tidak ada alert yang dapat dihasilkan")
		return
	}

	// TIDAK ADA pemeriksaan pita lintang di sini lagi, dan hilangnya adalah inti
	// perbaikan ini. Dahulu sebuah node di luar |lat| <= 12° dicatat pada ERROR
	// karena pembuktian kecukupan lingkungan 3x3 tidak lagi berlaku di sana.
	// Lingkungan yang diselidiki sekarang diturunkan dari lintang observasi itu
	// sendiri (probeSpan), jadi kecukupannya tidak lagi bergantung pada pita mana
	// pun: baris ERROR itu akan menjadi peringatan yang SALAH, dan peringatan
	// startup yang salah adalah peringatan yang akan diabaikan operator ketika
	// suatu hari ia benar.
	// Penghitung yang SAMA dengan gerbang CONFIRMED (independence.go), atas
	// koordinat fleet: sebuah pemeriksaan-diri yang memakai aritmetika berbeda dari
	// yang digerbangnya adalah pemeriksaan yang dapat lulus untuk fleet yang tidak
	// akan pernah dikonfirmasi.
	pts := make([]geoPoint, 0, len(nodes))
	for _, n := range nodes {
		pts = append(pts, geoPoint{id: n.StationID, lat: n.Lat, lon: n.Lon})
	}
	independent := independentCount(pts, t.opt.IndependenceCellKm)

	if independent < t.opt.MinIndependentCells {
		t.log.Warn("consensus: CONFIRMED tidak dapat dicapai — alert akan berhenti di UNCONFIRMED",
			"active_verified_nodes", len(nodes),
			"independence_cells", independent,
			"independence_cell_km", t.opt.IndependenceCellKm,
			"need", t.opt.MinIndependentCells)
		return
	}

	t.log.Info("event: pemeriksaan independensi fleet lulus",
		"active_verified_nodes", len(nodes),
		"independence_cells", independent,
		"independence_cell_km", t.opt.IndependenceCellKm)
}
