package ingest

import (
	"encoding/json"
	"errors"
	"regexp"
)

// nodeIDPattern mencerminkan contracts/mqtt/trigger.schema.json (^NODE-[0-9A-F]{8}$).
var nodeIDPattern = regexp.MustCompile(`^NODE-[0-9A-F]{8}$`)

// Trigger adalah payload sensor trigger sesuai contracts/mqtt/trigger.schema.json.
// Satuan kanonik: pga=gal, ts=ms epoch UTC, dur_ms=ms.
type Trigger struct {
	NodeID    string  `json:"node_id"`
	PGA       float64 `json:"pga"`
	DurMs     int64   `json:"dur_ms"`
	TS        int64   `json:"ts"`
	Signature string  `json:"signature"`
}

var (
	ErrInvalidJSON   = errors.New("payload trigger bukan JSON valid")
	ErrInvalidNodeID = errors.New("node_id tidak sesuai pola NODE-XXXXXXXX")
	ErrInvalidPGA    = errors.New("pga di luar rentang [0,2000] gal")
	ErrInvalidDur    = errors.New("dur_ms di luar rentang [0,60000]")
	ErrInvalidTS     = errors.New("ts bukan ms epoch UTC yang wajar")
	ErrInvalidSig    = errors.New("signature bukan hex 64-char")
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
	// 1.7e12 ms ~ 2023-11; batas bawah kontrak.
	if t.TS < 1700000000000 {
		return nil, ErrInvalidTS
	}
	if !sigPattern.MatchString(t.Signature) {
		return nil, ErrInvalidSig
	}
	return &t, nil
}
