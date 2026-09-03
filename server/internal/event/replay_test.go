package event

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// Uji P4-M4′. Tiga hal yang diperiksa berkas ini, dan hanya tiga:
//
//  1. GERBANG: label algo_ver yang tak kompatibel DITOLAK, bukan diputar.
//  2. DETERMINISME: baris yang sama menghasilkan keputusan yang sama.
//  3. BIJEKSI PENGELOMPOKAN (F2) dan DELTA WAKTU (F3): setiap event historis
//     berpasangan 1:1 dengan tepat satu event replay, dan selisih waktunya
//     dilaporkan sebagai delta — bukan sebagai kesetaraan timestamp.
//
// Riwayat "historis" di dalam uji ini dibangun lewat Tracker.unitFor, mapper
// PRODUKSI yang menulis event_state_log. Bukan lewat penulis kedua: bila uji
// punya pemetaan Snapshot->baris sendiri, yang diuji bukan lagi baris yang
// benar-benar tersimpan.

// ---- pembangun baris ledger ------------------------------------------------

type rpRow struct {
	id       int64
	node     string
	phase    string
	pga      float64
	durMs    int64
	publish  int64
	received int64

	// onset != 0 berarti onset_ts_source = SENSOR; kalau tidak PUBLISH_BOUND.
	onset int64
	upper int64
	seq   int64

	lat, lon float64
	noLoc    bool
	verify   string
}

func (r rpRow) obs() store.ReplayObservation {
	o := store.ReplayObservation{
		ObservationID: r.id,
		NodeID:        r.node,
		Phase:         r.phase,
		PGAGal:        r.pga,
		DurMs:         r.durMs,
		PublishTS:     r.publish,
		ReceivedTS:    r.received,
		VerifyResult:  r.verify,
	}
	if o.VerifyResult == "" {
		o.VerifyResult = "OK"
	}
	if r.onset != 0 {
		v := r.onset
		o.OnsetTS = &v
		o.OnsetTSSource = OnsetSourceSensor
	} else {
		o.OnsetTSSource = OnsetSourcePublish
	}
	if r.upper != 0 {
		v := r.upper
		o.OnsetTSUpperBound = &v
	}
	if r.seq != 0 {
		v := r.seq
		o.ObsSeq = &v
		p := int16(2)
		o.ProtoVer = &p
	}
	if !r.noLoc {
		lat, lon := r.lat, r.lon
		o.Lat, o.Lon = &lat, &lon
	}
	return o
}

func rpObs(rows ...rpRow) []store.ReplayObservation {
	out := make([]store.ReplayObservation, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.obs())
	}
	return out
}

// histFrom mengubah frame menjadi baris event_state_log lewat mapper PRODUKSI
// (Tracker.unitFor), sehingga fixture riwayat punya bentuk yang sama persis
// dengan baris yang benar-benar ditulis — termasuk evidence_summary yang
// diserialkan oleh EvidenceSummary.JSON().
//
// Dipakai untuk uji yang menanyakan "apakah pembanding menyatakan cocok ketika
// memang cocok". Uji sensor nyata TIDAK memakainya: di sana riwayat disusun dari
// skalar yang benar-benar terbaca dari ledger produksi.
func histFrom(opt Options, frames []Snapshot) []store.EventStateLog {
	trk := NewTracker(&replayLocator{}, opt, slog.New(slog.NewTextHandler(io.Discard, nil)))
	out := make([]store.EventStateLog, 0, len(frames))
	for _, f := range frames {
		out = append(out, *trk.unitFor(f).Log)
	}
	return out
}

func rpProfile(mutate ...func(*Options)) ReplayProfile {
	p := ReplayProfile{Options: defaultOptions()}
	for _, m := range mutate {
		m(&p.Options)
	}
	return p
}

// ---- 1. gerbang algo_ver ---------------------------------------------------

func TestReplayParseAlgoVer(t *testing.T) {
	base, ic, err := ParseAlgoVer("phase3-1.1/ic=5")
	if err != nil {
		t.Fatalf("ParseAlgoVer galat: %v", err)
	}
	if base != "phase3-1.1" || ic != 5 {
		t.Fatalf("ParseAlgoVer = %q,%g; mau phase3-1.1,5", base, ic)
	}

	for _, bad := range []string{"", "phase3-1.1", "phase3-1.1/ic=", "phase3-1.1/ic=abc", "ic=5"} {
		if _, _, err := ParseAlgoVer(bad); !errors.Is(err, ErrNoAlgoVer) {
			t.Errorf("ParseAlgoVer(%q) galat = %v; mau ErrNoAlgoVer", bad, err)
		}
	}
}

