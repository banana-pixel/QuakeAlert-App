package store

// --- Integrasi Postgres untuk event_near_confirmed (migrasi 000009, P4-M2′) ---
//
// Butuh Postgres NYATA, dan alasannya sama seperti event_lifecycle_test.go: yang
// diuji di sini adalah perilaku SKEMA dan KUERI-nya, bukan perilaku Go. Empat hal
// yang tidak dapat diuji dengan fake:
//
//	merge monoton   — LEAST/GREATEST, pasangan puncak di CASE, dan COALESCE pada
//	                  ketiga kolom terminal. Antrean ledger boleh mengantar TIDAK
//	                  URUT dan boleh MEMBUANG, jadi satu-satunya jaminan bahwa dua
//	                  penulisan untuk satu event aman dalam urutan apa pun adalah
//	                  SQL ini. Sebuah UPDATE yang menimpa tetap terlihat benar dari
//	                  Go selama ujinya kebetulan mengirim urutan naik.
//	NULL vs nol     — NULL berarti BELUM PERNAH TERJADI. NULLIF($7,'') dan ketiga
//	                  kolom pointer hanya nyata di Postgres.
//	tanpa FK        — baris yatim (persilangan sunyi tanpa induk) harus dapat
//	                  ditulis. Itu justru yang akan gagal bila FK pernah masuk.
//	ORDER BY        — urutan bacanya harus sama dengan urutan Tracker di memori.
//
//	TEST_DATABASE_URL=postgres://... go test ./internal/store -run TestNearConfirmed
//
// Tanpa env itu seluruh test di berkas ini skip.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Id uji dipisahkan dari berkas integrasi lain supaya pembersihan satu berkas
// tidak dapat menghapus baris berkas lain.
const (
	ncID1 = "cccccccc-0000-4000-8000-000000000001"
	ncID2 = "cccccccc-0000-4000-8000-000000000002"
	ncID3 = "cccccccc-0000-4000-8000-000000000003"
)

// putNear menulis satu baris lewat penulis PRODUKSI dan mendaftarkan
// pembersihannya. Bukan INSERT tangan: yang diuji adalah UpsertNearConfirmed,
// termasuk bentuk parameternya.
func putNear(t *testing.T, st *Store, r *NearConfirmedRow) {
	t.Helper()
	ctx := context.Background()
	if err := st.UpsertNearConfirmed(ctx, r); err != nil {
		t.Fatalf("UpsertNearConfirmed(%s): %v", r.EventID, err)
	}
	t.Cleanup(func() {
		_, _ = st.pool.Exec(ctx, `DELETE FROM event_near_confirmed WHERE event_id = $1`, r.EventID)
	})
}

// getNear membaca satu baris uji lewat ListNearConfirmed, yaitu jalur baca
// produksi, lalu menyaring ke event yang diminta. Penyaringan perlu karena
// ListNearConfirmed sengaja tanpa jendela dan tanpa batas: basis data uji bersama
// dapat memuat baris milik berkas lain.
func getNear(t *testing.T, st *Store, eventID string) *NearConfirmedRow {
	t.Helper()
	rows, err := st.ListNearConfirmed(context.Background())
	if err != nil {
		t.Fatalf("ListNearConfirmed: %v", err)
	}
	for i := range rows {
		if rows[i].EventID == eventID {
			return &rows[i]
		}
	}
	t.Fatalf("baris %s tidak ditemukan di antara %d baris", eventID, len(rows))
	return nil
}

func i64(v int64) *int64   { return &v }
func str(v string) *string { return &v }

// silentCrossing adalah bentuk baris yang paling penting bagi P4-M2′: sebuah
// persilangan yang tidak pernah menghasilkan transisi, jadi tidak ada baris induk
// di earthquake_events dan tidak ada baris riwayat di event_state_log.
func silentCrossing(id string) *NearConfirmedRow {
	return &NearConfirmedRow{
		EventID:                id,
		FirstTwoIndependentAt:  1_700_000_001_000,
		IndependentCountAtPeak: 2,
		NodeCountAtPeak:        2,
		MinIndependentCells:    2,
		AlgoVer:                "phase3-1.1/ic=5",
	}
}

