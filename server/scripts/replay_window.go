//go:build ignore

// replay_window.go — alat operator P4-M4′: pemutaran ulang HANYA-BACA satu
// jendela ledger, dibandingkan dengan riwayat event_state_log.
//
// Usage:
//
//	FROM_TS=1788255600000 TO_TS=1788255710000 \
//	DATABASE_URL="postgres://..." go run scripts/replay_window.go
//
// Env jendela (wajib):
//
//	FROM_TS, TO_TS   — ms epoch UTC. FROM_TS <= received_ts <= TO_TS untuk
//	                   observasi, dan batas yang sama atas decided_at untuk
//	                   riwayat.
//
// Env profil parameter (opsional; nilai baku = default internal/config):
//
//	CORRELATION_WINDOW_MS, ATTACH_RADIUS_KM, INDEPENDENCE_CELL_KM,
//	MIN_INDEPENDENT_CELLS, MAX_EVENT_DIAMETER_KM, EVENT_RESOLVE_AFTER_MS,
//	EVENT_SWEEP_INTERVAL_MS
//
// Env pelaporan (opsional):
//
//	LEDGER_DROPS_KNOWN     — jumlah ledger_drops_total untuk jendela ini, bila
//	                         operator memilikinya dari log. Counter itu HANYA
//	                         masuk log, jadi tidak ada kueri yang dapat
//	                         memulihkannya.
//	DECIDED_AT_TOLERANCE_MS — toleransi selisih waktu RELATIF (F3).
//
// TIGA SIFAT yang menentukan bentuk berkas ini:
//
//  1. HANYA-BACA. Dua kueri SELECT, tidak ada satu pun INSERT/UPDATE/DELETE,
//     dan Tracker replay tidak pernah dipasangi persister (event.Replay).
//     Ia tidak dapat menulis ke basis data yang dibacanya.
//
//  2. PARAMETER DIASSERSI OPERATOR. Parameter keputusan produksi TIDAK terekam
//     di baris mana pun — deploy/ tidak menyetel satu pun variabel tracker —
//     jadi profil di atas adalah ASSERSI, bukan pemulihan. Spanduk di keluaran
//     mengatakannya, dan itu bukan hiasan: sebuah replay yang cocok membuktikan
//     "observasi ini, di bawah parameter INI, menghasilkan keputusan itu".
//
//  3. SATU algo_ver PER PEMUTARAN (V5). Jendela yang memuat dua label diputar
//     dua kali, satu per label, dan yang basisnya bukan basis biner ini DITOLAK
//     — bukan diputar lalu dilaporkan berbeda.
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

	fromTS := mustInt("FROM_TS")
	toTS := mustInt("TO_TS")
	if fromTS > toTS {
		die("FROM_TS (%d) > TO_TS (%d)", fromTS, toTS)
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

	obs, err := st.ListObservationsForReplay(ctx, fromTS, toTS)
	if err != nil {
		die("baca sensor_observations: %v", err)
	}
	hist, err := st.ListStateLogForReplay(ctx, fromTS, toTS)
	if err != nil {
		die("baca event_state_log: %v", err)
	}

	banner(fromTS, toTS, prof, len(obs), len(hist))

	if len(obs) == 0 {
		fmt.Println("TIDAK ADA OBSERVASI di jendela ini. Tidak ada yang dapat diputar.")
		os.Exit(2)
	}

	groups, vers := event.GroupByAlgoVer(hist)
	if len(vers) == 0 {
		fmt.Println("TIDAK ADA BARIS event_state_log di jendela ini.")
		fmt.Println("Pemutaran tetap dijalankan, tetapi TIDAK ADA yang dapat dibandingkan.")
		res, err := event.Replay(ctx, obs, prof)
		if err != nil {
			die("replay: %v", err)
		}
		printInput(res.Input)
		printReplayOnly(res)
		return
	}

	fail := false
	for _, ver := range vers {
		fmt.Printf("\n=== algo_ver %s (%d baris riwayat) ===\n", ver, len(groups[ver]))
		if err := event.CheckAlgoVer(ver, prof); err != nil {
			fmt.Printf("DITOLAK: %v\n", err)
			fmt.Println("Jendela ini TIDAK diputar di bawah label tersebut. Tidak ada hasil untuk dilaporkan.")
			fail = true
			continue
		}
		res, err := event.Replay(ctx, obs, prof)
		if err != nil {
			die("replay: %v", err)
		}
		rep := event.Compare(groups[ver], res, prof)
		if !printComparison(rep) {
			fail = true
		}
	}

	if fail {
		os.Exit(1)
	}
}