func TestReplayCheckAlgoVerAcceptsOwnLabel(t *testing.T) {
	p := rpProfile()
	trk := NewTracker(&replayLocator{}, p.Options, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := CheckAlgoVer(trk.algoVer(), p); err != nil {
		t.Fatalf("label yang dihasilkan biner ini sendiri ditolak: %v", err)
	}
}

// Basis algoritma yang berbeda berarti keputusan dibuat oleh aturan yang bukan
// aturan biner ini. Memutarnya akan melaporkan perbedaan ALGORITMA sebagai
// non-determinisme, jadi gerbangnya menolak, bukan memperingatkan.
func TestReplayCheckAlgoVerRejectsOtherBase(t *testing.T) {
	err := CheckAlgoVer("phase3-1.0/ic=5", rpProfile())
	if !errors.Is(err, ErrAlgoVerIncompatible) {
		t.Fatalf("galat = %v; mau ErrAlgoVerIncompatible", err)
	}
}

// IndependenceCellKm adalah SATU-SATUNYA parameter keputusan yang terbawa oleh
// baris historis. Profil operator tidak boleh menimpanya diam-diam.
func TestReplayCheckAlgoVerRejectsContradictingProfile(t *testing.T) {
	p := rpProfile(func(o *Options) { o.IndependenceCellKm = 50 })
	err := CheckAlgoVer(AlgoVerBase()+"/ic=5", p)
	if err == nil {
		t.Fatal("profil ic=50 atas baris ic=5 diterima; mau ditolak")
	}
	if errors.Is(err, ErrAlgoVerIncompatible) || errors.Is(err, ErrNoAlgoVer) {
		t.Fatalf("galat = %v; mau galat konflik parameter, bukan galat basis/urai", err)
	}
}

func TestReplayCheckAlgoVerRejectsEmpty(t *testing.T) {
	if err := CheckAlgoVer("", rpProfile()); !errors.Is(err, ErrNoAlgoVer) {
		t.Fatalf("galat = %v; mau ErrNoAlgoVer", err)
	}
}

// ---- 2. mesin replay -------------------------------------------------------

// Satu node: PRELIM di bawah lantai, lalu FINAL di atasnya. Bentuk yang sama
// dengan satu-satunya event sensor nyata yang dimiliki proyek ini (S2: fleet
// satu node membuat CONFIRMED tak terjangkau lewat kerapatan, bukan cacat).
func rpSingleNodeRows() []store.ReplayObservation {
	return rpObs(
		rpRow{id: 1, node: "NODE-A", phase: PhasePrelim, pga: 2.2952, durMs: 300,
			publish: 1000300, received: 1000330, onset: 1000000, upper: 1000000, seq: 100,
			lat: -6.8562093, lon: 107.5289622},
		rpRow{id: 2, node: "NODE-A", phase: PhaseFinal, pga: 73.0537, durMs: 6138,
			publish: 1006147, received: 1006164, onset: 1000000, upper: 1000009, seq: 100,
			lat: -6.8562093, lon: 107.5289622},
	)
}

func TestReplaySingleNodeFloorMetThenResolved(t *testing.T) {
	res, err := Replay(context.Background(), rpSingleNodeRows(), rpProfile())
	if err != nil {
		t.Fatalf("Replay galat: %v", err)
	}

	if res.Input.TotalRows != 2 || res.Input.FedRows != 2 || len(res.Input.Skipped) != 0 {
		t.Fatalf("InputReport = %+v; mau 2 baris, 2 diumpankan, 0 terbuang", res.Input)
	}
	if len(res.Events) != 1 {
		t.Fatalf("event replay = %d; mau 1", len(res.Events))
	}
	if res.AlgoVer != AlgoVerBase()+"/ic=5" {
		t.Fatalf("AlgoVer = %q; mau %s/ic=5", res.AlgoVer, AlgoVerBase())
	}

	var frames []Snapshot
	for _, fs := range res.Events {
		frames = fs
	}
	// PRELIM di bawah lantai TIDAK menghasilkan frame: DETECTED adalah state awal,
	// jadi tidak ada transisi untuk diemisikan.
	if len(frames) != 2 {
		t.Fatalf("frame = %d (%v); mau 2: UNCONFIRMED lalu RESOLVED", len(frames), rpStates(frames))
	}
	if frames[0].To != StateUnconfirmed || frames[0].Reason != ReasonFloorMet || frames[0].Revision != 1 {
		t.Errorf("frame[0] = %s/%s rev%d; mau UNCONFIRMED/FLOOR_MET rev1",
			frames[0].To, frames[0].Reason, frames[0].Revision)
	}
	if frames[0].From != StateDetected {
		t.Errorf("frame[0].From = %s; mau DETECTED", frames[0].From)
	}
	if frames[1].To != StateResolved || frames[1].Reason != ReasonNoNewEvidence || frames[1].Revision != 2 {
		t.Errorf("frame[1] = %s/%s rev%d; mau RESOLVED/NO_NEW_EVIDENCE rev2",
			frames[1].To, frames[1].Reason, frames[1].Revision)
	}
	// Puncaknya FINAL, bukan PRELIM: PGA hanya boleh naik.
	if got := round4(frames[0].PeakPGA); got != 73.0537 {
		t.Errorf("peak_pga = %v; mau 73.0537", got)
	}
	// Jangkar diambil dari kolom onset_ts, bukan dihitung publish_ts - dur_ms.
	if frames[0].OriginTS != 1000000 || frames[0].OriginTSSource != OnsetSourceSensor {
		t.Errorf("origin = %d/%s; mau 1000000/SENSOR", frames[0].OriginTS, frames[0].OriginTSSource)
	}
}

func rpStates(fs []Snapshot) []State {
	out := make([]State, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.To)
	}
	return out
}

