package api

// Uji ujung-ke-ujung §7.5: unverify operator -> pencabutan bukti pada Tracker
// sungguhan. Uji handler-level di admin_nodes_test.go membuktikan bahwa
// HandleVerifyNode memanggil InvalidateContributor pada perekam; yang ini
// membuktikan KABEL SEBENARNYA — sebuah *event.Tracker* nyata dipasang lewat
// SetEvidenceInvalidator, persis seperti wiring main.go, dan tiga panggilan HTTP
// unverify yang mencabut seluruh kontributor sebuah event CONFIRMED berakhir pada
// transisi CANCELLED dengan event_id yang sama dengan yang sudah diumumkan.
//
// Skenario mengikuti semantik §5.2 secara sengaja:
//   - tiga node di DUA sel independen (dua node berdekatan dalam satu sel,
//     satu node terpisah) -> CONFIRMED: kuorum 3 DAN independensi >= 2,
//   - dua unverify pertama TIDAK menghasilkan frame apa pun:
//     CONFIRMED -> UNCONFIRMED adalah transisi ilegal, jadi event tetap
//     CONFIRMED meski dua penyumbangnya hilang — monotonisitas, bukan kelemahan;
//     uji ini menguncinya pada tingkat integrasi, bukan hanya unit,
//   - unverify ketiga mengosongkan penyumbang -> EVIDENCE_INVALIDATED ->
//     CANCELLED, satu frame penarikan; TOTAL TIGA frame untuk event ini
//     (advisory, alert, penarikan) karena emisi hanya terjadi pada transisi (§6.7),
//     dan seluruhnya membawa SATU event_id — tidak ada alert kedua untuk gempa
//     yang sama.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/banana-pixel/quakealert/server/internal/event"
	"github.com/banana-pixel/quakealert/server/internal/store"
)

// locStub adalah event.nodeSource dari luar paket event: koordinat yang diketahui
// uji, tanpa Postgres. Interface pemiliknya tidak terekspor; kesesuaian bentuknya
// dibuktikan oleh baris event.NewTracker di bawah gagal build bila melenceng.
type locStub struct {
	nodes map[string]store.NodeLocation
}

func (f *locStub) GetNodeLocation(_ context.Context, id string) (*store.NodeLocation, error) {
	if nl, ok := f.nodes[id]; ok {
		return &nl, nil
	}
	return nil, store.ErrNodeNotFound
}

// recFrames merekam transisi yang diumumkan Tracker, persis peran recEmitter di
// paket event. Dari luar paket itu, antarmuka emisi diteruskan sebagai nilai yang
// memenuhi SetEmitter — nama tipenya tidak terekspor, metodenya iya.
type recFrames struct{ snaps []event.Snapshot }

func (r *recFrames) EmitTransition(_ context.Context, s event.Snapshot) { r.snaps = append(r.snaps, s) }

// obs v2 disederhanakan untuk uji ini: PRELIM, onset terukur, satu episode.
func phase3Obs(node string, pga float64, onset int64) event.Input {
	seq := int64(1)
	return event.Input{
		NodeID: node, PGA: pga, DurMs: 3000,
		PublishTS: onset + 3000, OnsetTS: onset, OnsetSource: event.OnsetSourceSensor,
		Phase: event.PhasePrelim, ObsSeq: &seq,
	}
}

