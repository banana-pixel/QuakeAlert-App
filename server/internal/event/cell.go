package event

import "math"

// Dua fungsi sel, dengan dua pekerjaan berbeda (§6.3):
//
//	lookupCell        — membatasi himpunan kandidat saat mencocokkan observasi ke
//	                    event. Sisinya harus MELEBIHI AttachRadiusKm agar
//	                    lingkungan 3x3 pasti memuat setiap event dalam radius.
//	independenceCell  — menghitung independensi geografis untuk konfirmasi.
//	                    Sisinya harus LEBIH KECIL dari jarak antarnode nyata agar
//	                    tiga sensor di satu gedung tidak menjadi tiga bukti.
//
// Satu ukuran tidak dapat melakukan keduanya pada kerapatan fleet ini: node
// referensi Cimahi–Bandung hanya ~9,4 km terpisah.
//
// KEDUANYA adalah pasangan bilangan bulat yang dipakai sebagai kunci struct,
// BUKAN string terformat. Itu menghilangkan cacat negative-zero (A17) secara
// konstruksi: math.Floor tidak punya kasus -0, dan tidak ada Sprintf di jalur ini.
const (
	// LookupCellDeg adalah sisi sel pencarian, dipakai untuk KEDUA sumbu.
	//
	// Nilai dibuktikan pada lintang terburuk yang didukung (§6.3.1):
	//	longitude: 0.60 * 111.32 * cos(12°) = 65.3 km >= 50 km  (margin 31%)
	//	latitude : 0.60 * 111.32            = 66.8 km >= 50 km  (margin 34%)
	//
	// SENGAJA konstanta compile-time, bukan env var: ia terikat pada
	// MaxFleetLatitudeDeg oleh pertidaksamaan di atas, dan operator yang dapat
	// mengubah satu tanpa yang lain dapat merusak cakupan korelasi tanpa gejala
	// apa pun sampai ada gempa yang terbelah menjadi dua event.
	LookupCellDeg = 0.60

	// MaxFleetLatitudeDeg adalah pita lintang yang didukung, |lat| <= 12°.
	// Kepulauan Indonesia berkisar 6°LU–11°LS, jadi 12° memuat setiap lokasi yang
	// dapat dideploy dengan sisa ruang. Menyatakan pitanya secara eksplisit itulah
	// yang membuat satu konstanta LookupCellDeg dapat DIBUKTIKAN, bukan diasumsikan.
	//
	// CheckFleetIndependence (§7.3) melog ERROR untuk node aktif di luar pita ini.
	MaxFleetLatitudeDeg = 12.0

	// KmPerDegree adalah panjang satu derajat lintang (km). Dipakai juga untuk
	// sumbu bujur pada independenceCell — lihat komentar fungsi itu.
	KmPerDegree = 111.32
)

// cellKey adalah indeks sel sebagai pasangan bilangan bulat. X = sumbu bujur,
// Y = sumbu lintang.
type cellKey struct {
	X int32
	Y int32
}

// cellAt memetakan koordinat ke indeks sel berukuran sizeDeg derajat pada kedua
// sumbu. math.Floor, bukan konversi int langsung: konversi memotong ke arah nol,
// yang akan melipat dua sel yang mengapit garis nol menjadi satu.
func cellAt(lat, lon, sizeDeg float64) cellKey {
	return cellKey{
		X: int32(math.Floor(lon / sizeDeg)),
		Y: int32(math.Floor(lat / sizeDeg)),
	}
}

// lookupCell mengembalikan sel pencarian sebuah titik.
func lookupCell(lat, lon float64) cellKey {
	return cellAt(lat, lon, LookupCellDeg)
}

// independenceCellDeg mengubah sisi sel independensi dari km ke derajat.
//
// Faktor yang sama dipakai untuk kedua sumbu — TANPA cos(lat) pada bujur — dan itu
// pilihan yang disengaja: sel yang lebarnya bergantung pada lintang membuat indeks
// sel sebuah titik bergantung pada siapa yang menghitungnya, dan indeks yang bukan
// fungsi dari posisi saja bukan indeks. Harganya adalah sel bujur ~0,7% lebih
// sempit dalam km pada lintang deployment referensi, yang tidak mengubah satu pun
// keputusan independensi.
func independenceCellDeg(cellKm float64) float64 {
	return cellKm / KmPerDegree
}

// independenceCell mengembalikan sel independensi sebuah titik untuk ukuran sel
// yang dikonfigurasi (IndependenceCellKm, default 5 km).
func independenceCell(lat, lon, cellKm float64) cellKey {
	return cellAt(lat, lon, independenceCellDeg(cellKm))
}

// onsetBucket mengembalikan ember waktu sebuah onset: floor(ms / windowMs).
//
// Ember HANYA indeks pencarian; yang memutuskan kecocokan adalah predikat §4.3.
// Observasi memeriksa ember b-1, b, b+1 — tepat cukup dan tidak lebih, karena
// predikat menerima |onset - origin_ts| <= W sehingga
// origin_ts ∈ [(b-1)W, (b+2)W). Bentuk dua ember (b-1, b) diam-diam
// mengasumsikan jangkar event tidak pernah LEBIH BARU dari onset yang datang,
// dan asumsi itu gagal pada kedatangan yang tidak berurutan (R-C1).
func onsetBucket(ms, windowMs int64) int64 {
	if windowMs <= 0 {
		return 0
	}
	// Pembagian bilangan bulat Go memotong ke arah nol; onset ms epoch selalu
	// positif, tetapi floor ditulis eksplisit agar aritmetikanya benar tanpa
	// bergantung pada asumsi itu.
	q := ms / windowMs
	if ms%windowMs != 0 && ms < 0 {
		q--
	}
	return q
}
