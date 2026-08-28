package event

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"testing"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// ---- fake eventStore -------------------------------------------------------

// fakeEvents adalah fakeLoc yang JUGA dapat menjawab dua pertanyaan startup.
// Dipisahkan dari fakeLoc supaya assertion tipe di Reconcile benar-benar diuji:
// harness biasa memasang sumber lokasi saja, dan jalur "toko tidak mendukung
// event" karenanya bukan jalur teoretis.
type fakeEvents struct {
	*fakeLoc

	open    []*store.EarthquakeEvent
	openErr error

	nodes    []store.NodeLocation
	nodesErr error

	loads int
}

func (f *fakeEvents) LoadOpenEvents(_ context.Context) ([]*store.EarthquakeEvent, error) {
	f.loads++
	if f.openErr != nil {
		return nil, f.openErr
	}
	return f.open, nil
}

func (f *fakeEvents) ListActiveNodeLocations(_ context.Context) ([]store.NodeLocation, error) {
	if f.nodesErr != nil {
		return nil, f.nodesErr
	}
	return f.nodes, nil
}

// restart menukar sumber lokasi harness dengan toko yang dapat memuat event, lalu
// mengembalikannya. Namanya harfiah: yang disimulasikan adalah proses yang mati
// dengan basis data yang selamat.
func (h *harness) restart(rows ...*store.EarthquakeEvent) *fakeEvents {
	h.t.Helper()
	fe := &fakeEvents{fakeLoc: h.loc, open: rows}
	h.trk.loc = fe
	return fe
}

// rowFrom membangun baris earthquake_events + evidence_summary DARI satuan
// persistensi yang benar-benar ditulis Tracker. Bukan baris yang dikarang: yang
// diuji §22.1(11) adalah perjalanan bolak-balik, dan baris buatan tangan akan
// menguji rekonstruksi terhadap tebakan uji tentang penulisan, bukan terhadap
// penulisannya.
func rowFrom(t *testing.T, u *store.EventUnit, decidedAt int64) *store.EarthquakeEvent {
	t.Helper()
	if u == nil || u.Event == nil {
		t.Fatal("satuan persistensi kosong: tidak ada yang dapat dimuat ulang")
	}
	row := *u.Event
	if u.Log != nil {
		row.LatestEvidence = u.Log.EvidenceSummary
		row.LatestDecidedAt = decidedAt
	}
	return &row
}

