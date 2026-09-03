package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- P4-M2′ (D-012): kontrak GET /api/v1/admin/tracker/near-confirmed ---
//
// Yang diuji di sini adalah BENTUK jawaban, bukan angkanya: nama bidang yang
// tertulis di contracts/openapi/openapi.yaml adalah kontraknya, dan alat forensik
// apa pun membacanya dari luar. Mengganti sebuah tag json diam-diam merusak
// pembaca itu tanpa satu pun kegagalan kompilasi.
//
// Tidak ada YAML yang di-parse: go.mod tidak membawa parser YAML, jadi daftar nama
// di bawah ini adalah transkripsi manual dari openapi.yaml — pola yang sama dengan
// admin_tracker_stats_test.go.

func trackerNearConfirmedRequest(key string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/tracker/near-confirmed", nil)
	if key != "" {
		req.Header.Set(AdminKeyHeader, key)
	}
	return req
}

// fullReport adalah satu entri dengan SETIAP bidang terisi, termasuk yang
// omitempty. Dipakai untuk memeriksa bahwa semua nama muncul; ketiadaan mereka saat
// nol diuji terpisah.
func fullReport() NearConfirmedReportJSON {
	return NearConfirmedReportJSON{
		Entries: []NearConfirmedEntryJSON{{
			EventID:                "A0000000-0000-4000-8000-000000000000",
			FirstTwoIndependentAt:  1_700_000_001_000,
			IndependentCountAtPeak: 3,
			NodeCountAtPeak:        4,
			ConfirmedAt:            1_700_000_002_000,
			TerminalState:          "RESOLVED",
			TerminalAt:             1_700_000_092_000,
			MinIndependentCells:    2,
			AlgoVer:                "phase3-1.1/ic=5",
			Source:                 "LOADED",
			UpdatedInProcess:       true,
		}},
		Coverage: NearConfirmedCoverageJSON{
			ProcessStartedAtMs:       1_700_000_000_000,
			AsOfMs:                   1_700_000_100_000,
			DurableReadAttempted:     true,
			DurableReadOK:            true,
			DurableReadAtMs:          1_700_000_000_500,
			DurableRowsLoaded:        1,
			DurableReadError:         "koneksi ditutup",
			EntriesRecordedInProcess: 0,
			EntriesLoadedFromDurable: 1,
			AlgoVer:                  "phase3-1.1/ic=9",
			MinIndependentCells:      2,
		},
	}
}

// nearConfirmedBody menjalankan permintaan dan membongkar badannya menjadi peta
// mentah, sehingga nama bidang yang HILANG dapat dibedakan dari nilai nol.
func nearConfirmedBody(t *testing.T, rep NearConfirmedReportJSON) map[string]json.RawMessage {
	t.Helper()
	src := &fakeTrackerStats{report: rep}
	rec := do(newTrackerStatsServer(src), trackerNearConfirmedRequest(adminTestKey))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", rec.Code, rec.Body.String())
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("respons bukan JSON objek: %v", err)
	}
	return got
}

func objectKeys(t *testing.T, raw json.RawMessage, what string) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%s bukan JSON objek: %v", what, err)
	}
	return m
}

