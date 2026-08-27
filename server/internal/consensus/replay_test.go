package consensus

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// §20.8 — harness replay offline
//
// Sebuah fixture baris ledger (bentuk sensor_observations) dijalankan ulang
// melalui consensus.Evaluate, lalu event yang dihasilkan dibandingkan dengan
// baris alert_emissions yang tercatat. Di Fase 1 algoritmanya TIDAK diubah,
// sehingga test ini tidak menguji korelasi apa pun — ia membuktikan HARNESS-nya
// benar, agar Fase 3 dapat memakainya untuk menunjukkan sebuah perubahan
// korelasi memang perbaikan.
//
// Batas yang disengaja, supaya harness ini tidak menjadi salinan kedua dari
// engine yang akan menyimpang darinya:
//
//   - Yang direplay hanya Evaluate (kluster -> status/centroid/MMI) plus
//     jendela sliding atas received_ts. Cooldown per-sel dan penguncian
//     event_id milik Engine TIDAK direplay; harness hanya melaporkan
//     PERUBAHAN status per sel (nil -> ADVISORY -> CONFIRMED), yang merupakan
//     kaidah eskalasi yang sama tanpa menyalin implementasinya.
//   - Urutan replay adalah received_ts, bukan publish_ts, karena itulah urutan
//     yang benar-benar dilihat server. onset_ts_upper_bound tidak dipakai untuk
//     mengurutkan apa pun: ia batas atas, bukan estimasi onset (§5.1).
// ---------------------------------------------------------------------------

// obsRow adalah satu baris sensor_observations sebagaimana dibaca dari ledger.
// Lat/Lon pointer supaya kasus A16 (verify_result 'OK' + node_location NULL)
// dapat diwakili apa adanya.
type obsRow struct {
	NodeID       string
	PGAGal       float64
	PublishTS    int64
	ReceivedTS   int64
	Lat, Lon     *float64
	VerifyResult string
	LocationName string
}

// emisRow adalah satu baris alert_emissions yang tercatat, dipangkas sampai
// kolom yang keputusan konsensusnya menentukan.
type emisRow struct {
	Status    Status
	MMI       string
	NodeCount int
	PGAGal    float64
}

func ptr(f float64) *float64 { return &f }

// replay memutar ulang baris ledger melalui Evaluate dan mengembalikan emisi
// yang seharusnya dihasilkan. Baris yang tidak dapat memengaruhi konsensus
// disaring lebih dulu, dengan alasan yang sama seperti pipa aslinya:
// verify_result != 'OK' berarti tidak terotentikasi, dan node_location NULL
// berarti tidak ada koordinat untuk mengklusterkannya (§A16).
func replay(rows []obsRow, window time.Duration) []emisRow {
	var (
		readings    []Reading
		out         []emisRow
		lastPerCell = map[string]Status{}
	)
	windowMs := int64(window / time.Millisecond)

	for _, r := range rows {
		if r.VerifyResult != "OK" || r.Lat == nil || r.Lon == nil {
			continue
		}
		readings = append(readings, Reading{
			NodeID:       r.NodeID,
			Lat:          *r.Lat,
			Lon:          *r.Lon,
			PGA:          r.PGAGal,
			TS:           r.PublishTS,
			LocationName: r.LocationName,
		})

		now := r.ReceivedTS
		live := readings[:0]
		for _, rd := range readings {
			if now-rd.TS <= windowMs {
				live = append(live, rd)
			}
		}
		readings = live

		ev := Evaluate(readings, now)
		if ev == nil {
			continue
		}
		cell := cellKey(ev.Centroid)
		if prev, seen := lastPerCell[cell]; seen && prev == ev.Status {
			continue // bukan perubahan status: engine tidak akan mengemisi ulang
		}
		if lastPerCell[cell] == StatusConfirmed && ev.Status == StatusAdvisory {
			continue // tidak ada de-eskalasi
		}
		lastPerCell[cell] = ev.Status
		out = append(out, emisRow{
			Status:    ev.Status,
			MMI:       ev.MMIScale,
			NodeCount: ev.NodeCount,
			PGAGal:    ev.MaxPGA,
		})
	}
	return out
}

// Fixture: tiga node dalam radius 50 km, tiba berurutan di dalam satu jendela
// 8 s. Emisi tercatat: ADVISORY pada node ke-1, lalu CONFIRMED pada node ke-3.
// Node ke-2 tidak mengemisi apa pun karena statusnya masih ADVISORY.
func TestReplayReproducesRecordedEmissions(t *testing.T) {
	const base = int64(1_700_000_000_000)
	rows := []obsRow{
		{NodeID: "N1", PGAGal: 120.0, PublishTS: base, ReceivedTS: base + 50,
			Lat: ptr(-6.90), Lon: ptr(107.60), VerifyResult: "OK", LocationName: "Bandung"},
		{NodeID: "N2", PGAGal: 90.0, PublishTS: base + 400, ReceivedTS: base + 460,
			Lat: ptr(-6.95), Lon: ptr(107.65), VerifyResult: "OK", LocationName: "Cimahi"},
		{NodeID: "N3", PGAGal: 200.0, PublishTS: base + 900, ReceivedTS: base + 980,
			Lat: ptr(-7.00), Lon: ptr(107.70), VerifyResult: "OK", LocationName: "Soreang"},
	}

	// Baris alert_emissions yang tercatat untuk kejadian yang sama.
	recorded := []emisRow{
		{Status: StatusAdvisory, MMI: "VI", NodeCount: 1, PGAGal: 120.0},
		{Status: StatusConfirmed, MMI: "VII", NodeCount: 3, PGAGal: 200.0},
	}

	got := replay(rows, 8*time.Second)
	assertEmissions(t, got, recorded)
}

