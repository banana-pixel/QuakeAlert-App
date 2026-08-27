package event

// classify memetakan bukti sebuah event ke state yang layak diembannya.
//
// Murni, total, dan dapat diuji tanpa jam, basis data, atau broker — itulah
// seluruh alasan ia berdiri sendiri. Monotonisitas TIDAK dikodekan di sini: ia
// ditegakkan oleh legal() (§5.2), sehingga classify yang mengembalikan state lebih
// rendah (misalnya karena satu kontributor dibatalkan) tidak dapat memundurkan
// event.
func classify(e *Event) State {
	switch {
	case e.Invalidated:
		return StateCancelled
	case e.peakPGA() < MinPGAGal:
		return StateDetected
	case len(e.Contributors) >= MinNodesConfirmed && e.independentCells() >= minIndependentCells(e):
		return StateConfirmed
	default:
		return StateUnconfirmed
	}
}

// minIndependentCells mengambil ambang independensi yang berlaku untuk event ini.
//
// Ambangnya dikonfigurasi (MIN_INDEPENDENT_CELLS, default 2) sementara classify
// harus tetap murni, jadi nilainya dibawa OLEH event — disalin dari Tracker saat
// event dibuat. Membaca variabel paket akan membuat classify tidak murni dan
// membuat dua uji yang berjalan bersamaan dapat saling mengubah ambang.
func minIndependentCells(e *Event) int {
	if e.minCells <= 0 {
		return 1
	}
	return e.minCells
}
