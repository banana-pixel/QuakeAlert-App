package store

// --- Integrasi Postgres untuk pembacaan garis-waktu event P4-M6′ ---
//
// Butuh Postgres NYATA, dan alasannya sama seperti trace_read_test.go dan
// replay_read_test.go: yang diuji di sini adalah perilaku KUERI-nya. Tiga hal
// yang tidak dapat diuji dengan fake, dan tanpa uji ini ketiga SELECT itu belum
// pernah benar-benar dieksekusi terhadap skema mana pun:
//
//	EventByID                       — SELECT PERTAMA per-event_id atas
//	                                  earthquake_events. Cast $1::uuid, ST_Y/ST_X
//	                                  atas geografi, COALESCE atas started_at yang
//	                                  NULLABLE, dan pemetaan pgx.ErrNoRows ->
//	                                  ErrEventNotFound hanya nyata di Postgres.
//	                                  Baris pra-Fase-3 (event_state NULL, origin_ts
//	                                  NULL) wajib TERBACA, bukan mematikan alat.
//	ListStateLogForEvent            — urutan revision ASC, TANPA jendela dan TANPA
//	                                  LIMIT. decided_at TIDAK boleh menjadi urutan:
//	                                  ia jam server, dan sebuah langkah NTP atau
//	                                  dua transisi pada milidetik yang sama membuat
//	                                  urutan itu salah tanpa satu pun baris hilang.
//	ListObservationsForNodesInWindow— relasi KEANGGOTAAN-DAN-WAKTU dalam bentuk
//	                                  kueri: node_id = ANY($1) dengan received_ts
//	                                  di interval TERTUTUP, urutan kanonik
//	                                  (received_ts, observation_id), dan sifat
//	                                  TIDAK MENYARING — verify_result != 'OK' dan
//	                                  node_location NULL harus tetap kembali supaya
//	                                  jumlah yang dibuang dapat DILAPORKAN.
//
//	TEST_DATABASE_URL=postgres://... go test ./internal/store -run TestEventTimeline
//
// Tanpa env itu seluruh test di berkas ini skip.
//
// Ketiga kueri ini READ-ONLY. Yang menulis di berkas ini hanyalah penyemai, dan
// penyemai memakai penulis PRODUKSI (UpsertEvent, AppendStateLog,
// InsertObservation) — sebuah uji yang menyisipkan dengan tangan akan menguji
// ketikannya sendiri, bukan bentuk baris yang benar-benar ditulis produksi.

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// etEvidence menyusun satu snapshot evidence_summary dengan tangan.
//
// Dengan tangan karena harus: package store tidak boleh mengimpor
// internal/event (event mengimpor store), jadi event.EvidenceSummary tidak
// tersedia di sini. Yang diuji berkas ini juga bukan bentuk JSON-nya melainkan
// apakah JSONB kembali utuh; pembacaan strukturnya diuji di
// internal/event/timeline_test.go lewat penulis produksi.
func etEvidence(nodes ...string) []byte {
	var b strings.Builder
	b.WriteString(`{"contributors":[`)
	for i, n := range nodes {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"node_id":"`)
		b.WriteString(n)
		b.WriteString(`","peak_pga":42.5,"phase":"FINAL","onset_ts":1766000000000,`)
		b.WriteString(`"onset_source":"SENSOR","obs_seq":1001,"cell":{"x":1,"y":2}}`)
	}
	b.WriteString(`],"independent_cells":`)
	if len(nodes) > 1 {
		b.WriteString("2")
	} else {
		b.WriteString("1")
	}
	b.WriteString(`,"cell_ids":[{"x":1,"y":2}],"origin_ts_source":"SENSOR"}`)
	return []byte(b.String())
}

// etSeedLog menulis satu baris transisi lewat penulis PRODUKSI (AppendStateLog).
// Pembersihannya sudah ditangani seedEvent, yang menghapus event_state_log lebih
// dulu — FK-nya menuntut urutan itu.
func etSeedLog(t *testing.T, st *Store, l *EventStateLog) {
	t.Helper()
	if err := st.AppendStateLog(context.Background(), l); err != nil {
		t.Fatalf("AppendStateLog(%s rev %d): %v", l.EventID, l.Revision, err)
	}
}

