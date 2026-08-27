package event

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// ---- fake nodeSource -------------------------------------------------------

type fakeLoc struct {
	nodes map[string]store.NodeLocation
	calls int
}

func (f *fakeLoc) GetNodeLocation(_ context.Context, id string) (*store.NodeLocation, error) {
	f.calls++
	nl, ok := f.nodes[id]
	if !ok {
		return nil, store.ErrNodeNotFound
	}
	return &nl, nil
}

func (f *fakeLoc) put(id string, lat, lon float64) {
	if f.nodes == nil {
		f.nodes = make(map[string]store.NodeLocation)
	}
	f.nodes[id] = store.NodeLocation{StationID: id, Lat: lat, Lon: lon, LocationName: id + " site"}
}

// ---- fake emitter ----------------------------------------------------------

type recEmitter struct{ frames []Snapshot }

func (r *recEmitter) EmitTransition(_ context.Context, s Snapshot) { r.frames = append(r.frames, s) }

func (r *recEmitter) states() []State {
	out := make([]State, 0, len(r.frames))
	for _, f := range r.frames {
		out = append(out, f.To)
	}
	return out
}

// countFor menghitung frame untuk satu state tujuan.
func (r *recEmitter) countFor(to State) int {
	n := 0
	for _, f := range r.frames {
		if f.To == to {
			n++
		}
	}
	return n
}

// eventIDs mengembalikan himpunan event_id yang pernah muncul di frame.
func (r *recEmitter) eventIDs() map[string]int {
	out := make(map[string]int)
	for _, f := range r.frames {
		out[f.EventID]++
	}
	return out
}

// ---- fake persister --------------------------------------------------------

// recPersister mencatat setiap satuan persistensi, dan opsional MENSIMULASIKAN
// kegagalan antrean maupun kegagalan penulisan.
//
// Kegagalan dilaporkan lewat callback yang sama yang dipakai ledger.Writer
// sungguhan (EventPersistObserver), bukan lewat nilai kembalian: penulisannya
// asinkron, jadi tidak ada nilai kembalian yang dapat berarti apa pun bagi
// pemanggil (§9.5).
type recPersister struct {
	trk *Tracker

	units []*store.EventUnit

	// dropAll mensimulasikan antrean penuh: satuan tidak pernah masuk.
	dropAll bool
	// failUpsert mensimulasikan UpsertEvent yang selalu gagal; aturan §9.5
	// menyusul dengan sendirinya — baris log DILEWATKAN, tidak pernah dicoba.
	failUpsert bool
	// failStateLog mensimulasikan AppendStateLog yang gagal setelah upsert
	// berhasil.
	failStateLog bool

	// calls adalah urutan panggilan store yang benar-benar DICOBA. Uji urutan FK
	// membacanya: satu-satunya hal yang berdiri di antara jalur tulis asinkron dan
	// pelanggaran foreign key adalah bahwa "append" tidak pernah muncul setelah
	// "upsert gagal".
	calls []string
}

func (p *recPersister) RecordEventUnit(u *store.EventUnit) {
	if p.dropAll {
		p.trk.EventPersistDropped()
		return
	}
	p.units = append(p.units, u)

	p.calls = append(p.calls, "upsert")
	if p.failUpsert {
		p.trk.EventUpsertFailed()
		if u.Log != nil {
			p.trk.EventStateLogSkipped()
		}
		return
	}
	if u.Log == nil {
		return
	}
	p.calls = append(p.calls, "append")
	if p.failStateLog {
		p.trk.EventStateLogFailed()
	}
}

// withPersister memasang perekam persistensi pada harness.
func (h *harness) withPersister(mutate ...func(*recPersister)) *recPersister {
	p := &recPersister{trk: h.trk}
	for _, m := range mutate {
		m(p)
	}
	h.trk.SetLedger(p)
	return p
}

// ---- jam palsu -------------------------------------------------------------

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }
func newFakeClock() *fakeClock               { return &fakeClock{t: time.UnixMilli(1_700_000_000_000)} }

// ---- harness ---------------------------------------------------------------

// harness adalah Tracker + jam palsu + toko lokasi palsu + emitter perekam, tanpa
// satu pun I/O. Setiap uji perilaku Tracker memakainya.
type harness struct {
	t     *testing.T
	trk   *Tracker
	loc   *fakeLoc
	emit  *recEmitter
	clock *fakeClock
	ids   int

	allowEvictions bool
}