func TestVerifyNodeUnverifyCancelsConfirmedEventEndToEnd(t *testing.T) {
	// --- Armada dua sel independensi ---
	//
	// Sel independensi default 5 km; N1 dan N2 berjarak ~0.5 km sehingga jatuh di
	// sel yang sama (grid-nya deterministik: bila mereka terbaca beda sel, kuorum
	// tetap tercapai tetapi premis "dua node satu sel" salah dan uji ini GAGAL
	// pd segmen persiapan, bukan diam-diam menguji hal lain). N3 sepuluh kilometer
	// ke utara: sel berbeda, jauh di dalam AttachRadiusKm, jauh di dalam diameter.
	nodes := map[string]store.NodeLocation{
		"NODE-00000001": {StationID: "NODE-00000001", Lat: -6.900, Lon: 107.600, LocationName: "cell A"},
		"NODE-00000002": {StationID: "NODE-00000002", Lat: -6.9045, Lon: 107.600, LocationName: "cell A"},
		"NODE-00000003": {StationID: "NODE-00000003", Lat: -6.810, Lon: 107.600, LocationName: "cell B"},
	}
	for _, n := range nodes {
		if n.Lat < -12 || n.Lat > 12 {
			t.Fatalf("lat %f keluar pita MaxFleetLatitudeDeg", n.Lat)
		}
	}

	loc := &locStub{nodes: nodes}
	frames := &recFrames{}
	trk := event.NewTracker(loc, event.Options{
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
	}, testLogger())
	trk.SetEmitter(frames)

	// --- Persiapan: satu gempa, tiga node, CONFIRMED. ---
	const onset = int64(1_700_000_000_123)
	const pga = float64(27.5) // di atas MinPGAGal 16.6
	ctx := context.Background()
	for _, id := range []string{"NODE-00000001", "NODE-00000002", "NODE-00000003"} {
		trk.Ingest(ctx, phase3Obs(id, pga, onset))
	}

	if got := trk.Created(); got != 1 {
		t.Fatalf("persiapan: event_created_total = %d, mau 1", got)
	}
	if got := trk.Transitions(event.StateUnconfirmed); got != 1 {
		t.Fatalf("persiapan: transisi UNCONFIRMED = %d, mau 1", got)
	}
	if got := trk.Transitions(event.StateConfirmed); got != 1 {
		t.Fatalf("persiapan: transisi CONFIRMED = %d, mau 1 — armada dua sel harus konfirmasi", got)
	}
	if len(frames.snaps) != 2 {
		t.Fatalf("persiapan: frame = %d, mau 2 (advisory + alert)", len(frames.snaps))
	}

	// --- Jalur operator: tiga unverify lewat HTTP, Tracker sebagai pencabut. ---
	h := newAdminServerWithInvalidator(&fakeRepo{}, trk)
	unverify := func(id string) *httptest.ResponseRecorder {
		req := adminNodeRequest(http.MethodPost,
			"/api/v1/admin/nodes/"+id+"/verify", `{"verified":false}`, adminTestKey)
		return do(h, req)
	}

	// Unverify #1 dan #2: event TETAP CONFIRMED, tanpa frame. Menurunkannya
	// adalah transisi ilegal (§5.2); jatuhnya bukan pembatalan karena masih ada
	// bukti tersisa (§7.5).
	for _, id := range []string{"NODE-00000001", "NODE-00000002"} {
		rec := unverify(id)
		if rec.Code != http.StatusOK {
			t.Fatalf("unverify %s: status = %d, mau 200: %s", id, rec.Code, rec.Body.String())
		}
		if got := trk.Transitions(event.StateCancelled); got != 0 {
			t.Fatalf("unverify %s: transisi CANCELLED = %d, mau 0 — monotonisitas CONFIRMED dilanggar", id, got)
		}
		if got := len(frames.snaps); got != 2 {
			t.Fatalf("unverify %s: frame = %d, masih 2 — tidak ada emisi di luar transisi", id, got)
		}
	}

	// Unverify #3: penyumbang habis -> seluruh bukti ditarik -> CANCELLED.
	rec := unverify("NODE-00000003")
	if rec.Code != http.StatusOK {
		t.Fatalf("unverify akhir: status = %d, mau 200: %s", rec.Code, rec.Body.String())
	}
	if got := trk.Transitions(event.StateCancelled); got != 1 {
		t.Fatalf("transisi CANCELLED = %d, mau 1 setelah seluruh bukti dicabut", got)
	}

	if len(frames.snaps) != 3 {
		t.Fatalf("frame = %d, mau 3 (advisory, alert, penarikan)", len(frames.snaps))
	}
	last := frames.snaps[2]
	if last.To != event.StateCancelled {
		t.Errorf("frame terakhir To = %s, mau CANCELLED", last.To)
	}
	if last.Reason != event.ReasonEvidenceInvalid {
		t.Errorf("reason = %q, mau %q", last.Reason, event.ReasonEvidenceInvalid)
	}
	if last.From != event.StateConfirmed {
		t.Errorf("from_state = %q, mau CONFIRMED", last.From)
	}
	// Satu gempa, satu identitas, tiga frame — termasuk penarikannya.
	firstID := frames.snaps[0].EventID
	if firstID == "" {
		t.Fatal("frame advisory tanpa event_id — P2 kambuh")
	}
	for i, s := range frames.snaps {
		if s.EventID != firstID {
			t.Errorf("frame #%d event_id = %q, mau %q: satu identitas sejak observasi pertama", i, s.EventID, firstID)
		}
	}
}