// Persilangan sunyi dapat ditulis TANPA baris induk apa pun, dan itu keseluruhan
// alasan tabel ini tidak punya FOREIGN KEY.
//
// Bila FK pernah masuk, uji ini gagal dengan 23503 — dan di produksi kegagalan yang
// sama akan mengubah pencatatan yang SAH menjadi kegagalan tulis pada jalur yang
// tidak punya siapa pun untuk melapor.
func TestNearConfirmedRowNeedsNoParentEvent(t *testing.T) {
	st := newTestStore(t)
	putNear(t, st, silentCrossing(ncID1))

	got := getNear(t, st, ncID1)
	if got.IndependentCountAtPeak != 2 || got.NodeCountAtPeak != 2 {
		t.Errorf("puncak = %d/%d, mau 2/2", got.IndependentCountAtPeak, got.NodeCountAtPeak)
	}
	if got.MinIndependentCells != 2 || got.AlgoVer != "phase3-1.1/ic=5" {
		t.Errorf("parameter tercatat = %d/%q, mau 2/phase3-1.1/ic=5",
			got.MinIndependentCells, got.AlgoVer)
	}
}

// NULL bertahan sebagai NULL, dan nol bertahan sebagai nol.
//
// NULL di ketiga kolom itu berarti BELUM PERNAH TERJADI. Sebuah event yang tidak
// pernah CONFIRMED bukan event yang CONFIRMED pada epoch, jadi kolom NOT NULL
// DEFAULT 0 akan menghapus perbedaan yang justru ditanyakan pertanyaan forensiknya.
func TestNearConfirmedNullIsNotZero(t *testing.T) {
	st := newTestStore(t)
	putNear(t, st, silentCrossing(ncID1))

	got := getNear(t, st, ncID1)
	if got.ConfirmedAt != nil {
		t.Errorf("confirmed_at = %d, mau NULL: event ini tidak pernah CONFIRMED", *got.ConfirmedAt)
	}
	if got.TerminalState != nil || got.TerminalAt != nil {
		t.Errorf("terminal = %v/%v, mau NULL keduanya: event ini masih terbuka",
			got.TerminalState, got.TerminalAt)
	}

	// Nol yang SUNGGUHAN, sebaliknya, harus kembali sebagai nol dan bukan NULL.
	zero := silentCrossing(ncID2)
	zero.ConfirmedAt = i64(0)
	putNear(t, st, zero)

	if c := getNear(t, st, ncID2).ConfirmedAt; c == nil || *c != 0 {
		t.Errorf("confirmed_at = %v, mau pointer ke 0: nol yang dikirim bukan NULL", c)
	}
}

// Merge monoton: puncak hanya naik, "pertama kali" hanya bergerak ke masa lalu, dan
// keduanya kebal urutan kedatangan.
//
// Ujinya mengirim urutan TURUN dengan sengaja. Antrean ledger boleh mengantar tidak
// urut, jadi sebuah UPDATE yang menimpa akan lolos dari uji yang hanya mengirim
// urutan naik.
func TestNearConfirmedMergeIsOrderIndependent(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// Kedatangan pertama: puncak 3 pada t+5000.
	high := silentCrossing(ncID1)
	high.FirstTwoIndependentAt = 1_700_000_005_000
	high.IndependentCountAtPeak = 3
	high.NodeCountAtPeak = 4
	putNear(t, st, high)

	// Kedatangan kedua, TERLAMBAT dan lebih rendah: puncak 2 pada t+1000.
	late := silentCrossing(ncID1)
	late.FirstTwoIndependentAt = 1_700_000_001_000
	late.IndependentCountAtPeak = 2
	late.NodeCountAtPeak = 2
	if err := st.UpsertNearConfirmed(ctx, late); err != nil {
		t.Fatalf("upsert terlambat: %v", err)
	}

	got := getNear(t, st, ncID1)
	if got.FirstTwoIndependentAt != 1_700_000_001_000 {
		t.Errorf("first_two_independent_at = %d, mau 1700000001000: LEAST — "+
			"\"pertama kali\" hanya bergerak ke masa lalu", got.FirstTwoIndependentAt)
	}
	if got.IndependentCountAtPeak != 3 {
		t.Errorf("independent_count_at_peak = %d, mau 3: GREATEST — puncak tidak turun",
			got.IndependentCountAtPeak)
	}
	// Inilah yang dijaga CASE: node_count bergerak HANYA bersama puncak yang baru.
	// Tanpa itu barisnya akan melaporkan puncak 3 dengan 2 node, yaitu dua angka
	// dari dua saat yang berbeda — dan tiga node di satu atap bukan tiga bukti,
	// jadi keduanya hanya bermakna berpasangan.
	if got.NodeCountAtPeak != 4 {
		t.Errorf("node_count_at_peak = %d, mau 4: ia hanya ikut ketika puncaknya naik",
			got.NodeCountAtPeak)
	}
}

