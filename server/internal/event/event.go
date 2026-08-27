// Package event memiliki identitas dan siklus hidup event gempa: korelasi
// observasi ke event, mesin keadaan lima state, dan emisi peringatan pada
// transisi.
//
// Kewenangan ada DI MEMORI, dengan basis data sebagai pengikut. Alasannya sama
// dengan alasan ledger boleh gagal (internal/ledger): keputusan peringatan tidak
// boleh menunggu satu pun INSERT. Konsekuensinya dinyatakan terbuka — sebuah
// event DETECTED tidak dapat direkonstruksi setelah restart, dan itu memang
// pilihannya (§9.5).
//
// Paket ini mengimpor internal/consensus sebagai pustaka geometri dan intensitas
// (§11.3): WeightedCentroid, MMIFromPGA, Intensity, HaversineKm dan kawan-kawan
// dipakai APA ADANYA. Tidak ada seismologi yang berpindah pada Fase 3.
package event

import (
	"github.com/banana-pixel/quakealert/server/internal/consensus"
)

// Ambang keputusan, SENGAJA konstanta compile-time. Sebuah ambang keselamatan
// yang dapat diturunkan operator lewat environment variable adalah ambang yang
// akan diturunkan saat insiden. Didefinisikan sebagai alias nilai di paket
// consensus supaya hanya ada satu definisi yang dapat menyimpang.
const (
	MinPGAGal         = consensus.MinPGAGal
	MinNodesConfirmed = consensus.MinNodesConfirmed
)

// State adalah posisi siklus hidup sebuah event. Nilainya identik dengan kolom
// earthquake_events.event_state.
type State string

const (
	// StateDetected: ada bukti, belum memenuhi lantai emisi. TIDAK publik.
	// Dibutuhkan agar kluster di bawah ambang tetap TERHITUNG — ia penyebut dari
	// setiap laju false-positive yang akan pernah diinginkan proyek ini.
	StateDetected State = "DETECTED"
	// StateUnconfirmed: dipublikasikan, keyakinan rendah. Menggantikan ADVISORY
	// dan memberinya event_id.
	StateUnconfirmed State = "UNCONFIRMED"
	// StateConfirmed: kuorum DAN independensi terpenuhi.
	StateConfirmed State = "CONFIRMED"
	// StateResolved: getaran berakhir; event tetap dianggap nyata.
	StateResolved State = "RESOLVED"
	// StateCancelled: tafsirnya ditarik. Berbeda dari RESOLVED karena "sudah
	// berhenti" dan "tidak pernah terjadi" adalah dua hal berbeda bagi seseorang.
	StateCancelled State = "CANCELLED"
)

// Kosakata tertutup kolom event_state_log.reason. Bukan teks bebas: tabel ini ada
// untuk diagregasi, dan teks bebas tidak dapat diagregasi.
const (
	ReasonFirstObservation  = "FIRST_OBSERVATION"
	ReasonFloorMet          = "FLOOR_MET"
	ReasonQuorumMet         = "QUORUM_MET"
	ReasonNoNewEvidence     = "NO_NEW_EVIDENCE"
	ReasonEvidenceInvalid   = "EVIDENCE_INVALIDATED"
	ReasonOperatorRetracted = "OPERATOR_RETRACTION"
)

// Asal onset sebuah observasi. Nilainya identik dengan
// sensor_observations.onset_source; event_drift_test.go menjaga agar salinan di
// sini tidak menyimpang dari paket ledger.
const (
	OnsetSourceSensor  = "SENSOR"
	OnsetSourcePublish = "PUBLISH_BOUND"
)

// Phase sebuah observasi (v1 selalu FINAL).
const (
	PhasePrelim = "PRELIM"
	PhaseFinal  = "FINAL"
)

// legalTransitions memuat TEPAT transisi yang diizinkan §5.2. Ditulis sebagai
// data, bukan sebagai rantai if, supaya seluruh 25 pasangan terurut dapat
// diperiksa satu per satu oleh uji.
//
// Dua ketiadaan di tabel ini menanggung beban:
//
//	CONFIRMED -> UNCONFIRMED tidak ada. Konfirmasi publik adalah klaim yang
//	ditindaklanjuti orang secara fisik — mereka berpindah, berhenti menyetir.
//	Menurunkannya diam-diam mengajari klien bahwa keyakinan berosilasi dan tidak
//	memberi UI perilaku yang benar. Bila konfirmasi ternyata salah, tindakan yang
//	jujur adalah PENARIKAN (CANCELLED), yang mengatakannya.
//
//	DETECTED -> RESOLVED tidak ada. Event yang belum pernah publik tidak
//	"selesai"; ia KEDALUWARSA (§5.2) dan dipersistensi sekali dengan state
//	DETECTED. Mengumumkan all-clear untuk sesuatu yang tidak pernah diumumkan
//	akan menjadi frame pertama DAN terakhir tentang event itu.
var legalTransitions = map[State]map[State]bool{
	StateDetected: {
		StateUnconfirmed: true,
		StateConfirmed:   true,
		StateCancelled:   true,
	},
	StateUnconfirmed: {
		StateConfirmed: true,
		StateResolved:  true,
		StateCancelled: true,
	},
	StateConfirmed: {
		StateResolved:  true,
		StateCancelled: true,
	},
	// RESOLVED dan CANCELLED tidak punya keluaran. AlertDedup di klien
	// memakai kunci TYPE:event_id dan sudah mengonsumsi EVENT_RESOLVED:<id>;
	// alarm kedua pada id yang sama akan dibuang setiap klien di lapangan.
	StateResolved:  {},
	StateCancelled: {},
}