// etObs membangun observasi minimal yang sah, dengan pga dan verify_result yang
// dapat dipilih: keduanya adalah kolom yang justru TIDAK boleh disaring kueri.
func etObs(node string, receivedTS int64, pga float64, verify string, withLoc bool) *Observation {
	proto := int16(2)
	seq := receivedTS % 1_000_000
	onset := receivedTS - 300
	upper := receivedTS - 280
	o := &Observation{
		NodeID: node, SourceClass: "FIXED_ESP32", Phase: "FINAL",
		ProtoVer: &proto, ObsSeq: &seq,
		PGAGal: pga, DurMs: 300,
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

// Baris event terbaca UTUH, dan event_id yang tidak ada menghasilkan
// ErrEventNotFound — bukan baris nol dan bukan panik.
//
// Yang dicegah regresinya: sebuah alat forensik yang mengembalikan
// EarthquakeEvent kosong untuk UUID yang salah ketik akan melaporkan "event ini
// tidak punya kontributor" alih-alih "event ini tidak ditemukan", dan kedua
// pernyataan itu tidak sama.
func TestEventTimelineEventByIDReadsRowAndReportsAbsence(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const id = "aaaaaaaa-0000-4000-8000-0000000006a1"

	want := newPhase3Event(id, EventStateConfirmed, 3)
	want.OriginTS = 1_766_000_000_000
	want.OriginTSSource = "SENSOR"
	want.IndependentCellCount = 2
	want.AlgoVer = "phase3-1.1/ic=5"
	seedEvent(t, st, want)

	got, err := st.EventByID(ctx, id)
	if err != nil {
		t.Fatalf("EventByID: %v", err)
	}
	if got.EventID != id || got.EventState != EventStateConfirmed || got.Revision != 3 {
		t.Errorf("identitas/state: id=%s state=%q rev=%d; mau %s/%s/3",
			got.EventID, got.EventState, got.Revision, id, EventStateConfirmed)
	}
	if got.OriginTS != want.OriginTS || got.OriginTSSource != "SENSOR" ||
		got.IndependentCellCount != 2 || got.AlgoVer != "phase3-1.1/ic=5" {
		t.Errorf("provenance lifecycle hilang: origin=%d/%q ic=%d algo=%q",
			got.OriginTS, got.OriginTSSource, got.IndependentCellCount, got.AlgoVer)
	}
	// Sentroid lewat ST_Y/ST_X: urutan (lon, lat) di ST_MakePoint terbalik dari
	// urutan (lat, lon) di struct, dan pertukaran itu menghasilkan koordinat yang
	// sah di Samudra Hindia — tidak ada galat yang akan menyebutkannya.
	if !closeEnough(got.CentroidLat, want.CentroidLat) || !closeEnough(got.CentroidLon, want.CentroidLon) {
		t.Errorf("sentroid = (%f, %f); mau (%f, %f) — ST_Y adalah lat, ST_X adalah lon",
			got.CentroidLat, got.CentroidLon, want.CentroidLat, want.CentroidLon)
	}
	if got.MaxPGA != want.MaxPGA || got.TriggeredNodes != 3 || got.Status != "HAPPENING" {
		t.Errorf("kolom Fase 2: pga=%f nodes=%d status=%q", got.MaxPGA, got.TriggeredNodes, got.Status)
	}
	if got.StartedAtMs != want.StartedAtMs {
		t.Errorf("started_at_ms = %d; mau %d", got.StartedAtMs, want.StartedAtMs)
	}
	// EventByID TIDAK mengisi kedua kolom turunan LoadOpenEvents. Membiarkannya
	// terisi di sini akan menaruh "bukti revisi tertinggi" di tempat kedua dan
	// mengundang pembaca menyangka itu ringkasan seluruh riwayat.
	if got.LatestEvidence != nil || got.LatestDecidedAt != 0 {
		t.Errorf("kolom turunan terisi: evidence=%s decided=%d — EventByID bukan LoadOpenEvents",
			got.LatestEvidence, got.LatestDecidedAt)
	}

	absent, err := st.EventByID(ctx, "aaaaaaaa-0000-4000-8000-0000000006ff")
	if !errors.Is(err, ErrEventNotFound) {
		t.Fatalf("event yang tidak ada: err = %v (baris %v); mau ErrEventNotFound", err, absent)
	}
	if absent != nil {
		t.Errorf("baris = %+v; mau nil bersama ErrEventNotFound", absent)
	}
}

// Baris PRA-Fase-3 harus terbaca, bukan mematikan alat.
//
// Baris seperti ini punya event_state NULL, origin_ts NULL, algo_ver NULL, dan
// started_at yang NULLABLE. Sebuah forensik yang panik di sini akan panik justru
// pada baris tertua — yang paling perlu dilihat setelah insiden.
func TestEventTimelineEventByIDReadsPrePhase3Row(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	// Lewat SaveEvent-nya jalur Fase 2, supaya bentuknya benar-benar bentuk lama.
	id, err := st.SaveEvent(ctx, &EarthquakeEvent{
		Status: "HAPPENING", CentroidLat: -6.9, CentroidLon: 107.6,
		LocationName: "Bandung", MMIScale: "IV", IntensityLabel: "Ringan",
		MaxPGA: 18.25, TriggeredNodes: 3, StartedAtMs: 1_700_000_000_000,
	})
	if err != nil {
		t.Fatalf("SaveEvent: %v", err)
	}
	t.Cleanup(func() {
		_, _ = st.pool.Exec(ctx, `DELETE FROM earthquake_events WHERE event_id = $1`, id)
	})

	got, err := st.EventByID(ctx, id)
	if err != nil {
		t.Fatalf("EventByID atas baris pra-Fase-3: %v", err)
	}
	// Kosong berarti TIDAK DIKETAHUI. Yang penting di sini: kosong, bukan galat,
	// dan bukan nilai yang dikarang.
	if got.EventState != "" || got.OriginTS != 0 || got.OriginTSSource != "" || got.AlgoVer != "" {
		t.Errorf("baris lama tidak boleh punya nilai lifecycle: state=%q origin=%d/%q algo=%q",
			got.EventState, got.OriginTS, got.OriginTSSource, got.AlgoVer)
	}
	if got.Revision != 0 {
		t.Errorf("revision = %d; mau 0 (DEFAULT kolom 000008)", got.Revision)
	}
	if got.Status != "HAPPENING" || got.MaxPGA != 18.25 {
		t.Errorf("kolom Fase 2 tidak terbaca utuh: %+v", got)
	}
}

// SELURUH riwayat, diurutkan revision ASC, tanpa jendela dan tanpa LIMIT.
//
// Yang dicegah regresinya, dan tiap satunya lolos dari pemeriksaan yang hanya
// melihat panjang:
//
//	ORDER BY decided_at   — decided_at adalah JAM SERVER. Dua revisi ditulis di
//	                        sini dengan decided_at yang TERBALIK terhadap
//	                        revision-nya (langkah jam ke belakang), jadi kueri
//	                        yang mengurut menurut waktu akan mengembalikan seluruh
//	                        baris yang benar dalam urutan yang salah.
//	LIMIT tersembunyi     — riwayat yang dipotong tidak dapat membedakan "revisi
//	                        itu tidak pernah ada" dari "revisi itu di luar
//	                        potongan".
//	kebocoran antar-event  — baris event LAIN dengan revisi yang sama tidak boleh
//	                        muncul.
func TestEventTimelineStateLogIsWholeHistoryOrderedByRevision(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const (
		id    = "aaaaaaaa-0000-4000-8000-0000000006a2"
		other = "aaaaaaaa-0000-4000-8000-0000000006a3"
	)
	seedEvent(t, st, newPhase3Event(id, EventStateResolved, 4))
	seedEvent(t, st, newPhase3Event(other, EventStateResolved, 4))

	base := int64(1_766_300_000_000)
	det := EventStateDetected
	unc := EventStateUnconfirmed
	con := EventStateConfirmed

	// Disisipkan di luar urutan revisi, dan rev 3 diberi decided_at LEBIH AWAL
	// dari rev 2 — jam server boleh melangkah ke belakang, revision tidak.
	etSeedLog(t, st, &EventStateLog{EventID: id, Revision: 2, FromState: &unc,
		ToState: EventStateConfirmed, Reason: "QUORUM_MET", DecidedAt: base + 5000,
		NodeCount: 3, IndependentCells: 2, EvidenceSummary: etEvidence("NODE-A", "NODE-B", "NODE-C"),
		AlgoVer: "phase3-1.1/ic=5"})
	etSeedLog(t, st, &EventStateLog{EventID: id, Revision: 4, FromState: &con,
		ToState: EventStateResolved, Reason: "NO_NEW_EVIDENCE", DecidedAt: base + 90000,
		NodeCount: 3, IndependentCells: 2, EvidenceSummary: etEvidence("NODE-A", "NODE-B", "NODE-C"),
		AlgoVer: "phase3-1.1/ic=5"})
	etSeedLog(t, st, &EventStateLog{EventID: id, Revision: 1, FromState: &det,
		ToState: EventStateUnconfirmed, Reason: "FLOOR_MET", DecidedAt: base,
		NodeCount: 1, IndependentCells: 1, EvidenceSummary: etEvidence("NODE-A"),
		AlgoVer: "phase3-1.1/ic=5"})
	etSeedLog(t, st, &EventStateLog{EventID: id, Revision: 3, FromState: &con,
		ToState: EventStateConfirmed, Reason: "QUORUM_MET", DecidedAt: base + 4000,
		NodeCount: 4, IndependentCells: 3, EvidenceSummary: etEvidence("NODE-A", "NODE-B", "NODE-C", "NODE-D"),
		AlgoVer: "phase3-1.1/ic=5"})
	// Event LAIN, revisi yang sama.
	etSeedLog(t, st, &EventStateLog{EventID: other, Revision: 1, FromState: &det,
		ToState: EventStateUnconfirmed, Reason: "FLOOR_MET", DecidedAt: base + 10,
		NodeCount: 1, IndependentCells: 1, EvidenceSummary: etEvidence("NODE-Z"),
		AlgoVer: "phase3-1.1/ic=5"})

	got, err := st.ListStateLogForEvent(ctx, id)
	if err != nil {
		t.Fatalf("ListStateLogForEvent: %v", err)
	}
	if len(got) != 4 {
		for _, l := range got {
			t.Logf("kembali: %s rev %d @ %d", l.EventID, l.Revision, l.DecidedAt)
		}
		t.Fatalf("baris = %d; mau 4 — SELURUH riwayat, tanpa jendela dan tanpa LIMIT", len(got))
	}
	for i, l := range got {
		if l.Revision != i+1 {
			t.Errorf("posisi %d: revision = %d; mau %d — urutan revision ASC, BUKAN decided_at",
				i, l.Revision, i+1)
		}
		if l.EventID != id {
			t.Errorf("posisi %d: event_id = %s; mau %s — kueri tidak boleh bocor antar-event",
				i, l.EventID, id)
		}
	}
	// Rev 3 lebih awal dari rev 2 di jam server: bila urutannya bertahan, ia
	// bertahan karena revision, bukan karena kebetulan.
	if got[1].DecidedAt <= got[2].DecidedAt {
		t.Errorf("decided_at rev2=%d rev3=%d; uji ini kehilangan maknanya bila rev3 "+
			"tidak lebih AWAL dari rev2", got[1].DecidedAt, got[2].DecidedAt)
	}
}

// Setiap kolom yang dibaca M6′ terbawa apa adanya, dan yang NULL tiba SEBAGAI
// NULL.
//
// evidence_summary adalah satu-satunya jalan pulang dari sebuah transisi ke
// node-node yang menyumbangnya (D12: tidak ada observation_id, tidak ada
// correlation_key, tidak ada FK ke sensor_observations). JSONB yang kembali
// terpotong atau ter-reorder akan menghapus satu-satunya relasi keanggotaan yang
// dimiliki skema.
//
// peak_pga NUMERIC(8,4) NULLABLE dan from_state NULLABLE: nol adalah PGA yang
// sah, dan "tidak ada state sebelumnya" bukan "state sebelumnya bernama kosong".
func TestEventTimelineStateLogCarriesEvidenceAndNullsAsNull(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const id = "aaaaaaaa-0000-4000-8000-0000000006a4"
	seedEvent(t, st, newPhase3Event(id, EventStateUnconfirmed, 2))

	base := int64(1_766_400_000_000)
	unc := EventStateUnconfirmed
	pga := 137.2500
	ev := etEvidence("NODE-A", "NODE-B", "NODE-C")

	// rev 1: from_state NULL dan peak_pga NULL — baris penciptaan.
	etSeedLog(t, st, &EventStateLog{EventID: id, Revision: 1, FromState: nil,
		ToState: EventStateUnconfirmed, Reason: "FIRST_OBSERVATION", DecidedAt: base,
		NodeCount: 1, IndependentCells: 1, PeakPGA: nil,
		EvidenceSummary: etEvidence("NODE-A"), AlgoVer: "phase3-1.1/ic=5"})
	// rev 2: keduanya terisi.
	etSeedLog(t, st, &EventStateLog{EventID: id, Revision: 2, FromState: &unc,
		ToState: EventStateConfirmed, Reason: "QUORUM_MET", DecidedAt: base + 3000,
		NodeCount: 3, IndependentCells: 2, PeakPGA: &pga,
		EvidenceSummary: ev, AlgoVer: "phase3-1.1/ic=5"})

	got, err := st.ListStateLogForEvent(ctx, id)
	if err != nil {
		t.Fatalf("ListStateLogForEvent: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("baris = %d; mau 2", len(got))
	}

	if got[0].FromState != nil {
		t.Errorf("rev1 from_state = %q; mau nil — baris penciptaan tidak punya state "+
			"sebelumnya, dan nil berbeda dari string kosong", *got[0].FromState)
	}
	if got[0].PeakPGA != nil {
		t.Errorf("rev1 peak_pga = %v; mau nil — 0 gal adalah bacaan yang sah dan tidak "+
			"boleh menggantikan NULL", *got[0].PeakPGA)
	}
	if got[1].FromState == nil || *got[1].FromState != EventStateUnconfirmed {
		t.Errorf("rev2 from_state = %v; mau UNCONFIRMED", got[1].FromState)
	}
	if got[1].PeakPGA == nil || !closeEnough(*got[1].PeakPGA, pga) {
		t.Errorf("rev2 peak_pga = %v; mau %f", got[1].PeakPGA, pga)
	}

	// JSONB: Postgres menormalkan spasi dan urutan kunci, jadi yang diperiksa
	// adalah ISI-nya, bukan byte-nya. Ketiga node harus ada, dan pembacaan JSON
	// harus berhasil — sebuah kolom yang kembali kosong akan membuat M6′
	// melaporkan "tanpa kontributor" untuk transisi yang punya tiga.
	for _, n := range []string{"NODE-A", "NODE-B", "NODE-C"} {
		if !strings.Contains(string(got[1].EvidenceSummary), n) {
			t.Errorf("evidence_summary kehilangan %s: %s", n, got[1].EvidenceSummary)
		}
	}
	if !strings.Contains(string(got[1].EvidenceSummary), `"independent_cells"`) ||
		!strings.Contains(string(got[1].EvidenceSummary), `"obs_seq"`) {
		t.Errorf("evidence_summary kehilangan kunci yang M6′ baca: %s", got[1].EvidenceSummary)
	}
	if len(got[0].EvidenceSummary) == 0 {
		t.Errorf("rev1 evidence_summary kosong; kolomnya NOT NULL")
	}
	for i, l := range got {
		if l.AlgoVer != "phase3-1.1/ic=5" {
			t.Errorf("posisi %d: algo_ver = %q; mau phase3-1.1/ic=5 — provenance per baris "+
				"(D-013) dan M6′ mencetaknya bersama setiap revisi", i, l.AlgoVer)
		}
		if l.Reason == "" || l.ToState == "" {
			t.Errorf("posisi %d: reason/to_state kosong: %+v", i, l)
		}
	}
}

// Riwayat yang KOSONG adalah hasil, bukan galat.
//
// Sebuah event yang satuan persistensinya dibuang-tertua (D17/D30) punya baris
// induk tanpa baris log. M6′ harus melaporkan itu sebagai "tidak ada yang
// tercatat", dan justru bukan sebagai "tidak ada yang terjadi".
func TestEventTimelineStateLogEmptyHistoryIsNotAnError(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const id = "aaaaaaaa-0000-4000-8000-0000000006a5"
	seedEvent(t, st, newPhase3Event(id, EventStateUnconfirmed, 1))

	got, err := st.ListStateLogForEvent(ctx, id)
	if err != nil {
		t.Fatalf("riwayat kosong tidak boleh galat: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("baris = %d; mau 0", len(got))
	}
}

// Interval TERTUTUP di kedua ujung, urutan kanonik (received_ts,
// observation_id), dan penyaring node_id = ANY yang benar-benar menyaring.
//
// Yang dicegah regresinya: sebuah `>` alih-alih `>=` membuang observasi PERTAMA
// sebuah jendela — dan pada M6′ observasi pertama itulah yang paling sering
// menjadi satu-satunya kandidat sebuah transisi UNCONFIRMED. Pemutus-seri
// observation_id adalah satu-satunya yang membuat urutan TOTAL, dan hanya terlihat
// pada dua baris dengan received_ts yang sama.
func TestEventTimelineObservationsWindowIsClosedAndOrderCanonical(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const (
		nodeA   = "NODE-6TL0001"
		nodeB   = "NODE-6TL0002"
		nodeOut = "NODE-6TL0003" // BUKAN anggota; barisnya harus tidak pernah kembali
	)
	cleanupObs(t, st, nodeA, nodeB, nodeOut)

	base := int64(1_766_500_000_000)
	// Ditulis SENGAJA di luar urutan waktu, supaya ORDER BY yang benar-benar
	// mengurutkan dapat dibedakan dari kueri yang hanya mewarisi urutan sisip.
	idMid := seedReplayObs(t, st, etObs(nodeB, base+1000, 120.5, "OK", true))
	idLo := seedReplayObs(t, st, etObs(nodeA, base, 120.5, "OK", true))
	idHi := seedReplayObs(t, st, etObs(nodeA, base+2000, 120.5, "OK", true))
	idTieA := seedReplayObs(t, st, etObs(nodeA, base+1500, 120.5, "OK", true))
	idTieB := seedReplayObs(t, st, etObs(nodeB, base+1500, 120.5, "OK", true))
	// Di LUAR jendela pada kedua ujung, keduanya milik node ANGGOTA.
	_ = seedReplayObs(t, st, etObs(nodeA, base-1, 120.5, "OK", true))
	_ = seedReplayObs(t, st, etObs(nodeB, base+2001, 120.5, "OK", true))
	// Di DALAM jendela tetapi bukan anggota: inilah separuh "keanggotaan" dari
	// relasi keanggotaan-dan-waktu, dan tanpa baris ini kueri tanpa penyaring
	// node pun lulus.
	_ = seedReplayObs(t, st, etObs(nodeOut, base+500, 120.5, "OK", true))

	got, err := st.ListObservationsForNodesInWindow(ctx, []string{nodeA, nodeB}, base, base+2000)
	if err != nil {
		t.Fatalf("ListObservationsForNodesInWindow: %v", err)
	}

	wantIDs := []int64{idLo, idMid, idTieA, idTieB, idHi}
	if len(got) != len(wantIDs) {
		for _, o := range got {
			t.Logf("kembali: id=%d node=%s received_ts=%d", o.ObservationID, o.NodeID, o.ReceivedTS)
		}
		t.Fatalf("baris = %d; mau %d — batas TERTUTUP di kedua ujung dan node_id = ANY "+
			"benar-benar menyaring", len(got), len(wantIDs))
	}
	for i, want := range wantIDs {
		if got[i].ObservationID != want {
			t.Errorf("posisi %d: observation_id = %d; mau %d (urutan kanonik received_ts "+
				"lalu observation_id)", i, got[i].ObservationID, want)
		}
	}
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
	for _, o := range got {
		if o.NodeID == nodeOut {
			t.Errorf("baris node non-anggota %s kembali (id=%d); keanggotaan berasal dari "+
				"evidence_summary.contributors[], bukan dari jendela waktu saja", o.NodeID, o.ObservationID)
		}
	}
}

// Kueri TIDAK MENYARING. verify_result != 'OK', PGA di bawah lantai, dan
// node_location NULL harus tetap kembali, dan NULL harus tiba SEBAGAI NULL.
//
// Alasannya sama dengan ListObservationsForReplay, dan pada M6′ ia lebih tajam:
// D-015 menuntut pelaporan cakupan yang eksplisit, dan sebuah baris yang tidak
// pernah kembali tidak dapat dihitung sebagai baris yang DIBUANG. Kueri yang
// menyaring sendiri mengubah "3 dari 7 baris memenuhi syarat" menjadi "3 baris",
// dan hanya yang pertama yang dapat dibaca sebagai bukti.
//
// (0,0) juga bukan detail kosmetik: ia koordinat yang sah di Teluk Guinea, dan
// sebuah observasi tanpa lokasi yang terbaca sebagai (0,0) akan tampak memenuhi
// syarat.
func TestEventTimelineObservationsAreUnfilteredAndNullsStayNull(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const node = "NODE-6TL0004"
	cleanupObs(t, st, node)

	base := int64(1_766_600_000_000)
	idOK := seedReplayObs(t, st, etObs(node, base, 120.5, "OK", true))
	idBadVerify := seedReplayObs(t, st, etObs(node, base+100, 120.5, "ErrBadSignature", true))
	idNoLoc := seedReplayObs(t, st, etObs(node, base+200, 120.5, "OK", false))
	// Di bawah MinPGAGal (16.6 gal): penyaringnya milik event.BuildTimeline, yang
	// menghitungnya sebagai BelowFloor. Kueri tidak boleh mendahuluinya.
	idLowPGA := seedReplayObs(t, st, etObs(node, base+300, 4.0, "OK", true))

	got, err := st.ListObservationsForNodesInWindow(ctx, []string{node}, base, base+300)
	if err != nil {
		t.Fatalf("ListObservationsForNodesInWindow: %v", err)
	}
	if len(got) != 4 {
		for _, o := range got {
			t.Logf("kembali: id=%d verify=%s pga=%f", o.ObservationID, o.VerifyResult, o.PGAGal)
		}
		t.Fatalf("baris = %d; mau 4 — kueri TIDAK boleh menyaring", len(got))
	}

	byID := make(map[int64]ReplayObservation, 4)
	for _, o := range got {
		byID[o.ObservationID] = o
	}

	if o := byID[idOK]; o.VerifyResult != "OK" || o.Lat == nil || o.Lon == nil {
		t.Errorf("baris OK: verify=%q lat=%v lon=%v; mau OK dengan koordinat",
			o.VerifyResult, o.Lat, o.Lon)
	}
	if o := byID[idBadVerify]; o.VerifyResult != "ErrBadSignature" {
		t.Errorf("baris gagal verifikasi: verify_result = %q; mau ErrBadSignature — "+
			"barisnya harus KEMBALI supaya dapat dihitung sebagai yang dibuang", o.VerifyResult)
	}
	if o := byID[idLowPGA]; !closeEnough(o.PGAGal, 4.0) {
		t.Errorf("baris di bawah lantai: pga = %f; mau 4.0 — lantai PGA adalah keputusan "+
			"event.BuildTimeline, bukan predikat SQL", o.PGAGal)
	}
	o := byID[idNoLoc]
	if o.Lat != nil || o.Lon != nil {
		t.Errorf("node_location NULL terbaca lat=%v lon=%v; mau nil/nil — (0,0) adalah "+
			"koordinat yang sah dan tidak boleh menggantikan NULL", o.Lat, o.Lon)
	}
	// Provenance 000007/v2 terbawa apa adanya: obs_seq adalah anotasi tautan M6′
	// (ObsSeqExact / ABSORBED_LE / LATER_GT), dan ia harus tiba sebagai pointer
	// supaya "tidak ada obs_seq" tetap dapat dibedakan dari obs_seq bernilai 0.
	if o.ObsSeq == nil || o.ProtoVer == nil || o.OnsetTS == nil || o.OnsetTSSource != "SENSOR" {
		t.Errorf("provenance hilang: seq=%v proto=%v onset=%v source=%q",
			o.ObsSeq, o.ProtoVer, o.OnsetTS, o.OnsetTSSource)
	}
}

// Himpunan node KOSONG mengembalikan nol baris tanpa galat, dan jendela TERBALIK
// adalah galat.
//
// Keduanya kontrak yang M6′ andalkan, dan keduanya berlawanan arah dengan
// sengaja: sebuah event yang seluruh evidence_summary-nya tak terbaca memang
// tidak punya kandidat — itu hasil untuk dilaporkan, bukan galat untuk dimatikan.
// Sedangkan fromTS > toTS berarti pemanggilnya salah menghitung jendela, dan
// nol baris di situ akan terbaca sebagai "tidak ada observasi", yaitu jawaban
// yang salah untuk pertanyaan yang tidak pernah diajukan.
//
// Yang dicegah regresinya: BETWEEN pada Postgres atas jendela terbalik
// mengembalikan nol baris DENGAN TENANG. Tanpa penjaga di Go, sebuah galat
// aritmetika jendela tidak dapat dibedakan dari ketiadaan bukti.
func TestEventTimelineObservationsEmptyNodesAndInvertedWindow(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const node = "NODE-6TL0005"
	cleanupObs(t, st, node)

	base := int64(1_766_700_000_000)
	_ = seedReplayObs(t, st, etObs(node, base, 120.5, "OK", true))

	got, err := st.ListObservationsForNodesInWindow(ctx, nil, base-5000, base+5000)
	if err != nil {
		t.Fatalf("himpunan node kosong tidak boleh galat: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("baris = %d; mau 0 — tanpa node anggota tidak ada kandidat", len(got))
	}
	got, err = st.ListObservationsForNodesInWindow(ctx, []string{}, base-5000, base+5000)
	if err != nil || len(got) != 0 {
		t.Errorf("slice kosong: baris = %d, err = %v; mau 0 tanpa galat", len(got), err)
	}

	if _, err := st.ListObservationsForNodesInWindow(ctx, []string{node}, base+1, base); err == nil {
		t.Error("jendela terbalik (fromTS > toTS) harus galat: nol baris di situ akan " +
			"terbaca sebagai ketiadaan bukti")
	}

	// Jendela sah dengan lebar NOL (fromTS == toTS) tetap mengembalikan baris yang
	// tepat berada di titik itu — konsekuensi langsung dari interval tertutup, dan
	// jendela selebar nol bukan jendela terbalik.
	got, err = st.ListObservationsForNodesInWindow(ctx, []string{node}, base, base)
	if err != nil {
		t.Fatalf("jendela selebar nol: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("baris = %d; mau 1 — [t, t] tertutup memuat t", len(got))
	}
}