// node_count_at_peak IKUT ketika puncaknya benar-benar naik, dan tidak ikut ketika
// puncaknya sama. Sisi lain dari uji di atas.
func TestNearConfirmedPeakPairMovesTogether(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	putNear(t, st, silentCrossing(ncID1)) // puncak 2, node 2

	rise := silentCrossing(ncID1)
	rise.IndependentCountAtPeak = 4
	rise.NodeCountAtPeak = 6
	if err := st.UpsertNearConfirmed(ctx, rise); err != nil {
		t.Fatalf("upsert kenaikan: %v", err)
	}
	if got := getNear(t, st, ncID1); got.IndependentCountAtPeak != 4 || got.NodeCountAtPeak != 6 {
		t.Fatalf("puncak = %d/%d, mau 4/6", got.IndependentCountAtPeak, got.NodeCountAtPeak)
	}

	// Puncak SAMA dengan node_count berbeda: pasangannya tidak boleh bergeser.
	same := silentCrossing(ncID1)
	same.IndependentCountAtPeak = 4
	same.NodeCountAtPeak = 99
	if err := st.UpsertNearConfirmed(ctx, same); err != nil {
		t.Fatalf("upsert seri: %v", err)
	}
	if got := getNear(t, st, ncID1); got.NodeCountAtPeak != 6 {
		t.Errorf("node_count_at_peak = %d, mau 6: puncaknya tidak naik, jadi "+
			"pasangannya tidak ikut", got.NodeCountAtPeak)
	}
}

// confirmed_at, terminal_state dan terminal_at adalah COALESCE: yang PERTAMA
// non-NULL menang, cermin dari penjaga `== 0` di memori.
//
// Sebuah pembaruan yang tiba terlambat tidak boleh memindahkan waktu CONFIRMED,
// dan tidak boleh mengubah state terminal yang sudah tercatat: keduanya terjadi
// sekali.
func TestNearConfirmedTerminalColumnsAreFirstWins(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	first := silentCrossing(ncID1)
	first.ConfirmedAt = i64(1_700_000_002_000)
	first.TerminalState = str("RESOLVED")
	first.TerminalAt = i64(1_700_000_092_000)
	putNear(t, st, first)

	contra := silentCrossing(ncID1)
	contra.ConfirmedAt = i64(1_700_000_009_000)
	contra.TerminalState = str("CANCELLED")
	contra.TerminalAt = i64(1_700_000_099_000)
	if err := st.UpsertNearConfirmed(ctx, contra); err != nil {
		t.Fatalf("upsert yang bertentangan: %v", err)
	}

	got := getNear(t, st, ncID1)
	if got.ConfirmedAt == nil || *got.ConfirmedAt != 1_700_000_002_000 {
		t.Errorf("confirmed_at = %v, mau 1700000002000: yang pertama menang", got.ConfirmedAt)
	}
	if got.TerminalState == nil || *got.TerminalState != "RESOLVED" {
		t.Errorf("terminal_state = %v, mau RESOLVED: terminal terjadi sekali", got.TerminalState)
	}
	if got.TerminalAt == nil || *got.TerminalAt != 1_700_000_092_000 {
		t.Errorf("terminal_at = %v, mau 1700000092000", got.TerminalAt)
	}
}