// legal melaporkan apakah from -> to adalah transisi yang sah. Transisi ke state
// yang sama BUKAN transisi dan karena itu tidak sah.
func legal(from, to State) bool {
	return legalTransitions[from][to]
}

// isTerminal melaporkan apakah state tidak punya transisi keluar.
func isTerminal(s State) bool {
	return s == StateResolved || s == StateCancelled
}

// isPublic melaporkan apakah state pernah terlihat klien.
func isPublic(s State) bool {
	return s != StateDetected
}

// Contributor adalah satu node yang menyumbang bukti ke sebuah event. Dikunci
// pada node_id: PRELIM dan FINAL dari satu node adalah SATU kontributor, satu
// suara.
type Contributor struct {
	NodeID       string
	Lat          float64
	Lon          float64
	LocationName string

	// Cell adalah sel independensi node, dihitung sekali saat pertama menempel.
	Cell cellKey

	// ObsSeq adalah episode yang tercatat (nil untuk v1, yang tidak punya).
	ObsSeq *int64

	Phase   string
	PeakPGA float64

	// OnsetTS DITULIS SEKALI dan tidak pernah dipindahkan — first-bound-wins
	// (D29). Untuk v2 ini formalitas: onset ada di dalam string kanonik yang
	// ditandatangani, jadi setiap revisi episode membawa nilai yang sama. Untuk v1
	// ini pilihan nyata: onset v1 adalah publish_ts - dur_ms, sebuah BATAS ATAS
	// yang galatnya adalah keterlambatan publish, dan ts distempel ulang pada
	// setiap retry — jadi setiap kedatangan berikutnya membawa batas yang LEBIH
	// LONGGAR. Batas pertama adalah yang terketat yang akan pernah ditawarkan node
	// itu; last-wins akan menggerus jangkar setiap kali ada retry.
	OnsetTS     int64
	OnsetSource string

	LastPublishTS int64
	DetriggerTS   *int64

	// Revisions adalah jumlah observasi yang diserap kontributor ini, termasuk
	// yang pertama. Untuk diagnosis, tidak pernah untuk keputusan.
	Revisions int
}

// Event adalah satu gempa sebagaimana dipahami server: identitas, jangkar waktu,
// himpunan kontributor, dan posisi siklus hidup.
type Event struct {
	ID    string
	State State

	// Revision naik HANYA pada transisi state, bukan pada setiap observasi.
	// Penyempitan yang disengaja: counter yang bergerak tiap observasi akan
	// menghasilkan log transisi yang didominasi baris from_state = to_state, dan
	// membuat alert_emissions.event_revision tak berguna sebagai pegangan korelasi.
	Revision int

	// OriginTS adalah onset yang MEMBUAT event ini, disetel sekali dan tidak pernah
	// dipindahkan. Menghitungnya ulang sebagai min(onset) lebih akurat secara fisik
	// dan tetap ditolak: jendela diukur DARI OriginTS, jadi membiarkannya bergeser
	// membiarkan event berjalan melintasi peta satu observasi terlambat sekaligus —
	// single linkage pada sumbu waktu.
	OriginTS       int64
	OriginTSSource string

	// DecidedAt adalah jam server saat state sekarang terbentuk.
	DecidedAt int64
	// LastEvidenceTS adalah jam server observasi penyumbang terakhir; tenggat
	// resolusi diturunkan dari sini, bukan dari timer hidup.
	LastEvidenceTS int64
	CreatedAt      int64

	Contributors map[string]*Contributor

	// Invalidated disetel oleh InvalidateContributor bila SELURUH bukti ditarik.
	Invalidated bool

	// EverConfirmed menentukan apakah RESOLVED/CANCELLED berhak atas FCM (§8.1):
	// all-clear hanya diutangkan kepada audiens yang menerima alarmnya.
	EverConfirmed bool

	// TerminalAt adalah DecidedAt saat event menjadi terminal; nol bila masih
	// terbuka. Tombstone dihapus sweeper ketika now - TerminalAt melewati
	// TerminalRetentionMs (§6.8).
	TerminalAt int64

	// Kunci indeks tempat event ini terdaftar. Disimpan supaya sweeper — satu-satunya
	// pemilik penghapusan — dapat mencabut entri tanpa menyapu seluruh indeks.
	lookupKey cellKey
	bucket    int64

	// minCells adalah MIN_INDEPENDENT_CELLS yang berlaku, disalin dari Tracker saat
	// event dibuat supaya classify tetap murni (lihat classify.go).
	minCells int

	// anchor adalah agregat yang dibaca dari BARIS earthquake_events, dipakai HANYA
	// bila event tidak punya satu pun kontributor. Itu terjadi tepat pada satu
	// jalur: event yang dimuat ulang saat boot tanpa baris event_state_log untuk
	// direkonstruksi (§15.3) — baris pra-Fase-3, atau baris yang satuan
	// persistensinya dibuang (D30).
	//
	// Ada karena alternatifnya lebih buruk. Tanpa jangkar, event seperti itu punya
	// centroid nol, sehingga bukti yang datang setelah restart tidak akan
	// mencocokinya dan akan membentuk event_id KEDUA untuk gempa yang sama — tepat
	// pembelahan yang §15.3 langkah 4 ada untuk mencegah. Ia BUKAN kontributor
	// palsu: tidak ada node_id yang dikarang, tidak ada suara yang ditambahkan, dan
	// classify tetap melihat nol kontributor sehingga tidak ada state yang dapat
	// dinaikkan olehnya.
	anchor *reloadAnchor
}

