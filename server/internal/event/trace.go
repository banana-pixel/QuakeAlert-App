// Keterlacakan pemicu (P4-M1′, D-011).
//
// Kriterianya: setiap observasi yang MEMENUHI SYARAT (pga >= MinPGAGal) di dalam
// satu jendela ledger berbatas harus dapat dilacak ke satu transisi UNCONFIRMED,
// ke event_id-nya, ke baris event_state_log-nya, dan ke emisi advisory-nya.
//
// Berkas ini MENGUKUR itu. Ia tidak menegakkannya. Empat sifat menentukan
// bentuknya, dan tiga di antaranya adalah batas pada apa yang boleh diklaim:
//
//  1. PEMETAAN N:1, BUKAN 1:1. Beberapa observasi dari satu node di dalam satu
//     jendela korelasi menempel ke event yang SAMA dan menghasilkan SATU transisi
//     UNCONFIRMED — UNCONFIRMED -> UNCONFIRMED bukan transisi yang sah
//     (legalTransitions). Karena itu yang diukur adalah "dapat DILACAK KE satu
//     transisi", dan sebuah baris yang berbagi transisi dengan baris lain
//     TERLACAK, bukan terhitung dua kali. Menuntut satu transisi PER BARIS akan
//     melaporkan kegagalan pada mesin yang bekerja tepat sesuai rancangannya.
//
//  2. TAUTAN observasi -> transisi adalah KEANGGOTAAN-DAN-WAKTU, BUKAN KAUSAL.
//     correlation_key dihitung dan tidak pernah disimpan (D12), event_state_log
//     tidak punya observation_id, dan tidak ada FK. Satu-satunya jalan pulang
//     dari sebuah transisi ke observasinya adalah node_id di dalam
//     evidence_summary.contributors[] pada jendela waktu. Itu bukti keanggotaan.
//     Laporan yang menyebutnya kausal akan berbohong dengan angka yang benar.
//
//  3. TAUTAN transisi -> emisi EKSAK untuk baris pasca-000008 (event_id +
//     event_revision ada di baris emisi), dan HANYA-WAKTU untuk baris sebelum
//     itu. Keduanya dilaporkan dengan label berbeda karena kekuatan buktinya
//     berbeda.
//
//  4. TIGA COUNTER kegagalan persistensi DILAPORKAN, TIDAK PERNAH DIJADIKAN
//     target nol. Ini forensik, bukan SLO reliabilitas. Counter hidup di dalam
//     proses dan tidak dapat dipulihkan dari tabel mana pun, jadi bila operator
//     tidak menyediakannya, laporan mengatakan TIDAK DIKETAHUI alih-alih nol.
package event

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/banana-pixel/quakealert/server/internal/dispatch"
	"github.com/banana-pixel/quakealert/server/internal/store"
)

// TraceProfile adalah parameter penelusuran.
//
// Options DIASSERSI OPERATOR, dengan alasan yang sama persis seperti
// ReplayProfile: hanya IndependenceCellKm yang terbawa oleh baris (lewat label
// algo_ver), sisanya tidak terekam di mana pun. Di sini yang benar-benar dipakai
// hanyalah CorrelationWindowMs — ia yang menentukan lebar jendela penerimaan
// tautan — tetapi seluruh Options dibawa supaya spanduk laporan dapat mencetak
// setelan yang diassersi secara utuh.
type TraceProfile struct {
	Options Options

	// LinkToleranceMs adalah kelonggaran TAMBAHAN di atas CorrelationWindowMs
	// untuk menerima sebuah transisi sebagai tautan.
	//
	// Ia ada karena kedua sisi perbandingan memakai jam yang sama tetapi diambil
	// pada titik berbeda: received_ts distempel saat ingest, decided_at saat
	// keputusan, dan di antaranya ada pencarian lokasi node serta drain antrean.
	// Nol berarti pakai defaultLinkToleranceMs.
	LinkToleranceMs int64
}

// defaultLinkToleranceMs sengaja kecil dibanding jendela korelasi: ia menutup
// selisih jam intra-proses, bukan memperluas jendela korelasi diam-diam.
const defaultLinkToleranceMs = 2000

