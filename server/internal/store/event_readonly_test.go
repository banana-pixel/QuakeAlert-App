package store

// --- Uji bukti sesi read-only P4-M6′ (D-015 delta) ---
//
// Unit murni (tanpa DB): validateReadOnly, isSafeAppName,
// EnsureApplicationName, DefaultAppName, FormatReadOnlyBanner.
// Integrasi PG (butuh TEST_DATABASE_URL): SessionReadOnly lewat pool yang
// sama — gagal-tertutup di sesi writable, lolos di sesi read-only DSN
// options=, galat-konteks gagal-tertutup, dan application_name terbawa
// sebagai korelasi (bukan bukti).

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestValidateReadOnlyPassesOnClient(t *testing.T) {
	if err := validateReadOnly("on", "client"); err != nil {
		t.Fatalf("on/client harus lolos: %v", err)
	}
	if err := validateDefaultRO("on", "client"); err != nil {
		t.Fatalf("default on/client harus lolos: %v", err)
	}
	if err := validateEffectiveRO("on"); err != nil {
		t.Fatalf("efektif on harus lolos: %v", err)
	}
}

func TestValidateReadOnlyOffFailsClosed(t *testing.T) {
	if err := validateReadOnly("off", "client"); err == nil {
		t.Error("off/client harus gagal-tertutup")
	}
	if err := validateReadOnly("off", "default"); err == nil {
		t.Error("off/default harus gagal-tertutup")
	}
	if err := validateDefaultRO("off", "client"); err == nil {
		t.Error("default off/client harus gagal-tertutup")
	}
	if err := validateEffectiveRO("off"); err == nil {
		t.Error("efektif off harus gagal-tertutup")
	}
}

func TestValidateReadOnlyUnexpectedSourceFailsClosed(t *testing.T) {
	// Default harus eksak on/client (penegakan DSN).
	for _, src := range []string{"default", "override", "configuration file", "", "CLIENT", "Client", "session"} {
		if err := validateDefaultRO("on", src); err == nil {
			t.Errorf("default on/%q harus gagal-tertutup (hanya on/client lolos)", src)
		}
	}
	// Efektif hanya menuntut on; source-nya override by design (termasuk
	// saat writable off|override), jadi tidak dipakai sebagai gerbang.
	// Dokumentasikan: efektif off tetap gagal walau source override.
	if err := validateEffectiveRO("off"); err == nil {
		t.Error("efektif off harus gagal-tertutup walau source override")
	}
}

func TestValidateReadOnlyMissingFailsClosed(t *testing.T) {
	if err := validateReadOnly("", ""); err == nil {
		t.Error("kosong/kosong harus gagal-tertutup (baris hilang)")
	}
	if err := validateReadOnly("on", ""); err == nil {
		t.Error("on/kosong harus gagal-tertutup")
	}
	if err := validateReadOnly("", "client"); err == nil {
		t.Error("kosong/client harus gagal-tertutup")
	}
	// Case-sensitif: Postgres mengembalikan huruf-kecil.
	if err := validateReadOnly("ON", "client"); err == nil {
		t.Error("ON/client harus gagal-tertutup (eksak huruf-kecil)")
	}
}

func TestIsSafeAppName(t *testing.T) {
	for _, ok := range []string{"m6-4fcc3374-20260906T120000Z", "a", "A-_.9", strings.Repeat("x", 64)} {
		if !isSafeAppName(ok) {
			t.Errorf("%q harus aman", ok)
		}
	}
	for _, bad := range []string{"", strings.Repeat("x", 65), "m6-a&b", "m6-a=b",
		"m6-a?b", "m6 a", "m6/a", "m6;a", "m6:b", "m6@x", "m6*x"} {
		if isSafeAppName(bad) {
			t.Errorf("%q harus ditolak", bad)
		}
	}
}

func TestEnsureApplicationNameAppendsAndPreserves(t *testing.T) {
	base := "postgres://u:p@localhost:5432/db?sslmode=disable"
	got, err := EnsureApplicationName(base, "m6-4fcc3374-20260906T120000Z")
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if !strings.Contains(got, "&application_name=m6-4fcc3374-20260906T120000Z") {
		t.Errorf("harus appended dengan &: %q", got)
	}
	if !strings.HasPrefix(got, base) {
		t.Errorf("kredensial/host/parameter lain tidak boleh diubah: %q", got)
	}

	noQuery := "postgres://u:p@localhost:5432/db"
	got, err = EnsureApplicationName(noQuery, "m6-abc")
	if err != nil {
		t.Fatalf("Ensure tanpa query: %v", err)
	}
	if !strings.Contains(got, "?application_name=m6-abc") {
		t.Errorf("harus appended dengan ?: %q", got)
	}

	already := base + "&application_name=existing-one"
	got, err = EnsureApplicationName(already, "m6-other")
	if err != nil {
		t.Fatalf("Ensure existing: %v", err)
	}
	if got != already {
		t.Errorf("bila sudah ada harus dikembalikan apa adanya: %q", got)
	}
	upper := base + "&APPLICATION_NAME=existing-one"
	if got, _ := EnsureApplicationName(upper, "m6-other"); got != upper {
		t.Errorf("pemeriksaan harus case-insensitive: %q", got)
	}

	if _, err := EnsureApplicationName(base, "m6-a&b"); err == nil {
		t.Error("appName tidak aman harus gagal-tertutup")
	}
}

