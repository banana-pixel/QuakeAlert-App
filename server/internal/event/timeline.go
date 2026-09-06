package event

// timeline.go — P4-M6′: garis waktu forensik SATU event, dari sisi TRANSISI.
//
// Kriterianya (ROADMAP.md § Phase 4, P4-M6′) menyebut empat keluaran untuk satu
// event_id: baris event, riwayat event_state_log-nya yang berurutan,
// evidence_summary per revisi, dan observasi yang berkontribusi. D-015
// menetapkan bentuknya. Berkas ini menghitung keempatnya dan tidak melakukan I/O
// sama sekali — pemanggil yang membaca, sama seperti Trace().
//
// EMPAT SIFAT yang menentukan bentuk berkas ini:
//
//  1. TRANSISI-DAHULU, bukan observasi-dahulu. Trace() (P4-M1′) berjalan dari
//     observasi ke transisi pada jendela N-baris terakhir dan HANYA melihat
//     transisi ke UNCONFIRMED (unconfirmedTransitions menyaring demikian). M6′
//     berjalan dari SATU event ke SELURUH revisinya — termasuk CONFIRMED,
//     RESOLVED dan CANCELLED. Karena itu ia tidak dapat memanggil Trace(), dan
//     karena itu pula ia TIDAK MENGUBAHNYA: perilaku M1′ yang sudah divalidasi
//     pemilik dipakai, tidak disunting (D-015). Yang dipakai ulang adalah
//     miliknya yang berarti — toleransi, predikat, kosakata tertutup,
//     historicEvidence, excludeReason, obsSeqLink — semuanya dipanggil dari sini.
//
//  2. TAUTAN observasi -> revisi adalah KEANGGOTAAN-DAN-WAKTU, BUKAN KAUSAL.
//     Persis batas yang sama dengan M1′ dan dengan alasan struktural yang sama:
//     correlation_key dihitung dan tidak pernah disimpan (D12), event_state_log
//     tidak punya observation_id, dan tidak ada FK ke sensor_observations. Satu-
//     satunya jalan pulang dari sebuah transisi ke observasinya adalah node_id di
//     dalam evidence_summary.contributors[] pada sebuah jendela waktu. Setiap
//     observasi di sini adalah KANDIDAT. Laporan yang menyebutnya sebab akan
//     berbohong dengan angka yang benar.
//
//  3. KETIDAKLENGKAPAN DINYATAKAN, tidak disimpulkan. Baris log dapat hilang
//     karena satuan persistensinya dibuang (D17/D30), baris observasi dapat tidak
//     pernah sampai ke disk karena ledger_drops_total hanya masuk log, dan
//     evidence_summary yang tak terurai tetap dihitung sebagai revisi yang nyata.
//     KETIADAAN DI DALAM CATATAN TIDAK PERNAH DILAPORKAN SEBAGAI BUKTI
//     KETIADAAN — itu kalimat D-015 dan Coverage di bawah ada untuk menegakkannya.
//
//  4. TIDAK ADA PENILAIAN. Tidak ada field bernama Passed, Satisfied, Complete
//     atau Valid, sama seperti TraceReport. Ini pembacaan; keputusan "apakah ini
//     cukup" adalah penilaian pemilik atas angkanya (PROJECT_RULES.md §8/§9).
//
// event_near_confirmed (migrasi 000009) TIDAK dibaca di sini: ia tidak terpasang
// pada schema_version = 8, dan tidak satu pun dari empat keluaran wajib boleh
// bergantung padanya (D-015 batasan 4).

