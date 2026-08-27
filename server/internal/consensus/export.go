package consensus

// Pembungkus terekspor untuk paket internal/event (§11.3).
//
// Sejak Fase 3, paket ini bukan lagi kewenangan pengambil keputusan melainkan
// pustaka geometri dan intensitas yang diimpor paket event. Dua helper yang
// dibutuhkannya tidak terekspor, dan MENGGANTI NAMANYA akan menyentuh berkas uji
// Fase 1/2 yang justru menjadi bukti bahwa seismologinya tidak berpindah. Jadi
// keduanya diekspos lewat pembungkus setipis mungkin: tidak ada perilaku baru,
// tidak ada satu pun baris lama yang berubah.

// HaversineKm menghitung jarak great-circle (km) antara dua titik lat/lon derajat.
func HaversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	return haversineKm(lat1, lon1, lat2, lon2)
}

// NearestToCentroid mengembalikan reading yang koordinatnya paling dekat dengan
// centroid. Memanggilnya dengan rs kosong adalah bug pemanggil; ia mengembalikan
// Reading kosong alih-alih panik, karena satu-satunya pemanggilnya berada di jalur
// peringatan.
func NearestToCentroid(rs []Reading, c Centroid) Reading {
	if len(rs) == 0 {
		return Reading{}
	}
	return nearestToCentroid(rs, c)
}
