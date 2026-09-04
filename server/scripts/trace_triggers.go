//go:build ignore

// trace_triggers.go — alat operator P4-M1′: penelusuran HANYA-BACA keterlacakan
// pemicu atas N observasi ledger TERAKHIR.
//
// Usage:
//
//	LAST_N=200 DATABASE_URL="postgres://..." go run scripts/trace_triggers.go
//
// Env jendela:
//
//	LAST_N   — jumlah observasi TERAKHIR yang ditelusuri. Bawaan 200.
//	           JUMLAH BARIS, bukan periode kalender: pada fleet satu-node sebuah
//	           rentang kalender dapat berisi nol baris atau ribuan, dan keduanya
//	           membuat angka hasilnya tak dapat dibandingkan antar-jalan.
//
// Env profil parameter (opsional; bawaan = DefaultTraceProfile):
//
//	CORRELATION_WINDOW_MS, INDEPENDENCE_CELL_KM, LINK_TOLERANCE_MS
//
// Env pelaporan (opsional):
//
//	TRACKER_STATS_FILE  — path berkas berisi BADAN respons
//	                      GET /api/v1/admin/tracker/stats. Alat ini TIDAK
//	                      melakukan panggilan HTTP dan TIDAK PERNAH menyentuh
//	                      kunci admin; operator mengambilnya sendiri. Tanpa ini,
//	                      counter dilaporkan TIDAK DIKETAHUI — bukan nol.
//	LEDGER_DROPS_KNOWN  — ledger_drops_total untuk jendela ini bila operator
//	                      memilikinya dari log. Counter itu HANYA masuk log
//	                      (D17/D30), jadi tidak ada kueri yang dapat
//	                      memulihkannya.
//
// EMPAT SIFAT yang menentukan bentuk berkas ini:
//
//  1. HANYA-BACA. Tiga kueri SELECT, tidak ada satu pun INSERT/UPDATE/DELETE,
//     tidak ada Tracker sama sekali. Ia mengukur baris yang sudah ada.
//
//  2. MENGUKUR, TIDAK MENEGAKKAN. Tidak ada exit code yang berarti "P4-M1′
//     LULUS". Keluar 0 bila laporan berhasil dibuat, 2 bila jendelanya kosong.
//     Nilai counter kegagalan persistensi TIDAK memengaruhi exit code sama
//     sekali: ini forensik, bukan SLO reliabilitas.
//
//  3. TAUTAN observasi -> transisi adalah KEANGGOTAAN-DAN-WAKTU, BUKAN KAUSAL.
//     correlation_key dihitung dan tidak pernah disimpan (D12) dan
//     event_state_log tidak punya observation_id. Spanduk mengatakannya sebelum
//     angka mana pun dicetak.
//
//  4. TIDAK ADA YANG DIBUANG DIAM-DIAM. Baris di bawah lantai, baris terkecuali,
//     tautan ambigu, transisi tak-teratribusi, dan emisi yang hilang semuanya
//     dicetak — masing-masing dengan namanya sendiri.
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/event"
	"github.com/banana-pixel/quakealert/server/internal/store"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	lastN := int(envInt("LAST_N", 200))
	if lastN <= 0 {
		die("LAST_N harus > 0, diberi %d", lastN)
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://quakealert:devpassword@localhost:5432/quakealert?sslmode=disable"
	}

	prof := profileFromEnv()

	st, err := store.New(ctx, dbURL)
	if err != nil {
		die("koneksi basis data: %v", err)
	}
	defer st.Close()

	obs, err := st.ListLastNObservations(ctx, lastN)
	if err != nil {
		die("baca sensor_observations: %v", err)
	}

	banner(lastN, len(obs), prof)

	if len(obs) == 0 {
		fmt.Println("TIDAK ADA OBSERVASI di ledger. Tidak ada yang dapat ditelusuri.")
		os.Exit(2)
	}

	fromTS, toTS, _ := event.TraceWindowBounds(obs, prof)
	hist, err := st.ListStateLogForReplay(ctx, fromTS, toTS)
	if err != nil {
		die("baca event_state_log: %v", err)
	}
	emis, err := st.ListEmissionsForTrace(ctx, fromTS, toTS)
	if err != nil {
		die("baca alert_emissions: %v", err)
	}

	rep := event.Trace(obs, hist, emis, prof, lastN)
	rep.Counters = countersFromEnv()
	rep.LedgerDropsKnown = int(envInt("LEDGER_DROPS_KNOWN", 0))

	printReport(rep, fromTS, toTS)
}

