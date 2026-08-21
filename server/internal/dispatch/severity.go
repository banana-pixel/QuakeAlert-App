package dispatch

import "strings"

// Ambang "gempa parah" yang MENGABAIKAN jarak.
//
// Aturan radius tetap (AlertRadiusKm) menjawab pertanyaan yang benar untuk
// mayoritas kejadian, tetapi salah untuk yang paling berbahaya: guncangan MMI
// VII ke atas merusak bangunan jauh di luar 200 km, dan orang di 260 km yang
// tidak dibangunkan adalah kegagalan yang tidak bisa dibenarkan dengan
// penghematan notifikasi. Karena itu kejadian sekelas ini dikirim ke SEMUA
// perangkat, tanpa filter jarak sama sekali.
const (
	// SevereMMI adalah MMI (angka Romawi pada payload) minimum yang memicu
	// override. VII = "very strong" pada skala Mercalli: kerusakan pada bangunan
	// biasa, bukan lagi sekadar terasa.
	SevereMMI = 7

	// SeverePGAGal adalah jalur kedua ke override, memakai besaran terukur
	// langsung. Ada di samping SevereMMI karena MMI pada payload sudah dibulatkan
	// ke bilangan Romawi: sebuah event 249 gal yang membulat ke VI tetap
	// mendekati ambang, dan satu-satunya arah yang aman untuk salah adalah
	// membangunkan terlalu banyak orang.
	SeverePGAGal = 250.0
)

// IsSevere melaporkan apakah sebuah event harus mengabaikan seluruh filter jarak.
// mmiRoman adalah field "mmi" pada payload (I..XII); pgaGal adalah PGA maksimum.
func IsSevere(mmiRoman string, pgaGal float64) bool {
	return romanMMI(mmiRoman) >= SevereMMI || pgaGal >= SeverePGAGal
}

// romanMMI mengubah angka Romawi MMI menjadi bilangan bulat, atau 0 bila tidak
// dikenali. Nol (bukan nilai tinggi) untuk yang tidak dikenali karena jalur PGA
// di IsSevere sudah menjadi jaring pengamannya — menebak "parah" dari string
// rusak akan membangunkan seluruh negara atas dasar sebuah bug parsing.
func romanMMI(s string) int {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "I":
		return 1
	case "II":
		return 2
	case "III":
		return 3
	case "IV":
		return 4
	case "V":
		return 5
	case "VI":
		return 6
	case "VII":
		return 7
	case "VIII":
		return 8
	case "IX":
		return 9
	case "X":
		return 10
	case "XI":
		return 11
	case "XII":
		return 12
	default:
		return 0
	}
}
