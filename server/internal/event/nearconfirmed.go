package event

import (
	"context"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// NearConfirmedEntry mencatat satu event yang pernah mencapai >= 2 kontributor
// independen tanpa (atau sebelum) dikonfirmasi. Dipakai untuk menjawab
// pertanyaan operasional pasca-kejadian:
//
//   - Berapa event yang macet di >= 2 independen?
//   - Berapa lama mereka macet?
//   - Berapa yang akhirnya CONFIRMED?
//   - Berapa yang mati tanpa konfirmasi?
//
// Semua stempel waktu adalah Unix millisecond (sama dengan OriginTS / DecidedAt
// di seluruh paket ini).
//
// Entry TIDAK dihapus saat event menjadi tombstone atau dievakuasi dari Tracker:
// tujuannya rekonstruksi pasca-kejadian, sehingga riwayat harus tetap ada.
//
// Sejak P4-M2′ (D-012) entri juga DURABLE: setiap perubahan diantre ke
// event_near_confirmed lewat antrean ledger, dan seluruh tabel dibaca kembali
// saat boot. Peta di memori tetap otoritasnya (§9.5) — tabel adalah pengikut —
// tetapi jawabannya kini tidak lagi lahir dan mati bersama satu proses.
type NearConfirmedEntry struct {
	EventID string `json:"event_id"`

	// FirstTwoIndependentAt adalah waktu pertama kali event ini memiliki >= 2
	// kontributor independen (independentCells() >= minIndependentCells). Selalu
	// terisi bila entry ada.
	FirstTwoIndependentAt int64 `json:"first_two_independent_at_ms"`

	// IndependentCountAtPeak adalah nilai independentCells() pada saat
	// FirstTwoIndependentAt dicatat. Berguna untuk membedakan "pas 2" dari
	// "sudah 3 tapi tidak kuorum".
	IndependentCountAtPeak int `json:"independent_count_at_peak"`

	// NodeCountAtPeak adalah len(Contributors) saat FirstTwoIndependentAt.
	NodeCountAtPeak int `json:"node_count_at_peak"`

	// ConfirmedAt > 0 berarti event ini akhirnya CONFIRMED. 0 berarti belum
	// atau tidak pernah.
	ConfirmedAt int64 `json:"confirmed_at_ms,omitempty"`

	// TerminalState adalah state terminal terakhir (RESOLVED atau CANCELLED).
	// Kosong berarti event masih terbuka saat snapshot diambil.
	TerminalState string `json:"terminal_state,omitempty"`

	// TerminalAt > 0 berarti event sudah terminal. 0 berarti masih terbuka.
	TerminalAt int64 `json:"terminal_at_ms,omitempty"`

	// MinIndependentCells adalah ambang yang BERLAKU saat persilangan dicatat,
	// dibawa apa adanya. Tanpanya "mencapai 2" tidak dapat ditafsirkan oleh
	// pembaca yang MIN_INDEPENDENT_CELLS-nya sudah berbeda — dan menghitungnya
	// ulang dari konfigurasi sekarang berarti menilai keputusan lampau dengan
	// parameter yang tidak menghasilkannya.
	MinIndependentCells int `json:"min_independent_cells"`

	// AlgoVer adalah label algoritma pada saat persilangan, memuat ic=<km>.
	// Sebuah hitungan independensi hanya dapat ditafsirkan bersama jarak
	// pemisahan yang menghasilkannya (V3/V6, D-006), dan ia TIDAK pernah ditulis
	// ulang oleh biner yang lebih baru.
	AlgoVer string `json:"algo_ver"`

	// Source adalah provenance entri INI, bukan mutu buktinya: apakah proses ini
	// benar-benar MENYAKSIKAN persilangannya, atau membacanya kembali dari tabel
	// durable saat boot. Perbedaan itu harus terlihat — sebuah entri yang dimuat
	// ulang tidak dapat menjawab pertanyaan yang membutuhkan bukti hidup, dan
	// sebuah jawaban yang menyembunyikan asalnya membuat kedua hal itu tampak
	// sama.
	Source string `json:"source"`

	// UpdatedInProcess benar bila entri yang DIMUAT dari tabel durable kemudian
	// masih berubah di proses ini (event menyeberangi restart lalu mendapat bukti
	// baru). Field terpisah, bukan Source yang berubah menjadi RECORDED: awal
	// entri tetap berasal dari basis data, dan menimpanya akan mengklaim
	// kesaksian yang tidak dimiliki proses ini.
	UpdatedInProcess bool `json:"updated_in_process,omitempty"`
}

// Nilai NearConfirmedEntry.Source. Kosakata tertutup dan kecil, seperti kosakata
// reason §5.3: ia ada supaya kolomnya dapat diagregasi.
const (
	// NearConfirmedSourceProcess — persilangan disaksikan proses ini.
	NearConfirmedSourceProcess = "RECORDED"
	// NearConfirmedSourceDurable — entri dibaca dari event_near_confirmed saat boot.
	NearConfirmedSourceDurable = "LOADED"
)

// NearConfirmedCoverage adalah selubung cakupan jawaban near-confirmed (B1,
// P4-M2′). Ia ada karena sebuah daftar KOSONG punya dua arti yang sangat berbeda,
// dan tanpa selubung ini keduanya terkirim sebagai byte yang identik:
//
//	"tidak ada satu pun event yang pernah melampaui ambang independensi"
//	"tidak ada yang dapat dijawab — peta baru dibangun ulang / pembacaan gagal"
//
// Pada fleet satu-node arti pertama adalah jawaban yang BENAR (S2: kuorum tidak
// terjangkau), jadi kriteria P4-M2′ justru menuntut keduanya dapat dibedakan.
//
// Bentuknya mengikuti TraceReport (P4-M1′) dengan sengaja, termasuk apa yang TIDAK
// ada di dalamnya: tidak ada field bernama Complete, Healthy, atau Valid. Ini
// pengukuran cakupan, bukan penilaian.
type NearConfirmedCoverage struct {
	// ProcessStartedAtMs dan AsOfMs adalah jendela yang DIJAMIN proses ini
	// sendiri: setiap persilangan di dalamnya disaksikan langsung.
	ProcessStartedAtMs int64 `json:"process_started_at_ms"`
	AsOfMs             int64 `json:"as_of_ms"`

	// Fakta pembacaan durable saat boot — dilaporkan, bukan disimpulkan.
	//
	// Awal cakupan durable TIDAK dapat diketahui dari dalam proses: tabelnya tidak
	// membawa penanda kapan ia mulai ditulis, jadi selubung ini melaporkan APA YANG
	// DIBACA alih-alih mengklaim sebuah titik mulai yang tidak dimilikinya.
	// DurableReadAttempted salah berarti Tracker berjalan tanpa toko yang dapat
	// membaca tabel itu — bukan berarti tabelnya kosong.
	DurableReadAttempted bool   `json:"durable_read_attempted"`
	DurableReadOK        bool   `json:"durable_read_ok"`
	DurableReadAtMs      int64  `json:"durable_read_at_ms,omitempty"`
	DurableRowsLoaded    int    `json:"durable_rows_loaded"`
	DurableReadError     string `json:"durable_read_error,omitempty"`

	// Pembilang provenance, dibawa supaya sebuah daftar nol-entri dapat dibedakan
	// dari daftar yang seluruh entrinya berasal dari satu sisi saja.
	EntriesRecordedInProcess int `json:"entries_recorded_in_process"`
	EntriesLoadedFromDurable int `json:"entries_loaded_from_durable"`

	// Parameter yang BERLAKU pada proses ini. Bukan parameter entri — masing-masing
	// entri membawa miliknya sendiri — melainkan parameter yang akan berlaku pada
	// persilangan berikutnya, sehingga pembaca dapat melihat bahwa keduanya
	// berbeda tanpa membandingkan entri satu per satu.
	AlgoVer             string `json:"algo_ver"`
	MinIndependentCells int    `json:"min_independent_cells"`
}

// NearConfirmedReport adalah jawaban lengkap endpoint near-confirmed: daftarnya
// beserta cakupan yang berlaku atasnya.
//
// Entries TETAP menjadi array tingkat atas dengan nama yang sama. Selubungnya
// ADITIF, bukan pembungkus: pembaca yang sudah ada membaca `.entries` seperti
// sebelumnya, dan yang berubah hanyalah bahwa sekarang ada tempat untuk menyatakan
// apa arti panjang nol.
type NearConfirmedReport struct {
	Entries  []NearConfirmedEntry  `json:"entries"`
	Coverage NearConfirmedCoverage `json:"coverage"`
}

// NearConfirmedLog mengembalikan salinan seluruh entri near-confirmed, diurutkan
// dari yang paling awal mencapai kondisi >= 2 independen. Aman dipanggil dari
// goroutine mana pun.
//
// "Stalled" (macet tanpa konfirmasi) = ConfirmedAt == 0.
// "Died without confirmation" = ConfirmedAt == 0 && TerminalAt > 0.
// "Eventually confirmed" = ConfirmedAt > 0.
// "Still open" = TerminalAt == 0.
func (t *Tracker) NearConfirmedLog() []NearConfirmedEntry {
	return t.NearConfirmedReport().Entries
}

// NearConfirmedReport mengembalikan daftar near-confirmed BESERTA selubung
// cakupannya (B1, P4-M2′). Satu-satunya pembeda antara daftar kosong yang berarti
// "tidak pernah ada persilangan" dan daftar kosong yang berarti "tidak ada yang
// dapat dijawab" ada di dalam Coverage, jadi keduanya diambil bersama-sama —
// bukan lewat dua panggilan yang dapat menggambarkan dua titik waktu berbeda.
func (t *Tracker) NearConfirmedReport() NearConfirmedReport {
	nowMs := t.now().UnixMilli()

	t.mu.Lock()
	out := make([]NearConfirmedEntry, 0, len(t.nearConfirmed))
	var recorded, loaded int
	for _, e := range t.nearConfirmed {
		cp := *e
		if cp.Source == NearConfirmedSourceDurable {
			loaded++
		} else {
			recorded++
		}
		out = append(out, cp)
	}
	cov := t.nearCoverage
	t.mu.Unlock()

	// Urutkan berdasarkan FirstTwoIndependentAt, lalu EventID untuk determinisme.
	sortNearConfirmed(out)

	// startedAtMs, opt dan algoVer() ditulis sekali saat konstruksi dan tidak
	// pernah bermutasi, jadi keduanya tidak memerlukan kunci.
	cov.ProcessStartedAtMs = t.startedAtMs
	cov.AsOfMs = nowMs
	cov.EntriesRecordedInProcess = recorded
	cov.EntriesLoadedFromDurable = loaded
	cov.AlgoVer = t.algoVer()
	cov.MinIndependentCells = t.opt.MinIndependentCells

	return NearConfirmedReport{Entries: out, Coverage: cov}
}

// recordNearConfirmedLocked dipanggil DI DALAM t.mu setiap kali independentCells
// event berubah. Ia mencatat entri baru bila event baru saja melampaui ambang,
// dan memperbarui ConfirmedAt / TerminalAt bila transisi state cocok.
//
// Dipanggil dari transitionLocked setelah classify, sehingga State sudah
// mencerminkan keputusan baru.
//
// MENGEMBALIKAN salinan entri bila entri itu benar-benar dibuat atau BERUBAH,
// dan nil bila tidak ada yang berubah. Nilai kembalian itu adalah keseluruhan
// jalur durable P4-M2′, dan bentuknya dipaksa oleh dua hal:
//
//   - Tracker tidak boleh melakukan I/O di bawah kunci. Jadi fungsi ini TIDAK
//     mengantre apa pun; ia hanya melaporkan apa yang perlu diantre, dan
//     pemanggilnya mengantrekannya SETELAH t.mu dilepas.
//   - Sebuah persilangan boleh terjadi TANPA transisi state (UNCONFIRMED ->
//     UNCONFIRMED ilegal, §5.2), sehingga tidak ada Snapshot dan tidak ada
//     EventUnit yang dapat membawanya. Kalau perubahan tidak dilaporkan di sini,
//     ia tidak dapat dilaporkan di mana pun.
//
// Salinan, bukan pointer: entri di dalam map masih akan berubah di bawah kunci
// setelah ini, dan sebuah pointer yang keluar dari kunci adalah baris yang
// dibaca goroutine drain sementara goroutine lain menulisinya.
func (t *Tracker) recordNearConfirmedLocked(e *Event, nowMs int64) *NearConfirmedEntry {
	indep := e.independentCells()
	threshold := minIndependentCells(e)

	entry, exists := t.nearConfirmed[e.ID]
	changed := false

	if !exists {
		// Hanya buat entri baru bila sudah memenuhi ambang independensi.
		// Terminal sebelum ambang (misalnya CANCELLED saat contributor < threshold)
		// tidak masuk log: event tidak pernah cukup mandiri.
		if indep < threshold {
			return nil
		}
		entry = &NearConfirmedEntry{
			EventID:                e.ID,
			FirstTwoIndependentAt:  nowMs,
			IndependentCountAtPeak: indep,
			NodeCountAtPeak:        len(e.Contributors),
			// Ambang dan versi algoritma direkam SEKALI, pada saat persilangan, dan
			// tidak pernah disegarkan sesudahnya: keduanya menjelaskan keputusan
			// yang sudah diambil.
			MinIndependentCells: threshold,
			AlgoVer:             t.algoVer(),
			Source:              NearConfirmedSourceProcess,
		}
		t.nearConfirmed[e.ID] = entry
		changed = true
	} else {
		// Entri sudah ada: perbarui puncak independensi jika naik.
		// Independensi boleh turun setelah invalidasi kontributor; puncak tetap.
		if indep > entry.IndependentCountAtPeak {
			entry.IndependentCountAtPeak = indep
			entry.NodeCountAtPeak = len(e.Contributors)
			changed = true
		}
	}

	// Catat CONFIRMED pertama kali.
	if e.State == StateConfirmed && entry.ConfirmedAt == 0 {
		entry.ConfirmedAt = nowMs
		changed = true
	}

	// Catat state terminal. Dilakukan meski indep kini < threshold (misalnya
	// kontributor diinvalidasi setelah entry sudah dibuat).
	if e.isTerminal() && entry.TerminalAt == 0 {
		entry.TerminalState = string(e.State)
		entry.TerminalAt = nowMs
		changed = true
	}

	if !changed {
		return nil
	}

	// Entri yang DIMUAT dari tabel durable lalu berubah di proses ini menandai
	// dirinya, tanpa mengubah Source: awalnya tetap berasal dari basis data, dan
	// menimpanya akan mengklaim kesaksian yang tidak dimiliki proses ini.
	if entry.Source == NearConfirmedSourceDurable {
		entry.UpdatedInProcess = true
	}

	cp := *entry
	return &cp
}

// nearRowFor menerjemahkan satu entri menjadi baris durable. Pointer NULL-able
// dibangun di sini, bukan di store: nol dan "tidak pernah terjadi" adalah dua hal
// berbeda, dan tempat yang tahu bedanya adalah tempat yang memegang aturannya.
func nearRowFor(e *NearConfirmedEntry) *store.NearConfirmedRow {
	r := &store.NearConfirmedRow{
		EventID:                e.EventID,
		FirstTwoIndependentAt:  e.FirstTwoIndependentAt,
		IndependentCountAtPeak: e.IndependentCountAtPeak,
		NodeCountAtPeak:        e.NodeCountAtPeak,
		MinIndependentCells:    e.MinIndependentCells,
		AlgoVer:                e.AlgoVer,
	}
	if e.ConfirmedAt > 0 {
		v := e.ConfirmedAt
		r.ConfirmedAt = &v
	}
	if e.TerminalAt > 0 {
		st, at := e.TerminalState, e.TerminalAt
		r.TerminalState = &st
		r.TerminalAt = &at
	}
	return r
}

// persistNearConfirmed mengantrekan sekumpulan entri yang berubah. WAJIB dipanggil
// di LUAR t.mu, sejajar dengan publish: itu satu-satunya alasan entri-entri ini
// dikumpulkan alih-alih dikirim di tempat.
func (t *Tracker) persistNearConfirmed(entries []NearConfirmedEntry) {
	if len(entries) == 0 || t.nearPersist == nil {
		return
	}
	for i := range entries {
		t.nearPersist.RecordNearConfirmed(nearRowFor(&entries[i]))
	}
}

// LoadNearConfirmed membaca kembali seluruh tabel event_near_confirmed dan
// menanamkannya ke peta di memori. Dipanggil SEKALI saat boot, dari Reconcile.
//
// Ia TIDAK memakai LoadOpenEvents: pertanyaan yang dijawab tabel ini mencakup
// event yang sudah RESOLVED, CANCELLED, bahkan yang tombstone-nya sudah
// dievakuasi — justru event-event itu yang paling sering ditanya pasca-kejadian,
// dan LoadOpenEvents hanya mengembalikan yang masih HAPPENING.
//
// Baris yang dibaca TIDAK dihitung ulang dari koordinat node sekarang. Yang
// ditanam adalah angka yang BENAR-BENAR direkam saat persilangan, beserta ambang
// dan algo_ver-nya sendiri. Menghitungnya ulang akan menilai keputusan lampau
// dengan parameter yang tidak menghasilkannya, dan itu pertanyaan terbuka yang
// belum diputuskan (U-007) — bukan sesuatu yang boleh dijawab oleh implementasi.
//
// Memori tetap otoritasnya (§9.5): entri yang sudah ada di peta TIDAK ditimpa.
func (t *Tracker) LoadNearConfirmed(ctx context.Context, src nearConfirmedReader) {
	cov := NearConfirmedCoverage{DurableReadAttempted: true}

	rows, err := src.ListNearConfirmed(ctx)
	cov.DurableReadAtMs = t.now().UnixMilli()
	if err != nil {
		// Kegagalan pembacaan DILAPORKAN, bukan disembunyikan sebagai daftar
		// kosong: dua arti daftar kosong itulah yang selubung cakupan ini ada untuk
		// membedakan, dan sebuah galat yang ditelan akan menghapus perbedaannya.
		cov.DurableReadError = err.Error()
		t.log.Warn("event: pembacaan near-confirmed durable gagal", "err", err)
		t.mu.Lock()
		t.nearCoverage = cov
		t.mu.Unlock()
		return
	}
	cov.DurableReadOK = true

	t.mu.Lock()
	for i := range rows {
		r := rows[i]
		if _, dup := t.nearConfirmed[r.EventID]; dup {
			continue
		}
		e := &NearConfirmedEntry{
			EventID:                r.EventID,
			FirstTwoIndependentAt:  r.FirstTwoIndependentAt,
			IndependentCountAtPeak: r.IndependentCountAtPeak,
			NodeCountAtPeak:        r.NodeCountAtPeak,
			MinIndependentCells:    r.MinIndependentCells,
			AlgoVer:                r.AlgoVer,
			Source:                 NearConfirmedSourceDurable,
		}
		if r.ConfirmedAt != nil {
			e.ConfirmedAt = *r.ConfirmedAt
		}
		if r.TerminalState != nil {
			e.TerminalState = *r.TerminalState
		}
		if r.TerminalAt != nil {
			e.TerminalAt = *r.TerminalAt
		}
		t.nearConfirmed[r.EventID] = e
		cov.DurableRowsLoaded++
	}
	t.nearCoverage = cov
	t.mu.Unlock()

	t.log.Info("event: near-confirmed durable dimuat",
		"baris", len(rows), "ditanam", cov.DurableRowsLoaded)
}

// sortNearConfirmed mengurutkan slice di tempat: ascending FirstTwoIndependentAt,
// seri diputus oleh EventID.
func sortNearConfirmed(entries []NearConfirmedEntry) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0; j-- {
			a, b := entries[j-1], entries[j]
			if a.FirstTwoIndependentAt > b.FirstTwoIndependentAt ||
				(a.FirstTwoIndependentAt == b.FirstTwoIndependentAt && a.EventID > b.EventID) {
				entries[j-1], entries[j] = entries[j], entries[j-1]
			} else {
				break
			}
		}
	}
}