// Determinisme (V7): dua pemutaran atas baris yang sama harus identik pada
// SETIAP field keputusan, bukan hanya pada jumlah event.
func TestReplayIsDeterministic(t *testing.T) {
	rows := rpMultiNodeRows()
	a, err := Replay(context.Background(), rows, rpProfile())
	if err != nil {
		t.Fatalf("Replay #1 galat: %v", err)
	}
	b, err := Replay(context.Background(), rows, rpProfile())
	if err != nil {
		t.Fatalf("Replay #2 galat: %v", err)
	}

	if len(a.Frames) != len(b.Frames) {
		t.Fatalf("jumlah frame = %d vs %d", len(a.Frames), len(b.Frames))
	}
	for i := range a.Frames {
		x, y := a.Frames[i], b.Frames[i]
		if x.EventID != y.EventID || x.Revision != y.Revision || x.To != y.To ||
			x.From != y.From || x.Reason != y.Reason || x.DecidedAt != y.DecidedAt ||
			x.NodeCount != y.NodeCount || x.IndependentCells != y.IndependentCells ||
			round4(x.PeakPGA) != round4(y.PeakPGA) ||
			string(x.Evidence.JSON()) != string(y.Evidence.JSON()) {
			t.Fatalf("frame[%d] menyimpang:\n a=%+v\n b=%+v", i, x, y)
		}
	}
}

// Baris yang diumpankan ulang apa adanya: obs_seq sama, jadi
// isSecondEpisodeLocked salah dan tidak ada episode kedua. Tidak boleh ada event
// baru maupun transisi baru.
func TestReplayDuplicateRowAddsNoEventOrTransition(t *testing.T) {
	base := rpSingleNodeRows()
	once, err := Replay(context.Background(), base, rpProfile())
	if err != nil {
		t.Fatalf("Replay galat: %v", err)
	}

	dup := base[1]
	dup.ObservationID = 3
	dup.ReceivedTS = base[1].ReceivedTS + 40
	twice, err := Replay(context.Background(), append(append([]store.ReplayObservation{}, base...), dup), rpProfile())
	if err != nil {
		t.Fatalf("Replay rangkap galat: %v", err)
	}

	if len(twice.Events) != len(once.Events) {
		t.Errorf("event = %d dengan baris rangkap, %d tanpanya; mau sama",
			len(twice.Events), len(once.Events))
	}
	if len(twice.Frames) != len(once.Frames) {
		t.Errorf("frame = %d dengan baris rangkap, %d tanpanya; mau sama",
			len(twice.Frames), len(once.Frames))
	}
	if twice.Input.FedRows != 3 {
		t.Errorf("FedRows = %d; mau 3 — baris rangkap tetap DIUMPANKAN dan terhitung",
			twice.Input.FedRows)
	}
}

// rpMultiNodeRows mencerminkan sim 3.1 (scripts/sim_multi_node.sh): tiga node
// Bandung, satu jendela korelasi, PGA jauh di atas lantai. Koordinatnya
// koordinat skrip itu, bukan koordinat baru, supaya replay diuji atas geometri
// yang benar-benar diproduksi simulasi.
func rpMultiNodeRows() []store.ReplayObservation {
	const onset = 2000000
	return rpObs(
		rpRow{id: 11, node: "NODE-53494D41", phase: PhaseFinal, pga: 300, durMs: 3500,
			publish: onset + 3500, received: onset + 3520, onset: onset, upper: onset, seq: 201,
			lat: -6.900, lon: 107.600},
		rpRow{id: 12, node: "NODE-53494D42", phase: PhaseFinal, pga: 300, durMs: 3500,
			publish: onset + 4500, received: onset + 4520, onset: onset + 1000, upper: onset + 1000, seq: 202,
			lat: -6.855, lon: 107.600},
		rpRow{id: 13, node: "NODE-53494D43", phase: PhaseFinal, pga: 300, durMs: 3500,
			publish: onset + 5500, received: onset + 5520, onset: onset + 2000, upper: onset + 2000, seq: 203,
			lat: -6.910, lon: 107.650},
	)
}

func TestReplayMultiNodeReachesConfirmed(t *testing.T) {
	res, err := Replay(context.Background(), rpMultiNodeRows(), rpProfile())
	if err != nil {
		t.Fatalf("Replay galat: %v", err)
	}
	if len(res.Events) != 1 {
		t.Fatalf("event = %d; mau 1 kluster tunggal", len(res.Events))
	}
	var frames []Snapshot
	for _, fs := range res.Events {
		frames = fs
	}
	want := []State{StateUnconfirmed, StateConfirmed, StateResolved}
	got := rpStates(frames)
	if len(got) != len(want) {
		t.Fatalf("state = %v; mau %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("state = %v; mau %v", got, want)
		}
	}
	conf := frames[1]
	if conf.Reason != ReasonQuorumMet {
		t.Errorf("alasan CONFIRMED = %q; mau QUORUM_MET", conf.Reason)
	}
	if conf.NodeCount != 3 {
		t.Errorf("node_count = %d; mau 3", conf.NodeCount)
	}
	if conf.IndependentCells < 2 {
		t.Errorf("independent_cells = %d; mau >= 2", conf.IndependentCells)
	}
}