import (
	"sort"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// Provenance toleransi tautan. Dicetak bersama nilainya karena sebuah angka
// tanpa asalnya tidak dapat ditafsirkan ulang oleh pembaca berikutnya (D-013).
const (
	// TolFromM1Default: nilai bawaan M1′, defaultLinkToleranceMs.
	TolFromM1Default = "M1_DEFAULT"

	// TolFromOperator: operator mengassersi LINK_TOLERANCE_MS sendiri.
	TolFromOperator = "OPERATOR_ASSERTED"
)

// EffectiveLinkTolerance mengembalikan toleransi yang BERLAKU beserta asalnya.
//
// Ada supaya alat operator dapat mencetak keduanya tanpa menebak dan tanpa
// menyalin ulang aturan bawaannya. Tidak ada toleransi ilmiah baru yang
// diperkenalkan M6′: nilainya milik M1′, dan p.linkTolerance() yang memutuskan
// (D-015 batasan 2).
func EffectiveLinkTolerance(p TraceProfile) (ms int64, provenance string) {
	if p.LinkToleranceMs > 0 {
		return p.linkTolerance(), TolFromOperator
	}
	return p.linkTolerance(), TolFromM1Default
}

// ---------------------------------------------------------------------------
// Keluaran 1: baris event
// ---------------------------------------------------------------------------

// Status ketersediaan sebuah keluaran wajib. Dilaporkan per keluaran karena
// D-015 menuntut arsipnya mencatat MANA dari keempatnya yang benar-benar
// teramati dan mana yang NOT OBSERVABLE.
const (
	// OutputObserved: keluaran ini ada dan terbaca.
	OutputObserved = "OBSERVED"

	// OutputEmpty: pembacaannya berhasil dan hasilnya NOL baris. Bukan galat, dan
	// bukan bukti bahwa tidak ada apa-apa — hanya bahwa tabel ini tidak memuatnya.
	OutputEmpty = "EMPTY"

	// OutputNotObservable: tidak dapat diamati sama sekali pada skema atau data
	// ini. Berbeda dari EMPTY, dan perbedaan itu seluruh alasan konstanta ini ada.
	OutputNotObservable = "NOT_OBSERVABLE"
)

// ---------------------------------------------------------------------------
// Keluaran 2 + 3: satu revisi, dengan evidence_summary-nya yang sudah diurai
// ---------------------------------------------------------------------------

// RevisionEntry adalah SATU baris event_state_log beserta bukti yang sudah
// diurai dan observasi yang menjadi kandidatnya.
//
// Keluaran 2 dan 3 kriteria bersatu di dalam satu tipe dan itu disengaja: sebuah
// evidence_summary tanpa barisnya tidak dapat ditafsirkan (revisi mana? state
// apa? diputuskan kapan?), dan dua daftar sejajar yang harus dicocokkan menurut
// indeks adalah cara paling mudah membuat laporan ini salah baca.
type RevisionEntry struct {
	Row store.EventStateLog

	// Evidence adalah evidence_summary yang SUDAH DIURAI. EvidenceParsed salah
	// berarti kolomnya ada tetapi tidak terurai; barisnya TETAP dilaporkan dengan
	// keanggotaan kosong — persis perlakuan unconfirmedTransitions — supaya baris
	// yang rusak muncul sebagai revisi tak-teratribusi alih-alih menghilang.
	Evidence       EvidenceSummary
	EvidenceParsed bool
	EvidenceError  string

	// ContributorNodes adalah node_id di dalam contributors[], terurut.
	// Keanggotaan, bukan sebab.
	ContributorNodes []string

	// WindowFromTS/WindowToTS adalah jendela KEANGGOTAAN-DAN-WAKTU revisi ini:
	//
	//	[decided_at - (CorrelationWindowMs + tol), decided_at + tol]
	//
	// Sama persis dengan predikat M1′ `off <= tol && off >= -win` dengan
	// off = decided_at - received_ts, hanya ditulis sebagai batas received_ts.
	// Diturunkan, bukan diassersi, supaya tidak ada toleransi kedua di sistem ini.
	WindowFromTS int64
	WindowToTS   int64

	// Candidates adalah observasi yang memenuhi keanggotaan-dan-waktu untuk revisi
	// ini. KANDIDAT, bukan sebab. Sebuah observasi dapat muncul pada lebih dari
	// satu revisi — jendela dua revisi yang berdekatan bertumpang-tindih — dan itu
	// AMBIGUITAS yang dilaporkan, bukan yang dipilih salah satunya.
	Candidates []ObservationCandidate

	// ExcludedCandidates adalah baris yang cocok keanggotaan-dan-waktu tetapi tidak
	// akan pernah dipakai konsensus produksi. Dipisahkan karena ia pertanyaan
	// ketiga, sama seperti ExcludedRow pada M1′.
	ExcludedCandidates []ExcludedRow

	// BelowFloor adalah jumlah kandidat dengan pga < MinPGAGal. Bukan pemicu,
	// bukan kegagalan — dihitung supaya penyebutnya utuh.
	BelowFloor int
}

// ---------------------------------------------------------------------------
// Keluaran 4: observasi yang berkontribusi
// ---------------------------------------------------------------------------

// ObservationCandidate adalah SATU observasi yang tertaut ke satu revisi menurut
// keanggotaan-dan-waktu.
//
// Namanya Candidate, bukan Cause, dan itu bukan kehati-hatian berlebihan: kolom
// yang akan membuktikan sebab tidak ada di skema (D12, tidak ada observation_id di
// event_state_log, tidak ada FK), jadi tipe yang menamai dirinya sebab akan
// membuat setiap pembacanya salah tanpa satu pun angka yang salah.
type ObservationCandidate struct {
	ObservationID int64
	NodeID        string
	PGAGal        float64
	ReceivedTS    int64
	ObsSeq        *int64

	// LagMs = decided_at - received_ts. Boleh NEGATIF dan itu bukan anomali:
	// observasi yang tiba SETELAH keputusan tetap kandidat bila ia di dalam
	// toleransi — evidence_summary adalah potret pada saat transisi, dan sebuah
	// baris dapat terserap ke event yang sama sesudahnya.
	LagMs int64

	// ObsSeqLink adalah anotasi kosakata tertutup M1′ (EXACT / CONSISTENT_LE /
	// LATER_GT / UNAVAILABLE_V1). ANOTASI, tidak pernah penyaring.
	ObsSeqLink string

	// Attribution adalah kosakata tautan M1′ yang dipakai dari sisi transisi:
	//
	//	TRACED                         — observasi ini kandidat TEPAT satu revisi.
	//	AMBIGUOUS_MULTIPLE_TRANSITIONS — kandidat lebih dari satu revisi. BUKAN
	//	                                 kecocokan dan BUKAN kegagalan; tautannya
	//	                                 tidak dapat diputuskan dari data yang ada.
	//
	// NO_UNCONFIRMED_TRANSITION tidak pernah muncul di sini: kandidat lahir DARI
	// sebuah revisi, jadi nol-revisi bukan keadaan yang dapat terjadi pada tipe
	// ini. Arah kebalikannya dilaporkan sebagai UnattributedRevisions.
	Attribution string

	// AttributedTo adalah seluruh revisi yang observasi ini menjadi kandidatnya,
	// terurut. Panjang 1 pada TRACED, > 1 pada AMBIGUOUS. Dibawa penuh supaya
	// ambiguitas dapat dibaca, bukan hanya dihitung.
	AttributedTo []int
}

// UnattributedRevision adalah revisi yang TIDAK satu pun observasi memenuhi
// syarat menjadi kandidatnya.
//
// Arah kebalikan wajib dilaporkan, dan pada M6′ penyebab sahnya ada tiga, semua
// bukan cacat: baris observasinya tidak pernah sampai ke disk (ledger drop-oldest,
// D17/D30), revisinya lahir dari SWEEP alih-alih dari observasi baru (RESOLVED
// karena NO_NEW_EVIDENCE, yang decided_at-nya jatuh ResolveAfterMs setelah
// observasi terakhir sehingga jendelanya memang kosong), atau evidence_summary-nya
// tidak terurai sehingga tidak ada node yang dapat dicari.
type UnattributedRevision struct {
	Revision  int
	ToState   string
	Reason    string
	DecidedAt int64
	NodeIDs   []string

	// NoContributors benar bila contributors[] kosong ATAU evidence_summary tidak
	// terurai — dua sebab berbeda yang keduanya membuat pencarian mustahil, dan
	// keduanya tetap dapat dibedakan lewat RevisionEntry.EvidenceParsed.
	NoContributors bool

	// MemberRowsFiltered adalah jumlah baris yang COCOK keanggotaan-dan-waktu untuk
	// revisi ini tetapi tersaring — di bawah lantai PGA, atau dibuang konsensus
	// sendiri. Bukan nol berarti "tanpa kandidat" di sini TIDAK berarti tidak ada
	// baris: ada, dan alasannya terbaca di RevisionEntry.
	MemberRowsFiltered int

	// NotObservationDriven benar bila reason-nya adalah transisi yang TIDAK lahir
	// dari observasi yang tiba: NO_NEW_EVIDENCE (sweep/rekonsiliasi),
	// EVIDENCE_INVALIDATED dan OPERATOR_RETRACTION (pencabutan kontributor).
	// Dibawa supaya "tidak ada kandidat" pada baris seperti ini terbaca sebagai HAL
	// YANG DIHARAPKAN, bukan sebagai observasi yang hilang.
	NotObservationDriven bool
}

// ---------------------------------------------------------------------------
// Selubung cakupan
// ---------------------------------------------------------------------------

// TimelineCoverage adalah pernyataan ketidaklengkapan yang menyertai setiap
// jawaban M6′.
//
// Bentuknya mengikuti NearConfirmedCoverage (P4-M2′) dengan sengaja, termasuk apa
// yang TIDAK ada di dalamnya: tidak ada field bernama Complete, Healthy atau
// Valid. Ia melaporkan APA YANG DIBACA, bukan menyimpulkan apa yang ada.
//
// Alasannya satu kalimat: daftar KOSONG punya beberapa arti yang sangat berbeda,
// dan tanpa selubung ini semuanya keluar sebagai byte yang identik. "Tidak ada
// baris riwayat" dapat berarti event ini memang belum pernah bertransisi secara
// publik (persistensi DETECTED lazy, §9.5), atau satuan persistensinya dibuang
// (D17/D30). "Tidak ada observasi kandidat" dapat berarti tidak ada yang tiba,
// atau barisnya tidak pernah sampai ke disk. KETIADAAN DI DALAM CATATAN BUKAN
// BUKTI KETIADAAN (D-015).
type TimelineCoverage struct {
	// Status keempat keluaran wajib kriteria, dalam urutan kriteria.
	EventRowStatus     string
	StateLogStatus     string
	EvidenceStatus     string
	ObservationsStatus string

	// Parameter yang BERLAKU pada pembacaan ini, dan asal toleransinya.
	CorrelationWindowMs int64
	LinkToleranceMs     int64
	ToleranceProvenance string

	// AlgoVersRow adalah algo_ver yang benar-benar TERBAWA baris riwayat event ini,
	// terurut dan unik. Lebih dari satu berarti revisi-revisinya diputuskan oleh
	// konfigurasi yang BERBEDA, dan membandingkannya sebagai satu deret tanpa
	// menyebut itu akan menilai keputusan lampau dengan parameter yang tidak
	// menghasilkannya (V5/V6, D-006).
	AlgoVersRow []string

	// AlgoVerBinary adalah basis algoritma biner yang membaca. Dibawa terpisah
	// karena ia BUKAN properti baris mana pun.
	AlgoVerBinary string

	// Pembilang mentah. Dilaporkan, tidak dijadikan target.
	StateLogRows            int
	RevisionsWithEvidence   int
	RevisionsEvidenceBroken int
	ContributorNodes        int
	ObservationRowsRead     int
	CandidateRows           int
	ExcludedRows            int
	BelowFloorRows          int
	AmbiguousCandidates     int
	UnattributedRevisions   int

	// RevisionGaps adalah nomor revisi yang HILANG dari deret riwayat: setiap
	// bilangan di antara revisi terkecil dan terbesar yang tidak punya baris.
	// Inilah satu-satunya bukti KEHILANGAN yang dapat dibaca dari dalam tabel —
	// UNIQUE (event_id, revision) menjamin revisi tidak berulang, dan Revision naik
	// satu per transisi, jadi sebuah lubang adalah satuan persistensi yang dibuang
	// (D17/D30). Deret yang tidak dimulai dari revisi terkecil yang mungkin BUKAN
	// lubang: DETECTED tidak pernah menjadi baris (§9.5).
	RevisionGaps []int

	// FirstRevision/LastRevision adalah tepi deret yang benar-benar ada. Nol pada
	// riwayat kosong, dan pembaca yang melihat StateLogRows == 0 tahu bahwa nol di
	// sini berarti "tidak ada", bukan "revisi 0".
	FirstRevision int
	LastRevision  int

	// LedgerDropsKnown adalah ledger_drops_total yang DIISI OPERATOR dari log.
	// Counter itu hanya masuk log (D17/D30) sehingga tidak ada kueri yang dapat
	// memulihkannya; nol di sini berarti TIDAK DIKETAHUI, bukan "tidak ada yang
	// hilang" — pembedaan yang sama dengan TraceReport.LedgerDropsKnown.
	LedgerDropsKnown int

	// SingleNodeContributors benar bila SELURUH riwayat event ini hanya menyebut
	// satu node. Pada fleet satu-node itu keadaan yang BENAR dan bukan cacat (S2):
	// kuorum butuh >= 3 kontributor terverifikasi di >= 2 sel independen, yang
	// tidak terjangkau menurut kerapatan jaringan.
	SingleNodeContributors bool

	// TerminalState berisi state terminal yang riwayat ini SUNGGUH memuat barisnya
	// (RESOLVED / CANCELLED), kosong bila belum ada. Kosong BUKAN berarti event ini
	// masih hidup: ia dapat sudah mati di memori tanpa barisnya pernah ditulis.
	TerminalState string
}

// ---------------------------------------------------------------------------
// Laporan
// ---------------------------------------------------------------------------

// EventTimeline adalah jawaban lengkap M6′ untuk SATU event.
//
// Keempat keluaran kriteria dapat ditunjuk satu per satu:
//
//  1. baris event                    -> Event
//  2. riwayat berurutan revision ASC -> Revisions[i].Row
//  3. evidence_summary per revisi     -> Revisions[i].Evidence
//  4. observasi yang berkontribusi    -> Revisions[i].Candidates dan Observations
//
// Emissions adalah keluaran KELIMA dan OPSIONAL. Ia tidak pernah menentukan
// diterima atau tidaknya M6′ (D-015 batasan 1): kriteria menyebut empat keluaran,
// dan sebuah bagian tambahan tidak boleh menjadi hal yang membuat sebuah milestone
// lulus atau gagal. Nil berarti operator tidak memintanya.
type EventTimeline struct {
	EventID string

	// Event nil berarti tidak ada baris earthquake_events dengan event_id ini.
	// Pembacaannya BUKAN galat pada tingkat ini — EventRowStatus yang mengatakannya
	// — karena riwayat dapat tetap terbaca walau induknya hilang, dan sebuah alat
	// forensik yang mati di situ menyembunyikan justru keadaan yang paling perlu
	// dilihat.
	Event *store.EarthquakeEvent

	Revisions []RevisionEntry

	// Observations adalah kandidat unik lintas seluruh revisi, terurut kanonik
	// (received_ts, observation_id). Satu observasi muncul SEKALI di sini meski ia
	// kandidat beberapa revisi; AttributedTo-nya yang membawa semuanya. Ini
	// keluaran keempat kriteria dalam bentuk daftar datar.
	Observations []ObservationCandidate

	Unattributed []UnattributedRevision

	// Emissions OPSIONAL (lihat komentar tipe). Terisi hanya bila pemanggil
	// menyediakan baris emisi.
	Emissions []RevisionEmission

	Coverage TimelineCoverage
}

// RevisionEmission adalah tautan satu revisi ke baris alert_emissions-nya.
//
// OPSIONAL, dan kekuatan buktinya dilaporkan dengan kosakata M1′ yang sama:
// MATCHED_BY_EVENT_ID_AND_REVISION (eksak, pasca-000008) berbeda dari
// MATCHED_BY_TIME_ONLY_PRE_000008 (lebih lemah). MISSING berarti tidak ada baris
// yang cocok — dan itu bukan kegagalan: hanya transisi yang menghasilkan frame
// yang punya baris emisi sama sekali.
type RevisionEmission struct {
	Revision   int
	ToState    string
	DecidedAt  int64
	Outcome    string
	EmissionID int64
	AlertType  string

	// WSClientCount NIL berarti hasil pengiriman TIDAK PERNAH DILAPORKAN (kolom
	// migrasi 000007), BUKAN nol klien.
	WSClientCount *int

	// SharedTimeOnlyLink benar bila emisi HANYA-WAKTU yang sama juga diklaim revisi
	// lain. Bukan galat dan bukan alasan membuang salah satu tautan: sebuah baris
	// pra-000008 tidak membawa event_revision sama sekali, jadi dua transisi yang
	// berjarak lebih dekat daripada toleransi memang tak terpisahkan olehnya.
	// Ditandai supaya pembaca tidak menghitung satu emisi sebagai dua.
	SharedTimeOnlyLink bool
}

// ---------------------------------------------------------------------------
// Perakitan
// ---------------------------------------------------------------------------

// TimelineWindowBounds mengembalikan rentang received_ts yang HARUS dibaca pada
// sensor_observations untuk seluruh riwayat sebuah event.
//
// Fungsi terpisah, bukan komentar, dengan alasan yang sama seperti
// TraceWindowBounds: salah menghitungnya adalah cara paling mudah membuat laporan
// ini berbohong. Tepinya diturunkan dari revisi PERTAMA dan TERAKHIR:
//
//	fromTS = decided_at revisi pertama - (CorrelationWindowMs + tol)
//	toTS   = decided_at revisi terakhir + tol
//
// Diambil dari MINIMUM dan MAKSIMUM decided_at, bukan dari elemen pertama dan
// terakhir slice: urutannya revision ASC, dan revisi yang lebih tinggi selalu
// diputuskan lebih akhir pada satu proses — tetapi baris yang dibaca dari basis
// data tidak berutang jaminan itu kepada siapa pun, dan sebuah asumsi urutan yang
// tidak dijamin adalah tepatnya jenis kesalahan yang tidak akan terlihat.
//
// ok salah berarti riwayatnya kosong: tidak ada jendela yang dapat dihitung, dan
// nol observasi adalah jawaban yang BENAR — bukan galat, dan bukan bukti bahwa
// tidak ada observasi.
func TimelineWindowBounds(hist []store.EventStateLog, p TraceProfile) (fromTS, toTS int64, ok bool) {
	if len(hist) == 0 {
		return 0, 0, false
	}
	tol := p.linkTolerance()
	win := p.Options.CorrelationWindowMs + tol
	lo, hi := hist[0].DecidedAt, hist[0].DecidedAt
	for _, r := range hist[1:] {
		if r.DecidedAt < lo {
			lo = r.DecidedAt
		}
		if r.DecidedAt > hi {
			hi = r.DecidedAt
		}
	}
	return lo - win, hi + tol, true
}

// TimelineContributorNodes mengembalikan gabungan node_id di seluruh
// contributors[] pada riwayat sebuah event, terurut dan unik.
//
// Ada supaya pemanggil dapat membangun kueri observasinya TANPA menguraikan
// evidence_summary sendiri: satu penafsir evidence_summary di dalam paket ini,
// bukan dua yang dapat menyimpang. Baris yang evidence_summary-nya tidak terurai
// tidak menyumbang node — dan itu benar, bukan diam-diam: BuildTimeline
// melaporkannya sebagai revisi tak-teratribusi dengan NoContributors.
func TimelineContributorNodes(hist []store.EventStateLog) []string {
	seen := make(map[string]struct{}, 4)
	for _, r := range hist {
		ev, err := historicEvidence(r.EvidenceSummary)
		if err != nil {
			continue
		}
		for _, c := range ev.Contributors {
			seen[c.NodeID] = struct{}{}
		}
	}
	return sortedKeys(seen)
}

// BuildTimeline merakit garis waktu forensik satu event. HANYA MENGHITUNG —
// tidak ada I/O, tidak ada Tracker, tidak ada jam. Keempat masukan sudah dibaca
// pemanggil.
//
// Kontrak masukan, dan melanggarnya membuat laporannya berbohong:
//
//	row   — baris earthquake_events, atau nil bila tidak ada. nil BUKAN galat.
//	hist  — SELURUH baris event_state_log event ini, revision ASC
//	        (store.ListStateLogForEvent sudah memberikannya demikian). Sebagian
//	        riwayat akan terlihat seperti riwayat yang berlubang.
//	obs   — observasi milik node kontributor pada rentang TimelineWindowBounds.
//	        Lebih sempit akan membuat kandidat yang benar-benar ada tampak hilang;
//	        karena itu perhitungan tepinya ada di TimelineWindowBounds, bukan di
//	        kepala pemanggil.
//	emis  — OPSIONAL. nil berarti bagian emisi tidak diminta, dan itu tidak pernah
//	        memengaruhi keempat keluaran wajib (D-015 batasan 1).
//
// Yang dilakukannya dua arah, dan keduanya wajib:
//
//	transisi -> observasi : untuk setiap revisi, baris mana yang memenuhi
//	                        keanggotaan-dan-waktu. Ini keluaran keempat kriteria.
//	observasi -> transisi : sebuah baris yang menjadi kandidat lebih dari satu
//	                        revisi dilabeli AMBIGUOUS. Jendela dua revisi
//	                        berdekatan memang bertumpang-tindih, jadi ini keadaan
//	                        yang DIHARAPKAN — dan menyembunyikannya dengan memilih
//	                        yang terdekat akan mengarang sebuah sebab.
func BuildTimeline(
	eventID string,
	row *store.EarthquakeEvent,
	hist []store.EventStateLog,
	obs []store.ReplayObservation,
	emis []store.TraceEmission,
	p TraceProfile,
) *EventTimeline {
	tol, prov := EffectiveLinkTolerance(p)
	win := p.Options.CorrelationWindowMs + tol

	tl := &EventTimeline{
		EventID: eventID,
		Event:   row,
		Coverage: TimelineCoverage{
			CorrelationWindowMs: p.Options.CorrelationWindowMs,
			LinkToleranceMs:     tol,
			ToleranceProvenance: prov,
			AlgoVerBinary:       AlgoVerBase(),
			StateLogRows:        len(hist),
			ObservationRowsRead: len(obs),
		},
	}

	tl.Coverage.EventRowStatus = OutputObserved
	if row == nil {
		tl.Coverage.EventRowStatus = OutputNotObservable
	}

	// --- keluaran 2 + 3: riwayat dan bukti per revisi ------------------------
	//
	// Diurai SEKALI di sini, dan hasilnya yang dipakai kedua arah. Menguraikannya
	// dua kali membuka jalan bagi dua penafsiran evidence_summary di dalam satu
	// laporan.
	nodeSeen := make(map[string]struct{}, 4)
	algoSeen := make(map[string]struct{}, 2)
	tl.Revisions = make([]RevisionEntry, 0, len(hist))
	for _, r := range hist {
		e := RevisionEntry{
			Row:          r,
			WindowFromTS: r.DecidedAt - win,
			WindowToTS:   r.DecidedAt + tol,
		}
		ev, err := historicEvidence(r.EvidenceSummary)
		if err == nil {
			e.Evidence = ev
			e.EvidenceParsed = true
			tl.Coverage.RevisionsWithEvidence++
			for _, c := range ev.Contributors {
				e.ContributorNodes = append(e.ContributorNodes, c.NodeID)
				nodeSeen[c.NodeID] = struct{}{}
			}
			sort.Strings(e.ContributorNodes)
		} else {
			e.EvidenceError = err.Error()
			tl.Coverage.RevisionsEvidenceBroken++
		}
		if r.AlgoVer != "" {
			algoSeen[r.AlgoVer] = struct{}{}
		}
		if isTerminal(State(r.ToState)) {
			tl.Coverage.TerminalState = r.ToState
		}
		tl.Revisions = append(tl.Revisions, e)
	}
	tl.Coverage.AlgoVersRow = sortedKeys(algoSeen)
	tl.Coverage.ContributorNodes = len(nodeSeen)
	tl.Coverage.SingleNodeContributors = len(nodeSeen) == 1
	tl.Coverage.StateLogStatus = statusFromCount(len(hist))
	tl.Coverage.EvidenceStatus = evidenceStatus(len(hist), tl.Coverage.RevisionsWithEvidence)
	tl.Coverage.FirstRevision, tl.Coverage.LastRevision, tl.Coverage.RevisionGaps = revisionSpan(hist)

	// --- keluaran 4: observasi yang berkontribusi ----------------------------
	//
	// attributedTo dibangun LEBIH DULU untuk seluruh baris, sebelum satu pun
	// kandidat dicatat, karena label TRACED dan AMBIGUOUS hanya dapat diputuskan
	// setelah SEMUA revisi diperiksa. Memutuskannya di dalam lingkaran per revisi
	// akan melabeli baris yang sama dua kali dengan dua jawaban.
	attributedTo := make(map[int64][]int, len(obs))
	for _, o := range obs {
		for i := range tl.Revisions {
			e := &tl.Revisions[i]
			if !e.EvidenceParsed {
				continue
			}
			if !containsString(e.ContributorNodes, o.NodeID) {
				continue
			}
			if o.ReceivedTS < e.WindowFromTS || o.ReceivedTS > e.WindowToTS {
				continue
			}
			attributedTo[o.ObservationID] = append(attributedTo[o.ObservationID], e.Row.Revision)
		}
	}

	// Penyaring diterapkan SETELAH keanggotaan, tidak sebelumnya: sebuah baris yang
	// dibuang konsensus tetap harus terlihat sebagai baris yang ADA pada revisi ini,
	// jika tidak "tanpa kandidat" akan terbaca sebagai "tidak ada observasi".
	uniq := make(map[int64]*ObservationCandidate, len(obs))
	for _, o := range obs {
		revs := attributedTo[o.ObservationID]
		if len(revs) == 0 {
			continue
		}
		if o.PGAGal < MinPGAGal {
			for _, rev := range revs {
				if e := revisionByNumber(tl.Revisions, rev); e != nil {
					e.BelowFloor++
				}
			}
			tl.Coverage.BelowFloorRows++
			continue
		}
		if reason, excluded := excludeReason(o); excluded {
			ex := ExcludedRow{
				SkippedRow: SkippedRow{ObservationID: o.ObservationID, NodeID: o.NodeID, Reason: reason},
				PGAGal:     o.PGAGal,
				ReceivedTS: o.ReceivedTS,
			}
			for _, rev := range revs {
				if e := revisionByNumber(tl.Revisions, rev); e != nil {
					e.ExcludedCandidates = append(e.ExcludedCandidates, ex)
				}
			}
			tl.Coverage.ExcludedRows++
			continue
		}

		attrib := TraceTraced
		if len(revs) > 1 {
			attrib = TraceAmbiguous
			tl.Coverage.AmbiguousCandidates++
		}
		for _, rev := range revs {
			e := revisionByNumber(tl.Revisions, rev)
			if e == nil {
				continue
			}
			c := ObservationCandidate{
				ObservationID: o.ObservationID,
				NodeID:        o.NodeID,
				PGAGal:        o.PGAGal,
				ReceivedTS:    o.ReceivedTS,
				ObsSeq:        o.ObsSeq,
				LagMs:         e.Row.DecidedAt - o.ReceivedTS,
				ObsSeqLink:    obsSeqLink(o.ObsSeq, recordedObsSeq(e.Evidence, o.NodeID)),
				Attribution:   attrib,
				AttributedTo:  revs,
			}
			e.Candidates = append(e.Candidates, c)
			tl.Coverage.CandidateRows++
			if _, ok := uniq[o.ObservationID]; !ok {
				flat := c
				uniq[o.ObservationID] = &flat
			}
		}
	}

	// Observations adalah daftar DATAR kandidat unik. LagMs dan ObsSeqLink-nya
	// milik revisi PERTAMA tempat ia muncul, dan itu tidak cukup untuk kasus
	// ambigu — karena itu AttributedTo ikut, dan bacaan per revisi tetap ada di
	// Revisions[i].Candidates. Yang satu tidak menggantikan yang lain.
	tl.Observations = make([]ObservationCandidate, 0, len(uniq))
	for _, o := range obs {
		if c, ok := uniq[o.ObservationID]; ok {
			tl.Observations = append(tl.Observations, *c)
			delete(uniq, o.ObservationID)
		}
	}
	tl.Coverage.ObservationsStatus = statusFromCount(len(tl.Observations))

	// --- arah kebalikan: revisi tanpa satu pun kandidat ---------------------
	for i := range tl.Revisions {
		e := &tl.Revisions[i]
		if len(e.Candidates) > 0 {
			continue
		}
		u := UnattributedRevision{
			Revision:             e.Row.Revision,
			ToState:              e.Row.ToState,
			Reason:               e.Row.Reason,
			DecidedAt:            e.Row.DecidedAt,
			NodeIDs:              e.ContributorNodes,
			NoContributors:       len(e.ContributorNodes) == 0,
			MemberRowsFiltered:   e.BelowFloor + len(e.ExcludedCandidates),
			NotObservationDriven: notObservationDriven(e.Row.Reason),
		}
		tl.Unattributed = append(tl.Unattributed, u)
	}
	tl.Coverage.UnattributedRevisions = len(tl.Unattributed)

	// --- keluaran kelima, OPSIONAL ------------------------------------------
	if emis != nil {
		tl.Emissions = linkRevisionEmissions(tl.Revisions, emis, tol)
	}

	return tl
}

// ---------------------------------------------------------------------------
// Pembantu
// ---------------------------------------------------------------------------

// statusFromCount menerjemahkan jumlah baris menjadi status keluaran.
//
// Nol menjadi EMPTY, bukan NOT_OBSERVABLE, dan pembedaan itu inti aturan D-015:
// pembacaannya BERHASIL dan hasilnya nol baris. Yang boleh disebut
// NOT_OBSERVABLE hanyalah hal yang tidak dapat diamati sama sekali.
func statusFromCount(n int) string {
	if n == 0 {
		return OutputEmpty
	}
	return OutputObserved
}

// evidenceStatus melaporkan ketersediaan keluaran KETIGA.
//
// Ia bukan salinan statusFromCount: sebuah riwayat dapat punya baris sementara
// TIDAK SATU PUN evidence_summary-nya terurai, dan keadaan itu bukan EMPTY —
// kolomnya ada, hanya tidak dapat dibaca. NOT_OBSERVABLE adalah laporan yang
// benar untuk itu, dan angka pastinya ada di RevisionsEvidenceBroken.
func evidenceStatus(rows, parsed int) string {
	switch {
	case rows == 0:
		return OutputEmpty
	case parsed == 0:
		return OutputNotObservable
	default:
		return OutputObserved
	}
}

// revisionSpan mengembalikan revisi terkecil, terbesar, dan nomor yang HILANG di
// antaranya.
//
// Lubang dihitung terhadap rentang yang BENAR-BENAR ADA, bukan terhadap nol:
// sebuah event menjadi durable pada transisi PUBLIK pertamanya, jadi riwayat yang
// dimulai pada revisi 2 adalah hal yang normal (§9.5) dan menyebutnya lubang akan
// melaporkan kehilangan yang tidak pernah terjadi. Sebaliknya sebuah nomor yang
// hilang DI DALAM rentang adalah bukti nyata: UNIQUE (event_id, revision) menjamin
// revisi tidak berulang dan Revision naik satu per transisi, jadi lubangnya adalah
// satuan persistensi yang dibuang (D17/D30).
func revisionSpan(hist []store.EventStateLog) (first, last int, gaps []int) {
	if len(hist) == 0 {
		return 0, 0, nil
	}
	seen := make(map[int]struct{}, len(hist))
	first, last = hist[0].Revision, hist[0].Revision
	for _, r := range hist {
		seen[r.Revision] = struct{}{}
		if r.Revision < first {
			first = r.Revision
		}
		if r.Revision > last {
			last = r.Revision
		}
	}
	for rev := first + 1; rev < last; rev++ {
		if _, ok := seen[rev]; !ok {
			gaps = append(gaps, rev)
		}
	}
	return first, last, gaps
}

// revisionByNumber mencari entri menurut nomor revisi.
//
// Pencarian linear, bukan map, dan itu bukan kelalaian: jumlah revisi satu event
// adalah satuan, dan sebuah map dari revisi ke pointer akan menjadi struktur kedua
// yang harus tetap sinkron dengan slice-nya. Mengembalikan pointer supaya
// pemanggil menyunting entri yang SAMA, bukan salinannya.
func revisionByNumber(revs []RevisionEntry, revision int) *RevisionEntry {
	for i := range revs {
		if revs[i].Row.Revision == revision {
			return &revs[i]
		}
	}
	return nil
}

// recordedObsSeq mengambil obs_seq yang TERCATAT pada kontributor sebuah node.
// nil berarti node itu tidak ada di contributors[] atau barisnya protokol v1 —
// dua sebab yang keduanya sah dan keduanya menghasilkan UNAVAILABLE_V1.
func recordedObsSeq(ev EvidenceSummary, nodeID string) *int64 {
	for _, c := range ev.Contributors {
		if c.NodeID == nodeID {
			return c.ObsSeq
		}
	}
	return nil
}

// containsString adalah keanggotaan pada slice pendek yang sudah terurut.
// Linear karena contributors[] satu revisi berjumlah satuan; sebuah pencarian
// biner di sini hanya menambah cara untuk salah.
func containsString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// notObservationDriven melaporkan apakah sebuah reason menandai transisi yang
// TIDAK dipicu observasi yang tiba.
//
// Ketiganya lahir tanpa bukti baru: NO_NEW_EVIDENCE dari sweep dan rekonsiliasi
// (§15.3, sweep.go), EVIDENCE_INVALIDATED dan OPERATOR_RETRACTION dari pencabutan
// kontributor (InvalidateContributor). Untuk baris seperti ini jendela kandidat
// yang kosong adalah hal yang DIHARAPKAN — decided_at-nya jatuh ResolveAfterMs
// setelah observasi terakhir — dan melaporkannya sebagai observasi yang hilang
// akan menuduh ledger atas sesuatu yang dilakukan penjadwal.
func notObservationDriven(reason string) bool {
	switch reason {
	case ReasonNoNewEvidence, ReasonEvidenceInvalid, ReasonOperatorRetracted:
		return true
	default:
		return false
	}
}

// linkRevisionEmissions menautkan setiap revisi ke baris alert_emissions-nya.
//
// Arahnya kebalikan linkEmission M1': di sana satu transisi terpilih dicarikan
// emisinya, di sini SETIAP revisi riwayat dilewati. Kosakata hasilnya sengaja
// dipakai ulang apa adanya — EmissionByID, EmissionByTimeOnly, EmissionMissing —
// supaya satu istilah berarti satu hal di kedua alat, dan aturan "eksak lebih
// dulu, eksak menang" juga dipertahankan.
//
// Bagian ini OPSIONAL menurut D-015. Ia keluaran KELIMA, bukan salah satu dari
// empat, dan tidak boleh menjadi penentu lulus atau tidaknya M6'. Sebuah baris
// MISSING karena itu adalah laporan, bukan temuan cacat: emisi dibatasi audiens
// dan status, dan tidak setiap revisi memang menghasilkan satu.
func linkRevisionEmissions(revs []RevisionEntry, emis []store.TraceEmission, tol int64) []RevisionEmission {
	if len(revs) == 0 {
		return nil
	}
	out := make([]RevisionEmission, 0, len(revs))
	for i := range revs {
		r := revs[i].Row
		re := RevisionEmission{
			Revision:  r.Revision,
			ToState:   r.ToState,
			DecidedAt: r.DecidedAt,
		}

		// Eksak lebih dulu, dan eksak menang: bila satu baris membawa event_id dan
		// event_revision yang cocok, kecocokan waktu pada baris lain tidak relevan.
		for _, e := range emis {
			if e.EventID == nil || e.EventRevision == nil {
				continue
			}
			if *e.EventID == r.EventID && *e.EventRevision == r.Revision {
				re.Outcome = EmissionByID
				re.EmissionID = e.EmissionID
				re.AlertType = e.AlertType
				re.WSClientCount = e.WSClientCount
				break
			}
		}

		if re.Outcome == "" {
			best := int64(-1)
			for _, e := range emis {
				if e.EventID != nil && e.EventRevision != nil {
					continue // baris beridentitas yang TIDAK cocok; bukan kandidat waktu
				}
				off := absInt64(e.DecidedAt - r.DecidedAt)
				if off > tol {
					continue
				}
				if best < 0 || off < best {
					best = off
					re.Outcome = EmissionByTimeOnly
					re.EmissionID = e.EmissionID
					re.AlertType = e.AlertType
					re.WSClientCount = e.WSClientCount
				}
			}
		}

		if re.Outcome == "" {
			re.Outcome = EmissionMissing
		}
		out = append(out, re)
	}

	// Tautan hanya-waktu yang DIBAGI ditandai pada kedua sisinya. Dikerjakan
	// setelah seluruh revisi tertaut karena "dibagi" hanya dapat diketahui setelah
	// semuanya terlihat — persis alasan yang sama dengan attributedTo.
	shared := make(map[int64]int, len(out))
	for _, m := range out {
		if m.Outcome == EmissionByTimeOnly {
			shared[m.EmissionID]++
		}
	}
	for i := range out {
		if out[i].Outcome == EmissionByTimeOnly && shared[out[i].EmissionID] > 1 {
			out[i].SharedTimeOnlyLink = true
		}
	}
	return out
}
