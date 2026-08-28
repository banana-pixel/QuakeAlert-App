package event

import (
	"math"

	"github.com/banana-pixel/quakealert/server/internal/consensus"
)

// Dua fungsi sel, dengan dua pekerjaan berbeda (§6.3):
//
//	lookupCell        — membatasi himpunan kandidat saat mencocokkan observasi ke
//	                    event. Lingkungan yang diselidiki DITURUNKAN dari radius
//	                    attach dan lintang observasi (lihat probeSpan), bukan
//	                    dipatok 3x3.
//	independenceCell  — label grid kasar sebuah titik, disimpan pada
//	                    evidence_summary.cell_ids. DESKRIPTIF, bukan penentu:
//	                    independensi yang menggerbangi CONFIRMED diukur sebagai
//	                    JARAK (lihat independence.go).
//
// KEDUANYA adalah pasangan bilangan bulat yang dipakai sebagai kunci struct,
// BUKAN string terformat. Itu menghilangkan cacat negative-zero (A17) secara
// konstruksi: math.Floor tidak punya kasus -0, dan tidak ada Sprintf di jalur ini.
//
// # INVARIAN CAKUPAN (I-COV) — jangan dilanggar
//
// Untuk setiap observasi, himpunan kandidat yang dihasilkan indeks WAJIB memuat
// SETIAP event terlacak (terbuka maupun tombstone) yang sentroidnya berada dalam
// AttachRadiusKm dari observasi itu — pada lintang observasi itu sendiri, pada
// kedua sumbu, dan melintasi antimeridian.
//
// Indeks hanya boleh KONSERVATIF LEBAR. Ia boleh mengembalikan kandidat yang
// kemudian ditolak matches(); ia tidak boleh MELEWATKAN satu pun. Kelalaian
// bukan sekadar ketidakefisienan: observasi yang tidak mencocoki apa pun
// membentuk event_id KEDUA untuk gempa yang sama — PEMBELAHAN — dan kedua
// belahannya dapat gagal mencapai kuorum, sehingga gempa nyata yang terdeteksi
// sensor tidak pernah menghasilkan satu pun push.
//
// Cacat yang diperbaiki di sini: sisi sel 0.60° hanya menutupi radius 50 km lewat
// lingkungan 3x3 selama 0.60 * 111.32 * cos(lat) >= 50, yang berhenti berlaku di
// |lat| ~= 41,5°. Angka 12° yang dahulu dideklarasikan berasal dari kepulauan
// Indonesia, dan produk ini global.
const (
	// LookupCellDeg adalah sisi sel pencarian, dipakai untuk KEDUA sumbu.
	//
	// Nilainya kini bebas dari pertidaksamaan cakupan: lebar lingkungan yang
	// diselidiki dihitung per observasi, jadi sisi sel hanya menentukan
	// GRANULARITAS indeks, bukan kebenarannya. Tetap 0.60° supaya setiap kunci
	// indeks yang pernah dihitung Fase 3 berarti hal yang sama.
	//
	// 360 / 0.60 = 600 sel bujur tepat; lonCellCount bergantung padanya dan
	// cell_test.go menjaga agar pembagiannya tetap bulat.
	LookupCellDeg = 0.60

	// KmPerDegree adalah panjang satu derajat lintang (km). Dipakai HANYA oleh
	// independenceCellDeg, supaya label grid yang sudah tersimpan tidak berubah
	// arti. Batas cakupan memakai consensus.EarthRadiusKm — lihat degPerKm.
	KmPerDegree = 111.32

	// MaxAttachRadiusKm adalah batas KEBERLAKUAN MATEMATIS bound bujur di
	// probeSpan: asin(sin(r/R)/cos φ) hanya sebuah batas atas selama r <= πR/2.
	// Di atas itu sin(r/R) MENGECIL dan rumusnya diam-diam meremehkan rentang,
	// yang berarti kelalaian indeks — tepat yang I-COV larang. Bukan pembatasan
	// geografis yang dikarang: ini titik tempat rumusnya berhenti benar.
	MaxAttachRadiusKm = math.Pi * consensus.EarthRadiusKm / 2

	// lonCellCount adalah jumlah sel bujur dalam satu lingkaran penuh. Indeks X
	// dilipat modulo nilai ini, sehingga 179,9° dan -179,9° menjadi sel yang
	// BERTETANGGA alih-alih terpisah 600 sel (cacat antimeridian).
	lonCellCount = int32(600)

	// coverageSlackDeg adalah kelonggaran float pada batas rentang probe: ~11 cm.
	// Ada karena batas cakupan dihitung dengan trigonometri floating point dan
	// sebuah pembulatan ke arah yang salah tepat di batas sel akan menjadi
	// kelalaian indeks, bukan sekadar hasil yang kurang rapi.
	coverageSlackDeg = 1e-6

	deg2rad = math.Pi / 180
)

