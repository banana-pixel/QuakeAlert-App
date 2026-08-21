package dispatch

import "testing"

// IsSevere adalah override intensitas: ambang di mana jarak berhenti menjadi
// pertimbangan. Test ini menjaga kedua jalurnya (MMI dan PGA) tetap independen —
// satu saja yang menyala sudah cukup, karena keduanya adalah perkiraan dari
// pengukuran yang sama dan yang satu bisa hilang/tak dikenali.
func TestIsSevere(t *testing.T) {
	cases := []struct {
		name string
		mmi  string
		pga  float64
		want bool
	}{
		{"MMI VII tepat di ambang", "VII", 10, true},
		{"MMI VIII di atas ambang", "VIII", 10, true},
		{"MMI XII maksimum", "XII", 0, true},
		{"MMI VI di bawah ambang", "VI", 10, false},
		{"PGA tepat di ambang", "V", SeverePGAGal, true},
		{"PGA di atas ambang", "IV", 900, true},
		{"PGA di bawah ambang", "VI", 249.9, false},
		{"MMI tak dikenal, PGA menyelamatkan", "", 400, true},
		{"MMI tak dikenal, PGA rendah", "???", 50, false},
		{"MMI huruf kecil tetap dikenali", "vii", 10, true},
		{"MMI dengan spasi tetap dikenali", " VII ", 10, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsSevere(c.mmi, c.pga); got != c.want {
				t.Fatalf("IsSevere(%q, %v) = %v, mau %v", c.mmi, c.pga, got, c.want)
			}
		})
	}
}

// romanMMI memetakan I..XII. Nilai tak dikenal WAJIB 0, bukan angka besar:
// kesalahan parsing yang menghasilkan nilai tinggi akan mengubah setiap gempa
// kecil menjadi sirene nasional.
func TestRomanMMIUnknownIsZero(t *testing.T) {
	for _, s := range []string{"", "XIII", "7", "M", "IIII", "V.5"} {
		if got := romanMMI(s); got != 0 {
			t.Fatalf("romanMMI(%q) = %d, mau 0", s, got)
		}
	}
	if romanMMI("VII") != SevereMMI {
		t.Fatalf("romanMMI(\"VII\") = %d, mau %d", romanMMI("VII"), SevereMMI)
	}
}
