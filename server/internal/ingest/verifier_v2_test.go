package ingest

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/ledger"
	"github.com/banana-pixel/quakealert/server/internal/store"
)

// signedV2 membangun payload v2 yang ditandatangani dengan secret uji, sehingga
// test dapat menjangkau langkah-langkah SESUDAH HMAC (dedup, anti-replay).
// detriggerTS 0 berarti "tidak ada" pada payload, sesuai kontrak PRELIM.
func signedV2(secret []byte, phase string, obsSeq int64, attemptNo int,
	pga float64, durMs, onsetTS, detriggerTS, ts int64,
) []byte {
	canonical := CanonicalStringV2(ProtoVerV2, "NODE-0A1B2C3D", phase, obsSeq,
		attemptNo, pga, durMs, onsetTS, detriggerTS, ts)

	raw := `{"proto_ver":2,"node_id":"NODE-0A1B2C3D","phase":"` + phase +
		`","obs_seq":` + strconv.FormatInt(obsSeq, 10) +
		`,"attempt_no":` + strconv.Itoa(attemptNo) +
		`,"pga":` + strconv.FormatFloat(pga, 'f', 4, 64) +
		`,"dur_ms":` + strconv.FormatInt(durMs, 10) +
		`,"onset_ts":` + strconv.FormatInt(onsetTS, 10)
	if detriggerTS != 0 {
		raw += `,"detrigger_ts":` + strconv.FormatInt(detriggerTS, 10)
	}
	raw += `,"ts":` + strconv.FormatInt(ts, 10) +
		`,"signature":"` + ComputeHMAC(secret, canonical) + `"}`
	return []byte(raw)
}

// Waktu uji: jam server tetap di 1_700_000_005_000 (newTestVerifier).
const (
	v2Now     = int64(1_700_000_005_000)
	v2OnsetPr = v2Now - 300 // PRELIM: dur_ms 300, publish satu iterasi setelah onset
	v2DurPr   = int64(300)
)

func activeNode() *fakeNodeSource {
	return &fakeNodeSource{node: &store.NodeSecret{
		StationID: "NODE-0A1B2C3D", IsActive: true, Verified: true,
	}}
}

// TestVerifyTrigger_V2Accepted adalah separuh kriteria keluar §18: node v2
// ter-ingest dengan benar, dan publish_ts - onset_ts sama dengan dur_ms untuk
// PRELIM (yaitu ~0 di luar durasi latch onset itu sendiri).
func TestVerifyTrigger_V2Accepted(t *testing.T) {
	src := activeNode()
	v := newTestVerifier(t, src)

	tr, err := v.Verify(context.Background(),
		signedV2([]byte("test-key"), PhasePrelim, 196609, 1, 0.4215, v2DurPr, v2OnsetPr, 0, v2Now))
	if err != nil {
		t.Fatalf("PRELIM v2 yang sah ditolak: %v", err)
	}
	if !tr.IsV2() || tr.Phase != PhasePrelim {
		t.Fatalf("trigger = v2:%v phase:%q, want v2:true PRELIM", tr.IsV2(), tr.Phase)
	}
	if got := tr.TS - *tr.OnsetTS; got != v2DurPr {
		t.Errorf("publish_ts - onset_ts = %d, want %d (dur_ms)", got, v2DurPr)
	}
	if !src.lastSeenHit {
		t.Error("anti-replay DB seharusnya dijalankan untuk v2 yang sah")
	}
}

// TestVerifyTrigger_V1AndV2Coexist adalah separuh kriteria keluar §18 yang lain:
// kedua versi ter-ingest oleh verifier yang SAMA, tanpa flag apa pun di antara
// keduanya. Node yang firmware-nya dirollback kembali menjadi node v1 dan tetap
// diterima (§12.1).
func TestVerifyTrigger_V1AndV2Coexist(t *testing.T) {
	secret := []byte("test-key")
	v := newTestVerifier(t, activeNode())

	if _, err := v.Verify(context.Background(),
		signedV2(secret, PhasePrelim, 196609, 1, 0.4215, v2DurPr, v2OnsetPr, 0, v2Now-1)); err != nil {
		t.Fatalf("v2 ditolak: %v", err)
	}
	if _, err := v.Verify(context.Background(), validTrigger(t, secret)); err != nil {
		t.Fatalf("v1 ditolak setelah v2 diterima: %v", err)
	}
}

