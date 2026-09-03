package store

// --- Integrasi Postgres untuk pembacaan replay deterministik P4-M4′ ---
//
// Butuh Postgres NYATA, dan alasannya sama seperti trace_read_test.go: yang
// diuji di sini adalah perilaku KUERI-nya. Empat hal yang tidak dapat diuji
// dengan fake, dan ketiadaan uji ini berarti kedua SELECT itu belum pernah
// benar-benar dieksekusi terhadap skema mana pun:
//
//	ListObservationsForReplay — urutan KANONIK (received_ts, observation_id) yang
//	                            REPLAY ANDALKAN sebagai satu-satunya urutan yang
//	                            dapat diulang; batas interval TERTUTUP di kedua
//	                            ujung; dan sifat TIDAK MENYARING — baris
//	                            verify_result != 'OK' dan node_location NULL
//	                            harus tetap kembali supaya jumlah yang tersaring
//	                            dapat DILAPORKAN, bukan hilang tanpa jejak.
//	ListStateLogForReplay     — urutan KANONIK (event_id, revision), yang membuat
//	                            perbandingan per-event sah; sweepLocked()
//	                            mengiterasi map sehingga urutan antar-event di
//	                            dalam satu tik tidak terdefinisi.
//
//	TEST_DATABASE_URL=postgres://... go test ./internal/store -run TestReplayRead
//
// Tanpa env itu seluruh test di berkas ini skip.

import (
	"context"
	"testing"
)

// seedReplayObs menulis satu observasi lewat penulis PRODUKSI
// (InsertObservation) dan mengembalikan observation_id yang Postgres berikan.
//
// Bukan INSERT tangan, dan bukan id yang ditentukan uji: observation_id adalah
// BIGSERIAL, jadi urutannya adalah urutan PENULISAN dan hanya basis data yang
// mengetahuinya. Sebuah uji yang menuliskan id sendiri akan menguji ketikannya
// sendiri, bukan ORDER BY.
func seedReplayObs(t *testing.T, st *Store, o *Observation) int64 {
	t.Helper()
	ctx := context.Background()
	if err := st.InsertObservation(ctx, o); err != nil {
		t.Fatalf("InsertObservation(%s @ %d): %v", o.NodeID, o.ReceivedTS, err)
	}
	var id int64
	if err := st.pool.QueryRow(ctx, `
		SELECT observation_id FROM sensor_observations
		WHERE node_id = $1 AND received_ts = $2
		ORDER BY observation_id DESC LIMIT 1`, o.NodeID, o.ReceivedTS).Scan(&id); err != nil {
		t.Fatalf("baca observation_id: %v", err)
	}
	return id
}

// rrObs membangun observasi v2 minimal yang sah. withLoc=false meninggalkan
// node_location NULL — kasus A16 yang wajib tetap tercatat.
func rrObs(node string, receivedTS int64, verify string, withLoc bool) *Observation {
	proto := int16(2)
	seq := receivedTS % 1_000_000
	onset := receivedTS - 300
	upper := receivedTS - 280
	o := &Observation{
		NodeID: node, SourceClass: "FIXED_ESP32", Phase: "FINAL",
		ProtoVer: &proto, ObsSeq: &seq,
		PGAGal: 120.5, DurMs: 300,
		PublishTS: receivedTS - 20, ReceivedTS: receivedTS,
		OnsetTS: &onset, OnsetTSUpperBound: &upper, OnsetTSSource: "SENSOR",
		VerifyResult: verify,
	}
	if withLoc {
		lat, lon := -6.9034443, 107.6431173
		o.Lat, o.Lon = &lat, &lon
	}
	return o
}

// cleanupObs menghapus setiap observasi milik node-node uji.
func cleanupObs(t *testing.T, st *Store, nodes ...string) {
	t.Helper()
	t.Cleanup(func() {
		for _, n := range nodes {
			_, _ = st.pool.Exec(context.Background(),
				`DELETE FROM sensor_observations WHERE node_id = $1`, n)
		}
	})
}

