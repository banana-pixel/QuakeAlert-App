package event

import "testing"

// allStates dalam urutan tetap, supaya tabel 25 pasangan di bawah dapat dibaca
// sebagai matriks.
var allStates = []State{
	StateDetected, StateUnconfirmed, StateConfirmed, StateResolved, StateCancelled,
}

// Kelima state kali kelima state: SETIAP pasangan terurut dinyatakan sah atau
// tidak sah, satu per satu. Tabel yang lengkap adalah satu-satunya cara sebuah
// transisi baru tidak dapat menyelinap masuk tanpa keputusan.
func TestLegalAllTwentyFivePairs(t *testing.T) {
	// Kunci = "from->to". Hanya yang terdaftar di sini yang sah.
	want := map[string]bool{
		"DETECTED->UNCONFIRMED": true,
		"DETECTED->CONFIRMED":   true,
		"DETECTED->CANCELLED":   true,

		"UNCONFIRMED->CONFIRMED": true,
		"UNCONFIRMED->RESOLVED":  true,
		"UNCONFIRMED->CANCELLED": true,

		"CONFIRMED->RESOLVED":  true,
		"CONFIRMED->CANCELLED": true,
	}

	pairs := 0
	for _, from := range allStates {
		for _, to := range allStates {
			pairs++
			key := string(from) + "->" + string(to)
			if got := legal(from, to); got != want[key] {
				t.Errorf("legal(%s) = %v, mau %v", key, got, want[key])
			}
		}
	}
	if pairs != 25 {
		t.Fatalf("tabel memeriksa %d pasangan, mau 25", pairs)
	}
	if len(want) != 8 {
		t.Fatalf("tabel menyatakan %d transisi sah, mau 8", len(want))
	}
}

// Dua ketiadaan yang menanggung beban, dinyatakan sendiri supaya alasannya
// terbaca di nama uji dan bukan hanya di dalam matriks.
func TestConfirmedToUnconfirmedIsIllegal(t *testing.T) {
	if legal(StateConfirmed, StateUnconfirmed) {
		t.Fatal("CONFIRMED -> UNCONFIRMED harus tidak sah: penarikan yang jujur adalah CANCELLED")
	}
}

func TestTerminalStatesHaveNoExit(t *testing.T) {
	for _, from := range []State{StateResolved, StateCancelled} {
		for _, to := range allStates {
			if legal(from, to) {
				t.Errorf("%s -> %s harus tidak sah: AlertDedup sudah mengonsumsi id itu", from, to)
			}
		}
	}
}

func TestDetectedToResolvedIsIllegal(t *testing.T) {
	if legal(StateDetected, StateResolved) {
		t.Fatal("DETECTED -> RESOLVED harus tidak sah: event yang belum pernah publik KEDALUWARSA, bukan selesai")
	}
}

// Transisi ke state yang sama bukan transisi.
func TestSelfTransitionIsIllegal(t *testing.T) {
	for _, s := range allStates {
		if legal(s, s) {
			t.Errorf("%s -> %s harus tidak sah", s, s)
		}
	}
}

func TestIsTerminalAndIsPublic(t *testing.T) {
	cases := []struct {
		state            State
		terminal, public bool
	}{
		{StateDetected, false, false},
		{StateUnconfirmed, false, true},
		{StateConfirmed, false, true},
		{StateResolved, true, true},
		{StateCancelled, true, true},
	}
	for _, c := range cases {
		if got := isTerminal(c.state); got != c.terminal {
			t.Errorf("isTerminal(%s) = %v, mau %v", c.state, got, c.terminal)
		}
		if got := isPublic(c.state); got != c.public {
			t.Errorf("isPublic(%s) = %v, mau %v", c.state, got, c.public)
		}
	}
}