// rpDualEventRows mencerminkan sim 3.2: kluster Bandung dan kluster Surabaya,
// ~570 km terpisah, dipublikasikan berselang-seling dalam satu jendela.
func rpDualEventRows() []store.ReplayObservation {
	const onset = 3000000
	b := [3]struct {
		id       string
		lat, lon float64
	}{
		{"NODE-53554241", -7.250, 112.750},
		{"NODE-53554242", -7.205, 112.750},
		{"NODE-53554243", -7.260, 112.800},
	}
	rows := rpMultiNodeRows()
	for i := range rows {
		rows[i].ReceivedTS += 1000000
		rows[i].PublishTS += 1000000
		o := *rows[i].OnsetTS + 1000000
		rows[i].OnsetTS = &o
		u := *rows[i].OnsetTSUpperBound + 1000000
		rows[i].OnsetTSUpperBound = &u
	}
	extra := make([]rpRow, 0, 3)
	for i, n := range b {
		extra = append(extra, rpRow{
			id: int64(21 + i), node: n.id, phase: PhaseFinal, pga: 300, durMs: 3500,
			publish:  onset + 4000 + int64(i)*1000,
			received: onset + 4020 + int64(i)*1000,
			onset:    onset + 500 + int64(i)*1000,
			upper:    onset + 500 + int64(i)*1000,
			seq:      int64(301 + i),
			lat:      n.lat, lon: n.lon,
		})
	}
	all := append(rows, rpObs(extra...)...)
	rpSortCanonical(all)
	return all
}

// rpSortCanonical menerapkan urutan kanonik received_ts lalu observation_id —
// urutan yang dijanjikan ListObservationsForReplay dan yang Replay asumsikan.
func rpSortCanonical(rows []store.ReplayObservation) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0; j-- {
			a, b := rows[j-1], rows[j]
			if a.ReceivedTS < b.ReceivedTS || (a.ReceivedTS == b.ReceivedTS && a.ObservationID <= b.ObservationID) {
				break
			}
			rows[j-1], rows[j] = b, a
		}
	}
}

func TestReplayDualEventStaysSeparate(t *testing.T) {
	res, err := Replay(context.Background(), rpDualEventRows(), rpProfile())
	if err != nil {
		t.Fatalf("Replay galat: %v", err)
	}
	if len(res.Events) != 2 {
		t.Fatalf("event = %d; mau 2 kluster terpisah (%d frame)", len(res.Events), len(res.Frames))
	}
	for id, fs := range res.Events {
		if fs[len(fs)-1].To != StateResolved {
			t.Errorf("event %s berakhir di %s; mau RESOLVED", id, fs[len(fs)-1].To)
		}
		if fs[len(fs)-1].NodeCount != 3 {
			t.Errorf("event %s node_count = %d; mau 3 — kluster tidak boleh bercampur",
				id, fs[len(fs)-1].NodeCount)
		}
	}
}

// ---- 3. akuntansi baris terbuang -------------------------------------------

// Konsensus produksi juga membuang baris-baris ini. Yang diuji di sini bukan
// bahwa ia membuangnya, melainkan bahwa ia membuangnya DENGAN JEJAK: sebuah
// replay yang cocok hanya bermakna bila diketahui apa yang diputar.
func TestReplayReportsSkippedRows(t *testing.T) {
	rows := rpObs(
		rpRow{id: 31, node: "NODE-BAD", phase: PhaseFinal, pga: 300, durMs: 3000,
			publish: 4003000, received: 4003020, onset: 4000000, upper: 4000000, seq: 401,
			lat: -6.9, lon: 107.6, verify: "SIGNATURE_INVALID"},
		rpRow{id: 32, node: "NODE-NOLOC", phase: PhaseFinal, pga: 300, durMs: 3000,
			publish: 4004000, received: 4004020, onset: 4001000, upper: 4001000, seq: 402,
			noLoc: true},
		// Kedua jangkar NULL: onset_ts_source PUBLISH_BOUND tetapi
		// onset_ts_upper_bound juga tidak ada.
		rpRow{id: 33, node: "NODE-NOANCHOR", phase: PhaseFinal, pga: 300, durMs: 3000,
			publish: 4005000, received: 4005020, lat: -6.9, lon: 107.6},
		rpRow{id: 34, node: "NODE-OK", phase: PhaseFinal, pga: 300, durMs: 3000,
			publish: 4006000, received: 4006020, onset: 4003000, upper: 4003000, seq: 404,
			lat: -6.9, lon: 107.6},
	)

	res, err := Replay(context.Background(), rows, rpProfile())
	if err != nil {
		t.Fatalf("Replay galat: %v", err)
	}

	if res.Input.TotalRows != 4 {
		t.Errorf("TotalRows = %d; mau 4", res.Input.TotalRows)
	}
	if res.Input.FedRows != 1 {
		t.Errorf("FedRows = %d; mau 1", res.Input.FedRows)
	}
	counts := res.Input.SkipCounts()
	for reason, want := range map[string]int{
		SkipVerifyNotOK:   1,
		SkipNoLocation:    1,
		SkipNoOnsetAnchor: 1,
	} {
		if counts[reason] != want {
			t.Errorf("SkipCounts[%s] = %d; mau %d (semua: %v)", reason, counts[reason], want, counts)
		}
	}
	// Baris terbuang harus dapat DISEBUT, bukan hanya dihitung.
	for _, s := range res.Input.Skipped {
		if s.ObservationID == 0 || s.NodeID == "" {
			t.Errorf("baris terbuang tanpa identitas: %+v", s)
		}
	}
	// LedgerDropsKnown nol berarti "tidak diketahui", bukan "tidak ada yang
	// hilang" — nilainya diisi operator dari log, bukan dihitung dari baris.
	if res.Input.LedgerDropsKnown != 0 {
		t.Errorf("LedgerDropsKnown = %d; mau 0 karena tidak ada operator yang mengisinya",
			res.Input.LedgerDropsKnown)
	}
}

