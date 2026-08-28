package event

import (
	"sort"

	"github.com/banana-pixel/quakealert/server/internal/consensus"
)

// Independensi geografis (§7.3) diukur sebagai JARAK, bukan sebagai jumlah sel
// grid yang terisi.
//
// # Mengapa berubah
//
// Sel independensi berukuran IndependenceCellKm derajat pada KEDUA sumbu, tanpa
// cos(lat) pada bujur. Lebar km sebuah sel bujur karenanya menyusut dengan
// lintang: pada setelan baku 5 km ia 5,00 km di ekuator, 3,21 km di 50°, dan
// 2,19 km di 64°. Konsekuensinya bukan ketidaktepatan, melainkan pergeseran
// AMBANG: dua sensor yang berjarak 2,5 km — dua alat di satu kompleks, di satu
// gedung, pada satu meja panjang — jatuh di sel bujur BERBEDA di lintang tinggi
// dan terhitung sebagai dua bukti independen. Itu tepat kegagalan yang
// MIN_INDEPENDENT_CELLS ada untuk mencegah, dan arahnya adalah CONFIRMED yang
// terlalu mudah, yakni alert publik atas getaran yang mungkin lokal.
//
// Yang dipilih: JARAK geodesik langsung antar kontributor, memakai
// consensus.HaversineKm yang sama yang sudah memutuskan penempelan (§4.3).
// "Independen" karenanya berarti satu hal yang sama di setiap lintang dan di
// kedua sisi antimeridian: TERPISAH SETIDAKNYA IndependenceCellKm KILOMETER.
//
// Yang TIDAK dipilih: sel berpita-lintang (lebar sel dipilih per pita lintang).
// Ia memperbaiki arah kesalahannya tetapi tidak artinya — ambangnya tetap
// bergantung pada di mana batas pita dan batas sel jatuh, sehingga dua sensor
// yang berjarak sama dapat dihitung berbeda hanya karena penyelarasan grid, dan
// ia menambah satu tabel pita yang harus benar. Jarak tidak punya batas untuk
// diselaraskan.
//
// # Yang TIDAK berubah
//
// Contributor.Cell dan evidence_summary.cell_ids tetap dihitung dan tetap
// ditulis, dengan rumus yang sama persis (lihat independenceCell). Perannya kini
// DESKRIPTIF: label grid kasar yang membuat baris lampau dapat dibandingkan
// dengan baris baru. Tidak ada migrasi, dan tidak ada baris tersimpan yang
// berubah arti — algoVerBase yang naik yang memberi tahu pembaca aturan mana
// yang menghasilkan angka independent_cell_count sebuah baris.

// independentCount menghitung banyaknya bukti yang saling independen di antara
// titik-titik yang diberikan: ukuran himpunan yang setiap pasangannya terpisah
// >= minSepKm.
//
// Algoritmenya rakus (greedy) atas urutan node_id yang stabil: sebuah titik
// menjadi perwakilan bila ia >= minSepKm dari SETIAP perwakilan yang sudah
// diterima. Dua sifat yang membuatnya benar untuk sebuah gerbang keselamatan:
//
//	KONSERVATIF — hasilnya selalu himpunan yang sah saling-independen, jadi
//	jumlahnya <= himpunan independen maksimum. Ia dapat MEREMEHKAN independensi
//	(CONFIRMED lebih sulit), tidak pernah membesar-besarkannya. Himpunan
//	independen maksimum adalah NP-hard; membayar itu untuk membuat CONFIRMED
//	lebih MUDAH adalah pertukaran yang salah arah.
//
//	DETERMINISTIK — urutan node_id diurutkan lebih dulu, sehingga jumlah yang
//	sama keluar dari himpunan kontributor yang sama, tidak bergantung pada urutan
//	iterasi map Go. Sebuah gerbang yang hasilnya bergantung pada urutan iterasi
//	adalah gerbang yang tidak dapat direproduksi dari barisnya.
//
// minSepKm <= 0 diperlakukan sebagai "setiap titik independen": Options yang
// dibangun tanpa nilai tidak boleh membuat gerbang tidak dapat dicapai secara
// diam-diam. NewTracker sudah memberi lantai pada nilainya.
func independentCount(pts []geoPoint, minSepKm float64) int {
	if len(pts) == 0 {
		return 0
	}
	if minSepKm <= 0 {
		return len(pts)
	}

	ordered := make([]geoPoint, len(pts))
	copy(ordered, pts)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].id < ordered[j].id })

	reps := make([]geoPoint, 0, len(ordered))
	for _, p := range ordered {
		ok := true
		for _, r := range reps {
			if consensus.HaversineKm(p.lat, p.lon, r.lat, r.lon) < minSepKm {
				ok = false
				break
			}
		}
		if ok {
			reps = append(reps, p)
		}
	}
	return len(reps)
}

// geoPoint adalah satu titik berlabel untuk independentCount. Tipe sempit, bukan
// []*Contributor, supaya pemeriksaan-diri fleet (§7.3) dapat memakai penghitung
// yang SAMA atas store.NodeLocation — dua penghitung independensi yang berbeda
// adalah dua ambang yang dapat menyimpang.
type geoPoint struct {
	id  string
	lat float64
	lon float64
}