// TestNearConfirmed_TopLevelShape — badan respons punya tepat dua bidang tingkat
// atas, `entries` dan `coverage`, keduanya wajib menurut kontrak.
//
// `entries` tetap array tingkat atas dengan nama yang sama seperti sebelum P4-M2′:
// selubungnya ADITIF, bukan pembungkus, jadi pembaca lama tetap menemukan
// daftarnya di tempat yang sama.
func TestNearConfirmed_TopLevelShape(t *testing.T) {
	got := nearConfirmedBody(t, fullReport())

	for _, field := range []string{"entries", "coverage"} {
		if _, ok := got[field]; !ok {
			t.Errorf("bidang wajib %q tidak ada di respons", field)
		}
	}
	if len(got) != 2 {
		t.Errorf("bidang tingkat atas = %d (%v), mau tepat 2: selubungnya aditif, "+
			"bukan pembungkus baru", len(got), rawKeys(got))
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(got["entries"], &arr); err != nil {
		t.Fatalf("entries bukan array: %v", err)
	}
}

func rawKeys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestNearConfirmed_EntryFieldNames — kesebelas nama bidang entri persis seperti
// yang tertulis di openapi.yaml.
//
// min_independent_cells dan algo_ver ada di daftar wajib dan itu disengaja: sebuah
// hitungan independensi hanya dapat ditafsirkan bersama ambang dan versi algoritma
// yang menghasilkannya, jadi entri tanpa keduanya adalah angka tanpa satuan.
func TestNearConfirmed_EntryFieldNames(t *testing.T) {
	got := nearConfirmedBody(t, fullReport())

	var arr []json.RawMessage
	if err := json.Unmarshal(got["entries"], &arr); err != nil {
		t.Fatalf("entries bukan array: %v", err)
	}
	if len(arr) != 1 {
		t.Fatalf("entri = %d, mau 1", len(arr))
	}
	entry := objectKeys(t, arr[0], "entries[0]")

	want := []string{
		// wajib menurut kontrak
		"event_id",
		"first_two_independent_at_ms",
		"independent_count_at_peak",
		"node_count_at_peak",
		"min_independent_cells",
		"algo_ver",
		"source",
		// opsional, hadir di potret ini karena semuanya terisi
		"confirmed_at_ms",
		"terminal_state",
		"terminal_at_ms",
		"updated_in_process",
	}
	for _, field := range want {
		if _, ok := entry[field]; !ok {
			t.Errorf("bidang entri %q tidak ada", field)
		}
	}
	for field := range entry {
		if !contains(want, field) {
			t.Errorf("bidang entri %q tidak ada di kontrak", field)
		}
	}
}

// TestNearConfirmed_EntryRequiredFieldsSurviveZero — ketujuh bidang WAJIB entri
// tetap terkirim ketika nilainya nol, dan keempat bidang opsional justru hilang.
//
// Perbedaannya bermakna dan bukan kosmetik: terminal_state yang absen berarti event
// masih terbuka, sedangkan independent_count_at_peak yang absen tidak berarti
// apa-apa — pembaca tidak dapat memulihkan angka yang tidak dikirim. Karena itu
// yang wajib tidak boleh omitempty dan yang opsional harus.
func TestNearConfirmed_EntryRequiredFieldsSurviveZero(t *testing.T) {
	// Entri paling kosong yang masih sah: sebuah persilangan sunyi pada event yang
	// masih terbuka, belum pernah CONFIRMED.
	rep := NearConfirmedReportJSON{Entries: []NearConfirmedEntryJSON{{
		EventID:                "A0000000-0000-4000-8000-000000000000",
		FirstTwoIndependentAt:  1_700_000_001_000,
		IndependentCountAtPeak: 0,
		NodeCountAtPeak:        0,
		MinIndependentCells:    0,
		AlgoVer:                "",
		Source:                 "RECORDED",
	}}}
	got := nearConfirmedBody(t, rep)

	var arr []json.RawMessage
	if err := json.Unmarshal(got["entries"], &arr); err != nil {
		t.Fatalf("entries bukan array: %v", err)
	}
	entry := objectKeys(t, arr[0], "entries[0]")

	for _, field := range []string{
		"event_id", "first_two_independent_at_ms", "independent_count_at_peak",
		"node_count_at_peak", "min_independent_cells", "algo_ver", "source",
	} {
		if _, ok := entry[field]; !ok {
			t.Errorf("bidang wajib %q hilang saat nol — ia tidak boleh omitempty", field)
		}
	}
	for _, field := range []string{
		"confirmed_at_ms", "terminal_state", "terminal_at_ms", "updated_in_process",
	} {
		if _, ok := entry[field]; ok {
			t.Errorf("bidang %q hadir padahal nol — absennya yang menyatakan "+
				"\"tidak pernah terjadi\"", field)
		}
	}
}

// TestNearConfirmed_CoverageFieldNames — kesembilan bidang wajib selubung cakupan
// hadir, dan tidak ada nama di luar kontrak.
func TestNearConfirmed_CoverageFieldNames(t *testing.T) {
	got := nearConfirmedBody(t, fullReport())
	cov := objectKeys(t, got["coverage"], "coverage")

	want := []string{
		// wajib
		"process_started_at_ms",
		"as_of_ms",
		"durable_read_attempted",
		"durable_read_ok",
		"durable_rows_loaded",
		"entries_recorded_in_process",
		"entries_loaded_from_durable",
		"algo_ver",
		"min_independent_cells",
		// opsional, terisi di potret ini
		"durable_read_at_ms",
		"durable_read_error",
	}
	for _, field := range want {
		if _, ok := cov[field]; !ok {
			t.Errorf("bidang coverage %q tidak ada", field)
		}
	}
	for field := range cov {
		if !contains(want, field) {
			t.Errorf("bidang coverage %q tidak ada di kontrak", field)
		}
	}

	// Ketiadaan ini yang disengaja: ini pengukuran cakupan, bukan penilaian. Sebuah
	// bidang bernama complete/healthy/valid akan mengubah jawaban jujur "belum
	// dicoba" menjadi rapor yang lulus atau gagal.
	for _, forbidden := range []string{"complete", "healthy", "valid", "ok", "status"} {
		if _, ok := cov[forbidden]; ok {
			t.Errorf("bidang coverage %q ada — selubung ini mengukur cakupan, "+
				"bukan menilai", forbidden)
		}
	}
}

// TestNearConfirmed_EmptyIsExplicitlyCovered — inilah alasan B1 ada.
//
// Pada fleet satu-node daftar yang BENAR adalah kosong (S2 — kuorum tidak
// terjangkau). Jawaban itu tidak boleh terkirim sebagai byte yang identik dengan
// "tidak ada yang dapat dijawab", jadi kedua bidang boolean pembacaan durable dan
// ketiga hitungannya harus tetap ada meskipun semuanya nol atau false.
func TestNearConfirmed_EmptyIsExplicitlyCovered(t *testing.T) {
	rep := NearConfirmedReportJSON{Coverage: NearConfirmedCoverageJSON{
		ProcessStartedAtMs:  1_700_000_000_000,
		AsOfMs:              1_700_000_010_000,
		AlgoVer:             "phase3-1.1/ic=5",
		MinIndependentCells: 2,
		// durable_read_attempted=false, rows=0, kedua hitungan entri 0.
	}}
	got := nearConfirmedBody(t, rep)

	if string(got["entries"]) != "[]" {
		t.Errorf("entries = %s, mau []: pembaca yang menghitung panjangnya tidak "+
			"boleh melihat dua bentuk untuk kosong", got["entries"])
	}
	cov := objectKeys(t, got["coverage"], "coverage")
	for _, field := range []string{
		"durable_read_attempted", "durable_read_ok", "durable_rows_loaded",
		"entries_recorded_in_process", "entries_loaded_from_durable",
	} {
		if _, ok := cov[field]; !ok {
			t.Errorf("bidang %q hilang saat nol/false — tanpa dia daftar kosong yang "+
				"BENAR tidak dapat dibedakan dari tidak ada jawaban", field)
		}
	}
	// Jendelanya tetap dinyatakan meski tidak ada satu pun entri: itu keseluruhan
	// gunanya.
	if _, ok := cov["process_started_at_ms"]; !ok {
		t.Error("process_started_at_ms hilang — awal jendela cakupan wajib dinyatakan")
	}
	if _, ok := cov["as_of_ms"]; !ok {
		t.Error("as_of_ms hilang — ujung jendela cakupan wajib dinyatakan")
	}
	// Yang tidak pernah dicoba tidak melaporkan waktu maupun galat.
	for _, field := range []string{"durable_read_at_ms", "durable_read_error"} {
		if _, ok := cov[field]; ok {
			t.Errorf("bidang %q hadir padahal pembacaan durable tidak pernah dicoba", field)
		}
	}
}

// TestNearConfirmed_NilEntriesNeverSerializesAsNull — slice nil dinormalkan menjadi
// array kosong di handler, bukan null.
func TestNearConfirmed_NilEntriesNeverSerializesAsNull(t *testing.T) {
	got := nearConfirmedBody(t, NearConfirmedReportJSON{Entries: nil})
	if string(got["entries"]) != "[]" {
		t.Errorf("entries = %s, mau []", got["entries"])
	}
}

// TestNearConfirmed_ReadFailureIsNotAnEmptyAnswer — daftar kosong yang disebabkan
// pembacaan durable yang GAGAL membawa galatnya, dan itu bentuk yang berbeda dari
// daftar kosong pada fleet satu-node.
func TestNearConfirmed_ReadFailureIsNotAnEmptyAnswer(t *testing.T) {
	rep := NearConfirmedReportJSON{Coverage: NearConfirmedCoverageJSON{
		ProcessStartedAtMs:   1_700_000_000_000,
		AsOfMs:               1_700_000_010_000,
		DurableReadAttempted: true,
		DurableReadOK:        false,
		DurableReadAtMs:      1_700_000_000_400,
		DurableReadError:     "koneksi ditutup",
		AlgoVer:              "phase3-1.1/ic=5",
		MinIndependentCells:  2,
	}}
	got := nearConfirmedBody(t, rep)
	cov := objectKeys(t, got["coverage"], "coverage")

	if string(got["entries"]) != "[]" {
		t.Fatalf("entries = %s, mau []", got["entries"])
	}
	if string(cov["durable_read_attempted"]) != "true" || string(cov["durable_read_ok"]) != "false" {
		t.Errorf("attempted/ok = %s/%s, mau true/false",
			cov["durable_read_attempted"], cov["durable_read_ok"])
	}
	if _, ok := cov["durable_read_error"]; !ok {
		t.Error("durable_read_error hilang — galatnya dilaporkan, tidak ditelan " +
			"menjadi daftar kosong")
	}
}

// TestNearConfirmed_SourceEnumValues — kedua nilai enum provenance terkirim apa
// adanya, dan sebuah entri LOADED yang berubah di proses ini TIDAK naik menjadi
// RECORDED.
func TestNearConfirmed_SourceEnumValues(t *testing.T) {
	rep := NearConfirmedReportJSON{Entries: []NearConfirmedEntryJSON{
		{EventID: "A", FirstTwoIndependentAt: 1, Source: "RECORDED"},
		{EventID: "B", FirstTwoIndependentAt: 2, Source: "LOADED", UpdatedInProcess: true},
	}}
	got := nearConfirmedBody(t, rep)

	var arr []struct {
		Source           string `json:"source"`
		UpdatedInProcess bool   `json:"updated_in_process"`
	}
	if err := json.Unmarshal(got["entries"], &arr); err != nil {
		t.Fatalf("entries tidak dapat dibongkar: %v", err)
	}
	if len(arr) != 2 || arr[0].Source != "RECORDED" || arr[1].Source != "LOADED" {
		t.Fatalf("source = %+v, mau [RECORDED LOADED]", arr)
	}
	if arr[1].UpdatedInProcess != true {
		t.Error("updated_in_process = false untuk entri LOADED yang berubah di sini")
	}
	if arr[0].UpdatedInProcess != false {
		t.Error("updated_in_process = true untuk entri RECORDED — bidang itu hanya " +
			"berbicara tentang entri yang dibaca dari tabel durable")
	}
}

// TestNearConfirmed_DisabledTrackerUnchanged — tanpa sumber statistik, endpoint
// tetap 503 seperti sebelum P4-M2′. Penambahan selubung bersifat aditif dan tidak
// boleh mengubah jalur EVENT_TRACKER_ENABLED=false.
func TestNearConfirmed_DisabledTrackerUnchanged(t *testing.T) {
	rec := do(newTrackerStatsServer(nil), trackerNearConfirmedRequest(adminTestKey))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, mau 503 saat Tracker tidak aktif", rec.Code)
	}
}

// TestNearConfirmed_RequiresAdminKey — endpointnya di belakang kunci operator,
// sama seperti /stats. Sebuah log forensik yang dapat dibaca tanpa kunci adalah
// peta sebaran node yang dapat dibaca tanpa kunci.
func TestNearConfirmed_RequiresAdminKey(t *testing.T) {
	src := &fakeTrackerStats{report: fullReport()}
	for _, key := range []string{"", "kunci-salah"} {
		rec := do(newTrackerStatsServer(src), trackerNearConfirmedRequest(key))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status untuk kunci %q = %d, mau 401", key, rec.Code)
		}
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
