package api

import (
	"strings"
	"unicode"
)

// Batas kunci kanal regional. channel_id adalah VARCHAR(50), dan kode negara
// plus pemisah memakan 3 karakter, jadi slug admin1 dibatasi 46.
const (
	maxRegionCodeLen = 50
	maxAdmin1SlugLen = maxRegionCodeLen - 3
)

// RegionCode menyusun kunci kanal regional dari hasil reverse-geocode klien:
// "<ISO2>-<admin1-slug>", mis. ("ID", "Jawa Barat") -> "ID-jawa-barat".
//
// Dinormalisasi di SERVER, bukan dipercaya dari klien: dua ponsel bisa mengirim
// "Jawa Barat", "jawa  barat" atau "Jawa-Barat" untuk provinsi yang sama, dan
// setiap ejaan yang lolos apa adanya akan menjadi ruang chat terpisah yang
// masing-masing terasa kosong.
//
// Mengembalikan string kosong bila salah satu bagian tidak dapat dipakai —
// yang berarti user hanya punya kanal global. Ruang tanpa nama yang bisa
// dipercaya lebih buruk daripada tidak ada ruang regional sama sekali.
func RegionCode(countryISO, admin1 string) string {
	country := strings.ToUpper(strings.TrimSpace(countryISO))
	if len(country) != 2 || !isASCIILetters(country) {
		return ""
	}
	slug := slugify(admin1)
	if slug == "" {
		return ""
	}
	if len(slug) > maxAdmin1SlugLen {
		slug = strings.Trim(slug[:maxAdmin1SlugLen], "-")
	}
	return country + "-" + slug
}

// slugify menurunkan teks bebas menjadi token ASCII lowercase yang dipisah
// tanda hubung. Diakritik dilewatkan, bukan ditranslit: tabel translit penuh
// adalah dependensi tersendiri, sementara nama admin1 yang seluruhnya
// non-ASCII akan menghasilkan slug kosong dan jatuh ke kanal global — yang
// aman, dan tercatat di docs/CHAT_DESIGN.md sebagai batasan yang diketahui.
func slugify(s string) string {
	var b strings.Builder
	lastDash := true // menekan tanda hubung di awal
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func isASCIILetters(s string) bool {
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}
