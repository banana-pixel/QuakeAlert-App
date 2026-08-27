package event

import (
	"fmt"
	"testing"
)

// Regresi yang diwajibkan §18.2 untuk R-C1.
//
// Dua node 10 km berjarak, SATU gempa fisik. Kedua onsetnya mengapit sebuah batas
// ember: satu di k·W − 1 ms, satu di k·W + 1 ms, jadi ember mereka berbeda
// meskipun selisih onsetnya 2 ms. Ketika observasi ber-onset LEBIH BARU yang
// menjangkar event, ember jangkar menjadi LEBIH TINGGI daripada ember observasi
// yang datang kemudian — tepat kasus yang gagal pada penyelidikan dua ember, yang
// mengandaikan jangkar tidak pernah lebih baru dari onset yang sedang datang.
//
// Kegagalannya adalah PEMBELAHAN: dua event_id, dua alert, satu gempa. Uji inilah
// yang gagal bila ada yang mempersempit penyelidikan menjadi dua ember lagi.
func TestLaterOnsetAnchorsFirstThenEarlierOnsetArrives(t *testing.T) {
	const window = int64(20000)
	// onsetBase adalah kelipatan tepat dari window, jadi ia SEBUAH batas ember.
	if onsetBase%window != 0 {
		t.Fatalf("uji ini menuntut onsetBase (%d) tepat di batas ember %d", onsetBase, window)
	}
	before, after := onsetBase-1, onsetBase+1
	if onsetBucket(before, window) == onsetBucket(after, window) {
		t.Fatalf("persiapan salah: kedua onset harus berada di ember berbeda")
	}

	// Keempat permutasi yang mengapit batas: urutan masuk kali penukaran peran.
	for _, first := range []string{"NB", "NA"} {
		for _, swap := range []bool{false, true} {
			name := fmt.Sprintf("%s-dulu/peran-ditukar=%v", first, swap)
			t.Run(name, func(t *testing.T) {
				h := newHarness(t)
				h.node("NA", baseLat, baseLon)
				h.nodeAt("NB", baseLat, baseLon, 10, 90)

				onsetA, onsetB := before, after
				if swap {
					onsetA, onsetB = after, before
				}
				obsA := v2("NA", MinPGAGal+10, onsetA, PhasePrelim, 1)
				obsB := v2("NB", MinPGAGal+12, onsetB, PhasePrelim, 1)

				if first == "NB" {
					h.ingest(obsB)
					h.ingest(obsA)
				} else {
					h.ingest(obsA)
					h.ingest(obsB)
				}

				e := h.only()
				if len(e.Contributors) != 2 {
					t.Fatalf("kontributor = %d, mau 2: satu gempa, dua node", len(e.Contributors))
				}
				for _, n := range []string{"NA", "NB"} {
					if _, ok := e.Contributors[n]; !ok {
						t.Errorf("%s bukan kontributor event %s", n, e.ID)
					}
				}
				if got := h.trk.Created(); got != 1 {
					t.Errorf("event_created_total = %d, mau 1 — lebih dari satu berarti PEMBELAHAN", got)
				}
				if ids := h.emit.eventIDs(); len(ids) > 1 {
					t.Errorf("frame membawa %d event_id berbeda (%v); satu gempa hanya boleh satu",
						len(ids), ids)
				}
			})
		}
	}
}

// Penyelidikan tiga ember harus menemukan jangkar untuk SETIAP posisi onset di
// dalam jendela, bukan hanya di batas. Uji ini memindai seluruh jendela pada kedua
// arah dengan jangkar yang diletakkan tepat di batas ember.
func TestThreeBucketProbeFindsAnchorAcrossWholeWindow(t *testing.T) {
	const window = int64(20000)
	for _, d := range []int64{-window, -window + 1, -window / 2, -1, 0, 1, window / 2, window - 1, window} {
		t.Run(fmt.Sprintf("d=%dms", d), func(t *testing.T) {
			h := newHarness(t)
			h.node("NA", baseLat, baseLon)
			h.nodeAt("NB", baseLat, baseLon, 10, 90)

			h.ingest(v2("NA", MinPGAGal+10, onsetBase, PhasePrelim, 1))
			h.ingest(v2("NB", MinPGAGal+10, onsetBase+d, PhasePrelim, 1))

			if n := len(h.events()); n != 1 {
				t.Fatalf("event = %d, mau 1: |d| <= jendela harus selalu cocok (%s)",
					n, describe(h.events()))
			}
		})
	}
}

// Dan tepat di luar jendela ia HARUS membelah — kalau tidak, jendela tidak berarti
// apa pun dan uji di atas akan lulus secara hampa.
func TestJustOutsideWindowFormsTwoEvents(t *testing.T) {
	const window = int64(20000)
	for _, d := range []int64{-window - 1, window + 1} {
		h := newHarness(t)
		h.node("NA", baseLat, baseLon)
		h.nodeAt("NB", baseLat, baseLon, 10, 90)

		h.ingest(v2("NA", MinPGAGal+10, onsetBase, PhasePrelim, 1))
		h.ingest(v2("NB", MinPGAGal+10, onsetBase+d, PhasePrelim, 1))

		if n := len(h.events()); n != 2 {
			t.Errorf("d=%d: event = %d, mau 2", d, n)
		}
	}
}