func (p TraceProfile) linkTolerance() int64 {
	if p.LinkToleranceMs > 0 {
		return p.LinkToleranceMs
	}
	return defaultLinkToleranceMs
}

// DefaultTraceProfile mengembalikan profil dengan Options yang sama dengan
// DefaultReplayProfile. Satu sumber, karena kedua alat menelusuri baris yang
// sama dan dua daftar default akan menyimpang tanpa ada yang menyadarinya.
func DefaultTraceProfile() TraceProfile {
	return TraceProfile{Options: DefaultReplayProfile().Options}
}

// Hasil penelusuran satu observasi ke transisi. Kosakata tertutup supaya
// laporan dapat diagregasi.
const (
	// TraceTraced: tepat satu transisi UNCONFIRMED memenuhi keanggotaan-dan-waktu.
	TraceTraced = "TRACED"

	// TraceAmbiguous: lebih dari satu. Ini BUKAN gagal-lacak dan BUKAN terlacak:
	// tautannya tidak dapat diputuskan dari data yang ada. Melaporkannya sebagai
	// salah satu dari keduanya akan menyembunyikan justru kasus yang perlu dilihat
	// manusia.
	TraceAmbiguous = "AMBIGUOUS_MULTIPLE_TRANSITIONS"

	// TraceNoTransition: nol. Inilah satu-satunya keluaran yang berarti "ada
	// pemicu yang memenuhi syarat tanpa transisi yang dapat ditemukan".
	TraceNoTransition = "NO_UNCONFIRMED_TRANSITION"
)

// Hasil penautan transisi ke baris alert_emissions.
const (
	// EmissionByID: event_id DAN event_revision baris emisi cocok. Tautan eksak.
	EmissionByID = "MATCHED_BY_EVENT_ID_AND_REVISION"

	// EmissionByTimeOnly: baris emisi tidak membawa event_id/event_state — ia
	// ditulis sebelum migrasi 000008. Cocok hanya menurut waktu, dan karena itu
	// bukti yang LEBIH LEMAH. Tetap dilaporkan, tidak dibuang.
	EmissionByTimeOnly = "MATCHED_BY_TIME_ONLY_PRE_000008"

	// EmissionMissing: tidak ada baris advisory untuk transisi ini.
	EmissionMissing = "MISSING"

	// EmissionNotApplicable: tidak ada transisi terlacak, jadi tidak ada yang
	// dapat ditautkan. Berbeda dari MISSING.
	EmissionNotApplicable = "NOT_APPLICABLE_NO_TRANSITION"
)

// Anotasi obs_seq. HANYA anotasi: obs_seq pada kontributor adalah nilai
// TERTINGGI yang pernah diserap dan hanya naik, sementara baris event_state_log
// ditulis pada saat transisi. Sebuah observasi dengan obs_seq lebih tinggi dari
// yang tercatat karena itu NORMAL — ia diserap setelah transisi. Konsekuensinya
// tegas: obs_seq dapat MENGUATKAN tautan, tidak pernah membatalkannya, dan
// karena itu ia tidak pernah dipakai sebagai penyaring kandidat.
const (
	ObsSeqExact       = "EXACT"          // sama dengan yang tercatat
	ObsSeqAbsorbedLE  = "CONSISTENT_LE"  // <= yang tercatat: sudah terserap
	ObsSeqLaterGT     = "LATER_GT"       // > yang tercatat: diserap pasca-transisi
	ObsSeqUnavailable = "UNAVAILABLE_V1" // salah satu sisi NULL (protokol v1)
)

