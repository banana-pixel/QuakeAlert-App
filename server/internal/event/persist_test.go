package event

import (
	"context"
	"testing"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// §18.2 R-H3 — persistensi TIDAK PERNAH menggerbangi pengiriman.
//
// Dua mode kegagalan diuji terpisah karena keduanya gagal di tempat berbeda:
// toko yang selalu menolak menulis, dan antrean yang tidak pernah menerima
// satuannya. Keduanya harus tidak terlihat sama sekali dari sisi klien.
func TestPersistenceNeverGatesDelivery(t *testing.T) {
	t.Run("toko yang selalu gagal", func(t *testing.T) {
		h := newHarness(t)
		p := h.withPersister(func(p *recPersister) { p.failUpsert = true })

		e := h.confirmThreeNodes()

		if e.State != StateConfirmed {
			t.Fatalf("state = %s, mau CONFIRMED: kegagalan tulis bukan rollback", e.State)
		}
		if got := h.emit.countFor(StateUnconfirmed); got != 1 {
			t.Errorf("frame UNCONFIRMED = %d, mau 1", got)
		}
		if got := h.emit.countFor(StateConfirmed); got != 1 {
			t.Errorf("frame CONFIRMED = %d, mau 1", got)
		}
		// Frame CONFIRMED adalah satu-satunya yang berhak atas FCM (§8.1); haknya
		// tidak boleh bergantung pada apakah barisnya pernah tersimpan.
		var confirmed Snapshot
		for _, f := range h.emit.frames {
			if f.To == StateConfirmed {
				confirmed = f
			}
		}
		if _, push, ok := FrameFor(confirmed); !ok || !push {
			t.Errorf("FrameFor(CONFIRMED) push=%v ok=%v, mau true/true", push, ok)
		}

		// Setiap transisi tetap DICOBA, dan setiap kegagalan terhitung.
		if len(p.units) != 2 {
			t.Errorf("satuan persistensi = %d, mau 2 (UNCONFIRMED lalu CONFIRMED)", len(p.units))
		}
		if got := h.trk.UpsertFailures(); got != 2 {
			t.Errorf("event_upsert_failures_total = %d, mau 2", got)
		}
		if got := h.trk.StateLogSkipped(); got != 2 {
			t.Errorf("event_state_log_skipped_total = %d, mau 2", got)
		}
		if got := h.trk.StateLogFailures(); got != 0 {
			t.Errorf("event_state_log_failures_total = %d, mau 0: baris log DILEWATKAN, bukan gagal", got)
		}
		if got := h.trk.PersistDropped(); got != 0 {
			t.Errorf("event_persist_dropped_total = %d, mau 0", got)
		}

		// Dan siklus hidupnya bertahan: RESOLVED keluar dengan event_id yang SAMA,
		// membuktikan identitas hidup di memori dan bukan di basis data (§9.5).
		h.clock.advance(msDur(defaultOptions().ResolveAfterMs + 1))
		h.trk.sweep(context.Background())
		if e.State != StateResolved {
			t.Fatalf("state = %s, mau RESOLVED", e.State)
		}
		if ids := h.emit.eventIDs(); len(ids) != 1 || ids[e.ID] != 3 {
			t.Errorf("event_id pada frame = %v, mau tepat satu id dengan 3 frame", ids)
		}
	})

	t.Run("antrean penuh", func(t *testing.T) {
		h := newHarness(t)
		p := h.withPersister(func(p *recPersister) { p.dropAll = true })

		e := h.confirmThreeNodes()

		if e.State != StateConfirmed {
			t.Fatalf("state = %s, mau CONFIRMED", e.State)
		}
		if len(p.units) != 0 {
			t.Fatalf("satuan yang masuk = %d, mau 0: antrean penuh", len(p.units))
		}
		if got := h.emit.countFor(StateConfirmed); got != 1 {
			t.Errorf("frame CONFIRMED = %d, mau 1", got)
		}

		h.clock.advance(msDur(defaultOptions().ResolveAfterMs + 1))
		h.trk.sweep(context.Background())
		if e.State != StateResolved {
			t.Fatalf("state = %s, mau RESOLVED", e.State)
		}
		if got := h.trk.PersistDropped(); got != 3 {
			t.Errorf("event_persist_dropped_total = %d, mau 3", got)
		}
		if got := h.trk.UpsertFailures(); got != 0 {
			t.Errorf("event_upsert_failures_total = %d, mau 0: tidak ada yang pernah dicoba", got)
		}
		if ids := h.emit.eventIDs(); len(ids) != 1 || ids[e.ID] != 3 {
			t.Errorf("event_id pada frame = %v, mau tepat satu id dengan 3 frame", ids)
		}
	})
}

// orderRec mencatat berapa frame yang SUDAH keluar saat satuan persistensinya
// tiba. Urutan §9.5 dinyatakan sebagai angka, bukan sebagai komentar.
type orderRec struct {
	emit       *recEmitter
	framesSeen []int
}

func (o *orderRec) RecordEventUnit(_ *store.EventUnit) {
	o.framesSeen = append(o.framesSeen, len(o.emit.frames))
}

// Emisi mendahului persistensi, per transisi. Satuan ke-n harus tiba setelah
// frame ke-n keluar — bukan sebelum, dan bukan setelah seluruh batch.
func TestEmissionPrecedesPersistence(t *testing.T) {
	h := newHarness(t)
	o := &orderRec{emit: h.emit}
	h.trk.SetLedger(o)

	h.confirmThreeNodes()

	if len(o.framesSeen) != 2 {
		t.Fatalf("satuan = %d, mau 2", len(o.framesSeen))
	}
	for i, seen := range o.framesSeen {
		if seen != i+1 {
			t.Errorf("satuan #%d tiba setelah %d frame, mau %d", i+1, seen, i+1)
		}
	}
}

// §18.2 R-H3 — DETECTED tidak pernah dipersistensi.
func TestDetectedIsNeverPersisted(t *testing.T) {
	h := newHarness(t)
	p := h.withPersister()
	h.node("N1", baseLat, baseLon)

	h.ingest(v2("N1", MinPGAGal-0.1, onsetBase, PhasePrelim, 1))

	e := h.only()
	if e.State != StateDetected {
		t.Fatalf("state = %s, mau DETECTED", e.State)
	}
	if len(p.calls) != 0 {
		t.Errorf("panggilan store = %v, mau tidak ada: DETECTED tidak punya baris", p.calls)
	}
	if len(h.emit.frames) != 0 {
		t.Errorf("frame = %v, mau tidak ada", h.emit.states())
	}
	if got := h.trk.Created(); got != 1 {
		t.Errorf("event_created_total = %d, mau 1: yang di bawah lantai tetap TERHITUNG", got)
	}

	// Naik melewati lantai: TEPAT satu satuan, dan riwayatnya tetap lengkap —
	// baris lognya membawa from_state = DETECTED walau state itu tak pernah
	// menjadi baris.
	h.ingest(v2("N1", MinPGAGal+5, onsetBase, PhaseFinal, 2))

	if e.State != StateUnconfirmed {
		t.Fatalf("state = %s, mau UNCONFIRMED", e.State)
	}
	if len(p.units) != 1 {
		t.Fatalf("satuan = %d, mau tepat 1", len(p.units))
	}
	u := p.units[0]
	if u.Log == nil {
		t.Fatalf("satuan tanpa baris log; riwayat transisi tidak boleh hilang")
	}
	if u.Log.FromState == nil || *u.Log.FromState != string(StateDetected) {
		t.Errorf("from_state = %v, mau DETECTED", u.Log.FromState)
	}
	if u.Log.ToState != string(StateUnconfirmed) {
		t.Errorf("to_state = %s, mau UNCONFIRMED", u.Log.ToState)
	}
	if u.Event.EventState != string(StateUnconfirmed) {
		t.Errorf("event_state = %s, mau UNCONFIRMED", u.Event.EventState)
	}
	if u.Event.Status != "HAPPENING" {
		t.Errorf("status = %s, mau HAPPENING", u.Event.Status)
	}
	if u.Event.AlgoVer != u.Log.AlgoVer || u.Event.AlgoVer != "phase3-1.0/ic=5" {
		t.Errorf("algo_ver = %q/%q, mau phase3-1.0/ic=5 pada keduanya", u.Event.AlgoVer, u.Log.AlgoVer)
	}
	if u.Event.StartedAtMs != e.CreatedAt {
		t.Errorf("started_at = %d, mau CreatedAt %d", u.Event.StartedAtMs, e.CreatedAt)
	}
}

// Baris log yang GAGAL setelah upsert BERHASIL adalah kegagalan, bukan
// pelewatan: keduanya dihitung terpisah supaya operator dapat membedakan basis
// data yang menolak segalanya dari basis data yang kehilangan riwayat saja.
func TestStateLogFailureCountsSeparately(t *testing.T) {
	h := newHarness(t)
	p := h.withPersister(func(p *recPersister) { p.failStateLog = true })

	h.confirmThreeNodes()

	if got := h.trk.StateLogFailures(); got != 2 {
		t.Errorf("event_state_log_failures_total = %d, mau 2", got)
	}
	if got := h.trk.StateLogSkipped(); got != 0 {
		t.Errorf("event_state_log_skipped_total = %d, mau 0", got)
	}
	if want := []string{"upsert", "append", "upsert", "append"}; len(p.calls) != len(want) {
		t.Fatalf("urutan panggilan = %v, mau %v", p.calls, want)
	}
}