func nodeIDsOf(e *Event) []string {
	out := make([]string, 0, len(e.Contributors))
	for id := range e.Contributors {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// §22.1(11) — event -> restart -> event_id yang SAMA, kontributor yang sama.
func TestReconcileRoundTripKeepsIdentityAndContributors(t *testing.T) {
	h := newHarness(t)
	p := h.withPersister()
	before := h.confirmThreeNodes()
	wantID, wantNodes := before.ID, nodeIDsOf(before)
	last := p.units[len(p.units)-1]

	// Proses baru: Tracker baru, jam yang sama, basis data yang selamat.
	h2 := newHarness(t)
	h2.loc = h.loc
	h2.trk.loc = h.loc
	fe := h2.restart(rowFrom(t, last, h2.clock.now().UnixMilli()))

	if err := h2.trk.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if fe.loads != 1 {
		t.Errorf("LoadOpenEvents dipanggil %d kali, mau 1", fe.loads)
	}

	e := h2.only()
	if e.ID != wantID {
		t.Errorf("event_id = %q, mau %q: identitas TIDAK boleh lahir kembali", e.ID, wantID)
	}
	if got := nodeIDsOf(e); strings.Join(got, ",") != strings.Join(wantNodes, ",") {
		t.Errorf("kontributor = %v, mau %v", got, wantNodes)
	}
	if e.State != StateConfirmed {
		t.Errorf("state = %s, mau CONFIRMED", e.State)
	}
	if !e.EverConfirmed {
		t.Error("EverConfirmed harus true: baris CONFIRMED berarti alarmnya sudah dikirim")
	}
	if e.Revision != before.Revision {
		t.Errorf("revision = %d, mau %d", e.Revision, before.Revision)
	}
	if e.independentCells() != before.independentCells() {
		t.Errorf("sel independen = %d, mau %d", e.independentCells(), before.independentCells())
	}
	if got := h2.trk.Reconciled(); got != 1 {
		t.Errorf("event_reconciled_total = %d, mau 1", got)
	}
	// Rekonsiliasi bukan transisi: tidak ada frame yang boleh keluar untuk event
	// yang hanya dipulihkan.
	if len(h2.emit.frames) != 0 {
		t.Errorf("frame = %v, mau tidak ada", h2.emit.states())
	}
}

// §15.3 langkah 4 — observasi setelah restart MENEMPEL pada event_id sebelum
// restart alih-alih membentuk event kedua. Ini seluruh alasan Reconcile ada.
func TestReconciledEventAdoptsLateObservation(t *testing.T) {
	h := newHarness(t)
	p := h.withPersister()
	before := h.confirmThreeNodes()
	last := p.units[len(p.units)-1]

	h2 := newHarness(t)
	h2.loc = h.loc
	h2.trk.loc = h.loc
	h2.restart(rowFrom(t, last, h2.clock.now().UnixMilli()))
	if err := h2.trk.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	h2.node("N4", baseLat, baseLon)
	h2.nodeAt("N4", baseLat, baseLon, 20, 90)
	h2.ingest(v2("N4", MinPGAGal+30, onsetBase+2000, PhaseFinal, 1))

	e := h2.only()
	if e.ID != before.ID {
		t.Errorf("event_id = %q, mau %q: bukti pasca-restart membelah event", e.ID, before.ID)
	}
	if _, ok := e.Contributors["N4"]; !ok {
		t.Error("N4 harus menempel pada event yang dipulihkan")
	}
	if got := h2.trk.Created(); got != 0 {
		t.Errorf("event_created_total = %d, mau 0: tidak ada event baru yang lahir", got)
	}
}

// §15.3 langkah 3 — bukti yang kedaluwarsa di seberang restart DISELESAIKAN
// segera, dan frame all-clear-nya benar-benar keluar.
func TestReconcileResolvesStaleEventAndEmits(t *testing.T) {
	h := newHarness(t)
	p := h.withPersister()
	h.confirmThreeNodes()
	last := p.units[len(p.units)-1]

	h2 := newHarness(t)
	h2.loc = h.loc
	h2.trk.loc = h.loc
	decided := h2.clock.now().UnixMilli()
	h2.restart(rowFrom(t, last, decided))
	h2.clock.advance(msDur(defaultOptions().ResolveAfterMs + 1))

	p2 := h2.withPersister()
	if err := h2.trk.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	e := h2.only()
	if e.State != StateResolved {
		t.Fatalf("state = %s, mau RESOLVED", e.State)
	}
	if len(h2.emit.frames) != 1 {
		t.Fatalf("frame = %v, mau tepat satu RESOLVED", h2.emit.states())
	}
	f := h2.emit.frames[0]
	if f.From != StateConfirmed || f.To != StateResolved || f.Reason != ReasonNoNewEvidence {
		t.Errorf("frame = %s->%s/%q, mau CONFIRMED->RESOLVED/NO_NEW_EVIDENCE", f.From, f.To, f.Reason)
	}
	if !f.EverConfirmed {
		t.Error("EverConfirmed harus true: all-clear diutangkan kepada audiens yang menerima alarmnya")
	}
	// Baris terminalnya ikut ditulis: sebuah event yang diselesaikan hanya di memori
	// adalah baris yang menggantung di HAPPENING selamanya.
	if len(p2.units) != 1 || p2.units[0].Event.Status != "RESOLVED" {
		t.Errorf("satuan persistensi = %d, mau satu baris RESOLVED", len(p2.units))
	}
	// Tetap dilacak sebagai tombstone: bukti yang terlambat tidak boleh melahirkan
	// alert publik kedua (§6.8).
	if e.TerminalAt == 0 {
		t.Error("TerminalAt harus disetel supaya tombstone dapat menua")
	}
	if got := h2.trk.TombstoneGauge(); got != 1 {
		t.Errorf("event_tombstone_gauge = %d, mau 1", got)
	}
}

// Baris tanpa satu pun baris event_state_log — pra-Fase-3, atau satuannya dibuang
// (D30). Ia dimuat memakai AGREGAT barisnya, dan bukti setelahnya tetap menempel:
// tanpa jangkar itu, centroid nol akan membuat event kedua.
func TestReconcileRowWithoutStateLogUsesRowAggregates(t *testing.T) {
	h := newHarness(t)
	h.threeNodeCluster()
	row := &store.EarthquakeEvent{
		EventID:              "F0000000-0000-4000-8000-000000000000",
		Status:               "HAPPENING",
		CentroidLat:          baseLat,
		CentroidLon:          baseLon,
		LocationName:         "Bandung",
		MaxPGA:               MinPGAGal + 60,
		TriggeredNodes:       3,
		StartedAtMs:          h.clock.now().UnixMilli(),
		EventState:           string(StateConfirmed),
		Revision:             2,
		OriginTS:             onsetBase,
		OriginTSSource:       OnsetSourceSensor,
		IndependentCellCount: 3,
	}
	h.restart(row)
	if err := h.trk.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	e := h.only()
	if len(e.Contributors) != 0 {
		t.Fatalf("kontributor = %d, mau 0: tidak ada bukti untuk dibangun ulang", len(e.Contributors))
	}
	if got := e.nodeCount(); got != 3 {
		t.Errorf("node_count = %d, mau 3 dari agregat baris", got)
	}
	if got := e.peakPGA(); got != MinPGAGal+60 {
		t.Errorf("peak_pga = %v, mau %v dari agregat baris", got, MinPGAGal+60)
	}
	if got := e.independentCells(); got != 3 {
		t.Errorf("sel independen = %d, mau 3 dari agregat baris", got)
	}
	if c := e.centroid(); c.Lat != baseLat || c.Lon != baseLon {
		t.Errorf("centroid = %v,%v, mau centroid baris", c.Lat, c.Lon)
	}

	h.ingest(v2("N1", MinPGAGal+10, onsetBase+1000, PhaseFinal, 7))
	e = h.only()
	if e.ID != row.EventID {
		t.Errorf("event_id = %q, mau %q", e.ID, row.EventID)
	}
	if got := e.nodeCount(); got != 1 {
		t.Errorf("node_count = %d, mau 1: begitu ada kontributor nyata, jangkar berhenti berbicara", got)
	}
}

// Baris pra-Fase-3 (event_state NULL) dimuat sebagai UNCONFIRMED, sehingga
// all-clear-nya membersihkan layar TANPA mengirim push ke seluruh fleet.
func TestReconcilePrePhase3RowResolvesWithoutPushRights(t *testing.T) {
	h := newHarness(t)
	row := &store.EarthquakeEvent{
		EventID:        "G0000000-0000-4000-8000-000000000000",
		Status:         "HAPPENING",
		CentroidLat:    baseLat,
		CentroidLon:    baseLon,
		LocationName:   "Bandung",
		MaxPGA:         MinPGAGal + 5,
		TriggeredNodes: 2,
		StartedAtMs:    h.clock.now().UnixMilli(),
	}
	h.restart(row)
	h.clock.advance(msDur(defaultOptions().ResolveAfterMs + 1))
	if err := h.trk.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if len(h.emit.frames) != 1 {
		t.Fatalf("frame = %v, mau satu RESOLVED", h.emit.states())
	}
	f := h.emit.frames[0]
	if f.From != StateUnconfirmed || f.To != StateResolved {
		t.Errorf("frame = %s->%s, mau UNCONFIRMED->RESOLVED", f.From, f.To)
	}
	if f.EverConfirmed {
		t.Error("EverConfirmed harus false: sepuluh baris menggantung tidak boleh menjadi sepuluh push")
	}
}

// Baris yang saling membantah dan baris non-publik yang kedaluwarsa DILEWATI, dan
// dilewati tanpa transisi: mesin keadaan tidak dipaksa menerima apa pun yang
// tidak diizinkannya.
func TestReconcileSkipsUnusableRows(t *testing.T) {
	cases := []struct {
		name  string
		state string
		stale bool
	}{
		{"event_state terminal pada baris HAPPENING", string(StateResolved), false},
		{"event_state tak dikenal", "TIDAK_ADA", false},
		{"DETECTED kedaluwarsa: D->RESOLVED ilegal", string(StateDetected), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.restart(&store.EarthquakeEvent{
				EventID:     "H0000000-0000-4000-8000-000000000000",
				Status:      "HAPPENING",
				CentroidLat: baseLat, CentroidLon: baseLon,
				StartedAtMs: h.clock.now().UnixMilli(),
				EventState:  tc.state,
			})
			if tc.stale {
				h.clock.advance(msDur(defaultOptions().ResolveAfterMs + 1))
			}
			if err := h.trk.Reconcile(context.Background()); err != nil {
				t.Fatalf("Reconcile: %v", err)
			}
			if got := len(h.events()); got != 0 {
				t.Errorf("event terlacak = %d, mau 0", got)
			}
			if len(h.emit.frames) != 0 {
				t.Errorf("frame = %v, mau tidak ada", h.emit.states())
			}
			if got := h.trk.Reconciled(); got != 0 {
				t.Errorf("event_reconciled_total = %d, mau 0", got)
			}
		})
	}
}

// DETECTED yang MASIH segar tetap dimuat: ia tidak publik, jadi tidak ada frame,
// tetapi ia memegang identitas yang bukti berikutnya harus menempel padanya.
func TestReconcileKeepsFreshDetectedRow(t *testing.T) {
	h := newHarness(t)
	h.restart(&store.EarthquakeEvent{
		EventID:     "I0000000-0000-4000-8000-000000000000",
		Status:      "HAPPENING",
		CentroidLat: baseLat, CentroidLon: baseLon,
		StartedAtMs: h.clock.now().UnixMilli(),
		OriginTS:    onsetBase,
		EventState:  string(StateDetected),
	})
	if err := h.trk.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	e := h.only()
	if e.State != StateDetected {
		t.Errorf("state = %s, mau DETECTED", e.State)
	}
	if len(h.emit.frames) != 0 {
		t.Errorf("frame = %v, mau tidak ada: DETECTED tidak pernah publik", h.emit.states())
	}
}

// Reconcile dipanggil dua kali tidak boleh menggandakan apa pun: memori adalah
// otoritas, jadi yang sudah hidup di memori yang menang (§9.5).
func TestReconcileTwiceIsIdempotent(t *testing.T) {
	h := newHarness(t)
	p := h.withPersister()
	before := h.confirmThreeNodes()
	row := rowFrom(t, p.units[len(p.units)-1], h.clock.now().UnixMilli())

	h2 := newHarness(t)
	h2.trk.loc = h.loc
	h2.restart(row)
	for i := 0; i < 2; i++ {
		if err := h2.trk.Reconcile(context.Background()); err != nil {
			t.Fatalf("Reconcile #%d: %v", i+1, err)
		}
	}
	e := h2.only()
	if e.ID != before.ID {
		t.Errorf("event_id = %q, mau %q", e.ID, before.ID)
	}
	if got := h2.trk.Reconciled(); got != 1 {
		t.Errorf("event_reconciled_total = %d, mau 1", got)
	}
}

// §15.4 — langit-langit event terbuka berlaku pada jalur boot juga, dan berhenti
// MEMUAT alih-alih mendorong keluar apa yang baru saja dimuat.
func TestReconcileStopsAtMaxOpen(t *testing.T) {
	h := newHarness(t, func(o *Options) { o.MaxOpen = 1 })
	rows := make([]*store.EarthquakeEvent, 0, 2)
	for i := 0; i < 2; i++ {
		rows = append(rows, &store.EarthquakeEvent{
			EventID:     string(rune('J'+i)) + "0000000-0000-4000-8000-000000000000",
			Status:      "HAPPENING",
			CentroidLat: baseLat + float64(i)*3, CentroidLon: baseLon,
			StartedAtMs: h.clock.now().UnixMilli(),
			OriginTS:    onsetBase,
			EventState:  string(StateConfirmed),
		})
	}
	h.restart(rows...)
	if err := h.trk.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got := len(h.events()); got != 1 {
		t.Errorf("event terlacak = %d, mau 1", got)
	}
	if got := h.trk.ForcedResolutions(); got != 0 {
		t.Errorf("event_forced_resolutions_total = %d, mau 0: tidak ada yang didorong keluar", got)
	}
}

// Kegagalan pembacaan dikembalikan sebagai galat dan tidak menyentuh apa pun.
// Pemanggil (§15.3 langkah 5) mencatatnya dan tetap menyala.
func TestReconcileReadFailureIsReportedNotFatal(t *testing.T) {
	h := newHarness(t)
	fe := h.restart()
	fe.openErr = errors.New("basis data mati")

	err := h.trk.Reconcile(context.Background())
	if err == nil {
		t.Fatal("galat = nil, mau galat pembacaan")
	}
	if got := len(h.events()); got != 0 {
		t.Errorf("event terlacak = %d, mau 0", got)
	}
}

// Kontributor yang lokasinya tidak lagi dapat dicari HILANG, event tidak: satu
// suara jauh lebih ringan daripada identitas event.
func TestReconcileDropsContributorWithoutLocation(t *testing.T) {
	h := newHarness(t)
	p := h.withPersister()
	h.confirmThreeNodes()
	row := rowFrom(t, p.units[len(p.units)-1], h.clock.now().UnixMilli())

	h2 := newHarness(t)
	h2.node("N1", baseLat, baseLon)
	h2.nodeAt("N2", baseLat, baseLon, 8, 90)
	// N3 dihapus dari fleet.
	h2.restart(row)
	if err := h2.trk.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	e := h2.only()
	if got := nodeIDsOf(e); strings.Join(got, ",") != "N1,N2" {
		t.Errorf("kontributor = %v, mau [N1 N2]", got)
	}
}

// Toko yang tidak dapat membaca event terbuka bukan galat: Tracker tanpa
// persistensi tetap sah (§9.5), dan seluruh uji paket ini dibangun begitu.
func TestReconcileWithoutEventStoreIsNoop(t *testing.T) {
	h := newHarness(t)
	if err := h.trk.Reconcile(context.Background()); err != nil {
		t.Errorf("galat = %v, mau nil", err)
	}
	h.trk.CheckFleetIndependence(context.Background())
	if got := len(h.events()); got != 0 {
		t.Errorf("event terlacak = %d, mau 0", got)
	}
}

// ---- §7.3 / §6.3.1 pemeriksaan-diri --------------------------------------

// checker membangun Tracker dengan log yang dapat dibaca uji: yang diuji di sini
// adalah BARIS LOG-nya, karena baris log itulah keseluruhan mekanismenya.
func checker(t *testing.T, nodes []store.NodeLocation, mutate ...func(*Options)) (*Tracker, *bytes.Buffer) {
	t.Helper()
	opt := defaultOptions()
	for _, m := range mutate {
		m(&opt)
	}
	var buf bytes.Buffer
	fe := &fakeEvents{fakeLoc: &fakeLoc{}, nodes: nodes}
	trk := NewTracker(fe, opt, slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	return trk, &buf
}

// sameCellFleet membangun n node yang PASTI lebih dekat dari cellKm satu sama
// lain: titik pertama adalah pusat sel grid yang memuat acuan, sisanya digeser
// paling banyak seperempat sisi sel, sehingga pemisahan terbesarnya jauh di bawah
// cellKm dan seluruh fleet hanya satu bukti independen.
func sameCellFleet(cellKm float64, n int) []store.NodeLocation {
	deg := independenceCellDeg(cellKm)
	k := independenceCell(baseLat, baseLon, cellKm)
	cLat := (float64(k.Y) + 0.5) * deg
	cLon := (float64(k.X) + 0.5) * deg

	out := make([]store.NodeLocation, 0, n)
	for i := 0; i < n; i++ {
		step := (float64(i) / float64(n)) * (deg / 4)
		out = append(out, store.NodeLocation{
			StationID: "N" + string(rune('1'+i)),
			Lat:       cLat + step,
			Lon:       cLon + step,
		})
	}
	return out
}

func TestCheckFleetIndependenceWarnsWhenConfirmedUnreachable(t *testing.T) {
	// Tiga node di DALAM satu sel independensi 10 km: CONFIRMED tidak dapat
	// dicapai. Koordinatnya dijepret ke pusat selnya, bukan digeser sejauh satu
	// kilometer dari titik acuan: yang kedua akan menyeberang batas sel bila titik
	// acuan kebetulan berada di pinggirnya, dan ujinya akan lulus atau gagal karena
	// aritmetika grid alih-alih karena perilaku yang diuji.
	trk, buf := checker(t, sameCellFleet(10, 3), func(o *Options) { o.IndependenceCellKm = 10 })

	trk.CheckFleetIndependence(context.Background())

	out := buf.String()
	if !strings.Contains(out, "level=WARN") {
		t.Fatalf("tidak ada baris WARN:\n%s", out)
	}
	for _, want := range []string{
		"CONFIRMED tidak dapat dicapai",
		"active_verified_nodes=3",
		"independence_cells=1",
		"independence_cell_km=10",
		"need=2",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("baris peringatan tidak menyebut %q:\n%s", want, out)
		}
	}
}

func TestCheckFleetIndependencePassesOnSpreadFleet(t *testing.T) {
	lat2, lon2 := destinationKm(baseLat, baseLon, 8, 90)
	trk, buf := checker(t, []store.NodeLocation{
		{StationID: "N1", Lat: baseLat, Lon: baseLon},
		{StationID: "N2", Lat: lat2, Lon: lon2},
	})

	trk.CheckFleetIndependence(context.Background())

	out := buf.String()
	if strings.Contains(out, "level=WARN") || strings.Contains(out, "level=ERROR") {
		t.Errorf("fleet yang tersebar tidak boleh mengeluh:\n%s", out)
	}
	if !strings.Contains(out, "independence_cells=2") {
		t.Errorf("baris lulus harus menyebut jumlah selnya:\n%s", out)
	}
}

// Pemeriksaan-diri fleet TIDAK LAGI mengeluh tentang lintang. Dahulu sebuah node
// di luar |lat| <= 12° dicatat pada ERROR karena pembuktian kecukupan lingkungan
// 3x3 tidak berlaku di sana; lebar pencarian sekarang diturunkan dari lintang
// observasi itu sendiri, jadi peringatan itu akan SALAH. Diuji secara eksplisit,
// bukan sekadar dihapus: sebuah fleet global yang menyala harus menyala TANPA
// baris galat yang tidak berarti apa pun.
func TestCheckFleetIndependenceIsSilentAtAnyLatitude(t *testing.T) {
	for _, lat := range []float64{0, 12.5, 45.46, 61.2, 64.13, -49.28} {
		lat2, lon2 := destinationKm(lat, baseLon, 8, 90)
		trk, buf := checker(t, []store.NodeLocation{
			{StationID: "FAR", Lat: lat, Lon: baseLon},
			{StationID: "N2", Lat: lat2, Lon: lon2},
		})

		trk.CheckFleetIndependence(context.Background())

		out := buf.String()
		if strings.Contains(out, "level=ERROR") || strings.Contains(out, "level=WARN") {
			t.Errorf("lintang %.2f: fleet yang tersebar dikeluhkan:\n%s", lat, out)
		}
		if !strings.Contains(out, "independence_cells=2") {
			t.Errorf("lintang %.2f: dua node terpisah 8 km harus dua bukti:\n%s", lat, out)
		}
	}
}

// Dua node yang berjarak 2,5 km TIDAK independen, di lintang mana pun — inti
// perbaikan D2. Dengan sel grid, pasangan ini jatuh di sel bujur berbeda di
// lintang tinggi (lebar sel bujur 5 km menyusut menjadi ~2,2 km di 64°) dan
// terhitung dua bukti, sehingga CONFIRMED menjadi lebih MUDAH justru di tempat
// grid-nya paling salah.
func TestCheckFleetIndependenceCountsNearbyPairAsOneAtHighLatitude(t *testing.T) {
	for _, lat := range []float64{0, 45.46, 64.13} {
		lat2, lon2 := destinationKm(lat, baseLon, 2.5, 90)
		trk, buf := checker(t, []store.NodeLocation{
			{StationID: "N1", Lat: lat, Lon: baseLon},
			{StationID: "N2", Lat: lat2, Lon: lon2},
		})

		trk.CheckFleetIndependence(context.Background())

		out := buf.String()
		if !strings.Contains(out, "independence_cells=1") {
			t.Errorf("lintang %.2f: pasangan 2,5 km harus satu bukti:\n%s", lat, out)
		}
		if !strings.Contains(out, "CONFIRMED tidak dapat dicapai") {
			t.Errorf("lintang %.2f: fleet satu-bukti harus diperingatkan:\n%s", lat, out)
		}
	}
}

func TestCheckFleetIndependenceReportsEmptyAndFailingFleet(t *testing.T) {
	trk, buf := checker(t, nil)
	trk.CheckFleetIndependence(context.Background())
	if !strings.Contains(buf.String(), "tidak ada node aktif terverifikasi") {
		t.Errorf("fleet kosong harus terlihat:\n%s", buf.String())
	}

	trk2, buf2 := checker(t, nil)
	trk2.loc.(*fakeEvents).nodesErr = errors.New("basis data mati")
	trk2.CheckFleetIndependence(context.Background())
	if !strings.Contains(buf2.String(), "pemeriksaan independensi fleet gagal") {
		t.Errorf("kegagalan pembacaan harus terlihat:\n%s", buf2.String())
	}
}