// Sebuah baris yang masih terbuka DAPAT menjadi terminal: COALESCE mengisi yang
// masih NULL. Yang dilarang hanya menulis ULANG yang sudah terisi.
func TestNearConfirmedOpenRowCanBecomeTerminal(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	putNear(t, st, silentCrossing(ncID1)) // ketiga kolom NULL

	closed := silentCrossing(ncID1)
	closed.TerminalState = str("CANCELLED")
	closed.TerminalAt = i64(1_700_000_050_000)
	if err := st.UpsertNearConfirmed(ctx, closed); err != nil {
		t.Fatalf("upsert penutupan: %v", err)
	}

	got := getNear(t, st, ncID1)
	if got.TerminalState == nil || *got.TerminalState != "CANCELLED" {
		t.Errorf("terminal_state = %v, mau CANCELLED", got.TerminalState)
	}
	if got.TerminalAt == nil || *got.TerminalAt != 1_700_000_050_000 {
		t.Errorf("terminal_at = %v, mau 1700000050000", got.TerminalAt)
	}
	if got.ConfirmedAt != nil {
		t.Errorf("confirmed_at = %d, mau tetap NULL: event ini mati tanpa konfirmasi",
			*got.ConfirmedAt)
	}
}

// min_independent_cells dan algo_ver DIBEKUKAN pada nilai kedatangan pertama.
//
// Ini batas U-007 yang ditegakkan di SQL: sebuah biner dengan ambang atau jarak
// pemisahan yang sudah berbeda tidak boleh menulis ulang parameter baris lampau
// (V3/V6, D-006). Menilai keputusan lampau dengan parameter yang tidak
// menghasilkannya bukan koreksi, itu pemalsuan.
func TestNearConfirmedRecordedParametersAreNeverRewritten(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	putNear(t, st, silentCrossing(ncID1)) // cells 2, phase3-1.1/ic=5

	newer := silentCrossing(ncID1)
	newer.MinIndependentCells = 7
	newer.AlgoVer = "phase9-9.9/ic=50"
	if err := st.UpsertNearConfirmed(ctx, newer); err != nil {
		t.Fatalf("upsert biner baru: %v", err)
	}

	got := getNear(t, st, ncID1)
	if got.MinIndependentCells != 2 {
		t.Errorf("min_independent_cells = %d, mau tetap 2", got.MinIndependentCells)
	}
	if got.AlgoVer != "phase3-1.1/ic=5" {
		t.Errorf("algo_ver = %q, mau tetap phase3-1.1/ic=5", got.AlgoVer)
	}
}

// Satu baris per EVENT, bukan satu baris per penulisan: PRIMARY KEY pada event_id
// adalah cermin langsung map[event_id]*NearConfirmedEntry di memori, yang tetap
// menjadi otoritas (§9.5). Dua baris untuk satu event akan berarti dua kebenaran
// untuk satu event.
func TestNearConfirmedIsOneRowPerEvent(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	putNear(t, st, silentCrossing(ncID1))
	for i := 0; i < 3; i++ {
		if err := st.UpsertNearConfirmed(ctx, silentCrossing(ncID1)); err != nil {
			t.Fatalf("upsert ke-%d: %v", i+2, err)
		}
	}

	var n int
	if err := st.pool.QueryRow(ctx,
		`SELECT count(*) FROM event_near_confirmed WHERE event_id = $1`, ncID1).Scan(&n); err != nil {
		t.Fatalf("hitung baris: %v", err)
	}
	if n != 1 {
		t.Errorf("baris untuk satu event = %d, mau 1", n)
	}
}

// Urutan bacanya (first_two_independent_at, event_id), sama dengan urutan Tracker
// di memori.
//
// Kesamaan itu bukan kosmetik: jawaban yang dibangun ulang dari basis data harus
// tidak dapat dibedakan dari jawaban yang lahir di proses ini KECUALI oleh field
// provenance-nya, dan urutan yang berbeda adalah perbedaan kedua yang terlihat.
// Pemutus-seri leksikografis diuji dengan dua baris berwaktu SAMA.
func TestNearConfirmedListOrder(t *testing.T) {
	st := newTestStore(t)

	late := silentCrossing(ncID3)
	late.FirstTwoIndependentAt = 1_700_000_009_000
	putNear(t, st, late)

	// ncID2 dan ncID1 berwaktu sama: yang menang adalah event_id terkecil.
	tieB := silentCrossing(ncID2)
	tieB.FirstTwoIndependentAt = 1_700_000_001_000
	putNear(t, st, tieB)

	tieA := silentCrossing(ncID1)
	tieA.FirstTwoIndependentAt = 1_700_000_001_000
	putNear(t, st, tieA)

	rows, err := st.ListNearConfirmed(context.Background())
	if err != nil {
		t.Fatalf("ListNearConfirmed: %v", err)
	}
	// Saring ke baris milik uji ini saja: basis data uji bersama.
	var seq []string
	for _, r := range rows {
		switch r.EventID {
		case ncID1, ncID2, ncID3:
			seq = append(seq, r.EventID)
		}
	}
	want := []string{ncID1, ncID2, ncID3}
	if len(seq) != len(want) {
		t.Fatalf("baris uji terbaca = %d (%v), mau 3", len(seq), seq)
	}
	for i := range want {
		if seq[i] != want[i] {
			t.Fatalf("urutan = %v, mau %v", seq, want)
		}
	}
}