// TestVerifyTrigger_V2BadSignature memastikan tanda tangan v2 diperiksa terhadap
// bentuk kanonik v2 — bukan bentuk v1 yang kebetulan memakai field yang sama.
func TestVerifyTrigger_V2BadSignature(t *testing.T) {
	// Tanda tangan atas bentuk kanonik V1 dari field-field yang sama.
	v1Canonical := CanonicalString("NODE-0A1B2C3D", 0.4215, v2DurPr, v2Now)
	raw := `{"proto_ver":2,"node_id":"NODE-0A1B2C3D","phase":"PRELIM","obs_seq":1,` +
		`"attempt_no":1,"pga":0.4215,"dur_ms":300,"onset_ts":` +
		strconv.FormatInt(v2OnsetPr, 10) + `,"ts":` + strconv.FormatInt(v2Now, 10) +
		`,"signature":"` + ComputeHMAC([]byte("test-key"), v1Canonical) + `"}`

	v := newTestVerifier(t, activeNode())
	_, err := v.Verify(context.Background(), []byte(raw))
	if !errors.Is(err, ErrBadSignature) {
		t.Fatalf("err = %v, want ErrBadSignature (v2 ditandatangani dengan bentuk v1)", err)
	}
}

// ---------------------------------------------------------------------------
// §14.3 — deduplikasi berbasis phase
// ---------------------------------------------------------------------------

// TestVerifyTrigger_DuplicateObsSeqRejected: percobaan kirim ULANG dari observasi
// yang sama ditolak. ts distempel ULANG pada setiap percobaan, jadi gerbang
// last_seen_ts monotonik meloloskannya — hanya kunci (node_id, obs_seq, phase)
// yang menangkapnya.
func TestVerifyTrigger_DuplicateObsSeqRejected(t *testing.T) {
	secret := []byte("test-key")
	v := newTestVerifier(t, activeNode())

	first := signedV2(secret, PhasePrelim, 196609, 1, 0.4215, v2DurPr, v2OnsetPr, 0, v2Now-2)
	if _, err := v.Verify(context.Background(), first); err != nil {
		t.Fatalf("PRELIM pertama ditolak: %v", err)
	}

	// attempt_no 2, ts lebih BARU: lolos anti-replay, wajib gagal dedup.
	retry := signedV2(secret, PhasePrelim, 196609, 2, 0.4215, v2DurPr, v2OnsetPr, 0, v2Now)
	_, err := v.Verify(context.Background(), retry)
	if !errors.Is(err, ErrDuplicateObservation) {
		t.Fatalf("err = %v, want ErrDuplicateObservation", err)
	}
}

// Satu event v2 yang koheren, sebagaimana firmware memublikasikannya: onset,
// PRELIM satu latch (300 ms) sesudahnya, FINAL pada detrigger. Nilai-nilai ini
// harus lolos verifyOnsetCoherence tanpa toleransi apa pun terpakai.
const (
	evOnset    = v2Now - 2800 // onset menurut jam sensor
	evPrelimTS = evOnset + 300
	evFinalTS  = v2Now // = detrigger_ts; FINAL dipublish saat event ditutup
	evFinalDur = evFinalTS - evOnset
)

// TestVerifyTrigger_PrelimAndFinalBothAccepted: PRELIM dan FINAL berbagi obs_seq
// dan HARUS keduanya diterima — phase adalah bagian dari kunci, bukan hiasan.
func TestVerifyTrigger_PrelimAndFinalBothAccepted(t *testing.T) {
	secret := []byte("test-key")
	v := newTestVerifier(t, activeNode())

	if _, err := v.Verify(context.Background(),
		signedV2(secret, PhasePrelim, 196609, 1, 0.4215, v2DurPr, evOnset, 0, evPrelimTS)); err != nil {
		t.Fatalf("PRELIM ditolak: %v", err)
	}
	if _, err := v.Verify(context.Background(),
		signedV2(secret, PhaseFinal, 196609, 1, 1.8842, evFinalDur, evOnset, evFinalTS, evFinalTS)); err != nil {
		t.Fatalf("FINAL dengan obs_seq yang sama ditolak: %v", err)
	}
}

