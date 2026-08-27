package event

import "testing"

// Sembilan kasus §6.5, verbatim dalam urutannya. Masing-masing menegaskan jumlah
// kontributor, urutan state, dan frame yang keluar.

const (
	baseLat = -6.9
	baseLon = 107.6
)

// onsetBase adalah onset acuan; semua onset uji diturunkan darinya supaya jendela
// korelasi terlihat langsung di angka.
const onsetBase = int64(1_700_000_000_000)

// threeNodeCluster memasang tiga node yang terpisah 0/8/16 km ke timur: dalam
// radius menempel, di sel independensi 5 km yang berbeda.
func (h *harness) threeNodeCluster() {
	h.node("N1", baseLat, baseLon)
	h.nodeAt("N2", baseLat, baseLon, 8, 90)
	h.nodeAt("N3", baseLat, baseLon, 16, 90)
}

// Kasus 1 — satu node hanya PRELIM.
func TestCase1SinglePrelimOnly(t *testing.T) {
	t.Run("di bawah lantai tetap DETECTED dan tak terlihat", func(t *testing.T) {
		h := newHarness(t)
		h.node("N1", baseLat, baseLon)
		h.ingest(v2("N1", MinPGAGal-0.1, onsetBase, PhasePrelim, 1))

		e := h.only()
		if e.State != StateDetected {
			t.Errorf("state = %s, mau DETECTED", e.State)
		}
		if len(e.Contributors) != 1 {
			t.Errorf("kontributor = %d, mau 1", len(e.Contributors))
		}
		if len(h.emit.frames) != 0 {
			t.Errorf("frame = %v, mau kosong: DETECTED tidak pernah publik", h.emit.states())
		}
	})

	t.Run("di atas lantai jadi UNCONFIRMED sekali", func(t *testing.T) {
		h := newHarness(t)
		h.node("N1", baseLat, baseLon)
		h.ingest(v2("N1", MinPGAGal+1, onsetBase, PhasePrelim, 1))

		e := h.only()
		if e.State != StateUnconfirmed {
			t.Errorf("state = %s, mau UNCONFIRMED", e.State)
		}
		if e.Revision != 1 {
			t.Errorf("revision = %d, mau 1", e.Revision)
		}
		if got := h.emit.states(); len(got) != 1 || got[0] != StateUnconfirmed {
			t.Errorf("frame = %v, mau tepat satu UNCONFIRMED", got)
		}
		// Satu node, satu sel: tidak akan pernah CONFIRMED.
		if e.independentCells() != 1 {
			t.Errorf("sel independen = %d, mau 1", e.independentCells())
		}
	})
}

// Kasus 2 — satu node hanya FINAL (v1, atau v2 yang PRELIM-nya hilang). Identik
// dengan kasus 1: fase dicatat, tidak pernah disyaratkan.
func TestCase2SingleFinalOnlyIsIdenticalToPrelimOnly(t *testing.T) {
	h := newHarness(t)
	h.node("N1", baseLat, baseLon)
	h.ingest(v1("N1", MinPGAGal+1, onsetBase+3000, 3000))

	e := h.only()
	if e.State != StateUnconfirmed {
		t.Errorf("state = %s, mau UNCONFIRMED", e.State)
	}
	if e.Contributors["N1"].Phase != PhaseFinal {
		t.Errorf("phase = %q, mau FINAL", e.Contributors["N1"].Phase)
	}
	if e.OriginTSSource != OnsetSourcePublish {
		t.Errorf("origin_ts_source = %q, mau PUBLISH_BOUND", e.OriginTSSource)
	}
	if got := h.emit.states(); len(got) != 1 || got[0] != StateUnconfirmed {
		t.Errorf("frame = %v, mau tepat satu UNCONFIRMED", got)
	}
}

