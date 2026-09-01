package event

import "sort"

// Pengukuran latensi tahap server untuk P4-M3′ (Fase 4, D-011).
//
// DUA tahap, dan hanya dua — keduanya sepenuhnya di sisi server:
//
//	onset_ts   -> decided_at   : sejak tanah bergerak sampai Tracker memutuskan.
//	decided_at -> emit_at      : sejak keputusan sampai frame diserahkan ke sink.
//
// Tidak ada tahap sisi klien di sini. Waktu bangun perangkat, heads-up, dan sirene
// Android TIDAK masuk angka ini: keduanya diukur oleh jam yang berbeda pada
// perangkat yang berbeda, dan menjumlahkannya akan menghasilkan satu angka yang
// tidak dapat dipertanggungjawabkan oleh siapa pun.
//
// KENAPA onset dipisah per provenance. Sebuah observasi v1 tidak membawa onset
// terukur; jangkarnya adalah publish_ts - dur_ms (input.go), yaitu BATAS ATAS yang
// galatnya adalah keterlambatan publish. Latensi yang dihitung darinya karena itu
// juga sebuah batas atas, bukan pengukuran. Mencampurnya ke dalam satu persentil
// bersama onset SENSOR akan melaporkan angka yang lebih buruk dari kenyataan dan
// menyebutnya pengukuran — persis bentuk klaim yang §8 larang. Jadi keduanya
// dihitung terpisah dan dilaporkan terpisah, dan pembaca memutuskan sendiri apakah
// keduanya dapat dibandingkan.
//
// Tahap decided->emit TIDAK dipisah: kedua ujungnya adalah jam server yang sama,
// jadi provenance onset tidak mengubah artinya.

// latencySampleCap adalah jumlah sampel yang disimpan per seri. Cincin melingkar
// berukuran tetap: yang tertua ditimpa, tidak ada pertumbuhan tak berbatas, tidak
// ada alokasi setelah inisialisasi.
//
// 256 dipilih karena ia cukup untuk p50/p95 yang stabil sementara seluruh
// strukturnya tetap muat dalam beberapa kilobyte, dan karena batas atasnya harus
// ADA: sebuah slice yang tumbuh mengikuti jumlah event adalah kebocoran memori
// pada proses yang berjalan berbulan-bulan.
const latencySampleCap = 256

// latencySeries adalah satu cincin sampel milidetik.
//
// Tidak punya kunci sendiri. Setiap penulisan dan pembacaan terjadi di bawah
// t.mu — kunci yang SAMA dengan seluruh counter lain, dengan alasan yang sama
// (counters.go): dua mekanisme perlindungan untuk satu struct adalah dua tempat
// yang dapat saling menyimpang.
type latencySeries struct {
	samples [latencySampleCap]int64
	// n adalah jumlah sampel yang pernah ditulis, TIDAK dibatasi cap. Dipakai
	// untuk mengetahui panjang isi cincin (min(n, cap)) sekaligus melaporkan
	// berapa observasi yang sebenarnya diamati.
	n int64
	// next adalah posisi tulis berikutnya di dalam cincin.
	next int
}

// observe mencatat satu sampel. Nilai negatif DIBUANG, tidak dijepit ke nol.
//
// Latensi negatif berarti onset lebih baru daripada keputusan yang menjelaskannya,
// yang pada praktiknya berarti jam node mendahului jam server. Menjepitnya ke nol
// akan melaporkan "0 ms" — angka terbaik yang mungkin — untuk kondisi yang justru
// merupakan kerusakan. Membuangnya membuat sampel itu tidak terhitung, dan
// selisih antara n dan jumlah transisi yang terjadi adalah petunjuk bahwa ada
// yang salah dengan jam.
func (s *latencySeries) observe(ms int64) {
	if ms < 0 {
		return
	}
	s.samples[s.next] = ms
	s.next = (s.next + 1) % latencySampleCap
	s.n++
}

// len mengembalikan jumlah sampel yang saat ini ada di dalam cincin.
func (s *latencySeries) len() int {
	if s.n >= latencySampleCap {
		return latencySampleCap
	}
	return int(s.n)
}

// snapshot mengembalikan ringkasan seri: jumlah sampel yang diamati, p50, dan p95.
//
// Menyalin lalu mengurutkan, alih-alih memelihara struktur terurut secara
// inkremental. Alasannya biaya di tempat yang benar: penyalinan+pengurutan terjadi
// pada PEMBACAAN (satu permintaan admin, jarang), sementara jalur tulis — yang
// dilalui setiap transisi event — tetap satu penugasan array. Sebuah struktur
// terurut akan membalik itu.
func (s *latencySeries) snapshot() LatencyStats {
	n := s.len()
	out := LatencyStats{Observed: s.n}
	if n == 0 {
		return out
	}

	buf := make([]int64, n)
	copy(buf, s.samples[:n])
	sort.Slice(buf, func(i, j int) bool { return buf[i] < buf[j] })

	out.P50Ms = percentile(buf, 50)
	out.P95Ms = percentile(buf, 95)
	return out
}

// percentile mengembalikan persentil ke-p (metode nearest-rank) dari slice yang
// SUDAH terurut naik.
//
// Nearest-rank dan bukan interpolasi linier: nilai yang dikembalikan adalah sebuah
// pengamatan yang benar-benar terjadi, bukan rata-rata dua pengamatan yang tidak
// pernah terjadi. Untuk angka yang dibaca manusia dalam analisis pasca-kejadian,
// "salah satu latensi yang sungguh diamati" lebih dapat dipertahankan daripada
// sebuah nilai sintetis.
func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	// rank = ceil(p/100 * n), dijepit ke [1, n].
	rank := (p*len(sorted) + 99) / 100
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}

// LatencyStats adalah ringkasan satu seri latensi pada satu titik waktu.
//
// Observed adalah jumlah sampel KUMULATIF, bukan panjang cincin: ia mengatakan
// berapa banyak transisi yang benar-benar diukur, sementara p50/p95 hanya berbicara
// tentang paling banyak latencySampleCap sampel terakhir. Keduanya dibawa bersama
// supaya sebuah persentil tidak dapat dibaca seolah ia mewakili seluruh riwayat.
type LatencyStats struct {
	Observed int64 `json:"observed"`
	P50Ms    int64 `json:"p50_ms"`
	P95Ms    int64 `json:"p95_ms"`
}

// latency memegang ketiga seri yang dilaporkan P4-M3′.
//
// Onset dipecah dua karena provenance-nya mengubah ARTI angkanya (lihat catatan
// di kepala berkas). Emit tidak dipecah karena tidak.
type latency struct {
	onsetToDecidedSensor  latencySeries
	onsetToDecidedPublish latencySeries
	decidedToEmit         latencySeries
}

// observeOnsetToDecided mencatat satu sampel onset->decided ke seri yang sesuai
// dengan provenance onset. Provenance yang tidak dikenali DIBUANG alih-alih
// dimasukkan ke salah satu seri: sebuah sumber onset baru yang belum dipahami
// tidak boleh diam-diam ikut dihitung sebagai pengukuran sensor.
func (l *latency) observeOnsetToDecided(source string, ms int64) {
	switch source {
	case OnsetSourceSensor:
		l.onsetToDecidedSensor.observe(ms)
	case OnsetSourcePublish:
		l.onsetToDecidedPublish.observe(ms)
	}
}

// observeDecidedToEmit mencatat satu sampel decided->emit.
func (l *latency) observeDecidedToEmit(ms int64) { l.decidedToEmit.observe(ms) }