// ObservationTrace adalah hasil penelusuran SATU baris sensor_observations.
type ObservationTrace struct {
	ObservationID int64
	NodeID        string
	PGAGal        float64
	ReceivedTS    int64
	ObsSeq        *int64

	Outcome string

	// Terisi bila Outcome == TraceTraced.
	EventID   string
	Revision  int
	DecidedAt int64
	AlgoVer   string

	// LagMs = decided_at - received_ts. Boleh NEGATIF, dan itu bukan anomali:
	// event yang sudah UNCONFIRMED saat baris ini tiba bertransisi lebih dulu.
	LagMs int64

	ObsSeqLink string

	// Candidates terisi bila AMBIGUOUS: "event_id#revision" tiap kandidat.
	Candidates []string

	// NearestCandidate dan NearestCandidateOffMs terisi bila NO_UNCONFIRMED_TRANSITION
	// TETAPI ada transisi yang node_id-nya cocok di LUAR jendela. Dibawa supaya
	// operator dapat membedakan "tidak ada transisi" dari "jendela tautan terlalu
	// sempit" — dua kesimpulan yang sangat berbeda dengan angka yang sama.
	NearestCandidate      string
	NearestCandidateOffMs int64

	EmissionOutcome string
	EmissionID      int64

	// WSClientCount: NIL berarti hasil pengiriman TIDAK PERNAH DILAPORKAN
	// (kolom migrasi 000007), BUKAN nol klien. Nol klien juga bukan kegagalan —
	// baris emisi itu sendiri yang membuktikan frame diputuskan dan disiarkan.
	WSClientCount *int
}

// ExcludedRow adalah baris di ATAS lantai PGA yang TIDAK dapat ditelusuri karena
// konsensus produksi sendiri tidak akan pernah memakainya.
//
// Dipisahkan dari BelowFloor dan dari hasil penelusuran karena ia pertanyaan
// ketiga: bukan "apakah pemicu ini terlacak", melainkan "mengapa pemicu ini
// tidak pernah menjadi pemicu". NODE_LOCATION_NULL khususnya adalah gerbang
// fail-closed di Ingest — sebuah pemicu nyata yang dibuang karena lokasinya tak
// diketahui. Itu harus terlihat, bukan terkubur di dalam penyebut.
type ExcludedRow struct {
	SkippedRow
	PGAGal     float64
	ReceivedTS int64
}

// UnattributedTransition adalah transisi UNCONFIRMED di dalam jendela yang TIDAK
// satu pun observasi memenuhi syarat terlacak kepadanya.
//
// Arah kebalikan wajib dilaporkan: pada jendela N-baris terakhir, penyebabnya
// yang paling sering justru sah — observasi pemicunya berada SEBELUM tepi bawah
// jendela. Karena itu ia dilaporkan sebagai fakta bertepi, bukan sebagai cacat.
type UnattributedTransition struct {
	EventID      string
	Revision     int
	DecidedAt    int64
	NodeIDs      []string
	AlgoVer      string
	AtWindowEdge bool
}

// TraceCounters adalah keempat counter kegagalan persistensi.
//
// DILAPORKAN, TIDAK PERNAH DIJADIKAN target nol (§P4-M1′). Counter ini kumulatif
// SEJAK PROSES DIMULAI dan tidak diberi tanda waktu, jadi ia TIDAK dapat
// diatribusikan ke jendela ini — hanya ke masa hidup proses yang memuatnya.
// Known=false berarti operator tidak menyediakannya, dan nol yang tidak
// diketahui tidak boleh dicetak sebagai nol.
type TraceCounters struct {
	Known            bool
	PersistDropped   int64
	UpsertFailures   int64
	StateLogFailures int64
	StateLogSkipped  int64
}

// CountersFromStatsJSON mengurai badan respons GET /api/v1/admin/tracker/stats.
//
// Mengurai, bukan MENGAMBIL: alat ini tidak melakukan panggilan HTTP dan tidak
// pernah menyentuh kunci admin. Operator mengambilnya sendiri dan menyimpan
// badannya ke berkas. Konsekuensinya tidak ada kredensial yang dapat bocor lewat
// jalur ini.
//
// Diurai ke TrackerStats — struct yang sudah ada, bukan salinan barunya — supaya
// nama JSON tidak pernah menyimpang dari yang dikeluarkan endpoint.
func CountersFromStatsJSON(raw []byte) (TraceCounters, error) {
	var s TrackerStats
	if err := json.Unmarshal(raw, &s); err != nil {
		return TraceCounters{}, fmt.Errorf("urai tracker stats: %w", err)
	}
	return TraceCounters{
		Known:            true,
		PersistDropped:   s.PersistDropped,
		UpsertFailures:   s.UpsertFailures,
		StateLogFailures: s.StateLogFailures,
		StateLogSkipped:  s.StateLogSkipped,
	}, nil
}

