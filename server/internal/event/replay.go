// Pemutaran ulang deterministik (P4-M4′, V7).
//
// V7 berbunyi: "Replay must be reproducible: the same observations under the
// same algo_ver yield the same decisions." Berkas ini adalah cara pernyataan itu
// DIPERIKSA, bukan cara ia diasumsikan.
//
// Tiga sifat yang menentukan bentuk seluruh berkas ini:
//
//  1. READ-ONLY. Replay membaca sensor_observations dan event_state_log, lalu
//     MEMBANDINGKAN. Ia tidak menulis satu pun baris, tidak memperbaiki baris
//     lampau (V4), tidak menyentuh Tracker produksi, dan tidak memanggil
//     SetLedger. Tracker yang dipakai di sini adalah instans BARU yang hidup di
//     dalam satu pemanggilan fungsi.
//
//  2. Parameter keputusan DIASSERSI OPERATOR, BUKAN DIPULIHKAN DARI BARIS
//     HISTORIS. Lihat ReplayProfile. Hanya IndependenceCellKm yang benar-benar
//     terbawa oleh baris (lewat label algo_ver), dan justru karena itu ia
//     satu-satunya yang DIPERIKSA-SILANG di sini alih-alih dipercaya.
//
//  3. Perbandingan PER EVENT, tidak pernah sebagai satu aliran global.
//     sweepLocked() mengiterasi map, sehingga urutan RESOLVED antar-event di
//     dalam satu tik tidak terdefinisi. Satu aliran global karenanya akan
//     melaporkan divergensi yang tidak ada.
package event

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// ErrAlgoVerIncompatible dikembalikan bila basis algoritma sebuah baris
// historis bukan basis yang dipegang biner ini. Menolak, bukan memperingatkan:
// memutar ulang baris phase3-1.0 di bawah aturan 1.1 akan menghasilkan
// "divergensi" yang sebenarnya adalah dua algoritma berbeda yang bekerja dengan
// benar, dan laporan seperti itu lebih buruk daripada tidak ada laporan.
var ErrAlgoVerIncompatible = errors.New("algo_ver base tidak kompatibel dengan biner ini")

// ErrNoAlgoVer dikembalikan bila label algo_ver kosong atau tidak berbentuk
// base/ic=<km>.
var ErrNoAlgoVer = errors.New("algo_ver kosong atau tak dapat diurai")

// ReplayProfile adalah parameter yang dipakai pemutaran ulang.
//
// SELURUH ISI Options DI SINI DIASSERSI OLEH OPERATOR. Ia TIDAK dipulihkan dari
// baris historis, dan tidak bisa: hanya IndependenceCellKm yang muncul di label
// algo_ver (§V6), sementara CorrelationWindowMs, AttachRadiusKm,
// MinIndependentCells, MaxEventDiameterKm, ResolveAfterMs, dan SweepIntervalMs
// tidak terekam di baris mana pun. Tidak ada tabel riwayat konfigurasi, dan
// membuatnya bukan bagian dari milestone ini.
//
// Konsekuensinya harus dinyatakan di setiap laporan: sebuah replay yang cocok
// membuktikan "observasi ini, di bawah parameter INI, menghasilkan keputusan
// itu" — bukan "parameter ini yang berlaku saat itu". Operator yang salah
// mengassersi parameter akan melihat divergensi yang nyata dan salah tafsir.
//
// MinPGAGal dan MinNodesConfirmed tidak ada di sini karena keduanya konstanta
// compile-time (event.go): keduanya diassersi oleh BINER, dan algoVerBase yang
// menjadi labelnya.
type ReplayProfile struct {
	Options Options

	// DecidedAtToleranceMs adalah toleransi untuk pembandingan waktu RELATIF
	// (F3). Bukan untuk kesetaraan timestamp: decided_at historis adalah jam
	// server saat keputusan diambil, sementara replay memakai jam palsu yang
	// digerakkan received_ts, dan transisi yang digerakkan sweep terkuantisasi
	// ke SweepIntervalMs. Nol berarti SweepIntervalMs + 1000.
	DecidedAtToleranceMs int64
}

// tolerance mengembalikan toleransi efektif.
func (p ReplayProfile) tolerance() int64 {
	if p.DecidedAtToleranceMs > 0 {
		return p.DecidedAtToleranceMs
	}
	return p.Options.SweepIntervalMs + 1000
}

// ParseAlgoVer memecah label algo_ver menjadi basis algoritma dan
// IndependenceCellKm yang ikut di dalamnya.
//
// Formatnya TIDAK diubah oleh milestone ini: base + "/ic=" + float, sama seperti
// Tracker.algoVer().
func ParseAlgoVer(s string) (base string, cellKm float64, err error) {
	i := strings.Index(s, "/ic=")
	if s == "" || i < 0 {
		return "", 0, fmt.Errorf("%w: %q", ErrNoAlgoVer, s)
	}
	base = s[:i]
	cellKm, err = strconv.ParseFloat(s[i+len("/ic="):], 64)
	if err != nil {
		return "", 0, fmt.Errorf("%w: %q", ErrNoAlgoVer, s)
	}
	return base, cellKm, nil
}

