// Package consensus mengimplementasikan Spatial Consensus Engine QuakeAlert:
// sliding window in-memory, pengelompokan spasial berbasis Haversine, evaluasi
// >= 3 node -> CONFIRMED, kalkulasi weighted centroid + MMI, dan persistensi
// event ke PostGIS. Lihat docs/SYSTEM_SPEC.md Bab 5 & .clinerules/10 #6.
package consensus

import "math"

// EarthRadiusKm adalah radius rata-rata Bumi (kanonik, sama dengan klien Android).
const EarthRadiusKm = 6371.0

// haversineKm menghitung jarak great-circle (km) antara dua titik lat/lon derajat.
// O(1) memori, tanpa alokasi heap (Aturan Server #5: minim alokasi di hot path).
func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const deg2rad = math.Pi / 180.0
	dLat := (lat2 - lat1) * deg2rad
	dLon := (lon2 - lon1) * deg2rad
	rLat1 := lat1 * deg2rad
	rLat2 := lat2 * deg2rad

	sinDLat := math.Sin(dLat / 2)
	sinDLon := math.Sin(dLon / 2)
	a := sinDLat*sinDLat + math.Cos(rLat1)*math.Cos(rLat2)*sinDLon*sinDLon
	// clamp untuk stabilitas numerik: a dapat sedikit > 1 akibat pembulatan float.
	if a > 1 {
		a = 1
	}
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return EarthRadiusKm * c
}
