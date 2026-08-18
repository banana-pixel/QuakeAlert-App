package consensus

import "math"

// Reading adalah satu laporan node terverifikasi di dalam window konsensus.
// Lat/Lon berasal dari master node (bukan dari perangkat), PGA dalam gal.
type Reading struct {
	NodeID string
	Lat    float64
	Lon    float64
	PGA    float64 // gal
	TS     int64   // ms epoch UTC
}

// Centroid adalah hasil weighted centroid algorithm.
type Centroid struct {
	Lat float64
	Lon float64
}

// WeightedCentroid menghitung pusat massa stasiun pemicu dengan bobot PGA:
//
//	Lat_c = sum(Lat_i * PGA_i) / sum(PGA_i)
//	Lon_c = sum(Lon_i * PGA_i) / sum(PGA_i)
//
// Ini adalah estimated_centroid (BUKAN episenter). Bila total bobot 0
// (mis. semua PGA 0), fallback ke rata-rata aritmetik agar tidak divide-by-zero.
func WeightedCentroid(rs []Reading) Centroid {
	if len(rs) == 0 {
		return Centroid{}
	}
	var sumW, sumLat, sumLon float64
	for i := range rs {
		w := rs[i].PGA
		sumW += w
		sumLat += rs[i].Lat * w
		sumLon += rs[i].Lon * w
	}
	if sumW == 0 {
		var aLat, aLon float64
		for i := range rs {
			aLat += rs[i].Lat
			aLon += rs[i].Lon
		}
		n := float64(len(rs))
		return Centroid{Lat: aLat / n, Lon: aLon / n}
	}
	return Centroid{Lat: sumLat / sumW, Lon: sumLon / sumW}
}

// MaxPGA mengembalikan PGA tertinggi (gal) di antara reading.
func MaxPGA(rs []Reading) float64 {
	var max float64
	for i := range rs {
		if rs[i].PGA > max {
			max = rs[i].PGA
		}
	}
	return max
}

// MMIFromPGA mengonversi PGA (gal) ke nilai MMI numerik memakai relasi
// Wald (SYSTEM_SPEC Bab 5.3): MMI = 3.66*log10(PGA) - 1.66.
// Untuk PGA <= 0 dikembalikan MMI 1 (tak terasa) sebagai batas bawah aman.
func MMIFromPGA(pgaGal float64) float64 {
	if pgaGal <= 0 {
		return 1
	}
	mmi := 3.66*math.Log10(pgaGal) - 1.66
	if mmi < 1 {
		return 1
	}
	return mmi
}

// Intensity memetakan PGA (gal) ke label roman MMI + intensity_label sesuai
// tabel ambang SYSTEM_SPEC Bab 5.3 (ambang dalam gal, satuan kanonik).
//
//	PGA < 16.6            -> MMI II-III, "light"
//	16.6 <= PGA < 137.2   -> MMI IV-V,   "moderate"
//	PGA >= 137.2          -> MMI VI+,    "strong"
func Intensity(pgaGal float64) (mmiRoman, label string) {
	switch {
	case pgaGal < 16.6:
		return romanMMI(MMIFromPGA(pgaGal)), "light"
	case pgaGal < 137.2:
		return romanMMI(MMIFromPGA(pgaGal)), "moderate"
	default:
		return romanMMI(MMIFromPGA(pgaGal)), "strong"
	}
}

// romanMMI membulatkan MMI numerik ke bilangan bulat terdekat lalu mengubahnya
// ke angka Romawi (dibatasi rentang wajar I..XII skala Mercalli).
func romanMMI(mmi float64) string {
	n := int(math.Round(mmi))
	if n < 1 {
		n = 1
	}
	if n > 12 {
		n = 12
	}
	romans := []string{
		"", "I", "II", "III", "IV", "V", "VI",
		"VII", "VIII", "IX", "X", "XI", "XII",
	}
	return romans[n]
}