// Interval TERTUTUP di kedua ujung, dan urutan kanonik (received_ts,
// observation_id). Yang dicegah regresinya: sebuah `>` alih-alih `>=` membuang
// observasi PERTAMA sebuah jendela, dan replay yang kehilangan onset akan
// melaporkan divergensi yang seluruhnya adalah galat kueri.
func TestReplayReadObservationsClosedIntervalAndCanonicalOrder(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const node = "NODE-4RPLY001"
	cleanupObs(t, st, node)

	base := int64(1_766_000_000_000)
	// Ditulis SENGAJA di luar urutan waktu, supaya ORDER BY yang benar-benar
	// mengurutkan dapat dibedakan dari kueri yang hanya mewarisi urutan sisip.
	idMid := seedReplayObs(t, st, rrObs(node, base+1000, "OK", true))
	idLo := seedReplayObs(t, st, rrObs(node, base, "OK", true))
	idHi := seedReplayObs(t, st, rrObs(node, base+2000, "OK", true))
	// Dua baris pada received_ts YANG SAMA: pemutus-seri observation_id adalah
	// satu-satunya yang membuat urutan total, dan hanya di sini ia terlihat.
	idTieA := seedReplayObs(t, st, rrObs(node, base+1500, "OK", true))
	idTieB := seedReplayObs(t, st, rrObs(node, base+1500, "OK", true))
	// Di LUAR jendela pada kedua ujung.
	_ = seedReplayObs(t, st, rrObs(node, base-1, "OK", true))
	_ = seedReplayObs(t, st, rrObs(node, base+2001, "OK", true))

	got, err := st.ListObservationsForReplay(ctx, base, base+2000)
	if err != nil {
		t.Fatalf("ListObservationsForReplay: %v", err)
	}

	wantIDs := []int64{idLo, idMid, idTieA, idTieB, idHi}
	if len(got) != len(wantIDs) {
		for _, o := range got {
			t.Logf("kembali: id=%d received_ts=%d", o.ObservationID, o.ReceivedTS)
		}
		t.Fatalf("baris = %d; mau %d — batas [from, to] harus TERTUTUP di kedua ujung",
			len(got), len(wantIDs))
	}
	for i, want := range wantIDs {
		if got[i].ObservationID != want {
			t.Errorf("posisi %d: observation_id = %d; mau %d (urutan kanonik "+
				"received_ts lalu observation_id)", i, got[i].ObservationID, want)
		}
	}
	// Monotonisitas dinyatakan terpisah dari daftar id: ia sifat yang Replay
	// andalkan, dan ia harus berbunyi sendiri bila daftar id berubah.
	for i := 1; i < len(got); i++ {
		a, b := got[i-1], got[i]
		if a.ReceivedTS > b.ReceivedTS ||
			(a.ReceivedTS == b.ReceivedTS && a.ObservationID > b.ObservationID) {
			t.Errorf("urutan tidak monoton di posisi %d: (%d,%d) sebelum (%d,%d)",
				i, a.ReceivedTS, a.ObservationID, b.ReceivedTS, b.ObservationID)
		}
	}
	if got[0].ReceivedTS != base {
		t.Errorf("ujung bawah = %d; mau %d — batas bawah TERTUTUP", got[0].ReceivedTS, base)
	}
	if got[len(got)-1].ReceivedTS != base+2000 {
		t.Errorf("ujung atas = %d; mau %d — batas atas TERTUTUP",
			got[len(got)-1].ReceivedTS, base+2000)
	}
}