// profileFromEnv membangun profil operator. Nama variabelnya SAMA dengan nama
// yang dibaca internal/config, supaya operator dapat menyalin setelan produksi
// yang ia yakini berlaku alih-alih menerjemahkannya.
func profileFromEnv() event.ReplayProfile {
	p := event.DefaultReplayProfile()
	p.Options.CorrelationWindowMs = envInt("CORRELATION_WINDOW_MS", p.Options.CorrelationWindowMs)
	p.Options.AttachRadiusKm = envFloat("ATTACH_RADIUS_KM", p.Options.AttachRadiusKm)
	p.Options.IndependenceCellKm = envFloat("INDEPENDENCE_CELL_KM", p.Options.IndependenceCellKm)
	p.Options.MinIndependentCells = int(envInt("MIN_INDEPENDENT_CELLS", int64(p.Options.MinIndependentCells)))
	p.Options.MaxEventDiameterKm = envFloat("MAX_EVENT_DIAMETER_KM", p.Options.MaxEventDiameterKm)
	p.Options.ResolveAfterMs = envInt("EVENT_RESOLVE_AFTER_MS", p.Options.ResolveAfterMs)
	p.Options.SweepIntervalMs = envInt("EVENT_SWEEP_INTERVAL_MS", p.Options.SweepIntervalMs)
	p.DecidedAtToleranceMs = envInt("DECIDED_AT_TOLERANCE_MS", 0)
	return p
}