func TestReplayEmptyWindowIsError(t *testing.T) {
	if _, err := Replay(context.Background(), nil, rpProfile()); err == nil {
		t.Fatal("jendela kosong diterima; mau galat")
	}
}

// Baris v1 (tanpa obs_seq, PUBLISH_BOUND) memakai onset_ts_upper_bound sebagai
// jangkar dan TIDAK pernah dilaporkan sebagai pengukuran sensor.
func TestReplayV1UsesStoredUpperBoundNotRecomputed(t *testing.T) {
	// upper SENGAJA bukan publish - dur (yang akan 4000000): bila replay
	// menghitung ulang rumusnya alih-alih membaca kolom, OriginTS akan 4000000.
	rows := rpObs(rpRow{id: 41, node: "NODE-V1", phase: PhaseFinal, pga: 300, durMs: 3000,
		publish: 4003000, received: 4003020, upper: 3999123, lat: -6.9, lon: 107.6})

	res, err := Replay(context.Background(), rows, rpProfile())
	if err != nil {
		t.Fatalf("Replay galat: %v", err)
	}
	var frames []Snapshot
	for _, fs := range res.Events {
		frames = fs
	}
	if len(frames) == 0 {
		t.Fatal("tidak ada frame")
	}
	if frames[0].OriginTS != 3999123 {
		t.Errorf("OriginTS = %d; mau 3999123 dari KOLOM, bukan publish_ts - dur_ms",
			frames[0].OriginTS)
	}
	if frames[0].OriginTSSource != OnsetSourcePublish {
		t.Errorf("OriginTSSource = %q; mau PUBLISH_BOUND", frames[0].OriginTSSource)
	}
}

// ---- 4. bijeksi dan perbandingan -------------------------------------------

// Kasus dasar: riwayat DIBANGUN dari pemutaran yang sama, jadi pembanding wajib
// menyatakan bijeksi DAN kecocokan keputusan. Bila uji ini gagal, pembandingnya
// yang rusak — bukan trackernya.
func TestReplayCompareSelfConsistent(t *testing.T) {
	p := rpProfile()
	rows := rpMultiNodeRows()
	res, err := Replay(context.Background(), rows, p)
	if err != nil {
		t.Fatalf("Replay galat: %v", err)
	}
	hist := histFrom(p.Options, res.Frames)

	rep := Compare(hist, res, p)
	if !rep.Bijective() {
		t.Fatalf("tidak bijektif: historis tanpa pasangan=%v replay tanpa pasangan=%v ambigu=%v",
			rep.UnmatchedHistoric, rep.UnmatchedReplayed, rep.AmbiguousSignatures)
	}
	if !rep.DecisionsReproduced() {
		for _, e := range rep.Events {
			for _, d := range e.Diffs {
				t.Errorf("diff: %s", d)
			}
		}
		t.Fatal("keputusan tidak direproduksi atas datanya sendiri")
	}
	if len(rep.Events) != 1 {
		t.Fatalf("perbandingan event = %d; mau 1", len(rep.Events))
	}
	// F3: waktu dibandingkan sebagai delta, dan deltanya nol di sini karena kedua
	// sisi berasal dari satu pemutaran.
	c := rep.Events[0]
	if len(c.Timings) != 3 {
		t.Fatalf("Timings = %d; mau 3", len(c.Timings))
	}
	for _, tm := range c.Timings {
		if tm.DifferenceMs != 0 || !tm.WithinTolerance {
			t.Errorf("timing rev%d: selisih=%d dalam-toleransi=%v; mau 0/true",
				tm.Revision, tm.DifferenceMs, tm.WithinTolerance)
		}
	}
}

func TestReplayCompareDualEventBijection(t *testing.T) {
	p := rpProfile()
	res, err := Replay(context.Background(), rpDualEventRows(), p)
	if err != nil {
		t.Fatalf("Replay galat: %v", err)
	}
	hist := histFrom(p.Options, res.Frames)

	rep := Compare(hist, res, p)
	if !rep.Bijective() || !rep.DecisionsReproduced() {
		t.Fatalf("dua event tidak berpasangan 1:1: historis=%v replay=%v ambigu=%v",
			rep.UnmatchedHistoric, rep.UnmatchedReplayed, rep.AmbiguousSignatures)
	}
	if len(rep.Events) != 2 {
		t.Fatalf("perbandingan event = %d; mau 2", len(rep.Events))
	}
	// Tanda tangan kedua event harus BERBEDA: bila keduanya sama, bijeksi yang
	// lolos di atas tidak berarti apa pun.
	if rep.Events[0].Signature == rep.Events[1].Signature {
		t.Fatalf("kedua event bertanda tangan sama: %q", rep.Events[0].Signature)
	}
}