func TestDefaultAppNameSafeAndFailsClosed(t *testing.T) {
	got, err := DefaultAppName("4fcc3374-032a-440b-9d6f-609d8a4096ce", time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("DefaultAppName: %v", err)
	}
	if got != "m6-4fcc3374-20260906T120000Z" {
		t.Errorf("got %q; mau m6-4fcc3374-20260906T120000Z", got)
	}
	if !isSafeAppName(got) {
		t.Errorf("%q harus lolos isSafeAppName", got)
	}
	if _, err := DefaultAppName("!!!", time.Now()); err == nil {
		t.Error("event_id tanpa 8 alfanumerik harus gagal-tertutup")
	}
	if _, err := DefaultAppName("", time.Now()); err == nil {
		t.Error("event_id kosong harus gagal-tertutup")
	}
}

func TestFormatReadOnlyBannerNoSecret(t *testing.T) {
	out := FormatReadOnlyBanner(ReadOnlyProof{DefaultSetting: "on", DefaultSource: "client", EffectiveSetting: "on", EffectiveSource: "override", ApplicationName: "m6-4fcc3374-20260906T120000Z"})
	if !strings.Contains(out, "default_transaction_read_only=on/client") ||
		!strings.Contains(out, "effective_transaction_read_only=on/override") ||
		!strings.Contains(out, "m6-4fcc3374-20260906T120000Z") {
		t.Errorf("banner harus memuat kedua GUC + app: %q", out)
	}
	for _, pat := range []string{"postgres://", "password", "secret", "passwd", "DSN", "DATABASE_URL", "jwt", "token", "bearer", "api_key", "apikey"} {
		if strings.Contains(strings.ToLower(out), strings.ToLower(pat)) {
			t.Errorf("banner mengandung pola kredensial %q: %q", pat, out)
		}
	}
	empty := FormatReadOnlyBanner(ReadOnlyProof{DefaultSetting: "on", DefaultSource: "client", EffectiveSetting: "on", EffectiveSource: "override"})
	if !strings.Contains(empty, "(kosong)") {
		t.Errorf("app kosong harus eksplisit, bukan hilang: %q", empty)
	}
}

// --- Integrasi PG: butuh TEST_DATABASE_URL, skip bila absen ---

func roDSN(t *testing.T, base, app string) string {
	t.Helper()
	dsnWithApp, err := EnsureApplicationName(base, app)
	if err != nil {
		t.Fatalf("EnsureApplicationName: %v", err)
	}
	// Opsi read-only lewat DSN (pgx menghormatinya; PGOPTIONS diabaikan pgx).
	// Encoding sama dengan RUNBOOK: spasi=%20, ==%3D.
	sep := "?"
	if strings.Contains(dsnWithApp, "?") {
		sep = "&"
	}
	return dsnWithApp + sep + "options=-c%20default_transaction_read_only%3Don"
}

// Sesi writable bawaan harus gagal-tertutup (bukan lolos diam-diam).
func TestSessionReadOnlyFailsClosedOnWritable(t *testing.T) {
	st := newTestStore(t)
	if _, err := st.SessionReadOnly(context.Background()); err == nil {
		t.Fatal("sesi writable harus gagal-tertutup (mau galat, dapat lolos)")
	}
}

// Sesi dengan DSN options= harus lolos default on/client + efektif on dan membawa appName.
func TestSessionReadOnlyPassesOnReadOnlyDSN(t *testing.T) {
	base := testDBURL(t)
	const app = "m6-test-20260906T120000Z"
	st, err := New(context.Background(), roDSN(t, base, app))
	if err != nil {
		t.Fatalf("New read-only: %v", err)
	}
	t.Cleanup(st.Close)
	proof, err := st.SessionReadOnly(context.Background())
	if err != nil {
		t.Fatalf("sesi read-only DSN harus lolos: %v", err)
	}
	if proof.DefaultSetting != "on" || proof.DefaultSource != "client" {
		t.Errorf("default proof = %+v; mau on/client", proof)
	}
	if proof.EffectiveSetting != "on" {
		t.Errorf("efektif proof = %+v; mau on", proof)
	}
	if proof.ApplicationName != app {
		t.Errorf("application_name = %q; mau %q (korelasi, bukan bukti)", proof.ApplicationName, app)
	}
}

// Galat kueri (konteks dibatalkan) harus gagal-tertutup.
func TestSessionReadOnlyQueryErrorFailsClosed(t *testing.T) {
	st := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := st.SessionReadOnly(ctx); err == nil {
		t.Fatal("konteks dibatalkan harus gagal-tertutup")
	}
}