// TestVerifyTrigger_RetriedPrelimAfterFinalRejected adalah urutan yang menjadi
// SEBAB phase masuk ke dalam kunci dedup: PRELIM yang diulang SETELAH FINAL
// membawa ts yang paling baru dari ketiganya, jadi gerbang monotonik justru
// meloloskannya, dan konsensus akan melihat sebuah observasi "awal" yang datang
// sesudah observasi akhirnya.
func TestVerifyTrigger_RetriedPrelimAfterFinalRejected(t *testing.T) {
	secret := []byte("test-key")
	v := newTestVerifier(t, activeNode())

	if _, err := v.Verify(context.Background(),
		signedV2(secret, PhasePrelim, 196609, 1, 0.4215, v2DurPr, evOnset, 0, evPrelimTS)); err != nil {
		t.Fatalf("PRELIM ditolak: %v", err)
	}
	if _, err := v.Verify(context.Background(),
		signedV2(secret, PhaseFinal, 196609, 1, 1.8842, evFinalDur, evOnset, evFinalTS, evFinalTS)); err != nil {
		t.Fatalf("FINAL ditolak: %v", err)
	}

	// PRELIM diulang dan ts-nya distempel ULANG, sehingga ts > last_seen_ts yang
	// baru saja ditetapkan FINAL. Gerbang monotonik meloloskannya; dedup tidak.
	_, err := v.Verify(context.Background(),
		signedV2(secret, PhasePrelim, 196609, 3, 0.4215, v2DurPr, evOnset, 0, evFinalTS+500))
	if !errors.Is(err, ErrDuplicateObservation) {
		t.Fatalf("err = %v, want ErrDuplicateObservation", err)
	}
}

// TestVerifyTrigger_DedupIsAfterHMAC: tanda tangan yang SALAH tidak boleh
// mengisi cache dedup. Bila ia mengisi, siapa pun yang dapat memublikasikan ke
// broker dapat "memakai" obs_seq milik node lain lebih dulu sehingga observasi
// ASLI berikutnya ditolak sebagai duplikat.
func TestVerifyTrigger_DedupIsAfterHMAC(t *testing.T) {
	secret := []byte("test-key")
	v := newTestVerifier(t, activeNode())

	forged := signedV2([]byte("kunci-salah"), PhasePrelim, 196609, 1,
		0.4215, v2DurPr, v2OnsetPr, 0, v2Now-1)
	if _, err := v.Verify(context.Background(), forged); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("err = %v, want ErrBadSignature", err)
	}

	genuine := signedV2(secret, PhasePrelim, 196609, 1, 0.4215, v2DurPr, v2OnsetPr, 0, v2Now)
	if _, err := v.Verify(context.Background(), genuine); err != nil {
		t.Fatalf("observasi asli ditolak setelah payload palsu: %v", err)
	}
}

// ---------------------------------------------------------------------------
// seqCache
// ---------------------------------------------------------------------------

func TestSeqCache_AdmitAndPrune(t *testing.T) {
	c := newSeqCache(MaxTriggerAge)
	k := seqKey{nodeID: "NODE-0A1B2C3D", obsSeq: 1, phase: PhasePrelim}
	ttl := int64(MaxTriggerAge / time.Millisecond)

	if !c.admit(k, v2Now) {
		t.Fatal("entri pertama harus diterima")
	}
	if c.admit(k, v2Now) {
		t.Fatal("entri kedua yang identik harus ditolak")
	}
	// phase berbeda = kunci berbeda.
	if !c.admit(seqKey{nodeID: k.nodeID, obsSeq: k.obsSeq, phase: PhaseFinal}, v2Now) {
		t.Fatal("phase berbeda harus menjadi kunci berbeda")
	}
	if c.size() != 2 {
		t.Fatalf("size = %d, want 2", c.size())
	}

	// Sesudah TTL, kedatangan berikutnya membersihkan entri lama.
	if !c.admit(seqKey{nodeID: k.nodeID, obsSeq: 2, phase: PhasePrelim}, v2Now+ttl+1) {
		t.Fatal("entri baru sesudah TTL harus diterima")
	}
	if c.size() != 1 {
		t.Fatalf("size sesudah prune = %d, want 1 (hanya entri baru)", c.size())
	}
	// Dan kunci yang sudah dilupakan dapat diterima kembali — TTL cache SAMA
	// dengan jendela freshness, jadi duplikat sesungguhnya sudah ditolak
	// ErrClockSkew sebelum sampai ke sini.
	if !c.admit(k, v2Now+ttl+1) {
		t.Fatal("kunci yang sudah diprune harus dapat diterima kembali")
	}
}

// ---------------------------------------------------------------------------
// §14.4 — pemeriksaan waktu kedua & independen
// ---------------------------------------------------------------------------

