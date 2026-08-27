package ingest

import (
	"encoding/json"
	"errors"
	"regexp"
)

// nodeIDPattern mencerminkan contracts/mqtt/trigger.schema.json (^NODE-[0-9A-F]{8}$).
var nodeIDPattern = regexp.MustCompile(`^NODE-[0-9A-F]{8}$`)

// ProtoVerV2 adalah satu-satunya nilai proto_ver yang dikenal.
const ProtoVerV2 = 2

// Nilai phase yang dikenal kontrak. PRELIM dipublish pada konfirmasi onset,
// FINAL saat event ditutup — TEPAT DUA publikasi per event (D7).
const (
	PhasePrelim = "PRELIM"
	PhaseFinal  = "FINAL"
)

// minPayloadTS: 1.7e12 ms ~ 2023-11; batas bawah setiap timestamp di kontrak.
const minPayloadTS = 1700000000000

// Trigger adalah payload sensor trigger sesuai contracts/mqtt/trigger.schema.json.
// Satuan kanonik: pga=gal, semua timestamp=ms epoch UTC, dur_ms=ms.
//
// Field v2 bertipe pointer (kecuali Phase, yang string kosongnya sudah menjadi
// penanda "tidak ada"). Pointer, bukan nilai dengan zero value, karena 0 adalah
// nilai obs_seq yang SAH: node pada boot pertama dengan event pertama benar-benar
// mengirim obs_seq 0, dan menyamakannya dengan "tidak ada" akan membuat observasi
// pertama setiap node tidak dapat dideduplikasi.
type Trigger struct {
	NodeID    string  `json:"node_id"`
	PGA       float64 `json:"pga"`
	DurMs     int64   `json:"dur_ms"`
	TS        int64   `json:"ts"`
	Signature string  `json:"signature"`

	// --- Protokol v2 (opsional; ADA proto_ver = v2, TIDAK ADA = v1 legacy) ---
	ProtoVer    *int   `json:"proto_ver,omitempty"`
	Phase       string `json:"phase,omitempty"`
	ObsSeq      *int64 `json:"obs_seq,omitempty"`
	AttemptNo   *int   `json:"attempt_no,omitempty"`
	OnsetTS     *int64 `json:"onset_ts,omitempty"`
	DetriggerTS *int64 `json:"detrigger_ts,omitempty"`
}

// IsV2 melaporkan apakah payload ini protokol v2. Deteksi versi adalah
// ADA/TIDAKNYA proto_ver dan bukan flag konfigurasi (§12.2): sebuah flag akan
// membuat server dan perangkat dapat berbeda pendapat tentang versi, dan yang
// kalah dalam perbedaan itu adalah tanda tangannya.
func (t *Trigger) IsV2() bool { return t.ProtoVer != nil }

// EffectivePhase mengembalikan phase yang berlaku. Payload v1 tidak membawa
// phase dan selalu dipublish saat event SELESAI, jadi FINAL adalah kebenaran
// untuk v1 — bukan nilai bawaan yang dipilih karena mudah.
func (t *Trigger) EffectivePhase() string {
	if t.Phase == "" {
		return PhaseFinal
	}
	return t.Phase
}

var (
	ErrInvalidJSON   = errors.New("payload trigger bukan JSON valid")
	ErrInvalidNodeID = errors.New("node_id tidak sesuai pola NODE-XXXXXXXX")
	ErrInvalidPGA    = errors.New("pga di luar rentang [0,2000] gal")
	ErrInvalidDur    = errors.New("dur_ms di luar rentang [0,60000]")
	ErrInvalidTS     = errors.New("ts bukan ms epoch UTC yang wajar")
	ErrInvalidSig    = errors.New("signature bukan hex 64-char")

	ErrUnsignedV2Field  = errors.New("field v2 hadir tanpa proto_ver (tidak ikut ditandatangani)")
	ErrInvalidProtoVer  = errors.New("proto_ver tidak dikenal (hanya 2)")
	ErrInvalidPhase     = errors.New("phase bukan PRELIM atau FINAL")
	ErrInvalidObsSeq    = errors.New("obs_seq tidak ada atau negatif")
	ErrInvalidAttemptNo = errors.New("attempt_no di luar rentang [1,255]")
	ErrInvalidOnsetTS   = errors.New("onset_ts bukan ms epoch UTC yang wajar")
	ErrInvalidDetrigger = errors.New("detrigger_ts wajib pada FINAL dan harus absen pada PRELIM")
)

var sigPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ParseTrigger meng-unmarshal dan memvalidasi struktur payload (bukan HMAC).
func ParseTrigger(raw []byte) (*Trigger, error) {
	var t Trigger
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, ErrInvalidJSON
	}
	if !nodeIDPattern.MatchString(t.NodeID) {
		return nil, ErrInvalidNodeID
	}
	if t.PGA < 0 || t.PGA > 2000 {
		return nil, ErrInvalidPGA
	}
	if t.DurMs < 0 || t.DurMs > 60000 {
		return nil, ErrInvalidDur
	}
	if t.TS < minPayloadTS {
		return nil, ErrInvalidTS
	}
	if !sigPattern.MatchString(t.Signature) {
		return nil, ErrInvalidSig
	}
	if err := validateVersionFields(&t); err != nil {
		return nil, err
	}
	return &t, nil
}

// validateVersionFields menegakkan konsistensi antara proto_ver dan field v2.
//
// Payload v1 yang MEMBAWA field v2 ditolak, tidak diabaikan. Field itu tidak
// akan ikut ditandatangani (string kanonik v1 tidak memuatnya), sehingga
// menerimanya berarti menerima metadata yang dapat diubah siapa pun di jalur
// transport tentang laporan yang ditandatangani — dan bila kemudian ada satu
// saja pembaca yang memakainya, ia memakai data tak terotentikasi.
func validateVersionFields(t *Trigger) error {
	if !t.IsV2() {
		if t.Phase != "" || t.ObsSeq != nil || t.AttemptNo != nil ||
			t.OnsetTS != nil || t.DetriggerTS != nil {
			return ErrUnsignedV2Field
		}
		return nil
	}

	if *t.ProtoVer != ProtoVerV2 {
		return ErrInvalidProtoVer
	}
	if t.Phase != PhasePrelim && t.Phase != PhaseFinal {
		return ErrInvalidPhase
	}
	if t.ObsSeq == nil || *t.ObsSeq < 0 {
		return ErrInvalidObsSeq
	}
	if t.AttemptNo == nil || *t.AttemptNo < 1 || *t.AttemptNo > 255 {
		return ErrInvalidAttemptNo
	}
	if t.OnsetTS == nil || *t.OnsetTS < minPayloadTS {
		return ErrInvalidOnsetTS
	}
	// detrigger_ts HANYA pada FINAL. Pada PRELIM event-nya belum berakhir, jadi
	// field ini harus tidak ada — bukan 0. 0 akan lolos sebagai "ada" di setiap
	// pembaca yang memeriksa keberadaan saja, lalu tercatat sebagai instan di
	// tahun 1970.
	switch t.Phase {
	case PhaseFinal:
		if t.DetriggerTS == nil || *t.DetriggerTS < minPayloadTS {
			return ErrInvalidDetrigger
		}
	case PhasePrelim:
		if t.DetriggerTS != nil {
			return ErrInvalidDetrigger
		}
	}
	return nil
}
