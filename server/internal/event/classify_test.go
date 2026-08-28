package event

import (
	"fmt"
	"testing"
)

// eventWith membangun Event dengan n kontributor yang menempati cells POSISI
// berbeda dan PGA puncak peak. Hanya untuk uji: classify tidak peduli dari mana
// kontributornya datang.
//
// Posisi, bukan label sel: independensi diukur sebagai jarak (independence.go),
// jadi kontributor yang harus terhitung sebagai bukti berbeda ditempatkan
// setidaknya minSepKm terpisah dan yang harus berbagi bukti ditempatkan pada
// koordinat yang SAMA. Menyetel Cell tanpa koordinat akan menjadikan seluruh
// armada satu titik dan uji ini tidak mengukur apa pun.
func eventWith(n, cells int, peak float64, invalidated bool) *Event {
	const sepKm = 5.0
	e := &Event{
		State:        StateDetected,
		Contributors: make(map[string]*Contributor, n),
		Invalidated:  invalidated,
		minCells:     2,
		minSepKm:     sepKm,
	}
	for i := 0; i < n; i++ {
		slot := 0
		if cells > 0 {
			slot = i % cells
		}
		// Slot dipisah 3x jarak pemisahan minimum: jauh melewati ambang, jadi
		// hasilnya tidak diputuskan oleh pembulatan haversine.
		lat, lon := destinationKm(0, 0, float64(slot)*sepKm*3, 90)
		pga := peak
		if i > 0 {
			// Hanya satu kontributor yang memegang puncak; sisanya di bawahnya,
			// supaya peakPGA benar-benar diuji sebagai maksimum.
			pga = peak / 2
		}
		id := fmt.Sprintf("NODE-%08d", i+1)
		e.Contributors[id] = &Contributor{
			NodeID: id, PeakPGA: pga, Lat: lat, Lon: lon,
			Cell: independenceCell(lat, lon, sepKm),
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