func defaultOptions() Options {
	return Options{
		CorrelationWindowMs: 20000,
		AttachRadiusKm:      50,
		IndependenceCellKm:  5,
		MinIndependentCells: 2,
		MaxEventDiameterKm:  120,
		ResolveAfterMs:      90000,
		SweepIntervalMs:     5000,
		MaxOpen:             256,
		TerminalRetentionMs: 900000,
		MaxTombstones:       512,
	}
}

func newHarness(t *testing.T, mutate ...func(*Options)) *harness {
	t.Helper()
	opt := defaultOptions()
	for _, m := range mutate {
		m(&opt)
	}

	h := &harness{
		t:     t,
		loc:   &fakeLoc{},
		emit:  &recEmitter{},
		clock: newFakeClock(),
	}
	h.trk = NewTracker(h.loc, opt, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.trk.now = h.clock.now
	// Id yang dapat diprediksi membuat pemutus-seri leksikografis §4.3 dapat
	// diuji, dan membuat pesan kegagalan dapat dibaca.
	h.trk.newID = func() string {
		h.ids++
		return string(rune('A'+h.ids-1)) + "0000000-0000-4000-8000-000000000000"
	}
	h.trk.SetEmitter(h.emit)

	// §18.2: event_tombstone_evictions_total harus 0 di setiap uji yang tidak
	// menguji langit-langit itu sendiri. Dipasang di helper bersama supaya uji baru
	// tidak dapat diam-diam bergantung pada evakuasi dini.
	t.Cleanup(func() {
		if h.allowEvictions {
			return
		}
		if got := h.trk.TombstoneEvictions(); got != 0 {
			t.Errorf("event_tombstone_evictions_total = %d, harus 0 kecuali uji langit-langit tombstone", got)
		}
	})
	return h
}

// allowEvictions dibuka HANYA oleh uji langit-langit tombstone.
func (h *harness) permitEvictions() { h.allowEvictions = true }

// node mendaftarkan node pada koordinat tertentu.
func (h *harness) node(id string, lat, lon float64) { h.loc.put(id, lat, lon) }

// nodeAt mendaftarkan node pada jarak km ke arah bearing dari titik acuan.
func (h *harness) nodeAt(id string, refLat, refLon, km, bearing float64) (lat, lon float64) {
	lat, lon = destinationKm(refLat, refLon, km, bearing)
	h.loc.put(id, lat, lon)
	return lat, lon
}

// v2 membangun observasi v2 (onset terukur sensor).
func v2(node string, pga float64, onset int64, phase string, obsSeq int64) Input {
	seq := obsSeq
	in := Input{
		NodeID: node, PGA: pga, DurMs: 3000,
		PublishTS: onset + 3000, OnsetTS: onset, OnsetSource: OnsetSourceSensor,
		Phase: phase, ObsSeq: &seq,
	}
	if phase == PhaseFinal {
		d := onset + 3000
		in.DetriggerTS = &d
	}
	return in
}

// v1 membangun observasi legacy: onset adalah BATAS publish_ts - dur_ms.
func v1(node string, pga float64, publishTS, durMs int64) Input {
	return Input{
		NodeID: node, PGA: pga, DurMs: durMs,
		PublishTS: publishTS, OnsetTS: publishTS - durMs, OnsetSource: OnsetSourcePublish,
		Phase: PhaseFinal,
	}
}

func (h *harness) ingest(in Input) {
	h.t.Helper()
	h.trk.Ingest(context.Background(), in)
}

// events mengembalikan snapshot map event terlacak untuk pemeriksaan uji.
func (h *harness) events() map[string]*Event {
	h.trk.mu.Lock()
	defer h.trk.mu.Unlock()
	out := make(map[string]*Event, len(h.trk.events))
	for k, v := range h.trk.events {
		out[k] = v
	}
	return out
}

// only mengembalikan satu-satunya event terlacak, gagal bila jumlahnya bukan satu.
func (h *harness) only() *Event {
	h.t.Helper()
	es := h.events()
	if len(es) != 1 {
		h.t.Fatalf("event terlacak = %d, mau tepat 1 (%s)", len(es), describe(es))
	}
	for _, e := range es {
		return e
	}
	return nil
}

func describe(es map[string]*Event) string {
	s := ""
	for id, e := range es {
		nodes := ""
		for n := range e.Contributors {
			nodes += n + " "
		}
		s += id[:1] + "={" + string(e.State) + " nodes:" + nodes + "} "
	}
	return s
}