// 000009 dijalankan DUA KALI harus menjadi no-op, diterapkan pada basis data yang
// skemanya sudah dimigrasi. Bila CREATE TABLE kehilangan IF NOT EXISTS, penerapan
// kedua gagal di sini alih-alih di produksi.
func TestMigration000009IsIdempotent(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	up, err := os.ReadFile(filepath.Join("..", "..", "..",
		"contracts", "db", "migrations", "000009_near_confirmation_durability.up.sql"))
	if err != nil {
		t.Fatalf("baca migrasi: %v", err)
	}
	for i := 1; i <= 2; i++ {
		if _, err := st.pool.Exec(ctx, string(up)); err != nil {
			t.Fatalf("penerapan migrasi ke-%d gagal (idempotensi rusak): %v", i, err)
		}
	}

	var n int
	if err := st.pool.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.tables
		 WHERE table_schema = 'public' AND table_name = 'event_near_confirmed'`).Scan(&n); err != nil {
		t.Fatalf("periksa tabel: %v", err)
	}
	if n != 1 {
		t.Errorf("tabel event_near_confirmed ditemukan %d kali, mau 1", n)
	}
}

// §20.6 — rollback lengkap.
//
// down setelah up meninggalkan skema seperti sebelumnya, dan rollback ini lengkap
// justru karena 000009 tidak menyentuh satu pun tabel yang sudah ada: menghapus
// satu tabel baru sudah mengembalikan seluruh perubahan. Tidak ada migrasi
// berikutnya yang perlu diterapkan ulang — 000009 yang terakhir — jadi
// pemulihannya cukup menerapkan ulang berkas up-nya sendiri, dan itu dilakukan di
// Cleanup agar basis data uji keluar dalam keadaan termigrasi bagi berkas lain.
func TestMigration000009DownRestoresSchema(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	dir := filepath.Join("..", "..", "..", "contracts", "db", "migrations")
	down, err := os.ReadFile(filepath.Join(dir,
		"000009_near_confirmation_durability.down.sql"))
	if err != nil {
		t.Fatalf("baca migrasi down: %v", err)
	}
	up, err := os.ReadFile(filepath.Join(dir,
		"000009_near_confirmation_durability.up.sql"))
	if err != nil {
		t.Fatalf("baca migrasi up: %v", err)
	}

	t.Cleanup(func() {
		if _, err := st.pool.Exec(ctx, string(up)); err != nil {
			t.Fatalf("gagal menerapkan ulang 000009 setelah rollback: %v", err)
		}
	})

	if _, err := st.pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("rollback gagal: %v", err)
	}

	var n int
	if err := st.pool.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.tables
		 WHERE table_schema = 'public' AND table_name = 'event_near_confirmed'`).Scan(&n); err != nil {
		t.Fatalf("periksa tabel setelah rollback: %v", err)
	}
	if n != 0 {
		t.Errorf("tabel event_near_confirmed masih ada setelah rollback (%d)", n)
	}

	// Tabel lifecycle TIDAK boleh ikut hilang: 000009 tidak memilikinya, dan sebuah
	// down yang menjatuhkannya akan merusak setiap uji yang berjalan sesudah berkas
	// ini.
	for _, table := range []string{"earthquake_events", "event_state_log"} {
		if err := st.pool.QueryRow(ctx,
			`SELECT count(*) FROM information_schema.tables
			 WHERE table_schema = 'public' AND table_name = $1`, table).Scan(&n); err != nil {
			t.Fatalf("periksa tabel %s: %v", table, err)
		}
		if n != 1 {
			t.Errorf("tabel %s ditemukan %d kali setelah rollback 000009, mau 1", table, n)
		}
	}
}