// nodeCount adalah jumlah kontributor, dengan agregat baris sebagai cadangan bagi
// event yang dimuat ulang tanpa bukti: "0 node" pada frame all-clear akan
// membantah baris yang memicunya.
func (e *Event) nodeCount() int {
	if len(e.Contributors) == 0 && e.anchor != nil {
		return e.anchor.NodeCount
	}
	return len(e.Contributors)
}

// peakPGA adalah PGA tertinggi di antara kontributor, sama artinya dengan MaxPGA
// hari ini.
func (e *Event) peakPGA() float64 {
	if len(e.Contributors) == 0 && e.anchor != nil {
		return e.anchor.PeakPGA
	}
	var max float64
	for _, c := range e.Contributors {
		if c.PeakPGA > max {
			max = c.PeakPGA
		}
	}
	return max
}

// independentCells menghitung sel independensi yang berbeda di antara kontributor.
func (e *Event) independentCells() int {
	if len(e.Contributors) == 0 && e.anchor != nil {
		return e.anchor.Cells
	}
	seen := make(map[cellKey]struct{}, len(e.Contributors))
	for _, c := range e.Contributors {
		seen[c.Cell] = struct{}{}
	}
	return len(seen)
}

// readings membangun []consensus.Reading agar helper geometri paket consensus
// dapat dipakai apa adanya.
func (e *Event) readings() []consensus.Reading {
	out := make([]consensus.Reading, 0, len(e.Contributors))
	for _, c := range e.Contributors {
		out = append(out, consensus.Reading{
			NodeID: c.NodeID, Lat: c.Lat, Lon: c.Lon,
			PGA: c.PeakPGA, TS: c.OnsetTS, LocationName: c.LocationName,
		})
	}
	return out
}

// centroid adalah pusat massa stasiun berbobot PGA — sebuah estimated_centroid,
// BUKAN episenter.
func (e *Event) centroid() consensus.Centroid {
	if len(e.Contributors) == 0 && e.anchor != nil {
		return consensus.Centroid{Lat: e.anchor.Lat, Lon: e.anchor.Lon}
	}
	return consensus.WeightedCentroid(e.readings())
}

// isTerminal melaporkan apakah event ini sudah mencapai state terminal.
func (e *Event) isTerminal() bool { return isTerminal(e.State) }

// matches adalah predikat kecocokan §4.3, satu-satunya penentu apakah sebuah
// observasi milik sebuah event. Indeks sel dan ember hanya membatasi himpunan
// kandidat; ia tidak pernah memutuskan.
//
// Terminal bukan diskualifikasi: selama sebuah event terminal masih dilacak
// sebagai tombstone (§6.8), observasi yang terlambat HARUS mencocokinya — kalau
// tidak, ia tidak mencocoki apa pun dan menciptakan event_id kedua untuk gempa
// yang sama, yang jauh lebih buruk daripada tidak mencatatnya.
//
// Kedua batas TERTUTUP: sebuah observasi tepat pada W atau tepat pada radius masih
// menempel. Batas tertutup dipilih karena kegagalannya asimetris — menolak satu
// observasi di pinggir dapat membelah satu gempa menjadi dua peringatan publik,
// sementara menerimanya hanya menambah satu kontributor pada event yang benar.
func matches(e *Event, onsetTS int64, lat, lon float64, windowMs int64, radiusKm float64) bool {
	d := onsetTS - e.OriginTS
	if d < 0 {
		d = -d
	}
	if d > windowMs {
		return false
	}
	c := e.centroid()
	return consensus.HaversineKm(lat, lon, c.Lat, c.Lon) <= radiusKm
}