// ---------------------------------------------------------------------------
// Laporan
// ---------------------------------------------------------------------------

// TraceReport adalah hasil penelusuran satu jendela.
//
// TIDAK ada field bernama Passed, Satisfied, atau Valid, dan ketiadaan itu
// disengaja. P4-M1′ adalah pengukuran; keputusan "apakah ini cukup" adalah
// penilaian pemilik terhadap angka-angka ini, bukan boolean yang dihitung
// berkas ini.
type TraceReport struct {
	Profile TraceProfile

	// Jendela: DITENTUKAN oleh N baris terakhir, lalu diterjemahkan ke rentang
	// waktu untuk sisi state-log dan emisi. RequestedN adalah yang diminta,
	// TotalRows yang benar-benar ada — keduanya berbeda bila tabel lebih pendek
	// dari N, dan pembaca harus tahu yang mana yang ia lihat.
	RequestedN int
	TotalRows  int
	FromTS     int64
	ToTS       int64

	// Penyebut, dipecah. Ketiganya harus dijumlahkan menjadi TotalRows.
	BelowFloor int           // pga < MinPGAGal: bukan pemicu, bukan kegagalan
	Excluded   []ExcludedRow // >= lantai tetapi konsensus sendiri membuangnya
	Traces     []ObservationTrace

	Unattributed []UnattributedTransition

	// StateLogRows dan EmissionRows adalah jumlah baris mentah yang dibaca pada
	// jendela waktu, SEBELUM penyaringan apa pun. Dibawa supaya sebuah laporan
	// nol-tautan dapat dibedakan dari laporan atas jendela yang kosong.
	StateLogRows int
	EmissionRows int

	Counters TraceCounters

	// LedgerDropsKnown: ledger_drops_total HANYA masuk log (D17/D30), jadi
	// jendela ini DAPAT kehilangan observasi tanpa satu pun jejak di dalam tabel.
	// Nol di sini berarti TIDAK DIKETAHUI, bukan "tidak ada yang hilang".
	LedgerDropsKnown int

	// SingleNodeFleet benar bila seluruh jendela hanya memuat satu node_id.
	// Wajib eksplisit (S2): pada fleet satu-node CONFIRMED tidak dapat dicapai
	// menurut kerapatan, jadi UNCONFIRMED adalah state tujuan yang BENAR dan
	// ketiadaan CONFIRMED bukan cacat.
	SingleNodeFleet bool
	NodeIDs         []string
}

// Outcomes mengelompokkan hasil penelusuran menurut Outcome.
func (r TraceReport) Outcomes() map[string]int {
	out := make(map[string]int, 3)
	for _, t := range r.Traces {
		out[t.Outcome]++
	}
	return out
}

// EmissionOutcomes mengelompokkan hasil penautan emisi.
func (r TraceReport) EmissionOutcomes() map[string]int {
	out := make(map[string]int, 4)
	for _, t := range r.Traces {
		out[t.EmissionOutcome]++
	}
	return out
}

// ExcludeCounts mengelompokkan baris terkecuali menurut alasan.
func (r TraceReport) ExcludeCounts() map[string]int {
	out := make(map[string]int, 3)
	for _, e := range r.Excluded {
		out[e.Reason]++
	}
	return out
}

// DistinctTransitions adalah jumlah transisi UNCONFIRMED BERBEDA yang terlacak.
//
// Ada di samping len(Traces) karena pemetaannya N:1 (sifat 1 di kepala berkas):
// dua puluh observasi yang terlacak ke satu transisi adalah dua puluh baris
// terlacak dan SATU transisi. Melaporkan hanya salah satunya membuat pembaca
// menyimpulkan yang lain secara salah.
func (r TraceReport) DistinctTransitions() int {
	seen := make(map[string]struct{}, len(r.Traces))
	for _, t := range r.Traces {
		if t.Outcome != TraceTraced {
			continue
		}
		seen[transitionKey(t.EventID, t.Revision)] = struct{}{}
	}
	return len(seen)
}

func transitionKey(eventID string, rev int) string {
	return eventID + "#" + strconv.Itoa(rev)
}

