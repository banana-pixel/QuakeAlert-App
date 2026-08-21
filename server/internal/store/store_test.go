package store

import (
	"strings"
	"testing"
	"time"
)

// listEventsQuery menyusun SQL secara dinamis, jadi yang diuji di sini adalah
// hal yang bisa rusak tanpa terlihat: penomoran placeholder harus urut dan cocok
// dengan urutan args, karena satu pergeseran saja membuat radius dibandingkan
// dengan LIMIT tanpa Postgres pernah mengeluh soal sintaksis.

func TestListEventsQuery_TanpaFilter(t *testing.T) {
	q, args := listEventsQuery(nil, 20, 40)

	if strings.Contains(q, "WHERE") {
		t.Fatalf("query tanpa filter tidak boleh punya WHERE:\n%s", q)
	}
	if !strings.Contains(q, "LIMIT $1 OFFSET $2") {
		t.Fatalf("placeholder paginasi salah:\n%s", q)
	}
	if len(args) != 2 || args[0] != 20 || args[1] != 40 {
		t.Fatalf("args = %v, mau [20 40]", args)
	}
}

// Filter non-nil yang kosong diperlakukan seperti nil oleh ListEvents; di sini
// kita pastikan builder-nya pun tidak menghasilkan WHERE hampa.
func TestListEventsQuery_FilterKosong(t *testing.T) {
	if (&EventFilter{}).HasCriteria() {
		t.Fatal("filter tanpa kriteria harus dilaporkan kosong")
	}
	q, args := listEventsQuery(&EventFilter{}, 20, 0)
	if strings.Contains(q, "WHERE") || len(args) != 2 {
		t.Fatalf("query = %s, args = %v", q, args)
	}
}

func TestListEventsQuery_SemuaKriteria(t *testing.T) {
	pga := 137.2
	since := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	f := &EventFilter{Lat: -6.9, Lon: 107.6, RangeKm: 250, MinPGA: &pga, Since: &since, Until: &until}

	q, args := listEventsQuery(f, 20, 0)

	for _, want := range []string{
		"ST_MakePoint($1, $2), 4326)::geography, $3)",
		"max_pga >= $4",
		"started_at >= $5",
		"started_at <= $6",
		"LIMIT $7 OFFSET $8",
	} {
		if !strings.Contains(q, want) {
			t.Fatalf("query kehilangan %q:\n%s", want, q)
		}
	}
	if strings.Count(q, " AND ") != 3 {
		t.Fatalf("empat kriteria harus digabung dengan 3 AND:\n%s", q)
	}

	// Urutan args wajib sama dengan urutan placeholder; radius dikirim dalam meter.
	want := []any{107.6, -6.9, 250_000.0, 137.2, since, until, 20, 0}
	if len(args) != len(want) {
		t.Fatalf("args = %v, mau %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d] = %v, mau %v", i, args[i], want[i])
		}
	}
}

// Kriteria yang tidak dikirim tidak boleh menyisakan placeholder kosong: hanya
// yang aktif yang muncul, dan paginasi selalu menempati dua nomor terakhir.
func TestListEventsQuery_HanyaIntensitas(t *testing.T) {
	pga := 16.6
	q, args := listEventsQuery(&EventFilter{MinPGA: &pga}, 5, 10)

	if strings.Contains(q, "ST_DWithin") || strings.Contains(q, "started_at >=") {
		t.Fatalf("kriteria tak aktif ikut terpasang:\n%s", q)
	}
	if !strings.Contains(q, "WHERE max_pga >= $1") || !strings.Contains(q, "LIMIT $2 OFFSET $3") {
		t.Fatalf("penomoran placeholder salah:\n%s", q)
	}
	if len(args) != 3 || args[0] != 16.6 || args[1] != 5 || args[2] != 10 {
		t.Fatalf("args = %v", args)
	}
}

// RangeKm == 0 berarti "seluruh wilayah", bukan radius nol kilometer.
func TestListEventsQuery_RangeNolBukanRadius(t *testing.T) {
	since := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	f := &EventFilter{Lat: -6.9, Lon: 107.6, RangeKm: 0, Since: &since}

	q, args := listEventsQuery(f, 20, 0)

	if strings.Contains(q, "ST_DWithin") {
		t.Fatalf("RangeKm 0 tidak boleh memasang predikat spasial:\n%s", q)
	}
	if f.HasSpatial() {
		t.Fatal("HasSpatial harus false saat RangeKm 0")
	}
	if len(args) != 3 || args[0] != since {
		t.Fatalf("args = %v", args)
	}
}