// Kasus 3 — banyak node, semuanya PRELIM. CONFIRMED dapat dicapai dari PRELIM
// saja: itulah nilai peringatan dini, dan menunggu FINAL berarti membuang detik
// yang justru dibeli Fase 2.
func TestCase3MultipleNodesAllPrelimReachesConfirmed(t *testing.T) {
	h := newHarness(t)
	h.threeNodeCluster()

	h.ingest(v2("N1", MinPGAGal+10, onsetBase, PhasePrelim, 1))
	h.ingest(v2("N2", MinPGAGal+20, onsetBase+2000, PhasePrelim, 1))
	h.ingest(v2("N3", MinPGAGal+5, onsetBase+4000, PhasePrelim, 1))

	e := h.only()
	if e.State != StateConfirmed {
		t.Fatalf("state = %s, mau CONFIRMED", e.State)
	}
	if len(e.Contributors) != 3 {
		t.Errorf("kontributor = %d, mau 3", len(e.Contributors))
	}
	for _, c := range e.Contributors {
		if c.Phase != PhasePrelim {
			t.Errorf("%s: phase = %q, mau PRELIM", c.NodeID, c.Phase)
		}
	}
	if got := h.emit.states(); len(got) != 2 || got[0] != StateUnconfirmed || got[1] != StateConfirmed {
		t.Errorf("urutan frame = %v, mau UNCONFIRMED lalu CONFIRMED", got)
	}
	if !e.EverConfirmed {
		t.Error("EverConfirmed harus true setelah CONFIRMED: all-clear berutang pada audiens alarmnya")
	}
}

// Kasus 4 — FINAL node A datang sebelum PRELIM node B. Tidak ada kasus khusus di
// kode, dan itulah intinya.
func TestCase4FinalBeforeOtherNodesPrelimIsOrderIndependent(t *testing.T) {
	run := func(t *testing.T, reverse bool) map[string]*Event {
		h := newHarness(t)
		h.threeNodeCluster()
		obs := []Input{
			v2("N1", MinPGAGal+30, onsetBase, PhaseFinal, 1),
			v2("N2", MinPGAGal+10, onsetBase+1500, PhasePrelim, 1),
			v2("N3", MinPGAGal+8, onsetBase+3000, PhasePrelim, 1),
		}
		if reverse {
			obs[0], obs[2] = obs[2], obs[0]
		}
		for _, o := range obs {
			h.ingest(o)
		}
		if e := h.only(); e.State != StateConfirmed {
			t.Fatalf("state = %s, mau CONFIRMED (reverse=%v)", e.State, reverse)
		}
		return h.events()
	}

	a := run(t, false)
	b := run(t, true)
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("kedua urutan harus menghasilkan satu event: %d dan %d", len(a), len(b))
	}
}

// Kasus 5 — kedatangan tak berurut secara umum: pencocokan berkunci pada onset_ts,
// tidak pernah pada urutan kedatangan.
func TestCase5OutOfOrderArrivalMatchesOnOnset(t *testing.T) {
	h := newHarness(t)
	h.threeNodeCluster()

	// Onset naik N3 < N2 < N1, kedatangan justru sebaliknya.
	h.ingest(v2("N1", MinPGAGal+5, onsetBase+8000, PhasePrelim, 1))
	h.ingest(v2("N2", MinPGAGal+5, onsetBase+4000, PhasePrelim, 1))
	h.ingest(v2("N3", MinPGAGal+5, onsetBase, PhasePrelim, 1))

	e := h.only()
	if len(e.Contributors) != 3 {
		t.Fatalf("kontributor = %d, mau 3", len(e.Contributors))
	}
	// origin_ts adalah onset yang MEMBUAT event, bukan yang paling awal (§4.3).
	if e.OriginTS != onsetBase+8000 {
		t.Errorf("origin_ts = %d, mau onset pembuat %d — ia tidak pernah dipindahkan",
			e.OriginTS, onsetBase+8000)
	}
	if e.State != StateConfirmed {
		t.Errorf("state = %s, mau CONFIRMED", e.State)
	}
}