// Baris yang gagal verifikasi tidak boleh ikut menyumbang suara menuju ambang
// 3-node, sekalipun ia ada di ledger. Ini justru gunanya ledger: penolakan
// tersimpan sebagai bukti, bukan sebagai masukan konsensus.
func TestReplayExcludesRejectedAndUnlocatedRows(t *testing.T) {
	const base = int64(1_700_000_000_000)
	rows := []obsRow{
		{NodeID: "N1", PGAGal: 120.0, PublishTS: base, ReceivedTS: base + 50,
			Lat: ptr(-6.90), Lon: ptr(107.60), VerifyResult: "OK", LocationName: "Bandung"},
		{NodeID: "N2", PGAGal: 400.0, PublishTS: base + 100, ReceivedTS: base + 160,
			Lat: ptr(-6.95), Lon: ptr(107.65), VerifyResult: "ErrBadSignature", LocationName: "Cimahi"},
		// A16: otentik, tetapi lokasinya tidak diketahui saat itu.
		{NodeID: "N3", PGAGal: 300.0, PublishTS: base + 200, ReceivedTS: base + 260,
			VerifyResult: "OK", LocationName: ""},
	}

	// Tercatat: satu ADVISORY dari satu node. Tidak pernah CONFIRMED.
	recorded := []emisRow{
		{Status: StatusAdvisory, MMI: "VI", NodeCount: 1, PGAGal: 120.0},
	}

	got := replay(rows, 8*time.Second)
	assertEmissions(t, got, recorded)
}

// Reading yang lebih tua dari jendela dipangkas: tiga node yang tersebar dalam
// 30 s tidak pernah mencapai konsensus dalam jendela 8 s. Bila harness lupa
// memangkas, ia akan mengemisi CONFIRMED yang tidak pernah tercatat — kegagalan
// yang tepat ingin ditangkap sebelum Fase 3 mempercayai harness ini.
func TestReplayPrunesOutsideWindow(t *testing.T) {
	const base = int64(1_700_000_000_000)
	rows := []obsRow{
		{NodeID: "N1", PGAGal: 120.0, PublishTS: base, ReceivedTS: base + 10,
			Lat: ptr(-6.90), Lon: ptr(107.60), VerifyResult: "OK", LocationName: "Bandung"},
		{NodeID: "N2", PGAGal: 121.0, PublishTS: base + 15_000, ReceivedTS: base + 15_010,
			Lat: ptr(-6.95), Lon: ptr(107.65), VerifyResult: "OK", LocationName: "Cimahi"},
		{NodeID: "N3", PGAGal: 122.0, PublishTS: base + 30_000, ReceivedTS: base + 30_010,
			Lat: ptr(-7.00), Lon: ptr(107.70), VerifyResult: "OK", LocationName: "Soreang"},
	}

	for _, e := range replay(rows, 8*time.Second) {
		if e.Status == StatusConfirmed {
			t.Fatalf("CONFIRMED dari tiga laporan terpisah 15 s dalam jendela 8 s: %+v", e)
		}
		if e.NodeCount != 1 {
			t.Errorf("NodeCount = %d, want 1 (setiap laporan sendirian di jendelanya)", e.NodeCount)
		}
	}
}

// Di bawah MinPGAGal tidak ada emisi sama sekali, sekalipun tiga node sepakat.
func TestReplayBelowMinPGAEmitsNothing(t *testing.T) {
	const base = int64(1_700_000_000_000)
	var rows []obsRow
	for i, id := range []string{"N1", "N2", "N3"} {
		rows = append(rows, obsRow{
			NodeID: id, PGAGal: MinPGAGal - 1.0,
			PublishTS: base + int64(i)*100, ReceivedTS: base + int64(i)*100 + 20,
			Lat: ptr(-6.90 - float64(i)*0.02), Lon: ptr(107.60 + float64(i)*0.02),
			VerifyResult: "OK", LocationName: id,
		})
	}
	if got := replay(rows, 8*time.Second); len(got) != 0 {
		t.Errorf("emisi = %+v, want kosong (di bawah MinPGAGal)", got)
	}
}

func assertEmissions(t *testing.T, got, want []emisRow) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("jumlah emisi = %d, want %d\ngot:  %+v\nwant: %+v",
			len(got), len(want), got, want)
	}
	for i := range want {
		if got[i].Status != want[i].Status {
			t.Errorf("emisi[%d].Status = %q, want %q", i, got[i].Status, want[i].Status)
		}
		if got[i].MMI != want[i].MMI {
			t.Errorf("emisi[%d].MMI = %q, want %q", i, got[i].MMI, want[i].MMI)
		}
		if got[i].NodeCount != want[i].NodeCount {
			t.Errorf("emisi[%d].NodeCount = %d, want %d", i, got[i].NodeCount, want[i].NodeCount)
		}
		if d := got[i].PGAGal - want[i].PGAGal; d > 1e-9 || d < -1e-9 {
			t.Errorf("emisi[%d].PGAGal = %f, want %f", i, got[i].PGAGal, want[i].PGAGal)
		}
	}
}