func profileFromEnv() event.TraceProfile {
	p := event.DefaultTraceProfile()
	p.Options.CorrelationWindowMs = envInt("CORRELATION_WINDOW_MS", p.Options.CorrelationWindowMs)
	p.Options.IndependenceCellKm = envFloat("INDEPENDENCE_CELL_KM", p.Options.IndependenceCellKm)
	p.LinkToleranceMs = envInt("LINK_TOLERANCE_MS", 0)
	return p
}

// countersFromEnv membaca counter dari BERKAS, bukan lewat HTTP. Tidak ada kunci
// admin yang masuk ke proses ini, jadi tidak ada kredensial yang dapat bocor
// lewat jalur ini maupun lewat keluarannya.
func countersFromEnv() event.TraceCounters {
	path := os.Getenv("TRACKER_STATS_FILE")
	if path == "" {
		return event.TraceCounters{}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		die("baca TRACKER_STATS_FILE: %v", err)
	}
	c, err := event.CountersFromStatsJSON(raw)
	if err != nil {
		die("urai TRACKER_STATS_FILE: %v", err)
	}
	return c
}

// banner mencetak apa yang diassersi dan apa yang TIDAK dibuktikan, SEBELUM
// angka mana pun. Urutannya sengaja: pembaca yang berhenti di paruh pertama tetap
// sudah membaca batasnya.
func banner(lastN, gotRows int, p event.TraceProfile) {
	fmt.Println("===========================================================================")
	fmt.Println("QuakeAlert P4-M1' — KETERLACAKAN PEMICU (PENGUKURAN, HANYA-BACA)")
	fmt.Println("===========================================================================")
	fmt.Println("YANG DIUKUR: setiap observasi dengan pga >= MIN_PGA_GAL di dalam jendela")
	fmt.Println("ini, dan apakah ia dapat DILACAK KE satu transisi UNCONFIRMED, event_id-nya,")
	fmt.Println("baris event_state_log-nya, dan emisi advisory-nya.")
	fmt.Println("")
	fmt.Println("YANG *TIDAK* DIBUKTIKAN — tiga batas, dan ketiganya struktural:")
	fmt.Println("  1. TAUTAN observasi->transisi adalah KEANGGOTAAN-DAN-WAKTU, BUKAN KAUSAL.")
	fmt.Println("     correlation_key dihitung dan tidak pernah disimpan (D12), event_state_log")
	fmt.Println("     tidak punya observation_id, dan tidak ada FK. Satu-satunya jalan pulang")
	fmt.Println("     adalah node_id di evidence_summary.contributors[] pada jendela waktu.")
	fmt.Println("  2. PEMETAAN N:1. Beberapa observasi di satu jendela korelasi BERBAGI satu")
	fmt.Println("     transisi: UNCONFIRMED->UNCONFIRMED bukan transisi yang sah. Baris yang")
	fmt.Println("     berbagi transisi TERLACAK — bukan terhitung dua kali.")
	fmt.Println("  3. COUNTER kegagalan persistensi kumulatif SEJAK PROSES DIMULAI dan tidak")
	fmt.Println("     bertanda waktu, jadi ia TIDAK dapat diatribusikan ke jendela ini.")
	fmt.Println("")
	fmt.Println("Alat ini TIDAK punya exit code yang berarti 'P4-M1' LULUS'. Nilai counter")
	fmt.Println("kegagalan TIDAK memengaruhi exit code: ini forensik, bukan SLO reliabilitas.")
	fmt.Println("---------------------------------------------------------------------------")
	fmt.Printf("jendela        : %d observasi TERAKHIR (diminta), %d terbaca\n", lastN, gotRows)
	fmt.Println("                 BATAS JUMLAH BARIS, bukan periode kalender.")
	fmt.Printf("basis algoritma: %s (biner ini)\n", event.AlgoVerBase())
	fmt.Println("--- profil parameter yang DIASSERSI ---------------------------------------")
	fmt.Printf("  CORRELATION_WINDOW_MS = %d\n", p.Options.CorrelationWindowMs)
	fmt.Printf("  INDEPENDENCE_CELL_KM  = %g\n", p.Options.IndependenceCellKm)
	fmt.Printf("  LINK_TOLERANCE_MS     = %d   (kelonggaran di ATAS jendela korelasi)\n", p.LinkToleranceMs)
	fmt.Printf("  MIN_PGA_GAL           = %g   (konstanta BINER, bukan env)\n", event.MinPGAGal)
	fmt.Printf("  MIN_NODES_CONFIRMED   = %d    (konstanta BINER, bukan env)\n", event.MinNodesConfirmed)
	fmt.Println("===========================================================================")
}