// Kasus 6 — observasi duplikat. Jumlah node tidak dapat bergerak.
func TestCase6DuplicateObservationCannotBecomeSecondVote(t *testing.T) {
	h := newHarness(t)
	h.node("N1", baseLat, baseLon)

	o := v2("N1", MinPGAGal+1, onsetBase, PhasePrelim, 1)
	h.ingest(o)
	h.ingest(o)
	h.ingest(o)

	e := h.only()
	if len(e.Contributors) != 1 {
		t.Fatalf("kontributor = %d, mau 1", len(e.Contributors))
	}
	if e.Contributors["N1"].Revisions != 3 {
		t.Errorf("revisions = %d, mau 3: duplikat diserap, bukan dibuang",
			e.Contributors["N1"].Revisions)
	}
	if e.Revision != 1 {
		t.Errorf("revision event = %d, mau 1: revisi hanya naik pada TRANSISI", e.Revision)
	}
	if len(h.emit.frames) != 1 {
		t.Errorf("frame = %v, mau tepat satu", h.emit.states())
	}
}

// Kasus 7 — node hilang setelah PRELIM dan tidak pernah mengirim FINAL. Tidak ada
// yang menunggunya.
func TestCase7NodeDisappearsAfterPrelimNothingWaits(t *testing.T) {
	h := newHarness(t)
	h.threeNodeCluster()

	h.ingest(v2("N1", MinPGAGal+10, onsetBase, PhasePrelim, 1))
	h.ingest(v2("N2", MinPGAGal+10, onsetBase+1000, PhasePrelim, 1))
	h.ingest(v2("N3", MinPGAGal+10, onsetBase+2000, PhasePrelim, 1))
	e := h.only()
	if e.State != StateConfirmed {
		t.Fatalf("state = %s, mau CONFIRMED tanpa satu pun FINAL", e.State)
	}

	// Jam berjalan; tanpa bukti baru, last_evidence_ts tidak maju dan tidak ada
	// transisi yang terjadi hanya karena waktu lewat di dalam Ingest.
	before := e.LastEvidenceTS
	h.clock.advance(60_000_000_000) // 60 s
	if e.LastEvidenceTS != before {
		t.Errorf("last_evidence_ts bergerak tanpa bukti: %d -> %d", before, e.LastEvidenceTS)
	}
	if e.State != StateConfirmed {
		t.Errorf("state = %s, mau tetap CONFIRMED", e.State)
	}
}

// Kasus 8 — FINAL yang tidak pernah datang untuk SETIAP kontributor: sama seperti
// kasus 7 pada skala event. Event tidak dibatalkan — bukti PRELIM yang memenuhi
// lantai adalah guncangan nyata sejauh yang sistem tahu.
func TestCase8NoFinalFromAnyoneIsNotCancelled(t *testing.T) {
	h := newHarness(t)
	h.threeNodeCluster()
	for i, n := range []string{"N1", "N2", "N3"} {
		h.ingest(v2(n, MinPGAGal+10, onsetBase+int64(i)*1000, PhasePrelim, 1))
	}

	e := h.only()
	if e.State == StateCancelled {
		t.Fatal("CANCELLED hanya bila bukti DITARIK, bukan karena FINAL tak datang")
	}
	if e.Invalidated {
		t.Error("Invalidated harus false")
	}
}

// Kasus 9 — PRELIM satu node yang sekejap dan memenuhi lantai: dipublikasikan
// sebagai UNCONFIRMED, dan hanya menjadi CANCELLED bila kontributornya
// diinvalidasi (§7.5), tidak pernah semata karena ia tetap sendiri.
func TestCase9TransientSinglePrelimIsUnconfirmedNotCancelled(t *testing.T) {
	h := newHarness(t)
	h.node("N1", baseLat, baseLon)
	h.ingest(v2("N1", MinPGAGal+50, onsetBase, PhasePrelim, 1))

	e := h.only()
	if e.State != StateUnconfirmed {
		t.Fatalf("state = %s, mau UNCONFIRMED", e.State)
	}
	h.clock.advance(300_000_000_000) // 300 s
	if e.State != StateCancelled {
		return // benar: waktu saja tidak membatalkan apa pun
	}
	t.Fatal("berlalunya waktu tidak boleh membuat event menjadi CANCELLED")
}