// banner mencetak apa yang diassersi, SEBELUM hasil apa pun. Urutannya sengaja:
// pembaca yang berhenti setelah baris pertama tetap sudah membaca peringatannya.
func banner(fromTS, toTS int64, p event.ReplayProfile, nObs, nHist int) {
	fmt.Println("===========================================================================")
	fmt.Println("QuakeAlert P4-M4' — PEMUTARAN ULANG HANYA-BACA")
	fmt.Println("===========================================================================")
	fmt.Println("PARAMETER KEPUTUSAN DI BAWAH INI DIASSERSI OLEH OPERATOR.")
	fmt.Println("PARAMETER TERSEBUT TIDAK DIPULIHKAN DARI BARIS HISTORIS, DAN TIDAK DAPAT")
	fmt.Println("DIPULIHKAN: tidak ada tabel riwayat konfigurasi, dan satu-satunya parameter")
	fmt.Println("yang terekam pada baris adalah INDEPENDENCE_CELL_KM lewat label algo_ver.")
	fmt.Println("")
	fmt.Println("Sebuah pemutaran yang COCOK karena itu membuktikan:")
	fmt.Println("  \"observasi ini, DI BAWAH PARAMETER INI, menghasilkan keputusan itu\"")
	fmt.Println("dan BUKAN:")
	fmt.Println("  \"parameter inilah yang berlaku saat keputusan itu diambil\"")
	fmt.Println("---------------------------------------------------------------------------")
	fmt.Printf("jendela        : %d .. %d  (%s .. %s UTC)\n",
		fromTS, toTS,
		time.UnixMilli(fromTS).UTC().Format(time.RFC3339),
		time.UnixMilli(toTS).UTC().Format(time.RFC3339))
	fmt.Printf("basis algoritma: %s (biner ini)\n", event.AlgoVerBase())
	fmt.Printf("observasi      : %d baris\n", nObs)
	fmt.Printf("riwayat        : %d baris event_state_log\n", nHist)
	fmt.Println("--- profil parameter yang DIASSERSI ---------------------------------------")
	o := p.Options
	fmt.Printf("  CORRELATION_WINDOW_MS   = %d\n", o.CorrelationWindowMs)
	fmt.Printf("  ATTACH_RADIUS_KM        = %g\n", o.AttachRadiusKm)
	fmt.Printf("  INDEPENDENCE_CELL_KM    = %g   (satu-satunya yang DIVERIFIKASI ke label)\n", o.IndependenceCellKm)
	fmt.Printf("  MIN_INDEPENDENT_CELLS   = %d\n", o.MinIndependentCells)
	fmt.Printf("  MAX_EVENT_DIAMETER_KM   = %g\n", o.MaxEventDiameterKm)
	fmt.Printf("  EVENT_RESOLVE_AFTER_MS  = %d\n", o.ResolveAfterMs)
	fmt.Printf("  EVENT_SWEEP_INTERVAL_MS = %d\n", o.SweepIntervalMs)
	fmt.Printf("  MIN_PGA_GAL             = %g   (konstanta BINER, bukan env)\n", event.MinPGAGal)
	fmt.Printf("  MIN_NODES_CONFIRMED     = %d    (konstanta BINER, bukan env)\n", event.MinNodesConfirmed)
	fmt.Println("--- asumsi lain yang wajib dinyatakan ------------------------------------")
	fmt.Println("  urutan masuk : DIDEKLARASIKAN received_ts, observation_id.")
	fmt.Println("                 Handler MQTT produksi berjalan SetOrderMatters(false),")
	fmt.Println("                 jadi urutan pemrosesan sebenarnya tidak tersimpan.")
	fmt.Println("  fase tik sweep: dihitung dari received_ts baris PERTAMA. Fase tik")
	fmt.Println("                 produksi bergantung pada kapan proses start dan tidak")
	fmt.Println("                 terekam.")
	fmt.Println("  koordinat     : SNAPSHOT node_location tiap baris, bukan iot_nodes hari ini.")
	fmt.Println("  event_id      : dibandingkan sebagai BIJEKSI PENGELOMPOKAN, bukan kesetaraan UUID.")
	fmt.Println("  decided_at    : dibandingkan sebagai DELTA relatif, bukan timestamp.")
	fmt.Println("===========================================================================")
}

func printInput(in event.InputReport) {
	fmt.Printf("\n--- masukan ---------------------------------------------------------------\n")
	fmt.Printf("baris total     : %d\n", in.TotalRows)
	fmt.Printf("diumpankan      : %d\n", in.FedRows)
	fmt.Printf("terbuang        : %d\n", len(in.Skipped))
	for reason, n := range in.SkipCounts() {
		fmt.Printf("  %-22s %d\n", reason, n)
	}
	for _, s := range in.Skipped {
		fmt.Printf("  observation_id=%d node=%s alasan=%s\n", s.ObservationID, s.NodeID, s.Reason)
	}
	if drops := envInt("LEDGER_DROPS_KNOWN", 0); drops > 0 {
		fmt.Printf("ledger_drops    : %d (DIISI OPERATOR dari log)\n", drops)
	} else {
		fmt.Println("ledger_drops    : TIDAK DIKETAHUI. ledger_drops_total hanya masuk log,")
		fmt.Println("                  jadi jendela ini DAPAT kehilangan observasi tanpa jejak")
		fmt.Println("                  di dalam tabel. Nol di sini bukan berarti tidak ada.")
	}
}