// UUID historis SENGAJA berbeda dari id replay (F2). Pemasangan tetap harus
// terjadi, dan id kedua sisi dilaporkan apa adanya.
func TestReplayCompareIgnoresUUIDEquality(t *testing.T) {
	p := rpProfile()
	res, err := Replay(context.Background(), rpMultiNodeRows(), p)
	if err != nil {
		t.Fatalf("Replay galat: %v", err)
	}
	hist := histFrom(p.Options, res.Frames)
	const histID = "9f9f9f9f-1111-4000-8000-000000000000"
	for i := range hist {
		hist[i].EventID = histID
	}

	rep := Compare(hist, res, p)
	if !rep.DecisionsReproduced() {
		t.Fatalf("event_id berbeda membuat perbandingan gagal; F2 minta bijeksi, bukan kesetaraan UUID (%v/%v)",
			rep.UnmatchedHistoric, rep.UnmatchedReplayed)
	}
	c := rep.Events[0]
	if c.HistoricEventID != histID {
		t.Errorf("HistoricEventID = %q; mau %q", c.HistoricEventID, histID)
	}
	if c.ReplayEventID == histID || c.ReplayEventID == "" {
		t.Errorf("ReplayEventID = %q; mau id replay yang berbeda dan tidak kosong", c.ReplayEventID)
	}
}

// decided_at historis digeser SERAGAM: delta antar-revisi tidak berubah, jadi
// F3 harus tetap lolos meski tidak satu pun timestamp cocok.
func TestReplayCompareDecidedAtIsRelative(t *testing.T) {
	p := rpProfile()
	res, err := Replay(context.Background(), rpMultiNodeRows(), p)
	if err != nil {
		t.Fatalf("Replay galat: %v", err)
	}
	hist := histFrom(p.Options, res.Frames)
	const shift = 987_654_321
	for i := range hist {
		hist[i].DecidedAt += shift
	}

	rep := Compare(hist, res, p)
	if !rep.DecisionsReproduced() {
		t.Fatalf("pergeseran timestamp seragam dilaporkan sebagai divergensi keputusan: %v",
			rep.Events[0].Diffs)
	}
	c := rep.Events[0]
	if !c.TimingWithinTolerance() {
		t.Errorf("timing di luar toleransi meski delta identik: %+v", c.Timings)
	}
	for _, tm := range c.Timings {
		if tm.DifferenceMs != 0 {
			t.Errorf("selisih delta rev%d = %d; mau 0", tm.Revision, tm.DifferenceMs)
		}
	}
}

// Delta yang benar-benar BERBEDA dilaporkan di luar toleransi, dan itu TIDAK
// membuat keputusan dianggap gagal: keduanya pertanyaan terpisah.
func TestReplayCompareTimingDivergenceIsSeparateFromDecisions(t *testing.T) {
	p := rpProfile()
	res, err := Replay(context.Background(), rpMultiNodeRows(), p)
	if err != nil {
		t.Fatalf("Replay galat: %v", err)
	}
	hist := histFrom(p.Options, res.Frames)
	// Hanya revisi terakhir yang digeser: deltanya kini berbeda jauh melebihi
	// toleransi SweepIntervalMs + 1000.
	hist[len(hist)-1].DecidedAt += 600_000

	rep := Compare(hist, res, p)
	if !rep.DecisionsReproduced() {
		t.Fatalf("keputusan dilaporkan gagal padahal hanya waktunya menyimpang: %v",
			rep.Events[0].Diffs)
	}
	if rep.Events[0].TimingWithinTolerance() {
		t.Error("selisih delta 600 s dilaporkan dalam toleransi")
	}
}

// Divergensi keputusan yang NYATA harus muncul sebagai diff dengan field yang
// disebut namanya. "Berbeda" tanpa mengatakan apa yang berbeda tidak dapat
// ditindaklanjuti.
func TestReplayCompareReportsDecisionDivergence(t *testing.T) {
	p := rpProfile()
	res, err := Replay(context.Background(), rpMultiNodeRows(), p)
	if err != nil {
		t.Fatalf("Replay galat: %v", err)
	}
	hist := histFrom(p.Options, res.Frames)
	hist[1].IndependentCells = 99
	hist[1].NodeCount = 42

	rep := Compare(hist, res, p)
	if !rep.Bijective() {
		t.Fatalf("bijeksi rusak padahal pengelompokan tidak diubah: %v/%v",
			rep.UnmatchedHistoric, rep.UnmatchedReplayed)
	}
	if rep.DecisionsReproduced() {
		t.Fatal("node_count 42 dan independent_cells 99 dilaporkan cocok")
	}
	fields := make(map[string]bool)
	for _, d := range rep.Events[0].Diffs {
		fields[d.Field] = true
	}
	for _, want := range []string{"node_count", "independent_cells"} {
		if !fields[want] {
			t.Errorf("tidak ada diff untuk %q; diff yang ada: %v", want, fields)
		}
	}
}