// ---------------------------------------------------------------------------
// Mesin penelusuran
// ---------------------------------------------------------------------------

// unconfirmedTransition adalah satu baris event_state_log yang masuk ke
// UNCONFIRMED, beserta keanggotaan node yang tercatat pada baris itu.
type unconfirmedTransition struct {
	row      store.EventStateLog
	nodes    map[string]*int64 // node_id -> obs_seq tercatat (nil untuk v1)
	nodeList []string
}

// Trace menelusuri satu jendela. HANYA MENGHITUNG — tidak ada I/O, tidak ada
// Tracker, tidak ada jam. Ketiga masukan sudah dibaca pemanggil, yang juga yang
// memilih lebar jendelanya.
//
// obs HARUS jendela N-baris terakhir dalam urutan received_ts naik
// (store.ListLastNObservations sudah memberikannya demikian). hist dan emis
// HARUS mencakup PALING TIDAK rentang [obs pertama - jendela korelasi, obs
// terakhir + toleransi]; pemanggil yang mempersempitnya akan melihat tautan
// hilang yang sebenarnya ada — karena itu perhitungan tepinya ada di
// TraceWindowBounds, bukan di kepala pemanggil.
func Trace(
	obs []store.ReplayObservation,
	hist []store.EventStateLog,
	emis []store.TraceEmission,
	p TraceProfile,
	requestedN int,
) *TraceReport {
	rep := &TraceReport{
		Profile:      p,
		RequestedN:   requestedN,
		TotalRows:    len(obs),
		StateLogRows: len(hist),
		EmissionRows: len(emis),
	}
	if len(obs) > 0 {
		rep.FromTS = obs[0].ReceivedTS
		rep.ToTS = obs[len(obs)-1].ReceivedTS
	}

	nodeSeen := make(map[string]struct{}, 4)
	for _, o := range obs {
		nodeSeen[o.NodeID] = struct{}{}
	}
	rep.NodeIDs = sortedKeys(nodeSeen)
	rep.SingleNodeFleet = len(rep.NodeIDs) == 1

	trans := unconfirmedTransitions(hist)
	adv := advisoryEmissions(emis)
	tol := p.linkTolerance()
	win := p.Options.CorrelationWindowMs + tol

	// attributed melacak transisi mana yang dijangkau SETIDAKNYA satu observasi,
	// untuk arah kebalikannya di bawah.
	attributed := make(map[string]struct{}, len(trans))

	for _, o := range obs {
		if o.PGAGal < MinPGAGal {
			rep.BelowFloor++
			continue
		}
		if reason, excluded := excludeReason(o); excluded {
			rep.Excluded = append(rep.Excluded, ExcludedRow{
				SkippedRow: SkippedRow{ObservationID: o.ObservationID, NodeID: o.NodeID, Reason: reason},
				PGAGal:     o.PGAGal,
				ReceivedTS: o.ReceivedTS,
			})
			continue
		}

		tr := ObservationTrace{
			ObservationID: o.ObservationID,
			NodeID:        o.NodeID,
			PGAGal:        o.PGAGal,
			ReceivedTS:    o.ReceivedTS,
			ObsSeq:        o.ObsSeq,
		}

		var hits []*unconfirmedTransition
		var nearest *unconfirmedTransition
		nearestOff := int64(0)
		for i := range trans {
			t := &trans[i]
			if _, ok := t.nodes[o.NodeID]; !ok {
				continue
			}
			off := t.row.DecidedAt - o.ReceivedTS
			if off <= tol && off >= -win {
				hits = append(hits, t)
				continue
			}
			// Cocok node tetapi di LUAR jendela. Disimpan yang terdekat supaya
			// "tidak ada transisi" dapat dibedakan dari "jendela terlalu sempit".
			if nearest == nil || absInt64(off) < absInt64(nearestOff) {
				nearest, nearestOff = t, off
			}
		}

		switch len(hits) {
		case 1:
			t := hits[0]
			tr.Outcome = TraceTraced
			tr.EventID = t.row.EventID
			tr.Revision = t.row.Revision
			tr.DecidedAt = t.row.DecidedAt
			tr.AlgoVer = t.row.AlgoVer
			tr.LagMs = t.row.DecidedAt - o.ReceivedTS
			tr.ObsSeqLink = obsSeqLink(o.ObsSeq, t.nodes[o.NodeID])
			attributed[transitionKey(t.row.EventID, t.row.Revision)] = struct{}{}
			linkEmission(&tr, t, adv, tol)
		case 0:
			tr.Outcome = TraceNoTransition
			tr.EmissionOutcome = EmissionNotApplicable
			if nearest != nil {
				tr.NearestCandidate = transitionKey(nearest.row.EventID, nearest.row.Revision)
				tr.NearestCandidateOffMs = nearestOff
			}
		default:
			tr.Outcome = TraceAmbiguous
			tr.EmissionOutcome = EmissionNotApplicable
			for _, t := range hits {
				tr.Candidates = append(tr.Candidates, transitionKey(t.row.EventID, t.row.Revision))
				// Sebuah transisi yang ambigu tetap TERATRIBUSI: ia dijangkau oleh
				// observasi ini walau tidak dapat dipastikan sendirian. Menghitungnya
				// sebagai tak-teratribusi akan melaporkannya dua kali sebagai masalah.
				attributed[transitionKey(t.row.EventID, t.row.Revision)] = struct{}{}
			}
			sort.Strings(tr.Candidates)
		}
		rep.Traces = append(rep.Traces, tr)
	}

	// Arah kebalikan: transisi UNCONFIRMED yang tidak dijangkau observasi mana pun.
	edge := rep.FromTS + win
	for i := range trans {
		t := &trans[i]
		if _, ok := attributed[transitionKey(t.row.EventID, t.row.Revision)]; ok {
			continue
		}
		rep.Unattributed = append(rep.Unattributed, UnattributedTransition{
			EventID:      t.row.EventID,
			Revision:     t.row.Revision,
			DecidedAt:    t.row.DecidedAt,
			NodeIDs:      t.nodeList,
			AlgoVer:      t.row.AlgoVer,
			AtWindowEdge: t.row.DecidedAt <= edge,
		})
	}

	return rep
}