func printReplayOnly(res *event.ReplayResult) {
	fmt.Printf("\n--- keputusan hasil pemutaran (TANPA pembanding) --------------------------\n")
	fmt.Printf("algo_ver replay : %s\n", res.AlgoVer)
	fmt.Printf("event           : %d\n", len(res.Events))
	for _, f := range res.Frames {
		fmt.Printf("  %s rev%d %s->%s %s node=%d cells=%d peak=%.4f\n",
			f.EventID, f.Revision, f.From, f.To, f.Reason, f.NodeCount, f.IndependentCells, f.PeakPGA)
	}
}

// printComparison mencetak laporan dan mengembalikan false bila ada yang tidak
// direproduksi. Ketiga pertanyaan dicetak TERPISAH — bijeksi, keputusan, waktu —
// karena ketiganya gagal karena sebab yang berbeda.
func printComparison(rep *event.ComparisonReport) bool {
	printInput(rep.Input)

	fmt.Printf("\n--- bijeksi pengelompokan (F2) -------------------------------------------\n")
	fmt.Printf("event berpasangan        : %d\n", len(rep.Events))
	fmt.Printf("historis tanpa pasangan  : %d %v\n", len(rep.UnmatchedHistoric), rep.UnmatchedHistoric)
	fmt.Printf("replay tanpa pasangan    : %d %v\n", len(rep.UnmatchedReplayed), rep.UnmatchedReplayed)
	if len(rep.AmbiguousSignatures) > 0 {
		fmt.Printf("tanda tangan AMBIGU      : %d\n", len(rep.AmbiguousSignatures))
		for _, s := range rep.AmbiguousSignatures {
			fmt.Printf("  %s\n", s)
		}
		fmt.Println("  Ambigu berarti bijeksi TIDAK DAPAT DIPERIKSA. Bukan cocok, bukan gagal cocok.")
	}
	fmt.Printf("BIJEKTIF                 : %v\n", rep.Bijective())

	fmt.Printf("\n--- keputusan per event ---------------------------------------------------\n")
	for _, c := range rep.Events {
		fmt.Printf("historis %s  <->  replay %s\n", c.HistoricEventID, c.ReplayEventID)
		fmt.Printf("  tanda tangan: %s\n", c.Signature)
		if c.Matched() {
			fmt.Println("  keputusan   : COCOK")
		} else {
			fmt.Printf("  keputusan   : %d DIFF\n", len(c.Diffs))
			for _, d := range c.Diffs {
				fmt.Printf("    %s\n", d)
			}
		}
		fmt.Printf("  waktu (F3, delta relatif, toleransi terpakai):\n")
		for _, tm := range c.Timings {
			mark := "ok"
			if !tm.WithinTolerance {
				mark = "DI LUAR TOLERANSI"
			}
			fmt.Printf("    rev%-3d historis=%+dms replay=%+dms selisih=%dms %s\n",
				tm.Revision, tm.HistoricMs, tm.ReplayedMs, tm.DifferenceMs, mark)
		}
	}

	fmt.Printf("\n--- ringkasan -------------------------------------------------------------\n")
	fmt.Printf("KEPUTUSAN DIREPRODUKSI : %v\n", rep.DecisionsReproduced())
	timingOK := true
	for _, c := range rep.Events {
		if !c.TimingWithinTolerance() {
			timingOK = false
		}
	}
	fmt.Printf("WAKTU DALAM TOLERANSI  : %v  (pertanyaan TERPISAH dari yang di atas)\n", timingOK)
	fmt.Println("Alat ini TIDAK menyatakan P4-M4' VALIDATED. Itu penilaian pemilik.")
	return rep.DecisionsReproduced()
}

// ---- pembaca env -----------------------------------------------------------

func mustInt(key string) int64 {
	v := os.Getenv(key)
	if v == "" {
		die("%s wajib diisi (ms epoch UTC)", key)
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		die("%s=%q bukan bilangan bulat", key, v)
	}
	return n
}

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
	fmt.Fprintf(os.Stderr, "replay_window: "+format+"\n", args...)
	os.Exit(1)
}