// degPerKm adalah derajat lintang per kilometer pada model bola yang SAMA yang
// dipakai consensus.HaversineKm. Sengaja bukan 1/111.32: 111.32 km/derajat lebih
// besar dari nilai bola sebenarnya (111.195), jadi memakainya untuk membatasi
// rentang akan MEREMEHKAN rentangnya — konservatif ke arah yang salah.
var degPerKm = 180 / (math.Pi * consensus.EarthRadiusKm)

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

// lookupCell mengembalikan sel pencarian sebuah titik, dengan sumbu bujur SUDAH
// dilipat ke rentang kanonik. Pelipatan terjadi di sini dan bukan di pemanggil
// supaya tidak ada jalur yang dapat mendaftarkan sebuah event pada kunci yang
// tidak akan pernah diselidiki oleh observasi di sebelahnya.
func lookupCell(lat, lon float64) cellKey {
	k := cellAt(lat, consensus.NormalizeLon(lon), LookupCellDeg)
	k.X = wrapCellX(k.X)
	return k
}

// wrapCellX melipat indeks sel bujur ke [-lonCellCount/2, lonCellCount/2), yaitu
// rentang yang dihasilkan bujur di [-180, 180). Ditulis dengan dua modulo karena
// operator % Go dapat mengembalikan nilai negatif.
func wrapCellX(x int32) int32 {
	half := lonCellCount / 2
	m := (x + half) % lonCellCount
	if m < 0 {
		m += lonCellCount
	}
	return m - half
}

// probeSpan mengembalikan lebar lingkungan sel yang WAJIB diselidiki untuk sebuah
// observasi di lintang lat dengan radius attach radiusKm: nx sel ke kiri/kanan,
// ny sel ke atas/bawah. allLon berarti lingkaran radius itu memuat sebuah kutub,
// sehingga SETIAP bujur berada di dalam radius dan tidak ada bound bujur yang
// bermakna.
//
// Sumbu lintang: sebuah titik pada jarak r tidak dapat berbeda lintang lebih dari
// r * degPerKm derajat.
//
// Sumbu bujur: selisih bujur terbesar pada lingkaran jarak r di sekitar lintang φ
// adalah asin(sin(r/R) / cos φ) — titik singgung lingkaran, bukan titik yang
// sebujur-lintang. Rumus itu tidak terdefinisi ketika cos φ <= sin(r/R), dan
// itulah TEPAT kasus lingkaran yang menelan kutub: dilaporkan sebagai allLon
// alih-alih dipaksa menjadi angka.
func probeSpan(lat, radiusKm float64) (nx, ny int32, allLon bool) {
	if radiusKm > MaxAttachRadiusKm {
		// Di luar keberlakuan bound. Jangan meremehkan: perlakukan sebagai seluruh
		// globe. Config menolak nilai ini saat boot; ini jaring untuk Tracker yang
		// dibangun langsung oleh uji.
		radiusKm = MaxAttachRadiusKm
	}

	latSpan := radiusKm*degPerKm + coverageSlackDeg
	ny = int32(math.Ceil(latSpan / LookupCellDeg))

	sinR := math.Sin(radiusKm / consensus.EarthRadiusKm)
	cosPhi := math.Cos(lat * deg2rad)
	if cosPhi <= sinR {
		return lonCellCount, ny, true
	}

	lonSpan := math.Asin(sinR/cosPhi)/deg2rad + coverageSlackDeg
	if lonSpan >= 180 {
		return lonCellCount, ny, true
	}
	nx = int32(math.Ceil(lonSpan / LookupCellDeg))
	if nx*2+1 >= lonCellCount {
		return lonCellCount, ny, true
	}
	return nx, ny, false
}

// independenceCellDeg mengubah sisi sel independensi dari km ke derajat.
//
// Faktor yang sama untuk kedua sumbu, TANPA cos(lat) pada bujur — dipertahankan
// APA ADANYA supaya setiap cell_ids yang sudah tersimpan tetap dapat ditafsirkan
// dengan aturan yang sama yang menuliskannya. Yang berubah adalah PERANNYA: sel
// ini tidak lagi menggerbangi CONFIRMED, karena lebar km-nya menyusut dengan
// lintang (5,00 km di ekuator, 2,19 km di 64°) sehingga dua sensor di satu jalan
// dapat jatuh di sel berbeda dan terhitung sebagai dua bukti. Gerbangnya kini
// jarak (independence.go).
func independenceCellDeg(cellKm float64) float64 {
	return cellKm / KmPerDegree
}

// independenceCell mengembalikan label sel independensi sebuah titik.
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