func printReport(r *event.TraceReport, histFrom, histTo int64) {
	fmt.Printf("\n--- jendela --------------------------------------------------------------\n")
	fmt.Printf("observasi        : %d baris  (%s .. %s UTC)\n",
		r.TotalRows, msUTC(r.FromTS), msUTC(r.ToTS))
	fmt.Printf("rentang riwayat  : %d .. %d  (dilebarkan ke belakang satu jendela korelasi)\n", histFrom, histTo)
	fmt.Printf("event_state_log  : %d baris mentah\n", r.StateLogRows)
	fmt.Printf("alert_emissions  : %d baris mentah\n", r.EmissionRows)
	fmt.Printf("node dalam jendela: %d %v\n", len(r.NodeIDs), r.NodeIDs)
	if r.SingleNodeFleet {
		fmt.Println("FLEET SATU-NODE (S2): CONFIRMED tidak terjangkau menurut kerapatan.")
		fmt.Println("  UNCONFIRMED adalah state tujuan yang BENAR di sini. Ketiadaan CONFIRMED")
		fmt.Println("  BUKAN cacat, dan tidak ada angka di bawah yang boleh dibaca demikian.")
	}

	fmt.Printf("\n--- penyebut (ketiganya berjumlah %d) -------------------------------------\n", r.TotalRows)
	fmt.Printf("di bawah lantai  : %d  (pga < %g: bukan pemicu, bukan kegagalan)\n", r.BelowFloor, event.MinPGAGal)
	fmt.Printf("terkecuali       : %d  (>= lantai, tetapi konsensus sendiri membuangnya)\n", len(r.Excluded))
	for reason, n := range r.ExcludeCounts() {
		fmt.Printf("    %-22s %d\n", reason, n)
	}
	for _, e := range r.Excluded {
		fmt.Printf("    observation_id=%d node=%s pga=%.4f received=%d alasan=%s\n",
			e.ObservationID, e.NodeID, e.PGAGal, e.ReceivedTS, e.Reason)
	}
	fmt.Printf("memenuhi syarat  : %d  (inilah yang kriteria P4-M1' tanyakan)\n", len(r.Traces))

	fmt.Printf("\n--- keterlacakan ---------------------------------------------------------\n")
	oc := r.Outcomes()
	fmt.Printf("TRACED                          : %d\n", oc[event.TraceTraced])
	fmt.Printf("AMBIGUOUS_MULTIPLE_TRANSITIONS  : %d  (tak dapat diputuskan; bukan lulus, bukan gagal)\n",
		oc[event.TraceAmbiguous])
	fmt.Printf("NO_UNCONFIRMED_TRANSITION       : %d\n", oc[event.TraceNoTransition])
	fmt.Printf("transisi UNCONFIRMED BERBEDA    : %d  (pemetaan N:1; lihat batas 2)\n", r.DistinctTransitions())

	fmt.Printf("\n--- emisi advisory (kaki WebSocket) --------------------------------------\n")
	ec := r.EmissionOutcomes()
	fmt.Printf("%-34s: %d  (event_id + event_revision; bukti EKSAK)\n", event.EmissionByID, ec[event.EmissionByID])
	fmt.Printf("%-34s: %d  (pra-000008; bukti LEBIH LEMAH)\n", event.EmissionByTimeOnly, ec[event.EmissionByTimeOnly])
	fmt.Printf("%-34s: %d\n", event.EmissionMissing, ec[event.EmissionMissing])
	fmt.Printf("%-34s: %d  (tak ada transisi untuk ditautkan)\n", event.EmissionNotApplicable, ec[event.EmissionNotApplicable])

	fmt.Printf("\n--- rantai per observasi memenuhi syarat ---------------------------------\n")
	for _, t := range r.Traces {
		switch t.Outcome {
		case event.TraceTraced:
			fmt.Printf("obs=%d node=%s pga=%.4f received=%d\n", t.ObservationID, t.NodeID, t.PGAGal, t.ReceivedTS)
			fmt.Printf("    -> event=%s rev%d decided_at=%d lag=%+dms algo_ver=%s obs_seq=%s\n",
				t.EventID, t.Revision, t.DecidedAt, t.LagMs, t.AlgoVer, t.ObsSeqLink)
			fmt.Printf("    -> emisi %s%s ws_clients=%s\n",
				t.EmissionOutcome, emissionIDSuffix(t.EmissionID), wsCount(t.WSClientCount))
		case event.TraceAmbiguous:
			fmt.Printf("obs=%d node=%s pga=%.4f received=%d\n", t.ObservationID, t.NodeID, t.PGAGal, t.ReceivedTS)
			fmt.Printf("    -> AMBIGU antara %v. Tautan TIDAK DAPAT DIPUTUSKAN dari data yang ada.\n", t.Candidates)
		case event.TraceNoTransition:
			fmt.Printf("obs=%d node=%s pga=%.4f received=%d\n", t.ObservationID, t.NodeID, t.PGAGal, t.ReceivedTS)
			if t.NearestCandidate != "" {
				fmt.Printf("    -> TANPA transisi di dalam jendela. Terdekat %s pada %+dms;\n",
					t.NearestCandidate, t.NearestCandidateOffMs)
				fmt.Println("       periksa apakah jendela tautan terlalu sempit sebelum menyimpulkan.")
			} else {
				fmt.Println("    -> TANPA transisi UNCONFIRMED mana pun yang memuat node ini.")
			}
		}
	}

	fmt.Printf("\n--- transisi UNCONFIRMED tanpa observasi terlacak ------------------------\n")
	if len(r.Unattributed) == 0 {
		fmt.Println("tidak ada.")
	}
	for _, u := range r.Unattributed {
		edge := ""
		if u.AtWindowEdge {
			edge = "  [DI TEPI JENDELA: pemicunya kemungkinan SEBELUM tepi bawah — sah]"
		}
		fmt.Printf("event=%s rev%d decided_at=%d nodes=%v algo_ver=%s%s\n",
			u.EventID, u.Revision, u.DecidedAt, u.NodeIDs, u.AlgoVer, edge)
	}

	fmt.Printf("\n--- counter kegagalan persistensi (DILAPORKAN, BUKAN target nol) --------\n")
	if r.Counters.Known {
		fmt.Printf("event_persist_dropped_total      : %d\n", r.Counters.PersistDropped)
		fmt.Printf("event_upsert_failures_total      : %d\n", r.Counters.UpsertFailures)
		fmt.Printf("event_state_log_failures_total   : %d\n", r.Counters.StateLogFailures)
		fmt.Printf("event_state_log_skipped_total    : %d\n", r.Counters.StateLogSkipped)
		fmt.Println("Keempatnya KUMULATIF SEJAK PROSES DIMULAI dan tidak bertanda waktu, jadi")
		fmt.Println("TIDAK dapat diatribusikan ke jendela ini. Nilai bukan nol BUKAN kegagalan")
		fmt.Println("P4-M1' — jalur peringatan sengaja tidak pernah menunggu pencatatan (§9.5).")
	} else {
		fmt.Println("TIDAK DIKETAHUI. Setel TRACKER_STATS_FILE ke berkas berisi badan respons")
		fmt.Println("GET /api/v1/admin/tracker/stats. Tanpa itu keempat counter tidak dicetak")
		fmt.Println("sebagai nol, karena nol yang tidak diketahui adalah angka yang berbohong.")
	}

	fmt.Printf("\n--- kelengkapan jendela --------------------------------------------------\n")
	if r.LedgerDropsKnown > 0 {
		fmt.Printf("ledger_drops    : %d (DIISI OPERATOR dari log). Jendela ini INCOMPLETE.\n", r.LedgerDropsKnown)
	} else {
		fmt.Println("ledger_drops    : TIDAK DIKETAHUI. ledger_drops_total hanya masuk log")
		fmt.Println("                  (D17/D30), jadi jendela ini DAPAT kehilangan observasi")
		fmt.Println("                  tanpa jejak di dalam tabel. Nol di sini bukan 'tidak ada'.")
	}

	fmt.Println("")
	fmt.Println("Alat ini TIDAK menyatakan P4-M1' SATISFIED. Itu penilaian pemilik atas")
	fmt.Println("angka-angka di atas, bukan boolean yang dihitung alat ini.")
}

func emissionIDSuffix(id int64) string {
	if id == 0 {
		return ""
	}
	return " emission_id=" + strconv.FormatInt(id, 10)
}

// wsCount membedakan NULL dari nol. Kolomnya nullable justru supaya keduanya
// tidak pernah dikonflasikan (migrasi 000007).
func wsCount(p *int) string {
	if p == nil {
		return "TIDAK DILAPORKAN (NULL, bukan nol)"
	}
	return strconv.Itoa(*p)
}

func msUTC(ms int64) string {
	if ms == 0 {
		return "-"
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}

// ---- pembaca env -----------------------------------------------------------

func envInt(key string, def int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		die("%s=%q bukan bilangan bulat", key, v)
	}
	return n
}

func envFloat(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		die("%s=%q bukan bilangan pecahan", key, v)
	}
	return f
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "trace_triggers: "+format+"\n", args...)
	os.Exit(1)
}