// Bukti historis yang diubah dilaporkan per FIELD bukti, bukan sebagai satu
// string JSON yang berbeda.
func TestReplayCompareReportsEvidenceFieldDivergence(t *testing.T) {
	p := rpProfile()
	res, err := Replay(context.Background(), rpMultiNodeRows(), p)
	if err != nil {
		t.Fatalf("Replay galat: %v", err)
	}
	hist := histFrom(p.Options, res.Frames)

	ev, err := historicEvidence(hist[0].EvidenceSummary)
	if err != nil {
		t.Fatalf("historicEvidence galat: %v", err)
	}
	ev.Contributors[0].PeakPGA = 1.5
	ev.OriginTSSource = OnsetSourcePublish
	hist[0].EvidenceSummary = ev.JSON()

	rep := Compare(hist, res, p)
	if rep.DecisionsReproduced() {
		t.Fatal("bukti yang diubah dilaporkan cocok")
	}
	fields := make(map[string]bool)
	for _, d := range rep.Events[0].Diffs {
		fields[d.Field] = true
	}
	for _, want := range []string{"evidence.contributors[0].peak_pga", "evidence.origin_ts_source"} {
		if !fields[want] {
			t.Errorf("tidak ada diff untuk %q; diff yang ada: %v", want, fields)
		}
	}
}

