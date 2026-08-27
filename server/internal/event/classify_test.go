package event

import (
	"fmt"
	"testing"
)

// eventWith membangun Event dengan n kontributor yang tersebar di cells sel
// berbeda dan PGA puncak peak. Hanya untuk uji: classify tidak peduli dari mana
// kontributornya datang.
func eventWith(n, cells int, peak float64, invalidated bool) *Event {
	e := &Event{
		State:        StateDetected,
		Contributors: make(map[string]*Contributor, n),
		Invalidated:  invalidated,
		minCells:     2,
	}
	for i := 0; i < n; i++ {
		cell := 0
		if cells > 0 {
			cell = i % cells
		}
		pga := peak
		if i > 0 {
			// Hanya satu kontributor yang memegang puncak; sisanya di bawahnya,
			// supaya peakPGA benar-benar diuji sebagai maksimum.
			pga = peak / 2
		}
		id := fmt.Sprintf("NODE-%08d", i+1)
		e.Contributors[id] = &Contributor{
			NodeID: id, PeakPGA: pga, Cell: cellKey{X: int32(cell), Y: 0},
		}
	}
	return e
}

// classify itu total dan murni, jadi tabelnya EKSHAUSTIF atas silang kartesian
// jumlah kontributor x jumlah sel x PGA puncak x invalidated — bukan
// representatif.
func TestClassifyExhaustive(t *testing.T) {
	nodeCounts := []int{0, 1, 2, 3, 4}
	cellCounts := []int{1, 2, 3}
	pgas := []float64{0, MinPGAGal - 0.1, MinPGAGal, MinPGAGal + 100}

	for _, n := range nodeCounts {
		for _, cells := range cellCounts {
			for _, pga := range pgas {
				for _, invalid := range []bool{false, true} {
					e := eventWith(n, cells, pga, invalid)

					// Ekspektasi ditulis ulang dari §7.2, bukan dipanggil dari kode
					// yang diuji.
					distinct := e.independentCells()
					var want State
					switch {
					case invalid:
						want = StateCancelled
					case e.peakPGA() < MinPGAGal:
						want = StateDetected
					case n >= MinNodesConfirmed && distinct >= 2:
						want = StateConfirmed
					default:
						want = StateUnconfirmed
					}

					if got := classify(e); got != want {
						t.Errorf("classify(n=%d cells=%d pga=%g invalid=%v) = %s, mau %s",
							n, cells, pga, invalid, got, want)
					}
				}
			}
		}
	}
}

// Lantai PGA itu tertutup di atas: TEPAT pada MinPGAGal sudah melewati lantai,
// karena 16.6 adalah batas bawah label "moderate" pada Intensity().
func TestClassifyFloorIsInclusive(t *testing.T) {
	if got := classify(eventWith(1, 1, MinPGAGal, false)); got != StateUnconfirmed {
		t.Fatalf("PGA tepat di lantai = %s, mau UNCONFIRMED", got)
	}
	if got := classify(eventWith(1, 1, MinPGAGal-0.0001, false)); got != StateDetected {
		t.Fatalf("PGA sedikit di bawah lantai = %s, mau DETECTED", got)
	}
}

// Kuorum node TANPA independensi tidak boleh menjadi CONFIRMED: tiga sensor di
// satu meja bukan tiga bukti.
func TestClassifyQuorumWithoutIndependenceStaysUnconfirmed(t *testing.T) {
	e := eventWith(5, 1, MinPGAGal*4, false)
	if got := classify(e); got != StateUnconfirmed {
		t.Fatalf("5 node dalam 1 sel = %s, mau UNCONFIRMED", got)
	}
}

// Invalidated menang atas segalanya, termasuk atas bukti yang cukup untuk
// CONFIRMED: bukti yang ditarik bukan bukti yang lemah.
func TestClassifyInvalidatedOverridesEverything(t *testing.T) {
	e := eventWith(5, 5, MinPGAGal*10, true)
	if got := classify(e); got != StateCancelled {
		t.Fatalf("event invalidated = %s, mau CANCELLED", got)
	}
}

// Ambang independensi dibawa oleh event, bukan oleh variabel paket: dua event
// dengan ambang berbeda harus terklasifikasi berbeda pada bukti yang sama.
func TestClassifyUsesPerEventIndependenceThreshold(t *testing.T) {
	strict := eventWith(3, 2, MinPGAGal*2, false)
	strict.minCells = 3
	if got := classify(strict); got != StateUnconfirmed {
		t.Fatalf("2 sel dengan ambang 3 = %s, mau UNCONFIRMED", got)
	}

	loose := eventWith(3, 2, MinPGAGal*2, false)
	loose.minCells = 1
	if got := classify(loose); got != StateConfirmed {
		t.Fatalf("2 sel dengan ambang 1 = %s, mau CONFIRMED", got)
	}
}