// Kueri TIDAK MENYARING. Baris verify_result != 'OK' dan baris node_location
// NULL harus tetap kembali, dan NULL harus tiba SEBAGAI NULL.
//
// Yang dicegah regresinya: sebuah WHERE verify_result = 'OK' membuat replay
// tampak lengkap padahal masukannya tidak — event.Replay-lah yang membuang baris
// itu, dan ia membuangnya ke dalam InputReport.Skipped supaya jumlahnya dapat
// DILAPORKAN. Sebuah kueri yang menyaring sendiri menghapus laporan itu.
//
// Lat/Lon nil juga bukan detail kosmetik: ST_Y/ST_X atas geografi NULL harus
// menghasilkan NULL, bukan nol. Nol adalah koordinat yang sah di Teluk Guinea,
// dan sebuah observasi tanpa lokasi yang terbaca sebagai (0,0) akan diumpankan
// ke Tracker alih-alih dilaporkan sebagai NODE_LOCATION_NULL.
func TestReplayReadObservationsAreUnfilteredAndNullLocationStaysNull(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const node = "NODE-4RPLY002"
	cleanupObs(t, st, node)

	base := int64(1_766_100_000_000)
	idOK := seedReplayObs(t, st, rrObs(node, base, "OK", true))
	idBadVerify := seedReplayObs(t, st, rrObs(node, base+100, "ErrBadSignature", true))
	idNoLoc := seedReplayObs(t, st, rrObs(node, base+200, "OK", false))

	got, err := st.ListObservationsForReplay(ctx, base, base+200)
	if err != nil {
		t.Fatalf("ListObservationsForReplay: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("baris = %d; mau 3 — kueri TIDAK boleh menyaring", len(got))
	}

	byID := make(map[int64]ReplayObservation, 3)
	for _, o := range got {
		byID[o.ObservationID] = o
	}

	if o := byID[idOK]; o.VerifyResult != "OK" || o.Lat == nil || o.Lon == nil {
		t.Errorf("baris OK: verify=%q lat=%v lon=%v; mau OK dengan koordinat",
			o.VerifyResult, o.Lat, o.Lon)
	}
	if o := byID[idBadVerify]; o.VerifyResult != "ErrBadSignature" {
		t.Errorf("baris gagal verifikasi: verify_result = %q; mau ErrBadSignature "+
			"— barisnya harus KEMBALI, penyaringannya milik pemanggil", o.VerifyResult)
	}
	o := byID[idNoLoc]
	if o.Lat != nil || o.Lon != nil {
		t.Errorf("node_location NULL terbaca lat=%v lon=%v; mau nil/nil — "+
			"(0,0) adalah koordinat yang sah dan tidak boleh menggantikan NULL",
			o.Lat, o.Lon)
	}

	// Kolom provenance migrasi 000007/v2 harus terbawa apa adanya: replay
	// memakai onset_ts_source untuk memilih jangkar onset, dan obs_seq untuk
	// tanda tangan pengelompokan (F2).
	if o.OnsetTSSource != "SENSOR" || o.OnsetTS == nil || o.ObsSeq == nil || o.ProtoVer == nil {
		t.Errorf("provenance hilang: source=%q onset=%v seq=%v proto=%v",
			o.OnsetTSSource, o.OnsetTS, o.ObsSeq, o.ProtoVer)
	}
}

// Urutan kanonik (event_id, revision) dan interval TERTUTUP atas decided_at.
//
// Urutan per-event itu bukan preferensi: sweepLocked() mengiterasi map, jadi
// urutan RESOLVED ANTAR-event di dalam satu tik tidak terdefinisi dan satu
// aliran global tidak dapat dibandingkan. Bentuk hasil inilah yang memaksa
// pemanggil membandingkan per event.
//
// Dua event dengan revisi yang SALING SILANG dalam waktu adalah yang membuat uji
// ini bermakna: sebuah ORDER BY decided_at akan mengembalikan baris yang benar
// dalam urutan yang salah, dan setiap pemeriksaan yang hanya melihat panjang
// tetap lolos.
func TestReplayReadStateLogCanonicalOrderAndClosedInterval(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	// evB diurut SETELAH evA secara leksikografis, dan disisipkan LEBIH DULU.
	const (
		evB = "bbbbbbbb-0000-4000-8000-0000000004b1"
		evA = "aaaaaaaa-0000-4000-8000-0000000004a2"
	)
	seedEvent(t, st, newPhase3Event(evB, EventStateResolved, 2))
	seedEvent(t, st, newPhase3Event(evA, EventStateResolved, 2))

	base := int64(1_766_200_000_000)
	det := EventStateDetected
	unc := EventStateUnconfirmed
	// Interleaved: B rev1, A rev1, B rev2, A rev2.
	rows := []*EventStateLog{
		{EventID: evB, Revision: 1, FromState: &det, ToState: EventStateUnconfirmed,
			Reason: "FLOOR_MET", DecidedAt: base, NodeCount: 1, IndependentCells: 1,
			EvidenceSummary: []byte(`{}`), AlgoVer: "phase3-1.1/ic=5"},
		{EventID: evA, Revision: 1, FromState: &det, ToState: EventStateUnconfirmed,
			Reason: "FLOOR_MET", DecidedAt: base + 1000, NodeCount: 1, IndependentCells: 1,
			EvidenceSummary: []byte(`{}`), AlgoVer: "phase3-1.1/ic=5"},
		{EventID: evB, Revision: 2, FromState: &unc, ToState: EventStateResolved,
			Reason: "NO_NEW_EVIDENCE", DecidedAt: base + 2000, NodeCount: 1, IndependentCells: 1,
			EvidenceSummary: []byte(`{}`), AlgoVer: "phase3-1.1/ic=5"},
		{EventID: evA, Revision: 2, FromState: &unc, ToState: EventStateResolved,
			Reason: "NO_NEW_EVIDENCE", DecidedAt: base + 3000, NodeCount: 1, IndependentCells: 1,
			EvidenceSummary: []byte(`{}`), AlgoVer: "phase3-1.1/ic=5"},
	}
	for _, l := range rows {
		if err := st.AppendStateLog(ctx, l); err != nil {
			t.Fatalf("AppendStateLog(%s rev %d): %v", l.EventID, l.Revision, err)
		}
	}
	// Di LUAR jendela, satu di tiap ujung.
	outside := []*EventStateLog{
		{EventID: evA, Revision: 3, FromState: &unc, ToState: EventStateResolved,
			Reason: "NO_NEW_EVIDENCE", DecidedAt: base - 1, NodeCount: 1, IndependentCells: 1,
			EvidenceSummary: []byte(`{}`), AlgoVer: "phase3-1.1/ic=5"},
		{EventID: evB, Revision: 3, FromState: &unc, ToState: EventStateResolved,
			Reason: "NO_NEW_EVIDENCE", DecidedAt: base + 3001, NodeCount: 1, IndependentCells: 1,
			EvidenceSummary: []byte(`{}`), AlgoVer: "phase3-1.1/ic=5"},
	}
	for _, l := range outside {
		if err := st.AppendStateLog(ctx, l); err != nil {
			t.Fatalf("AppendStateLog luar jendela: %v", err)
		}
	}

	got, err := st.ListStateLogForReplay(ctx, base, base+3000)
	if err != nil {
		t.Fatalf("ListStateLogForReplay: %v", err)
	}
	if len(got) != 4 {
		for _, l := range got {
			t.Logf("kembali: %s rev %d @ %d", l.EventID, l.Revision, l.DecidedAt)
		}
		t.Fatalf("baris = %d; mau 4 — batas decided_at TERTUTUP di kedua ujung", len(got))
	}

	type key struct {
		id  string
		rev int
	}
	want := []key{{evA, 1}, {evA, 2}, {evB, 1}, {evB, 2}}
	for i, w := range want {
		if got[i].EventID != w.id || got[i].Revision != w.rev {
			t.Errorf("posisi %d: (%s, rev %d); mau (%s, rev %d) — urutan kanonik "+
				"event_id lalu revision, BUKAN decided_at",
				i, got[i].EventID, got[i].Revision, w.id, w.rev)
		}
	}
	// AlgoVer terbawa apa adanya: ia gerbang replay (CheckAlgoVer) dan kunci
	// pengelompokan V5.
	for _, l := range got {
		if l.AlgoVer != "phase3-1.1/ic=5" {
			t.Errorf("%s rev %d: algo_ver = %q; mau phase3-1.1/ic=5",
				l.EventID, l.Revision, l.AlgoVer)
		}
		if l.FromState == nil {
			t.Errorf("%s rev %d: from_state nil; mau terbawa", l.EventID, l.Revision)
		}
	}
}