// evidence_summary yang tidak dapat diurai TIDAK menghentikan pemeriksaan: ia
// menjadi diff, dan event lain tetap diperiksa.
func TestReplayCompareUnparseableEvidenceIsReportedNotFatal(t *testing.T) {
	p := rpProfile()
	res, err := Replay(context.Background(), rpDualEventRows(), p)
	if err != nil {
		t.Fatalf("Replay galat: %v", err)
	}
	hist := histFrom(p.Options, res.Frames)
	broken := hist[0].EventID
	for i := range hist {
		if hist[i].EventID == broken {
			hist[i].EvidenceSummary = []byte(`{bukan json`)
		}
	}

	rep := Compare(hist, res, p)
	if rep.DecisionsReproduced() {
		t.Fatal("riwayat dengan evidence_summary rusak dilaporkan direproduksi")
	}
	found := false
	for _, e := range rep.Events {
		if e.HistoricEventID != broken {
			continue
		}
		for _, d := range e.Diffs {
			if d.Field == "evidence_summary" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("tidak ada diff evidence_summary untuk event %s; perbandingan: %+v", broken, rep.Events)
	}
	// Event kedua tetap ada di laporan, entah cocok atau tidak: satu baris rusak
	// tidak boleh menghapus pemeriksaan event lain.
	if len(rep.Events)+len(rep.UnmatchedHistoric) < 2 {
		t.Errorf("event kedua hilang dari laporan: events=%d unmatched=%v",
			len(rep.Events), rep.UnmatchedHistoric)
	}
}

// Pelanggaran bijeksi: sebuah event historis tanpa pasangan replay, dan sebuah
// event replay tanpa pasangan historis.
func TestReplayCompareReportsUnmatchedBothSides(t *testing.T) {
	p := rpProfile()
	res, err := Replay(context.Background(), rpDualEventRows(), p)
	if err != nil {
		t.Fatalf("Replay galat: %v", err)
	}
	full := histFrom(p.Options, res.Frames)

	// Buang seluruh baris satu event dari riwayat: sisi replay kini punya event
	// yang tidak ada pasangannya.
	drop := full[0].EventID
	var partial []store.EventStateLog
	for _, l := range full {
		if l.EventID != drop {
			partial = append(partial, l)
		}
	}
	rep := Compare(partial, res, p)
	if len(rep.UnmatchedReplayed) != 1 {
		t.Errorf("UnmatchedReplayed = %v; mau tepat 1", rep.UnmatchedReplayed)
	}
	if rep.Bijective() {
		t.Error("bijeksi dilaporkan terpenuhi padahal satu event historis dibuang")
	}

	// Sebaliknya: riwayat memuat event yang pengelompokannya tidak pernah muncul
	// di replay.
	ghost := append([]store.EventStateLog{}, full...)
	ev, err := historicEvidence(full[0].EvidenceSummary)
	if err != nil {
		t.Fatalf("historicEvidence galat: %v", err)
	}
	ev.Contributors[0].NodeID = "NODE-TIDAK-PERNAH-ADA"
	extra := full[0]
	extra.EventID = "deadbeef-0000-4000-8000-000000000000"
	extra.Revision = 1
	extra.EvidenceSummary = ev.JSON()
	ghost = append(ghost, extra)

	rep2 := Compare(ghost, res, p)
	if len(rep2.UnmatchedHistoric) != 1 || rep2.UnmatchedHistoric[0] != extra.EventID {
		t.Errorf("UnmatchedHistoric = %v; mau [%s]", rep2.UnmatchedHistoric, extra.EventID)
	}
}

// Tanda tangan ambigu: dua event historis dengan pengelompokan observasi yang
// identik. Bijeksi tidak dapat DIPERIKSA, jadi laporan harus menyebutnya — dan
// tidak boleh memasangkan salah satu secara sembarang.
func TestReplayCompareAmbiguousSignatureIsReportedNotMatched(t *testing.T) {
	p := rpProfile()
	res, err := Replay(context.Background(), rpMultiNodeRows(), p)
	if err != nil {
		t.Fatalf("Replay galat: %v", err)
	}
	hist := histFrom(p.Options, res.Frames)

	// Salinan seluruh riwayat di bawah event_id lain: tanda tangannya sama persis.
	twin := make([]store.EventStateLog, 0, len(hist)*2)
	twin = append(twin, hist...)
	for _, l := range hist {
		c := l
		c.EventID = "abcdabcd-0000-4000-8000-000000000000"
		twin = append(twin, c)
	}

	rep := Compare(twin, res, p)
	if len(rep.AmbiguousSignatures) != 1 {
		t.Fatalf("AmbiguousSignatures = %v; mau tepat 1", rep.AmbiguousSignatures)
	}
	if rep.Bijective() {
		t.Error("bijeksi dilaporkan terpenuhi atas tanda tangan ambigu")
	}
	if rep.DecisionsReproduced() {
		t.Error("keputusan dilaporkan direproduksi padahal pemeriksaan tidak dapat dilakukan")
	}
	if len(rep.Events) != 0 {
		t.Errorf("perbandingan event = %d; mau 0 — yang ambigu tidak boleh dipasangkan",
			len(rep.Events))
	}
	if len(rep.UnmatchedHistoric) != 2 {
		t.Errorf("UnmatchedHistoric = %v; mau kedua event ambigu", rep.UnmatchedHistoric)
	}
	if len(rep.UnmatchedReplayed) != 1 {
		t.Errorf("UnmatchedReplayed = %v; mau 1 — sisi replay pun tak terperiksa",
			rep.UnmatchedReplayed)
	}
}

// Baris v1 tidak punya obs_seq, jadi tanda tangannya jatuh ke node_id saja. Dua
// episode dari node yang SAMA karenanya menjadi ambigu, dan itu dilaporkan —
// bukan disembunyikan.
func TestReplayV1SignatureAmbiguityIsReported(t *testing.T) {
	ev := EvidenceSummary{Contributors: []ContributorEvidence{{NodeID: "NODE-V1"}}}
	if got := obsGroupSignature(ev); got != "NODE-V1" {
		t.Fatalf("tanda tangan v1 = %q; mau NODE-V1 saja", got)
	}
	seq := int64(7)
	ev2 := EvidenceSummary{Contributors: []ContributorEvidence{{NodeID: "NODE-V1", ObsSeq: &seq}}}
	if got := obsGroupSignature(ev2); got != "NODE-V1#7" {
		t.Fatalf("tanda tangan v2 = %q; mau NODE-V1#7", got)
	}
	// Urutan kontributor tidak boleh mengubah tanda tangan.
	a := EvidenceSummary{Contributors: []ContributorEvidence{{NodeID: "B"}, {NodeID: "A"}}}
	b := EvidenceSummary{Contributors: []ContributorEvidence{{NodeID: "A"}, {NodeID: "B"}}}
	if obsGroupSignature(a) != obsGroupSignature(b) {
		t.Errorf("tanda tangan bergantung urutan: %q vs %q", obsGroupSignature(a), obsGroupSignature(b))
	}
}

// ---- 5. pengelompokan algo_ver dan profil baku ------------------------------

// V5: analisis dikelompokkan menurut algo_ver, dan replay adalah analisis.
func TestReplayGroupByAlgoVer(t *testing.T) {
	hist := []store.EventStateLog{
		{EventID: "a", Revision: 1, AlgoVer: "phase3-1.1/ic=5"},
		{EventID: "b", Revision: 1, AlgoVer: "phase3-1.0/ic=5"},
		{EventID: "a", Revision: 2, AlgoVer: "phase3-1.1/ic=5"},
	}
	groups, vers := GroupByAlgoVer(hist)
	if len(vers) != 2 || vers[0] != "phase3-1.0/ic=5" || vers[1] != "phase3-1.1/ic=5" {
		t.Fatalf("versi = %v; mau terurut [phase3-1.0/ic=5 phase3-1.1/ic=5]", vers)
	}
	if len(groups["phase3-1.1/ic=5"]) != 2 || len(groups["phase3-1.0/ic=5"]) != 1 {
		t.Fatalf("ukuran kelompok = %d/%d; mau 2/1",
			len(groups["phase3-1.1/ic=5"]), len(groups["phase3-1.0/ic=5"]))
	}
}

// DefaultReplayProfile harus lolos gerbangnya sendiri untuk label default, dan
// nilainya harus sama dengan default config yang dipakai uji lain.
func TestReplayDefaultProfileMatchesDefaults(t *testing.T) {
	p := DefaultReplayProfile()
	if p.Options != defaultOptions() {
		t.Errorf("DefaultReplayProfile().Options = %+v; mau %+v", p.Options, defaultOptions())
	}
	if err := CheckAlgoVer(AlgoVerBase()+"/ic=5", p); err != nil {
		t.Errorf("profil baku ditolak gerbangnya sendiri: %v", err)
	}
	if p.tolerance() != p.Options.SweepIntervalMs+1000 {
		t.Errorf("toleransi = %d; mau SweepIntervalMs+1000", p.tolerance())
	}
}

// Diff harus dapat dibaca: laporan operator memuatnya apa adanya.
func TestReplayDiffString(t *testing.T) {
	d := Diff{EventID: "e1", Revision: 2, Field: "node_count", Want: "3", Got: "2"}
	s := d.String()
	for _, want := range []string{"e1", "node_count", "3", "2"} {
		if !strings.Contains(s, want) {
			t.Errorf("Diff.String() = %q; harus memuat %q", s, want)
		}
	}
}