// TraceWindowBounds mengembalikan rentang waktu yang HARUS dibaca pada
// event_state_log dan alert_emissions untuk sebuah jendela observasi.
//
// Fungsi terpisah, bukan komentar pada Trace, karena salah menghitungnya adalah
// cara paling mudah membuat laporan ini berbohong: sebuah observasi di tepi bawah
// jendela dapat menempel ke event yang bertransisi satu jendela korelasi lebih
// AWAL, dan baris itu berada di luar rentang received_ts observasi.
func TraceWindowBounds(obs []store.ReplayObservation, p TraceProfile) (fromTS, toTS int64, ok bool) {
	if len(obs) == 0 {
		return 0, 0, false
	}
	tol := p.linkTolerance()
	return obs[0].ReceivedTS - p.Options.CorrelationWindowMs - tol,
		obs[len(obs)-1].ReceivedTS + tol,
		true
}

// unconfirmedTransitions memilih baris yang masuk ke UNCONFIRMED dan mengurai
// keanggotaan node dari evidence_summary.
//
// evidence_summary yang tidak dapat diurai TIDAK membuat barisnya hilang: ia
// tetap masuk daftar dengan keanggotaan KOSONG, sehingga tidak ada observasi yang
// dapat tertaut kepadanya dan ia muncul sebagai transisi tak-teratribusi. Itu
// hasil yang benar — barisnya nyata, hanya buktinya tak terbaca — dan membuangnya
// akan membuat sebuah baris yang rusak tampak tidak pernah ada.
func unconfirmedTransitions(hist []store.EventStateLog) []unconfirmedTransition {
	out := make([]unconfirmedTransition, 0, 8)
	for _, r := range hist {
		if r.ToState != string(StateUnconfirmed) {
			continue
		}
		t := unconfirmedTransition{row: r, nodes: map[string]*int64{}}
		if ev, err := historicEvidence(r.EvidenceSummary); err == nil {
			for _, c := range ev.Contributors {
				t.nodes[c.NodeID] = c.ObsSeq
				t.nodeList = append(t.nodeList, c.NodeID)
			}
			sort.Strings(t.nodeList)
		}
		out = append(out, t)
	}
	return out
}

