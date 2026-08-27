package event

import (
	"github.com/banana-pixel/quakealert/server/internal/ingest"
)

// Input adalah satu observasi terverifikasi sebagaimana dilihat Tracker: sudah
// dinormalisasi antar-versi protokol, sehingga tidak ada satu pun cabang
// "kalau v1" di dalam logika korelasi.
//
// Satu struct, dibangun oleh mapper terekspor, supaya ia dapat diuji tanpa MQTT.
type Input struct {
	NodeID string
	PGA    float64 // gal
	DurMs  int64

	// PublishTS adalah ts payload — waktu PUBLISH, bukan waktu tanah bergerak.
	PublishTS int64

	// OnsetTS adalah jangkar korelasi: terukur sensor pada v2, dan batas
	// publish_ts - dur_ms pada v1. OnsetSource mengatakan yang mana, dan tidak ada
	// satu pun aritmetika di paket ini yang memperlakukan PUBLISH_BOUND seolah ia
	// SENSOR.
	OnsetTS     int64
	OnsetSource string

	Phase       string
	ObsSeq      *int64
	AttemptNo   *int
	DetriggerTS *int64

	// Lat/Lon/LocationName TIDAK diisi ObservationFrom: sumbernya
	// GetNodeLocation(NodeID), sebuah query yang harus terjadi di Tracker sebelum
	// lock dan yang gagal-tertutup. Mapper ini sengaja tetap murni.
	Lat          float64
	Lon          float64
	LocationName string
}

// ObservationFrom menormalisasi satu trigger terverifikasi menjadi Input,
// mengikuti tabel §6.1.
//
// v1 (tanpa proto_ver) tidak membawa onset. Yang tersedia adalah publish_ts dan
// dur_ms, jadi onsetnya adalah publish_ts - dur_ms: sebuah BATAS ATAS yang
// galatnya adalah keterlambatan publish. Batas itu tetap dipakai sebagai jangkar
// — jangkar NULL berarti seluruh fleet v1 tidak dapat membentuk event sama sekali
// — dan OnsetSource = PUBLISH_BOUND yang menjaganya agar tidak pernah menyamar
// sebagai pengukuran.
func ObservationFrom(t *ingest.Trigger) Input {
	in := Input{
		NodeID:    t.NodeID,
		PGA:       t.PGA,
		DurMs:     t.DurMs,
		PublishTS: t.TS,
		Phase:     t.EffectivePhase(),
	}

	if t.IsV2() && t.OnsetTS != nil {
		in.OnsetTS = *t.OnsetTS
		in.OnsetSource = OnsetSourceSensor
		in.ObsSeq = t.ObsSeq
		in.AttemptNo = t.AttemptNo
		in.DetriggerTS = t.DetriggerTS
		return in
	}

	in.OnsetTS = t.TS - t.DurMs
	in.OnsetSource = OnsetSourcePublish
	return in
}