// AlgoVerBase mengekspos basis algoritma biner ini supaya alat operator dapat
// melaporkannya tanpa menebak.
func AlgoVerBase() string { return algoVerBase }

// CheckAlgoVer adalah GERBANG pemutaran ulang: ia menolak label yang basisnya
// bukan basis biner ini, dan menolak profil yang IndependenceCellKm-nya
// bertentangan dengan label.
//
// Pemeriksaan kedua itu bukan pengulangan pemeriksaan pertama. IndependenceCellKm
// adalah satu-satunya parameter keputusan yang benar-benar TERBAWA oleh baris
// historis; membiarkan profil operator menimpanya diam-diam akan membuang
// satu-satunya bukti konfigurasi yang dimiliki ledger.
func CheckAlgoVer(algoVer string, p ReplayProfile) error {
	base, cellKm, err := ParseAlgoVer(algoVer)
	if err != nil {
		return err
	}
	if base != algoVerBase {
		return fmt.Errorf("%w: baris %q, biner %q", ErrAlgoVerIncompatible, base, algoVerBase)
	}
	if p.Options.IndependenceCellKm != cellKm {
		return fmt.Errorf(
			"profil operator INDEPENDENCE_CELL_KM=%g bertentangan dengan algo_ver baris (ic=%g); "+
				"parameter itu terekam di label dan tidak boleh diassersi berbeda",
			p.Options.IndependenceCellKm, cellKm)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Bagian mesin: jam, sumber lokasi, dan perekam. Ketiganya sengaja cermin dari
// komponen harness uji (tracker_harness_test.go) dan bukan komponen kedua yang
// berbeda perilakunya — bila keduanya menyimpang, yang diuji bukan lagi yang
// diputar ulang.
// ---------------------------------------------------------------------------

// replayClock adalah jam yang DIGERAKKAN DATA: waktunya selalu received_ts
// baris yang sedang diproses, atau batas tik sweep di antaranya. Tidak ada
// time.Now() di seluruh jalur replay.
type replayClock struct{ ms int64 }

func (c *replayClock) now() time.Time { return time.UnixMilli(c.ms) }
func (c *replayClock) set(ms int64)   { c.ms = ms }

// replayLocator mengembalikan SNAPSHOT node_location baris yang sedang diproses,
// bukan koordinat node sekarang.
//
// Inilah alasan ia bukan map yang diisi sekali: sebuah node yang dipindahkan
// setelah kejadian punya dua koordinat berbeda di dua baris ledger, dan
// keputusan historis dibuat dengan yang tercatat pada baris itu. Membaca
// iot_nodes hari ini akan memutar ulang gempa lampau dengan geometri hari ini.
type replayLocator struct {
	cur map[string]store.NodeLocation
}

func (l *replayLocator) GetNodeLocation(_ context.Context, id string) (*store.NodeLocation, error) {
	nl, ok := l.cur[id]
	if !ok {
		return nil, store.ErrNodeNotFound
	}
	return &nl, nil
}

func (l *replayLocator) set(id string, lat, lon float64) {
	if l.cur == nil {
		l.cur = make(map[string]store.NodeLocation, 8)
	}
	// LocationName sengaja node_id: nama lokasi TIDAK ada di sensor_observations,
	// jadi ia tidak dapat direproduksi — dan karena itu ia juga tidak
	// dibandingkan (lihat daftar field yang dikecualikan di compareEvent).
	l.cur[id] = store.NodeLocation{StationID: id, Lat: lat, Lon: lon, LocationName: id}
}

// replayRecorder mengumpulkan setiap transisi yang dihasilkan Tracker replay.
// Tidak ada persister sama sekali di jalur ini: SetLedger tidak pernah dipanggil,
// jadi replay secara struktural tidak dapat menulis.
type replayRecorder struct{ frames []Snapshot }

func (r *replayRecorder) EmitTransition(_ context.Context, s Snapshot) {
	r.frames = append(r.frames, s)
}

// ---------------------------------------------------------------------------
// Rekonstruksi masukan
// ---------------------------------------------------------------------------

// replayInputFrom membangun Input dari satu baris ledger.
//
// TIDAK memanggil ObservationFrom: mapper itu bekerja dari ingest.Trigger dan
// MENGHITUNG publish_ts - dur_ms untuk jalur v1. Menghitung ulang di sini akan
// menjadi replay atas rumusnya, bukan atas datanya — bila rumus itu pernah
// berubah, baris lampau tetap memegang batas yang benar-benar dipakai. Jadi
// jangkar diambil dari KOLOM: onset_ts bila onset_ts_source sudah SENSOR,
// onset_ts_upper_bound bila tidak.
func replayInputFrom(o store.ReplayObservation) (Input, error) {
	in := Input{
		NodeID:    o.NodeID,
		PGA:       o.PGAGal,
		DurMs:     o.DurMs,
		PublishTS: o.PublishTS,
		Phase:     o.Phase,
	}
	if o.Phase == "" {
		in.Phase = PhaseFinal
	}

	switch {
	case o.OnsetTSSource == OnsetSourceSensor && o.OnsetTS != nil:
		in.OnsetTS = *o.OnsetTS
		in.OnsetSource = OnsetSourceSensor
		in.ObsSeq = o.ObsSeq
		in.DetriggerTS = o.DetriggerTS
		if o.AttemptNo != nil {
			n := int(*o.AttemptNo)
			in.AttemptNo = &n
		}
	case o.OnsetTSUpperBound != nil:
		in.OnsetTS = *o.OnsetTSUpperBound
		in.OnsetSource = OnsetSourcePublish
	default:
		// Tanpa jangkar onset tidak ada korelasi yang dapat dilakukan. Baris ini
		// TIDAK dibuang diam-diam; pemanggil menghitung dan melaporkannya.
		return Input{}, fmt.Errorf("observation_id=%d: tak ada jangkar onset (onset_ts dan onset_ts_upper_bound keduanya NULL)", o.ObservationID)
	}
	return in, nil
}

// ---------------------------------------------------------------------------
// Laporan masukan
// ---------------------------------------------------------------------------

// SkippedRow adalah satu baris ledger yang TIDAK diumpankan ke Tracker, beserta
// alasannya.
//
// Ada sebagai daftar, bukan sebagai penghitung saja, karena "replay cocok"
// hanya bermakna bila diketahui APA yang diputar. Konsensus produksi juga
// membuang baris-baris ini, jadi membuangnya di sini benar — yang tidak boleh
// adalah membuangnya TANPA JEJAK.
type SkippedRow struct {
	ObservationID int64
	NodeID        string
	Reason        string
}

// Alasan pada SkippedRow. Kosakata tertutup supaya laporan dapat diagregasi.
const (
	SkipVerifyNotOK   = "VERIFY_RESULT_NOT_OK"
	SkipNoLocation    = "NODE_LOCATION_NULL"
	SkipNoOnsetAnchor = "NO_ONSET_ANCHOR"
)

// InputReport merangkum apa yang benar-benar masuk ke Tracker replay.
//
// Incomplete BUKAN dihitung dari baris: antrean ledger produksi boleh MEMBUANG
// yang tertua (D17/D30) dan ledger_drops_total hanya masuk log, jadi sebuah
// jendela dapat kehilangan observasi tanpa satu pun jejak di dalam tabel.
// Field ini karena itu diisi oleh operator bila ia punya angka drop dari log,
// dan nol di sini berarti "tidak diketahui", bukan "tidak ada yang hilang".
type InputReport struct {
	TotalRows int
	FedRows   int
	Skipped   []SkippedRow

	// LedgerDropsKnown diisi operator dari log bila tersedia. Lihat di atas.
	LedgerDropsKnown int
}

// SkipCounts mengelompokkan baris terbuang menurut alasan.
func (r InputReport) SkipCounts() map[string]int {
	out := make(map[string]int, 3)
	for _, s := range r.Skipped {
		out[s.Reason]++
	}
	return out
}

// ---------------------------------------------------------------------------
// Mesin replay
// ---------------------------------------------------------------------------

// ReplayResult adalah hasil satu pemutaran ulang: seluruh transisi yang
// dihasilkan Tracker baru, dikelompokkan per event.
type ReplayResult struct {
	Input   InputReport
	Profile ReplayProfile

	// AlgoVer adalah label yang akan ditulis Tracker replay. Dihitung dari
	// profil operator, jadi ia label yang DIASSERSI — bukan label yang dibaca
	// dari baris.
	AlgoVer string

	// Frames adalah seluruh transisi dalam urutan produksinya. Dipakai untuk
	// diagnosa, TIDAK untuk perbandingan: urutan antar-event di dalam satu tik
	// sweep tidak terdefinisi (map iteration di sweepLocked).
	Frames []Snapshot

	// Events adalah transisi yang dikelompokkan per event_id replay, terurut
	// revision. Ini bentuk yang dibandingkan.
	Events map[string][]Snapshot
}

// Replay memutar ulang observasi melalui Tracker BARU dan mengembalikan
// keputusan yang dihasilkannya.
//
// obs HARUS sudah dalam urutan kanonik received_ts lalu observation_id —
// ListObservationsForReplay mengembalikannya begitu. Urutan itu DIDEKLARASIKAN,
// bukan direkonstruksi: handler MQTT produksi berjalan dengan
// SetOrderMatters(false), jadi urutan pemrosesan yang sebenarnya tidak tersimpan
// di mana pun.
//
// Jam palsu digerakkan sepenuhnya oleh received_ts. Tik sweep disisipkan di
// antara baris pada kelipatan SweepIntervalMs dihitung DARI received_ts baris
// pertama; fase tik produksi yang sebenarnya (yang bergantung pada kapan proses
// start) tidak terekam dan tidak dapat dipulihkan. Ini asumsi yang wajib
// dinyatakan di laporan.
//
// Tracker di dalam fungsi ini tidak punya persister sama sekali dan tidak pernah
// direkonsiliasi. Ia tidak dapat menulis, dan ia tidak mewarisi satu pun state
// dari produksi.
func Replay(ctx context.Context, obs []store.ReplayObservation, p ReplayProfile) (*ReplayResult, error) {
	if len(obs) == 0 {
		return nil, errors.New("replay: tidak ada observasi dalam jendela")
	}

	res := &ReplayResult{
		Input:   InputReport{TotalRows: len(obs)},
		Profile: p,
		Events:  make(map[string][]Snapshot, 4),
	}

	clock := &replayClock{ms: obs[0].ReceivedTS}
	loc := &replayLocator{}
	rec := &replayRecorder{}

	trk := NewTracker(loc, p.Options, slog.New(slog.NewTextHandler(io.Discard, nil)))
	trk.now = clock.now
	// Id yang monoton dan dapat diulang. Sengaja BUKAN UUID acak: pemutus-seri
	// leksikografis §4.3 (bestMatch, oldestBy) membaca event_id, jadi id acak
	// akan membuat replay tidak dapat diulang. Konsekuensinya juga harus
	// dinyatakan: bila sebuah keputusan historis PERNAH ditentukan oleh
	// pemutus-seri itu, replay tidak dapat mereproduksinya — dan itu tepatnya
	// mengapa F2 meminta bijeksi, bukan kesetaraan UUID.
	seq := 0
	trk.newID = func() string {
		seq++
		return fmt.Sprintf("%08x-0000-4000-8000-000000000000", seq)
	}
	trk.SetEmitter(rec)
	res.AlgoVer = trk.algoVer()

	interval := p.Options.SweepIntervalMs
	if interval <= 0 {
		interval = 5000
	}
	nextSweep := obs[0].ReceivedTS + interval

	for _, o := range obs {
		if o.VerifyResult != "OK" {
			res.Input.Skipped = append(res.Input.Skipped,
				SkippedRow{o.ObservationID, o.NodeID, SkipVerifyNotOK})
			continue
		}
		if o.Lat == nil || o.Lon == nil {
			res.Input.Skipped = append(res.Input.Skipped,
				SkippedRow{o.ObservationID, o.NodeID, SkipNoLocation})
			continue
		}
		in, err := replayInputFrom(o)
		if err != nil {
			res.Input.Skipped = append(res.Input.Skipped,
				SkippedRow{o.ObservationID, o.NodeID, SkipNoOnsetAnchor})
			continue
		}

		// Tik sweep yang jatuh SEBELUM baris ini, satu per satu: sebuah event
		// dapat RESOLVED di antara dua observasi, dan menggabungkan beberapa tik
		// menjadi satu akan melewatkan transisi yang produksi hasilkan.
		for nextSweep <= o.ReceivedTS {
			clock.set(nextSweep)
			trk.sweep(ctx)
			nextSweep += interval
		}

		clock.set(o.ReceivedTS)
		loc.set(o.NodeID, *o.Lat, *o.Lon)
		trk.Ingest(ctx, in)
		res.Input.FedRows++
	}

	// Setelah baris terakhir, majukan sweep sampai tidak ada lagi event terbuka.
	// Batasnya ResolveAfterMs + satu interval: itu waktu TERLAMA yang dibutuhkan
	// sebuah event untuk kedaluwarsa setelah bukti terakhirnya.
	deadline := obs[len(obs)-1].ReceivedTS + p.Options.ResolveAfterMs + 2*interval
	for nextSweep <= deadline {
		clock.set(nextSweep)
		trk.sweep(ctx)
		nextSweep += interval
	}

	res.Frames = rec.frames
	for _, f := range rec.frames {
		res.Events[f.EventID] = append(res.Events[f.EventID], f)
	}
	for id := range res.Events {
		fs := res.Events[id]
		sort.Slice(fs, func(i, j int) bool { return fs[i].Revision < fs[j].Revision })
	}
	return res, nil
}

// ---------------------------------------------------------------------------
// Perbandingan
// ---------------------------------------------------------------------------

// Diff adalah satu ketidakcocokan yang ditemukan pembandingan.
type Diff struct {
	EventID  string // event_id HISTORIS
	Revision int    // -1 bila diff-nya tentang event, bukan tentang satu revisi
	Field    string
	Want     string // nilai historis
	Got      string // nilai replay
}

func (d Diff) String() string {
	if d.Revision < 0 {
		return fmt.Sprintf("event %s: %s: historis=%s replay=%s", d.EventID, d.Field, d.Want, d.Got)
	}
	return fmt.Sprintf("event %s rev %d: %s: historis=%s replay=%s",
		d.EventID, d.Revision, d.Field, d.Want, d.Got)
}

// TimingDelta adalah perbandingan decided_at sebagai waktu RELATIF (F3).
//
// decided_at TIDAK dibandingkan sebagai timestamp. Yang historis adalah jam
// server sungguhan; yang replay adalah jam palsu yang digerakkan received_ts,
// dan transisi yang digerakkan sweep terkuantisasi ke fase tik yang tidak
// terekam. Yang dapat dibandingkan adalah SELISIH terhadap transisi pertama
// event yang sama.
type TimingDelta struct {
	EventID         string
	Revision        int
	HistoricMs      int64 // decided_at - decided_at revisi pertama, historis
	ReplayedMs      int64 // idem, replay
	DifferenceMs    int64
	WithinTolerance bool
}

// EventComparison adalah hasil pembandingan satu event historis dengan
// pasangannya di replay.
type EventComparison struct {
	HistoricEventID string
	ReplayEventID   string

	// Signature adalah pengelompokan observasi yang dipakai untuk memasangkan
	// keduanya (F2). Kesamaannya ADALAH bagian "event_id direproduksi" —
	// kesetaraan UUID tidak, dan tidak dapat: id dibuat oleh newID pada saat
	// event lahir.
	Signature string

	Diffs   []Diff
	Timings []TimingDelta
}

// Matched benar bila tidak ada satu pun diff pada event ini.
func (c EventComparison) Matched() bool { return len(c.Diffs) == 0 }

// TimingWithinTolerance benar bila setiap delta waktu masih di dalam toleransi.
// Dipisahkan dari Matched karena waktu BUKAN keputusan: sebuah event yang
// keputusannya identik tetapi waktunya bergeser tetap reproduksi keputusan yang
// sama, dan menggabungkan keduanya akan menyembunyikan yang mana yang bergerak.
func (c EventComparison) TimingWithinTolerance() bool {
	for _, t := range c.Timings {
		if !t.WithinTolerance {
			return false
		}
	}
	return true
}

// ComparisonReport adalah hasil lengkap satu pemutaran ulang yang dibandingkan
// dengan riwayat.
type ComparisonReport struct {
	Input   InputReport
	Profile ReplayProfile
	AlgoVer string

	Events []EventComparison

	// UnmatchedHistoric dan UnmatchedReplayed adalah pelanggaran BIJEKSI: sebuah
	// event historis tanpa pasangan replay, atau sebaliknya. Keduanya kegagalan
	// V7, dan keduanya dilaporkan terpisah dari diff field supaya penyebabnya
	// tidak tertukar.
	UnmatchedHistoric []string
	UnmatchedReplayed []string

	// AmbiguousSignatures adalah pengelompokan observasi yang muncul lebih dari
	// sekali di salah satu sisi. Bijeksi tidak dapat ditegakkan atas dasar itu,
	// jadi ia dilaporkan sebagai KETIDAKMAMPUAN MEMERIKSA, bukan sebagai cocok.
	AmbiguousSignatures []string
}

// Bijective benar bila setiap event historis punya tepat satu pasangan replay
// dan sebaliknya, tanpa tanda tangan ambigu.
func (r ComparisonReport) Bijective() bool {
	return len(r.UnmatchedHistoric) == 0 &&
		len(r.UnmatchedReplayed) == 0 &&
		len(r.AmbiguousSignatures) == 0
}

// DecisionsReproduced benar bila bijeksi terpenuhi DAN setiap event cocok pada
// seluruh field keputusan. Waktu tidak termasuk — lihat TimingWithinTolerance.
func (r ComparisonReport) DecisionsReproduced() bool {
	if !r.Bijective() {
		return false
	}
	for _, e := range r.Events {
		if !e.Matched() {
			return false
		}
	}
	return true
}

// obsGroupSignature membangun tanda tangan PENGELOMPOKAN OBSERVASI sebuah event:
// daftar terurut kontributor, masing-masing dengan obs_seq-nya bila ada.
//
// Inilah definisi operasional "event_id direproduksi" (F2). Kesetaraan UUID
// tidak dipakai dan tidak dapat dipakai: id dibuat newID() saat event lahir,
// jadi ia properti dari proses yang menjalankan, bukan dari data. Yang dapat
// direproduksi — dan yang sebenarnya ditanyakan V7 — adalah apakah observasi
// yang sama DIKELOMPOKKAN menjadi event yang sama.
//
// obs_seq masuk ke dalam tanda tangan karena tanpanya dua episode dari satu node
// yang sama (isSecondEpisodeLocked) memiliki tanda tangan identik dan bijeksi
// tidak dapat ditegakkan. Baris v1 tidak punya obs_seq; di sana tanda tangan
// jatuh kembali ke node_id saja, dan ambiguitas yang muncul DILAPORKAN.
func obsGroupSignature(ev EvidenceSummary) string {
	parts := make([]string, 0, len(ev.Contributors))
	for _, c := range ev.Contributors {
		if c.ObsSeq != nil {
			parts = append(parts, c.NodeID+"#"+strconv.FormatInt(*c.ObsSeq, 10))
			continue
		}
		parts = append(parts, c.NodeID)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// round4 membulatkan ke 4 desimal, presisi kolom NUMERIC(8,4).
//
// Perbandingan pada presisi kolom, bukan presisi float64: pga_gal dibulatkan oleh
// Postgres saat ditulis, jadi membandingkan bit float64 penuh akan melaporkan
// divergensi yang seluruhnya adalah pembulatan kolom.
func round4(f float64) float64 { return math.Round(f*10000) / 10000 }

// historicEvidence mengurai kolom evidence_summary. Kegagalan urai dilaporkan
// sebagai diff, bukan sebagai galat fatal: satu baris rusak tidak boleh
// menghentikan pemeriksaan event lain.
func historicEvidence(raw []byte) (EvidenceSummary, error) {
	var ev EvidenceSummary
	if len(raw) == 0 {
		return ev, errors.New("evidence_summary kosong")
	}
	if err := json.Unmarshal(raw, &ev); err != nil {
		return ev, fmt.Errorf("urai evidence_summary: %w", err)
	}
	return ev, nil
}

// compareEvidence membandingkan potret bukti field per field.
//
// Field per field, bukan sebagai satu string JSON, karena laporan "evidence
// berbeda" tanpa mengatakan APA yang berbeda tidak dapat ditindaklanjuti.
func compareEvidence(eventID string, rev int, want, got EvidenceSummary) []Diff {
	var out []Diff
	add := func(field, w, g string) {
		if w != g {
			out = append(out, Diff{EventID: eventID, Revision: rev, Field: field, Want: w, Got: g})
		}
	}

	add("evidence.independent_cells", strconv.Itoa(want.IndependentCells), strconv.Itoa(got.IndependentCells))
	add("evidence.origin_ts_source", want.OriginTSSource, got.OriginTSSource)
	add("evidence.mixed_provenance", strconv.FormatBool(want.MixedProvenance), strconv.FormatBool(got.MixedProvenance))
	add("evidence.cell_ids", cellsString(want.CellIDs), cellsString(got.CellIDs))

	if len(want.Contributors) != len(got.Contributors) {
		add("evidence.contributors.len",
			strconv.Itoa(len(want.Contributors)), strconv.Itoa(len(got.Contributors)))
		return out
	}
	// Keduanya sudah terurut node_id oleh evidence(), jadi perbandingan
	// posisional sah dan tidak menyembunyikan urutan yang salah.
	for i := range want.Contributors {
		w, g := want.Contributors[i], got.Contributors[i]
		pfx := fmt.Sprintf("evidence.contributors[%d].", i)
		add(pfx+"node_id", w.NodeID, g.NodeID)
		add(pfx+"peak_pga", formatF(round4(w.PeakPGA)), formatF(round4(g.PeakPGA)))
		add(pfx+"phase", w.Phase, g.Phase)
		add(pfx+"onset_ts", strconv.FormatInt(w.OnsetTS, 10), strconv.FormatInt(g.OnsetTS, 10))
		add(pfx+"onset_source", w.OnsetSource, g.OnsetSource)
		add(pfx+"obs_seq", formatSeq(w.ObsSeq), formatSeq(g.ObsSeq))
		add(pfx+"cell", fmt.Sprintf("(%d,%d)", w.Cell.X, w.Cell.Y), fmt.Sprintf("(%d,%d)", g.Cell.X, g.Cell.Y))
	}
	return out
}

func cellsString(cs []CellID) string {
	parts := make([]string, 0, len(cs))
	for _, c := range cs {
		parts = append(parts, fmt.Sprintf("(%d,%d)", c.X, c.Y))
	}
	return strings.Join(parts, ",")
}

func formatF(f float64) string { return strconv.FormatFloat(f, 'f', 4, 64) }

func formatSeq(p *int64) string {
	if p == nil {
		return "null"
	}
	return strconv.FormatInt(*p, 10)
}

// Compare membandingkan riwayat historis dengan hasil replay, PER EVENT.
//
// hist harus berisi seluruh baris event_state_log untuk jendela yang sama —
// ListStateLogForReplay mengembalikannya terurut (event_id, revision).
//
// Pemasangan event memakai tanda tangan pengelompokan observasi pada revisi
// TERAKHIR masing-masing event (F2). Revisi terakhir, bukan pertama: bukti
// bertambah sepanjang hidup event, jadi hanya revisi terakhir yang memuat
// pengelompokan lengkap.
//
// Yang DIBANDINGKAN per revisi: revision, from_state, to_state, reason,
// node_count, independent_cells, peak_pga (4 desimal), dan seluruh
// evidence_summary.
//
// Yang SENGAJA TIDAK dibandingkan, dan alasannya:
//   - event_id sebagai UUID — dibuat newID(), properti proses bukan data (F2).
//   - decided_at sebagai timestamp — dibandingkan sebagai DELTA (F3), lihat
//     Timings.
//   - location_name — tidak ada di sensor_observations, jadi tidak dapat
//     direkonstruksi; ia berasal dari iot_nodes hari ini.
//   - started_at dan status — keduanya turunan (statusFor), bukan keputusan
//     independen, dan started_at adalah jam server saat baris lahir.
func Compare(hist []store.EventStateLog, res *ReplayResult, p ReplayProfile) *ComparisonReport {
	rep := &ComparisonReport{
		Input:   res.Input,
		Profile: p,
		AlgoVer: res.AlgoVer,
	}

	// Kelompokkan riwayat per event, urut revisi.
	byEvent := make(map[string][]store.EventStateLog, 4)
	order := make([]string, 0, 4)
	for _, l := range hist {
		if _, seen := byEvent[l.EventID]; !seen {
			order = append(order, l.EventID)
		}
		byEvent[l.EventID] = append(byEvent[l.EventID], l)
	}
	for id := range byEvent {
		ls := byEvent[id]
		sort.Slice(ls, func(i, j int) bool { return ls[i].Revision < ls[j].Revision })
	}

	// Tanda tangan kedua sisi, dari revisi TERAKHIR.
	histSig := make(map[string]string, len(byEvent))
	sigToHist := make(map[string][]string, len(byEvent))
	for _, id := range order {
		ls := byEvent[id]
		ev, err := historicEvidence(ls[len(ls)-1].EvidenceSummary)
		if err != nil {
			rep.Events = append(rep.Events, EventComparison{
				HistoricEventID: id,
				Diffs: []Diff{{EventID: id, Revision: -1, Field: "evidence_summary",
					Want: "dapat diurai", Got: err.Error()}},
			})
			continue
		}
		sig := obsGroupSignature(ev)
		histSig[id] = sig
		sigToHist[sig] = append(sigToHist[sig], id)
	}

	replaySig := make(map[string]string, len(res.Events))
	sigToReplay := make(map[string][]string, len(res.Events))
	replayIDs := make([]string, 0, len(res.Events))
	for id := range res.Events {
		replayIDs = append(replayIDs, id)
	}
	sort.Strings(replayIDs)
	for _, id := range replayIDs {
		fs := res.Events[id]
		sig := obsGroupSignature(fs[len(fs)-1].Evidence)
		replaySig[id] = sig
		sigToReplay[sig] = append(sigToReplay[sig], id)
	}

	// Tanda tangan ambigu: bijeksi tidak dapat DIPERIKSA, jadi ia dilaporkan
	// sebagai ketidakmampuan memeriksa — bukan sebagai cocok dan bukan sebagai
	// gagal cocok.
	ambiguous := make(map[string]bool)
	for sig, ids := range sigToHist {
		if len(ids) > 1 {
			ambiguous[sig] = true
		}
	}
	for sig, ids := range sigToReplay {
		if len(ids) > 1 {
			ambiguous[sig] = true
		}
	}
	for sig := range ambiguous {
		rep.AmbiguousSignatures = append(rep.AmbiguousSignatures, sig)
	}
	sort.Strings(rep.AmbiguousSignatures)

	matchedReplay := make(map[string]bool, len(replaySig))
	for _, hid := range order {
		sig, ok := histSig[hid]
		if !ok {
			continue // evidence tak dapat diurai; sudah dilaporkan di atas
		}
		if ambiguous[sig] {
			rep.UnmatchedHistoric = append(rep.UnmatchedHistoric, hid)
			continue
		}
		rids := sigToReplay[sig]
		if len(rids) != 1 {
			rep.UnmatchedHistoric = append(rep.UnmatchedHistoric, hid)
			continue
		}
		rid := rids[0]
		matchedReplay[rid] = true
		rep.Events = append(rep.Events, compareEvent(hid, rid, sig, byEvent[hid], res.Events[rid], p))
	}

	// Setiap event replay yang tidak terpasangkan adalah pelanggaran bijeksi,
	// termasuk yang tanda tangannya ambigu: ambiguitas menghalangi pemeriksaan,
	// dan yang tidak dapat diperiksa tidak boleh dilaporkan sebagai cocok.
	for _, rid := range replayIDs {
		if !matchedReplay[rid] {
			rep.UnmatchedReplayed = append(rep.UnmatchedReplayed, rid)
		}
	}
	return rep
}

// compareEvent membandingkan satu event historis dengan pasangan replay-nya.
//
// Rangkaian revisi dibandingkan sebagai RANGKAIAN, bukan sebagai himpunan:
// urutan transisi sebuah event ADALAH keputusannya, dan DETECTED->UNCONFIRMED
// yang tertukar dengan DETECTED->CONFIRMED adalah dua sistem berbeda.
func compareEvent(histID, repID, sig string, hist []store.EventStateLog, frames []Snapshot, p ReplayProfile) EventComparison {
	c := EventComparison{HistoricEventID: histID, ReplayEventID: repID, Signature: sig}
	add := func(rev int, field, w, g string) {
		if w != g {
			c.Diffs = append(c.Diffs, Diff{EventID: histID, Revision: rev, Field: field, Want: w, Got: g})
		}
	}

	if len(hist) != len(frames) {
		add(-1, "jumlah_transisi", strconv.Itoa(len(hist)), strconv.Itoa(len(frames)))
	}

	n := len(hist)
	if len(frames) < n {
		n = len(frames)
	}
	for i := 0; i < n; i++ {
		h, f := hist[i], frames[i]
		rev := h.Revision

		add(rev, "revision", strconv.Itoa(h.Revision), strconv.Itoa(f.Revision))
		add(rev, "to_state", h.ToState, string(f.To))
		add(rev, "from_state", derefState(h.FromState), string(f.From))
		add(rev, "reason", h.Reason, f.Reason)
		add(rev, "node_count", strconv.Itoa(h.NodeCount), strconv.Itoa(f.NodeCount))
		add(rev, "independent_cells", strconv.Itoa(h.IndependentCells), strconv.Itoa(f.IndependentCells))
		add(rev, "peak_pga", formatPeak(h.PeakPGA), formatF(round4(f.PeakPGA)))

		hev, err := historicEvidence(h.EvidenceSummary)
		if err != nil {
			add(rev, "evidence_summary", "dapat diurai", err.Error())
			continue
		}
		c.Diffs = append(c.Diffs, compareEvidence(histID, rev, hev, f.Evidence)...)
	}

	// decided_at sebagai DELTA terhadap transisi PERTAMA event yang sama (F3).
	// Basis relatif, bukan absolut, karena kedua sisi punya asal jam yang
	// berbeda by construction.
	if n > 0 {
		hBase, rBase := hist[0].DecidedAt, frames[0].DecidedAt
		tol := p.tolerance()
		for i := 0; i < n; i++ {
			hd := hist[i].DecidedAt - hBase
			rd := frames[i].DecidedAt - rBase
			diff := hd - rd
			if diff < 0 {
				diff = -diff
			}
			c.Timings = append(c.Timings, TimingDelta{
				EventID:         histID,
				Revision:        hist[i].Revision,
				HistoricMs:      hd,
				ReplayedMs:      rd,
				DifferenceMs:    diff,
				WithinTolerance: diff <= tol,
			})
		}
	}
	return c
}

func derefState(p *string) string {
	if p == nil {
		return "<null>"
	}
	return *p
}

// formatPeak memformat peak_pga historis, yang boleh NULL.
func formatPeak(p *float64) string {
	if p == nil {
		return "null"
	}
	return formatF(round4(*p))
}

// GroupByAlgoVer memecah baris riwayat menurut label algo_ver (V5).
//
// V5 mensyaratkan analisis dikelompokkan menurut algo_ver, dan replay adalah
// analisis. Menggabungkan dua label dalam satu pemutaran akan memutar keputusan
// yang dibuat oleh dua algoritma berbeda di bawah satu algoritma, lalu melaporkan
// selisihnya sebagai non-determinisme.
//
// Kembalian kedua adalah daftar label terurut, supaya pemanggil memutar dalam
// urutan yang dapat diulang.
func GroupByAlgoVer(hist []store.EventStateLog) (map[string][]store.EventStateLog, []string) {
	out := make(map[string][]store.EventStateLog, 2)
	for _, l := range hist {
		out[l.AlgoVer] = append(out[l.AlgoVer], l)
	}
	vers := make([]string, 0, len(out))
	for v := range out {
		vers = append(vers, v)
	}
	sort.Strings(vers)
	return out, vers
}

// DefaultReplayProfile adalah profil yang mencerminkan DEFAULT config
// (internal/config/config.go), bukan konfigurasi produksi yang sedang berjalan.
//
// Ada supaya alat operator punya titik awal yang tertulis di satu tempat, dan
// namanya sengaja menyebut "Default" bukan "Production": tidak ada satu pun
// pembacaan di berkas ini yang dapat mengetahui apa yang dijalankan produksi
// pada saat baris historis dibuat. Operator yang memakai profil ini apa adanya
// tetap MENGASSERSI bahwa produksi memakai default.
func DefaultReplayProfile() ReplayProfile {
	return ReplayProfile{
		Options: Options{
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
		},
	}
}