// advisoryEmissions memilih baris emisi yang membawa frame advisory.
//
// Disaring menurut alert_type, BUKAN menurut audience: audience='NONE' benar
// untuk advisory hari ini, tetapi ia properti keputusan FCM, sementara yang
// dicari kriteria adalah frame WEBSOCKET-nya. Menyaring dengan audience akan
// menautkan hal yang benar untuk alasan yang salah, dan berhenti bekerja pada
// hari pertama audience berubah.
func advisoryEmissions(emis []store.TraceEmission) []store.TraceEmission {
	out := make([]store.TraceEmission, 0, 8)
	for _, e := range emis {
		if e.AlertType == dispatch.TypeAdvisory {
			out = append(out, e)
		}
	}
	return out
}

// linkEmission menautkan satu transisi terlacak ke baris alert_emissions-nya.
//
// Dua kekuatan bukti, dilaporkan terpisah:
//
//	EKSAK      — baris membawa event_id DAN event_revision yang cocok (pasca-000008).
//	HANYA-WAKTU — baris tidak membawa keduanya (pra-000008). Dicocokkan menurut
//	              decided_at di dalam toleransi, dan dilabeli sebagai lebih lemah.
//
// Yang eksak dicari LEBIH DULU dan menang: bila satu baris eksak ada, sebuah
// kecocokan waktu pada baris lain tidak relevan.
func linkEmission(tr *ObservationTrace, t *unconfirmedTransition, adv []store.TraceEmission, tol int64) {
	for _, e := range adv {
		if e.EventID == nil || e.EventRevision == nil {
			continue
		}
		if *e.EventID == t.row.EventID && *e.EventRevision == t.row.Revision {
			tr.EmissionOutcome = EmissionByID
			tr.EmissionID = e.EmissionID
			tr.WSClientCount = e.WSClientCount
			return
		}
	}
	best := int64(-1)
	for _, e := range adv {
		if e.EventID != nil && e.EventRevision != nil {
			continue // baris beridentitas yang TIDAK cocok; bukan kandidat waktu
		}
		off := absInt64(e.DecidedAt - t.row.DecidedAt)
		if off > tol {
			continue
		}
		if best < 0 || off < best {
			best = off
			tr.EmissionOutcome = EmissionByTimeOnly
			tr.EmissionID = e.EmissionID
			tr.WSClientCount = e.WSClientCount
		}
	}
	if tr.EmissionOutcome == "" {
		tr.EmissionOutcome = EmissionMissing
	}
}

// excludeReason melaporkan mengapa sebuah baris di atas lantai PGA tidak akan
// pernah menjadi pemicu. Mencerminkan gerbang produksi, bukan menambahkannya:
//
//	verify_result != OK  — verifier menolak; ledger tetap mencatatnya (§16).
//	node_location NULL   — Ingest gagal-tertutup pada pencarian lokasi.
//	tanpa jangkar onset  — tidak ada korelasi yang dapat dilakukan sama sekali.
func excludeReason(o store.ReplayObservation) (string, bool) {
	switch {
	case o.VerifyResult != "OK":
		return SkipVerifyNotOK, true
	case o.Lat == nil || o.Lon == nil:
		return SkipNoLocation, true
	case o.OnsetTS == nil && o.OnsetTSUpperBound == nil:
		return SkipNoOnsetAnchor, true
	}
	return "", false
}

// obsSeqLink membandingkan obs_seq observasi dengan yang tercatat pada
// kontributor. ANOTASI, tidak pernah penyaring — lihat komentar kosakata di atas:
// obs_seq kontributor hanya naik, jadi nilai yang lebih tinggi pada observasi
// berarti ia diserap SETELAH transisi ini, yang normal.
func obsSeqLink(obs, recorded *int64) string {
	if obs == nil || recorded == nil {
		return ObsSeqUnavailable
	}
	switch {
	case *obs == *recorded:
		return ObsSeqExact
	case *obs < *recorded:
		return ObsSeqAbsorbedLE
	default:
		return ObsSeqLaterGT
	}
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