// TestVerifyOnsetCoherence menguji fungsinya langsung: ia adalah gerbang murni
// atas field-field yang DITANDATANGANI, jadi tidak butuh store, kripto, atau jam
// server. Semua kasus di sini lolos gerbang skew — yang gagal hanyalah
// konsistensi laporan terhadap dirinya sendiri.
func TestVerifyOnsetCoherence(t *testing.T) {
	tol := int64(MaxOnsetSkew / time.Millisecond)

	v2 := func(phase string, attemptNo int, durMs, onsetTS, detriggerTS, ts int64) *Trigger {
		pv, an := ProtoVerV2, attemptNo
		seq := int64(1)
		tr := &Trigger{
			NodeID: "NODE-0A1B2C3D", PGA: 0.4215, DurMs: durMs, TS: ts,
			ProtoVer: &pv, Phase: phase, ObsSeq: &seq, AttemptNo: &an,
			OnsetTS: &onsetTS,
		}
		if detriggerTS != 0 {
			tr.DetriggerTS = &detriggerTS
		}
		return tr
	}

	cases := []struct {
		name    string
		trigger *Trigger
		wantErr error
	}{
		{
			name:    "v1 tidak diperiksa",
			trigger: &Trigger{NodeID: "NODE-0A1B2C3D", DurMs: 8000, TS: v2Now},
		},
		{
			name:    "PRELIM rapat",
			trigger: v2(PhasePrelim, 1, 300, evOnset, 0, evOnset+300),
		},
		{
			name:    "FINAL rapat",
			trigger: v2(PhaseFinal, 1, 2800, evOnset, evOnset+2800, evOnset+2800),
		},
		{
			// Getaran tidak dapat dimulai sesudah laporannya dipublish.
			name:    "onset sesudah ts",
			trigger: v2(PhasePrelim, 1, 300, evOnset, 0, evOnset-1),
			wantErr: ErrOnsetIncoherent,
		},
		{
			// dur_ms adalah waktu yang BERLALU antara onset dan publish; ia tidak
			// dapat melebihi selisihnya.
			name:    "dur_ms melebihi selisih onset..ts",
			trigger: v2(PhasePrelim, 1, 5000, evOnset, 0, evOnset+300),
			wantErr: ErrOnsetIncoherent,
		},
		{
			name:    "batas bawah tepat di toleransi",
			trigger: v2(PhasePrelim, 1, 300+tol, evOnset, 0, evOnset+300),
		},
		{
			// Percobaan PERTAMA dipublish satu iterasi loop setelah keadaan
			// berubah, jadi batas ATAS berlaku untuknya.
			name:    "attempt 1 terlalu lambat",
			trigger: v2(PhasePrelim, 1, 300, evOnset, 0, evOnset+300+tol+1),
			wantErr: ErrOnsetIncoherent,
		},
		{
			// Percobaan berikutnya: keterlambatan retry TIDAK terbatas, dan
			// itulah yang attempt_no ada untuk membuat terlihat. Payload yang
			// sama persis harus LOLOS pada attempt 2.
			name:    "attempt 2 lambat tetap lolos",
			trigger: v2(PhasePrelim, 2, 300, evOnset, 0, evOnset+300+tol+1),
		},
		{
			name:    "FINAL: detrigger sebelum onset",
			trigger: v2(PhaseFinal, 1, 2800, evOnset, evOnset-1, evOnset+2800),
			wantErr: ErrOnsetIncoherent,
		},
		{
			name:    "FINAL: detrigger sesudah ts",
			trigger: v2(PhaseFinal, 1, 2800, evOnset, evOnset+2801, evOnset+2800),
			wantErr: ErrOnsetIncoherent,
		},
		{
			// Pemeriksaan terkuat: sepenuhnya di dalam jam sensor, tidak
			// bergantung sama sekali pada penundaan publish — jadi ia berlaku
			// pada attempt berapa pun.
			name:    "FINAL: detrigger-onset tidak sepadan dur_ms",
			trigger: v2(PhaseFinal, 7, 2800, evOnset, evOnset+2800+tol+1, evOnset+9000),
			wantErr: ErrOnsetIncoherent,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyOnsetCoherence(tc.trigger)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// TestVerifyTrigger_IncoherentOnsetRejectedBeforeDB: gerbang koherensi berdiri
// SEBELUM basis data dan kripto, seperti gerbang skew — penolakannya terlihat
// dari tidak pernah ada panggilan lanjutan.
func TestVerifyTrigger_IncoherentOnsetRejectedBeforeDB(t *testing.T) {
	src := activeNode()
	v := newTestVerifier(t, src)

	// dur_ms 5000 dengan selisih onset..ts hanya 300 ms.
	_, err := v.Verify(context.Background(),
		signedV2([]byte("test-key"), PhasePrelim, 1, 1, 0.4215, 5000, evOnset, 0, evOnset+300))
	if !errors.Is(err, ErrOnsetIncoherent) {
		t.Fatalf("err = %v, want ErrOnsetIncoherent", err)
	}
	if src.lastSeenHit {
		t.Error("anti-replay DB tersentuh untuk trigger yang ditolak sebelum DB")
	}
}

// ---------------------------------------------------------------------------
// §5.1/§5.2 — pemetaan provenance v2 ke baris ledger
// ---------------------------------------------------------------------------

func TestTriggerObservation_V2Provenance(t *testing.T) {
	secret := []byte("test-key")
	raw := signedV2(secret, PhaseFinal, 196609, 4, 1.8842, evFinalDur, evOnset, evFinalTS, evFinalTS)
	tr, err := ParseTrigger(raw)
	if err != nil {
		t.Fatalf("payload uji tidak sah: %v", err)
	}

	o := TriggerObservation(tr, v2Now+7, ledger.VerifyResultOK)

	if o.Phase != ledger.PhaseFinal {
		t.Errorf("phase = %q, want FINAL", o.Phase)
	}
	if o.ProtoVer == nil || *o.ProtoVer != ProtoVerV2 {
		t.Errorf("proto_ver = %v, want 2", o.ProtoVer)
	}
	if o.ObsSeq == nil || *o.ObsSeq != 196609 {
		t.Errorf("obs_seq = %v, want 196609", o.ObsSeq)
	}
	if o.AttemptNo == nil || *o.AttemptNo != 4 {
		t.Errorf("attempt_no = %v, want 4", o.AttemptNo)
	}
	if o.OnsetTS == nil || *o.OnsetTS != evOnset {
		t.Errorf("onset_ts = %v, want %d", o.OnsetTS, evOnset)
	}
	if o.DetriggerTS == nil || *o.DetriggerTS != evFinalTS {
		t.Errorf("detrigger_ts = %v, want %d", o.DetriggerTS, evFinalTS)
	}
	// SENSOR, bukan PUBLISH_BOUND: onset di sini diukur jam sensor dan ikut
	// ditandatangani. Diskriminator inilah yang membuat fleet campuran dapat
	// dikorelasikan dengan benar nanti (§12.3).
	if o.OnsetTSSource != ledger.OnsetSourceSensor {
		t.Errorf("onset_ts_source = %q, want SENSOR", o.OnsetTSSource)
	}
	// Batas atas TETAP dihitung untuk v2 (§5.1 aturan 2): membandingkannya
	// dengan onset yang sebenarnya adalah satu-satunya cara mengukur
	// publish_delay — dan publish_delay itulah yang membuat batas tersebut tidak
	// dapat dipakai mengkalibrasi apa pun.
	wantBound := tr.TS - tr.DurMs
	if o.OnsetTSUpperBound == nil || *o.OnsetTSUpperBound != wantBound {
		t.Errorf("onset_ts_upper_bound = %v, want %d", o.OnsetTSUpperBound, wantBound)
	}
	if o.ReceivedTS != v2Now+7 {
		t.Errorf("received_ts = %d, want %d", o.ReceivedTS, v2Now+7)
	}
}

// TestTriggerObservation_PrelimPhaseAndNoDetrigger: baris PRELIM membawa phase
// PRELIM dan detrigger_ts NULL — event-nya belum berakhir, jadi tidak ada
// instan penutup untuk dicatat.
func TestTriggerObservation_PrelimPhaseAndNoDetrigger(t *testing.T) {
	raw := signedV2([]byte("test-key"), PhasePrelim, 196609, 1, 0.4215, v2DurPr, evOnset, 0, evPrelimTS)
	tr, err := ParseTrigger(raw)
	if err != nil {
		t.Fatalf("payload uji tidak sah: %v", err)
	}

	o := TriggerObservation(tr, v2Now, ledger.VerifyResultOK)
	if o.Phase != ledger.PhasePrelim {
		t.Errorf("phase = %q, want PRELIM", o.Phase)
	}
	if o.DetriggerTS != nil {
		t.Errorf("detrigger_ts = %v, want nil pada PRELIM", *o.DetriggerTS)
	}
}
