package event

// NearConfirmedEntry mencatat satu event yang pernah mencapai >= 2 kontributor
// independen tanpa (atau sebelum) dikonfirmasi. Dipakai untuk menjawab
// pertanyaan operasional pasca-kejadian:
//
//   - Berapa event yang macet di >= 2 independen?
//   - Berapa lama mereka macet?
//   - Berapa yang akhirnya CONFIRMED?
//   - Berapa yang mati tanpa konfirmasi?
//
// Semua stempel waktu adalah Unix millisecond (sama dengan OriginTS / DecidedAt
// di seluruh paket ini).
//
// Entry TIDAK dihapus saat event menjadi tombstone atau dievakuasi dari Tracker:
// tujuannya rekonstruksi pasca-kejadian, sehingga riwayat harus tetap ada.
// Ukurannya dibatasi oleh jumlah event yang pernah dibuat dalam satu proses,
// yang kecil pada skala deployment ini.
type NearConfirmedEntry struct {
	EventID string `json:"event_id"`

	// FirstTwoIndependentAt adalah waktu pertama kali event ini memiliki >= 2
	// kontributor independen (independentCells() >= minIndependentCells). Selalu
	// terisi bila entry ada.
	FirstTwoIndependentAt int64 `json:"first_two_independent_at_ms"`

	// IndependentCountAtPeak adalah nilai independentCells() pada saat
	// FirstTwoIndependentAt dicatat. Berguna untuk membedakan "pas 2" dari
	// "sudah 3 tapi tidak kuorum".
	IndependentCountAtPeak int `json:"independent_count_at_peak"`

	// NodeCountAtPeak adalah len(Contributors) saat FirstTwoIndependentAt.
	NodeCountAtPeak int `json:"node_count_at_peak"`

	// ConfirmedAt > 0 berarti event ini akhirnya CONFIRMED. 0 berarti belum
	// atau tidak pernah.
	ConfirmedAt int64 `json:"confirmed_at_ms,omitempty"`

	// TerminalState adalah state terminal terakhir (RESOLVED atau CANCELLED).
	// Kosong berarti event masih terbuka saat snapshot diambil.
	TerminalState string `json:"terminal_state,omitempty"`

	// TerminalAt > 0 berarti event sudah terminal. 0 berarti masih terbuka.
	TerminalAt int64 `json:"terminal_at_ms,omitempty"`
}

// NearConfirmedLog mengembalikan salinan seluruh entri near-confirmed, diurutkan
// dari yang paling awal mencapai kondisi >= 2 independen. Aman dipanggil dari
// goroutine mana pun.
//
// "Stalled" (macet tanpa konfirmasi) = ConfirmedAt == 0.
// "Died without confirmation" = ConfirmedAt == 0 && TerminalAt > 0.
// "Eventually confirmed" = ConfirmedAt > 0.
// "Still open" = TerminalAt == 0.
func (t *Tracker) NearConfirmedLog() []NearConfirmedEntry {
	t.mu.Lock()
	out := make([]NearConfirmedEntry, 0, len(t.nearConfirmed))
	for _, e := range t.nearConfirmed {
		cp := *e
		out = append(out, cp)
	}
	t.mu.Unlock()

	// Urutkan berdasarkan FirstTwoIndependentAt, lalu EventID untuk determinisme.
	sortNearConfirmed(out)
	return out
}

// recordNearConfirmedLocked dipanggil DI DALAM t.mu setiap kali independentCells
// event berubah. Ia mencatat entri baru bila event baru saja melampaui ambang,
// dan memperbarui ConfirmedAt / TerminalAt bila transisi state cocok.
//
// Dipanggil dari transitionLocked setelah classify, sehingga State sudah
// mencerminkan keputusan baru.
func (t *Tracker) recordNearConfirmedLocked(e *Event, nowMs int64) {
	indep := e.independentCells()
	threshold := minIndependentCells(e)

	entry, exists := t.nearConfirmed[e.ID]

	if !exists {
		// Hanya buat entri baru bila sudah memenuhi ambang independensi.
		// Terminal sebelum ambang (misalnya CANCELLED saat contributor < threshold)
		// tidak masuk log: event tidak pernah cukup mandiri.
		if indep < threshold {
			return
		}
		entry = &NearConfirmedEntry{
			EventID:                e.ID,
			FirstTwoIndependentAt:  nowMs,
			IndependentCountAtPeak: indep,
			NodeCountAtPeak:        len(e.Contributors),
		}
		t.nearConfirmed[e.ID] = entry
	} else {
		// Entri sudah ada: perbarui puncak independensi jika naik.
		// Independensi boleh turun setelah invalidasi kontributor; puncak tetap.
		if indep > entry.IndependentCountAtPeak {
			entry.IndependentCountAtPeak = indep
			entry.NodeCountAtPeak = len(e.Contributors)
		}
	}

	// Catat CONFIRMED pertama kali.
	if e.State == StateConfirmed && entry.ConfirmedAt == 0 {
		entry.ConfirmedAt = nowMs
	}

	// Catat state terminal. Dilakukan meski indep kini < threshold (misalnya
	// kontributor diinvalidasi setelah entry sudah dibuat).
	if e.isTerminal() && entry.TerminalAt == 0 {
		entry.TerminalState = string(e.State)
		entry.TerminalAt = nowMs
	}
}

// sortNearConfirmed mengurutkan slice di tempat: ascending FirstTwoIndependentAt,
// seri diputus oleh EventID.
func sortNearConfirmed(entries []NearConfirmedEntry) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0; j-- {
			a, b := entries[j-1], entries[j]
			if a.FirstTwoIndependentAt > b.FirstTwoIndependentAt ||
				(a.FirstTwoIndependentAt == b.FirstTwoIndependentAt && a.EventID > b.EventID) {
				entries[j-1], entries[j] = entries[j], entries[j-1]
			} else {
				break
			}
		}
	}
}
