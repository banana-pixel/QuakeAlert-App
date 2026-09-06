//go:build ignore

// event_timeline.go — alat operator P4-M6′: garis waktu forensik HANYA-BACA
// untuk SATU event_id.
//
// Usage:
//
//	DATABASE_URL="postgres://..." go run scripts/event_timeline.go <event_id>
//
// TEPAT SATU event_id, sebagai argumen. Bukan env, bukan daftar, bukan rentang
// kalender: kriteria P4-M6′ (ROADMAP.md § Phase 4) berbunyi "untuk satu
// `event_id`", dan sebuah alat yang menerima banyak akan mengundang laporan
// agregat yang bukan yang diminta.
//
// Env profil parameter (opsional; bawaan = DefaultTraceProfile milik M1′):
//
//	CORRELATION_WINDOW_MS, INDEPENDENCE_CELL_KM, LINK_TOLERANCE_MS
//
//	LINK_TOLERANCE_MS adalah toleransi M1′ yang SAMA (defaultLinkToleranceMs =
//	2000 ms, aditif di ATAS jendela korelasi). M6′ tidak memperkenalkan
//	toleransi ilmiah baru (D-015 batasan 2), dan nilai berlaku beserta ASALNYA
//	dicetak di spanduk sebelum angka mana pun.
//
// Env pelaporan (opsional):
//
//	SCHEMA_VERSION      — schema_version basis data pada saat pembacaan.
//	                      DIASSERSI OPERATOR: tidak ada satu pun kode Go di repo
//	                      ini yang membacanya, jadi alat ini tidak dapat
//	                      membuktikannya. Tanpa ini ia dicetak TIDAK DIASSERSI —
//	                      bukan 8.
//	LEDGER_DROPS_KNOWN  — ledger_drops_total bila operator memilikinya dari log.
//	                      Counter itu HANYA masuk log (D17/D30); tidak ada kueri
//	                      yang dapat memulihkannya.
//	INCLUDE_EMISSIONS   — "0" mematikan bagian KELIMA yang opsional. Bawaan
//	                      menyala. Bagian itu bukan salah satu dari empat
//	                      keluaran wajib dan tidak boleh menjadi penentu lulus
//	                      atau tidaknya M6′ (D-015 batasan 1).
//
// EMPAT SIFAT yang menentukan bentuk berkas ini:
//
//  1. HANYA-BACA. Lima kueri SELECT lewat pool yang sama: satu bukti sesi
//     (pg_settings transaction_read_only, gagal-tertutup) + empat forensik
//     (yang keempat opsional), tidak ada satu pun INSERT/UPDATE/DELETE,
//     tidak ada net/http, tidak ada Tracker. Tracker adalah
//     otoritas state HIDUP (D-002/§9.5) tetapi tidak menyimpan riwayat revisi;
//     riwayat itu hanya ada di basis data, jadi tidak ada alasan menyentuhnya.
//     Bukti sesi membuktikan pool yang menjawab melaporkan setting=on
//     source=client; ia BUKAN sensus per-backend pool (lihat
//     internal/store/event_readonly.go).
//
//  2. MENGUKUR, TIDAK MENEGAKKAN. Tidak ada exit code yang berarti "P4-M6′
//     LULUS". Keluar 0 bila laporan berhasil dibuat, 2 bila event_id itu tidak
//     meninggalkan jejak sama sekali. Bagian yang KOSONG atau TIDAK DAPAT
//     DIAMATI tidak memengaruhi exit code.
//
//  3. TAUTAN observasi -> revisi adalah KEANGGOTAAN-DAN-WAKTU, BUKAN KAUSAL.
//     Spanduk mengatakannya sebelum angka mana pun dicetak, dan setiap kandidat
//     membawa labelnya sendiri.
//
//  4. TIDAK ADA YANG DIBUANG DIAM-DIAM. Baris di bawah lantai, baris terkecuali,
//     tautan ambigu, revisi tak-teratribusi, lubang revisi, evidence_summary yang
//     tak terurai, dan emisi yang hilang semuanya dicetak — masing-masing dengan
//     namanya sendiri. KETIADAAN DI DALAM CATATAN TIDAK PERNAH DICETAK SEBAGAI
//     BUKTI KETIADAAN.
package main

import (
	"context"
	"errors"
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

	args := os.Args[1:]
	if len(args) != 1 || args[0] == "" {
		die("butuh TEPAT SATU argumen event_id; diberi %d\n"+
			"  usage: DATABASE_URL=\"postgres://...\" go run scripts/event_timeline.go <event_id>", len(args))
	}
	eventID := args[0]

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://quakealert:devpassword@localhost:5432/quakealert?sslmode=disable"
	}

	// application_name hanya metadata korelasi (BUKAN bukti read-only).
	// Dibangun aman m6-<8hex>-<timestamp UTC>; APPLICATION_NAME env boleh
	// menimpa bila aman. DSN tidak pernah dicetak.
	appName, err := store.DefaultAppName(eventID, time.Now())
	if err != nil {
		die("application_name: %v", err)
	}
	if v := os.Getenv("APPLICATION_NAME"); v != "" {
		appName = v
	}
	dbURL, err = store.EnsureApplicationName(dbURL, appName)
	if err != nil {
		die("application_name: %v", err)
	}

	prof := profileFromEnv()

	st, err := store.New(ctx, dbURL)
	if err != nil {
		die("koneksi basis data: %v", err)
	}
	defer st.Close()

	// Bukti sesi lewat pool yang SAMA, gagal-tertutup. Harus setting=on
	// source=client; selain itu die sebelum satu pun keluaran forensik.
	proof, err := st.SessionReadOnly(ctx)
	if err != nil {
		die("verifikasi sesi read-only: %v", err)
	}
	// Satu-satunya yang dicetak dari bukti sesi: setting, source,
	// application_name. Tidak ada DSN/host/password/secret.
	fmt.Println(store.FormatReadOnlyBanner(proof))

	banner(eventID, prof)

	// Keluaran 1. Barisnya HILANG bukan galat pada level ini: event_state_log
	// punya FK ke earthquake_events, jadi baris log tanpa baris event tidak dapat
	// ada — tetapi kebalikannya bisa, dan sebuah event_id yang salah ketik juga
	// harus menghasilkan laporan, bukan panik.
	row, err := st.EventByID(ctx, eventID)
	if err != nil && !errors.Is(err, store.ErrEventNotFound) {
		die("baca earthquake_events: %v", err)
	}

	// Keluaran 2 + 3.
	hist, err := st.ListStateLogForEvent(ctx, eventID)
	if err != nil {
		die("baca event_state_log: %v", err)
	}

	if row == nil && len(hist) == 0 {
		fmt.Printf("\nevent_id=%s TIDAK MENINGGALKAN JEJAK di earthquake_events maupun\n", eventID)
		fmt.Println("event_state_log. Yang ini TIDAK membuktikan event itu tidak pernah ada:")
		fmt.Println("satuan persistensi diantre dan boleh dibuang-tertua (D17/D30), dan")
		fmt.Println("DETECTED memang tidak pernah menjadi baris (§9.5). Periksa ejaan UUID-nya.")
		os.Exit(2)
	}

	// Keluaran 4. Jendelanya DITURUNKAN dari riwayat, bukan diassersi: batasnya
	// predikat M1′ yang sama, ditulis sebagai batas received_ts.
	nodes := event.TimelineContributorNodes(hist)
	fromTS, toTS, haveWindow := event.TimelineWindowBounds(hist, prof)

	var obs []store.ReplayObservation
	if haveWindow && len(nodes) > 0 {
		obs, err = st.ListObservationsForNodesInWindow(ctx, nodes, fromTS, toTS)
		if err != nil {
			die("baca sensor_observations: %v", err)
		}
	}

	// Keluaran KELIMA, opsional. nil (bukan slice kosong) berarti tidak dibaca,
	// dan BuildTimeline memakai perbedaan itu untuk menghilangkan bagiannya alih-
	// alih mencetaknya sebagai nol emisi.
	var emis []store.TraceEmission
	includeEmis := os.Getenv("INCLUDE_EMISSIONS") != "0"
	if includeEmis && haveWindow {
		eFrom, eTo := emissionWindow(hist, prof)
		emis, err = st.ListEmissionsForTrace(ctx, eFrom, eTo)
		if err != nil {
			die("baca alert_emissions: %v", err)
		}
		if emis == nil {
			emis = []store.TraceEmission{} // dibaca dan kosong != tidak dibaca
		}
	}

	tl := event.BuildTimeline(eventID, row, hist, obs, emis, prof)
	tl.Coverage.LedgerDropsKnown = int(envInt("LEDGER_DROPS_KNOWN", 0))

	printTimeline(tl, fromTS, toTS, haveWindow, includeEmis)
}

func profileFromEnv() event.TraceProfile {
	p := event.DefaultTraceProfile()
	p.Options.CorrelationWindowMs = envInt("CORRELATION_WINDOW_MS", p.Options.CorrelationWindowMs)
	p.Options.IndependenceCellKm = envFloat("INDEPENDENCE_CELL_KM", p.Options.IndependenceCellKm)
	p.LinkToleranceMs = envInt("LINK_TOLERANCE_MS", 0)
	return p
}

// emissionWindow adalah rentang decided_at emisi, BUKAN rentang received_ts
// observasi. Keduanya berbeda dan mengkonflasikannya akan melebarkan pembacaan
// emisi satu jendela korelasi penuh ke belakang tanpa alasan: sebuah emisi
// diputuskan pada transisinya, jadi toleransi tautan saja yang berlaku di sini.
func emissionWindow(hist []store.EventStateLog, p event.TraceProfile) (from, to int64) {
	tol, _ := event.EffectiveLinkTolerance(p)
	from, to = hist[0].DecidedAt, hist[0].DecidedAt
	for _, r := range hist {
		if r.DecidedAt < from {
			from = r.DecidedAt
		}
		if r.DecidedAt > to {
			to = r.DecidedAt
		}
	}
	return from - tol, to + tol
}

// banner mencetak apa yang diassersi dan apa yang TIDAK dibuktikan, SEBELUM
// angka mana pun. Urutannya sengaja: pembaca yang berhenti di paruh pertama tetap
// sudah membaca batasnya.
func banner(eventID string, p event.TraceProfile) {
	tol, prov := event.EffectiveLinkTolerance(p)

	fmt.Println("===========================================================================")
	fmt.Println("QuakeAlert P4-M6' — GARIS WAKTU FORENSIK SATU EVENT (HANYA-BACA)")
	fmt.Println("===========================================================================")
	fmt.Println("YANG DIUKUR: untuk SATU event_id — baris earthquake_events-nya, riwayat")
	fmt.Println("event_state_log-nya terurut revision ASC, evidence_summary yang sudah")
	fmt.Println("DIURAI per revisi, dan observasi yang BERKONTRIBUSI. Empat keluaran itu.")
	fmt.Println("")
	fmt.Println("YANG *TIDAK* DIBUKTIKAN — empat batas, dan keempatnya struktural:")
	fmt.Println("  1. TAUTAN observasi->revisi adalah KEANGGOTAAN-DAN-WAKTU, BUKAN KAUSAL.")
	fmt.Println("     event_state_log tidak punya observation_id, tidak punya correlation_key")
	fmt.Println("     dan tidak punya FK ke sensor_observations; correlation_key dihitung dan")
	fmt.Println("     tidak pernah disimpan (D12). evidence_summary adalah POTRET, bukan join.")
	fmt.Println("     Satu-satunya jalan pulang adalah node_id di contributors[] pada sebuah")
	fmt.Println("     jendela waktu. Setiap baris di bawah adalah KANDIDAT (D-015).")
	fmt.Println("  2. AMBIGUITAS bukan kecocokan dan bukan kehilangan. Jendela dua revisi yang")
	fmt.Println("     berdekatan BERTUMPANG-TINDIH, jadi satu observasi dapat menjadi kandidat")
	fmt.Println("     dua revisi. Itu dilaporkan, tidak dipilih salah satunya.")
	fmt.Println("  3. KETIADAAN DI DALAM CATATAN BUKAN BUKTI KETIADAAN. Satuan persistensi")
	fmt.Println("     diantre, berbatas, dan boleh dibuang-tertua (D17/D30); ledger_drops_total")
	fmt.Println("     HANYA masuk log; DETECTED tidak pernah menjadi baris (§9.5), jadi riwayat")
	fmt.Println("     yang tidak dimulai dari revisi terendah BUKAN lubang.")
	fmt.Println("  4. earthquake_events DITIMPA saat eskalasi: barisnya menunjukkan bentuk")
	fmt.Println("     TERAKHIR saja. Hanya event_state_log yang membuktikan sebuah state")
	fmt.Println("     PERNAH dipegang.")
	fmt.Println("")
	fmt.Println("Alat ini TIDAK punya exit code yang berarti 'P4-M6' LULUS'. Bagian yang KOSONG")
	fmt.Println("atau TIDAK DAPAT DIAMATI tidak memengaruhi exit code: ini forensik.")
	fmt.Println("---------------------------------------------------------------------------")
	fmt.Printf("event_id       : %s\n", eventID)
	fmt.Printf("basis algoritma: %s (biner ini)\n", event.AlgoVerBase())
	fmt.Printf("schema_version : %s\n", assertedSchemaVersion())
	fmt.Println("                 event_near_confirmed (000009) TIDAK dibaca alat ini; tidak")
	fmt.Println("                 satu pun dari empat keluaran bergantung padanya (D-015).")
	fmt.Println("--- profil parameter yang DIASSERSI ---------------------------------------")
	fmt.Printf("  CORRELATION_WINDOW_MS = %d\n", p.Options.CorrelationWindowMs)
	fmt.Printf("  INDEPENDENCE_CELL_KM  = %g   (dibawa untuk provenance; M6' tidak menghitung sel)\n",
		p.Options.IndependenceCellKm)
	fmt.Printf("  toleransi tautan      = %d ms  asal=%s\n", tol, prov)
	fmt.Println("                          Toleransi M1' yang SAMA, aditif di ATAS jendela")
	fmt.Println("                          korelasi. Tidak ada toleransi baru di M6'.")
	fmt.Printf("  MIN_PGA_GAL           = %g   (konstanta BINER, bukan env)\n", event.MinPGAGal)
	fmt.Printf("  MIN_NODES_CONFIRMED   = %d    (konstanta BINER, bukan env)\n", event.MinNodesConfirmed)
	fmt.Println("===========================================================================")
}

// assertedSchemaVersion melaporkan schema_version sebagai ASSERSI OPERATOR.
// Tidak ada kode Go di repo ini yang membacanya — ia hanya muncul di metadata
// artefak sim_evidence.sh dan di gerbang CI — jadi alat ini tidak dapat
// membuktikannya, dan mencetak "8" karena itu yang biasanya benar akan mengarang
// bukti yang diminta arsip D-015.
func assertedSchemaVersion() string {
	v := os.Getenv("SCHEMA_VERSION")
	if v == "" {
		return "TIDAK DIASSERSI (setel SCHEMA_VERSION; alat ini tidak dapat membacanya)"
	}
	return v + " (DIASSERSI OPERATOR, bukan dibaca alat ini)"
}

// ---------------------------------------------------------------------------
// Laporan
// ---------------------------------------------------------------------------

func printTimeline(tl *event.EventTimeline, obsFrom, obsTo int64, haveWindow, includeEmis bool) {
	cov := tl.Coverage

	fmt.Printf("\n--- kelengkapan keempat keluaran WAJIB -----------------------------------\n")
	fmt.Printf("1 baris earthquake_events : %s\n", cov.EventRowStatus)
	fmt.Printf("2 riwayat event_state_log : %s  (%d baris, revision ASC)\n", cov.StateLogStatus, cov.StateLogRows)
	fmt.Printf("3 evidence_summary/revisi : %s  (%d terurai, %d TIDAK terurai)\n",
		cov.EvidenceStatus, cov.RevisionsWithEvidence, cov.RevisionsEvidenceBroken)
	fmt.Printf("4 observasi berkontribusi : %s  (%d kandidat unik)\n", cov.ObservationsStatus, len(tl.Observations))
	fmt.Println("OBSERVED = ada dan terbaca. EMPTY = pembacaan BERHASIL dan hasilnya nol")
	fmt.Println("baris. NOT_OBSERVABLE = tidak dapat diamati pada skema atau data ini.")
	fmt.Println("Ketiganya BERBEDA, dan tidak satu pun berarti 'tidak pernah terjadi'.")

	printEventRow(tl)
	printRevisions(tl)
	printObservations(tl, obsFrom, obsTo, haveWindow)
	printUnattributed(tl)
	if includeEmis {
		printEmissions(tl)
	}
	printCoverage(tl)
}

// printEventRow mencetak keluaran PERTAMA.
func printEventRow(tl *event.EventTimeline) {
	fmt.Printf("\n=== 1. baris earthquake_events ===========================================\n")
	if tl.Event == nil {
		fmt.Println("TIDAK ADA BARIS untuk event_id ini.")
		fmt.Println("  Bukan bukti event itu tidak pernah ada: UpsertEvent dipanggil dari")
		fmt.Println("  antrean ledger yang boleh membuang-tertua (D17), dan riwayat di bawah")
		fmt.Println("  — bila ada — tetap membuktikan transisinya. Lihat bagian 2.")
		return
	}
	e := tl.Event
	fmt.Printf("event_id          : %s\n", e.EventID)
	fmt.Printf("event_state       : %s   revision=%d\n", orUnknown(e.EventState), e.Revision)
	fmt.Printf("status (proyeksi) : %s\n", orUnknown(e.Status))
	fmt.Printf("origin_ts         : %d  (%s UTC)  sumber=%s\n",
		e.OriginTS, msUTC(e.OriginTS), orUnknown(e.OriginTSSource))
	fmt.Println("                    WAKTU TANAH BERGERAK, bukan waktu baris dibuat.")
	fmt.Printf("started_at        : %d  (%s UTC)  — kapan barisnya lahir\n", e.StartedAtMs, msUTC(e.StartedAtMs))
	fmt.Printf("centroid          : %.6f, %.6f  (%s)\n", e.CentroidLat, e.CentroidLon, orUnknown(e.LocationName))
	fmt.Println("                    ESTIMASI CENTROID, bukan episenter.")
	fmt.Printf("max_pga           : %.4f gal   mmi=%s (%s)\n",
		e.MaxPGA, orUnknown(e.MMIScale), orUnknown(e.IntensityLabel))
	fmt.Printf("triggered_nodes   : %d   independent_cells=%d\n", e.TriggeredNodes, e.IndependentCellCount)
	fmt.Printf("algo_ver          : %s\n", orUnknown(e.AlgoVer))
	fmt.Println("Baris ini DITIMPA pada setiap eskalasi: ia bentuk TERAKHIR, bukan riwayat.")
}

// printRevisions mencetak keluaran KEDUA dan KETIGA bersama, karena sebuah
// evidence_summary tanpa barisnya tidak dapat ditafsirkan.
func printRevisions(tl *event.EventTimeline) {
	fmt.Printf("\n=== 2+3. riwayat event_state_log + evidence_summary per revisi ===========\n")
	if len(tl.Revisions) == 0 {
		fmt.Println("TIDAK ADA BARIS RIWAYAT. Tidak ada state yang dapat dibuktikan pernah")
		fmt.Println("dipegang event ini. Lihat batas 3 di spanduk sebelum menyimpulkan.")
		return
	}
	for i := range tl.Revisions {
		e := &tl.Revisions[i]
		r := e.Row
		fmt.Printf("\nrev%-3d %s -> %-11s alasan=%-22s decided_at=%d (%s UTC)\n",
			r.Revision, fromState(r.FromState), r.ToState, r.Reason, r.DecidedAt, msUTC(r.DecidedAt))
		fmt.Printf("       node_count=%d independent_cells=%d peak_pga=%s algo_ver=%s\n",
			r.NodeCount, r.IndependentCells, peakPGA(r.PeakPGA), orUnknown(r.AlgoVer))

		if !e.EvidenceParsed {
			fmt.Printf("       evidence_summary TIDAK TERURAI: %s\n", e.EvidenceError)
			fmt.Println("       Barisnya TETAP nyata dan TETAP dihitung. Tanpa contributors[]")
			fmt.Println("       tidak ada keanggotaan, jadi revisi ini tidak dapat punya kandidat.")
			continue
		}
		ev := e.Evidence
		fmt.Printf("       evidence: independent_cells=%d cells=%v origin_ts_source=%s mixed_provenance=%t\n",
			ev.IndependentCells, ev.CellIDs, orUnknown(ev.OriginTSSource), ev.MixedProvenance)
		for _, c := range ev.Contributors {
			fmt.Printf("         kontributor node=%s peak_pga=%.4f phase=%s onset_ts=%d sumber=%s obs_seq=%s cell=(%d,%d)\n",
				c.NodeID, c.PeakPGA, c.Phase, c.OnsetTS, c.OnsetSource, obsSeqVal(c.ObsSeq), c.Cell.X, c.Cell.Y)
		}
		if len(ev.Contributors) == 0 {
			fmt.Println("         contributors[] KOSONG: terurai, tetapi tanpa satu pun node.")
		}
		fmt.Printf("       jendela kandidat received_ts: [%d .. %d]  (%s .. %s UTC)\n",
			e.WindowFromTS, e.WindowToTS, msUTC(e.WindowFromTS), msUTC(e.WindowToTS))
		fmt.Printf("       kandidat=%d  di bawah lantai=%d  terkecuali=%d\n",
			len(e.Candidates), e.BelowFloor, len(e.ExcludedCandidates))
	}
}

// printObservations mencetak keluaran KEEMPAT.
func printObservations(tl *event.EventTimeline, from, to int64, haveWindow bool) {
	cov := tl.Coverage
	fmt.Printf("\n=== 4. observasi yang BERKONTRIBUSI (KANDIDAT, NON-KAUSAL) ================\n")
	if !haveWindow {
		fmt.Println("TIDAK ADA JENDELA: tanpa baris riwayat tidak ada decided_at yang dapat")
		fmt.Println("menjadi pusat jendela. sensor_observations TIDAK dibaca sama sekali.")
		return
	}
	fmt.Printf("jendela baca : received_ts [%d .. %d]  (%s .. %s UTC)\n",
		from, to, msUTC(from), msUTC(to))
	fmt.Printf("node ditanya : %d (union contributors[] SELURUH revisi)\n", cov.ContributorNodes)
	fmt.Printf("baris terbaca: %d\n", cov.ObservationRowsRead)
	if cov.SingleNodeContributors {
		fmt.Println("KONTRIBUTOR SATU NODE (S2): quorum butuh >=3 kontributor terverifikasi di")
		fmt.Println("  >=2 sel independensi, dan itu TIDAK TERJANGKAU pada fleet satu-node fisik.")
		fmt.Println("  Ketiadaan CONFIRMED di sini adalah fakta KERAPATAN JARINGAN, bukan cacat,")
		fmt.Println("  dan tidak ada angka di bawah yang boleh dibaca demikian.")
	}

	fmt.Printf("\npenyebut (ketiganya berjumlah <= baris terbaca) ---------------------------\n")
	fmt.Printf("memenuhi syarat  : %d baris unik  (inilah yang kriteria P4-M6' tanyakan)\n", len(tl.Observations))
	fmt.Printf("di bawah lantai  : %d  (pga < %g: bukan pemicu, bukan kegagalan)\n", cov.BelowFloorRows, event.MinPGAGal)
	fmt.Printf("terkecuali       : %d  (>= lantai, tetapi konsensus sendiri membuangnya)\n", cov.ExcludedRows)
	fmt.Printf("tak-teratribusi  : %d  (terbaca, tetapi TIDAK memenuhi keanggotaan-dan-waktu\n",
		cov.ObservationRowsRead-len(tl.Observations)-cov.BelowFloorRows-cov.ExcludedRows)
	fmt.Println("                   satu revisi pun — node yang sama, waktu di luar jendela)")
	fmt.Printf("pasangan revisi  : %d  (satu baris dapat menjadi kandidat >1 revisi)\n", cov.CandidateRows)
	fmt.Printf("AMBIGU           : %d  (tak dapat diputuskan; bukan lulus, bukan gagal)\n", cov.AmbiguousCandidates)

	if len(tl.Observations) == 0 {
		fmt.Println("\ntidak ada kandidat yang memenuhi syarat.")
		return
	}
	fmt.Printf("\nper baris ----------------------------------------------------------------\n")
	for _, o := range tl.Observations {
		fmt.Printf("obs=%d node=%s pga=%.4f received=%d (%s UTC) obs_seq=%s\n",
			o.ObservationID, o.NodeID, o.PGAGal, o.ReceivedTS, msUTC(o.ReceivedTS), obsSeqVal(o.ObsSeq))
		switch o.Attribution {
		case event.TraceTraced:
			fmt.Printf("    -> %s ke rev%d  lag=%+dms  obs_seq=%s\n",
				o.Attribution, o.AttributedTo[0], o.LagMs, o.ObsSeqLink)
		case event.TraceAmbiguous:
			fmt.Printf("    -> %s antara rev%v.\n", o.Attribution, o.AttributedTo)
			fmt.Println("       Tautannya TIDAK DAPAT DIPUTUSKAN dari data yang ada. Bacaan per")
			fmt.Println("       revisi ada di bagian 2+3; tidak ada yang dipilih di sini.")
		}
	}
	fmt.Println("")
	fmt.Println("Setiap baris di atas adalah KANDIDAT menurut KEANGGOTAAN-DAN-WAKTU. Tidak")
	fmt.Println("satu pun terbukti MENYEBABKAN revisinya: kolom yang akan membuktikannya")
	fmt.Println("tidak ada di skema (D12, tanpa observation_id, tanpa FK).")
}

// printUnattributed mencetak arah KEBALIKAN: revisi tanpa satu pun kandidat.
func printUnattributed(tl *event.EventTimeline) {
	fmt.Printf("\n--- revisi TANPA satu pun observasi kandidat ------------------------------\n")
	if len(tl.Unattributed) == 0 {
		fmt.Println("tidak ada.")
		return
	}
	for _, u := range tl.Unattributed {
		fmt.Printf("rev%-3d -> %-11s alasan=%-22s decided_at=%d nodes=%v\n",
			u.Revision, u.ToState, u.Reason, u.DecidedAt, u.NodeIDs)
		switch {
		case u.NotObservationDriven:
			fmt.Println("    -> BUKAN DIPICU OBSERVASI. Alasan ini lahir dari penjadwal atau")
			fmt.Println("       pencabutan, bukan dari bukti yang tiba, jadi jendela kandidat yang")
			fmt.Println("       kosong DIHARAPKAN di sini — bukan observasi yang hilang.")
		case u.NoContributors:
			fmt.Println("    -> contributors[] KOSONG atau evidence_summary tidak terurai, jadi")
			fmt.Println("       tidak ada keanggotaan yang dapat diuji sama sekali.")
		case u.MemberRowsFiltered > 0:
			fmt.Printf("    -> %d baris MEMANG cocok keanggotaan-dan-waktu tetapi disaring (di\n",
				u.MemberRowsFiltered)
			fmt.Println("       bawah lantai atau terkecuali). Barisnya ADA; ia hanya bukan pemicu.")
		default:
			fmt.Println("    -> baris pemicunya kemungkinan tidak pernah sampai ke disk:")
			fmt.Println("       ledger_drops_total HANYA masuk log (D17/D30). Nol di sini bukan")
			fmt.Println("       'tidak ada'. Periksa juga apakah jendelanya terlalu sempit.")
		}
	}
}

// printEmissions mencetak bagian KELIMA yang OPSIONAL.
func printEmissions(tl *event.EventTimeline) {
	fmt.Printf("\n--- 5 (OPSIONAL, BUKAN kriteria) emisi advisory per revisi ----------------\n")
	fmt.Println("Kriteria P4-M6' menyebut EMPAT keluaran. Bagian ini informasi tambahan dan")
	fmt.Println("TIDAK boleh menjadi penentu lulus atau tidaknya milestone (D-015 batasan 1).")
	if tl.Emissions == nil {
		fmt.Println("TIDAK DIBACA (INCLUDE_EMISSIONS=0 atau tanpa jendela).")
		return
	}
	if len(tl.Emissions) == 0 {
		fmt.Println("tidak ada revisi untuk ditautkan.")
		return
	}
	for _, m := range tl.Emissions {
		fmt.Printf("rev%-3d -> %-11s decided_at=%d  %s%s\n",
			m.Revision, m.ToState, m.DecidedAt, m.Outcome, emissionIDSuffix(m.EmissionID))
		if m.Outcome == event.EmissionMissing {
			fmt.Println("    -> MISSING adalah LAPORAN, bukan cacat: emisi dibatasi audiens dan")
			fmt.Println("       status, dan tidak setiap revisi memang menghasilkan satu.")
			continue
		}
		fmt.Printf("    -> alert_type=%s ws_clients=%s\n", orUnknown(m.AlertType), wsCount(m.WSClientCount))
		if m.Outcome == event.EmissionByTimeOnly {
			fmt.Println("       Baris pra-000008: tanpa event_id/event_revision. Cocok HANYA menurut")
			fmt.Println("       waktu, jadi bukti yang LEBIH LEMAH. Dilaporkan, tidak dibuang.")
		}
		if m.SharedTimeOnlyLink {
			fmt.Println("       EMISI YANG SAMA juga diklaim revisi lain. Baris pra-000008 tidak")
			fmt.Println("       membawa event_revision, jadi dua transisi yang berjarak lebih dekat")
			fmt.Println("       daripada toleransi memang TAK TERPISAHKAN olehnya. JANGAN hitung")
			fmt.Println("       satu emisi ini sebagai dua.")
		}
	}
}

// printCoverage mencetak selubung ketidaklengkapan. Bagian TERAKHIR dan sengaja:
// ia yang menentukan bagaimana seluruh angka di atas boleh dibaca.
func printCoverage(tl *event.EventTimeline) {
	cov := tl.Coverage
	fmt.Printf("\n--- kelengkapan dan KETIDAKLENGKAPAN -------------------------------------\n")

	if len(cov.RevisionGaps) > 0 {
		fmt.Printf("LUBANG REVISI    : %v  (HILANG di antara rev%d dan rev%d)\n",
			cov.RevisionGaps, cov.FirstRevision, cov.LastRevision)
		fmt.Println("                   UNIQUE(event_id, revision) menjamin revisi tidak")
		fmt.Println("                   berulang dan naik satu per transisi, jadi nomor yang")
		fmt.Println("                   hilang DI DALAM rentang adalah satuan persistensi yang")
		fmt.Println("                   DIBUANG (D17/D30) — bukti nyata, bukan dugaan.")
	} else {
		fmt.Printf("lubang revisi    : tidak ada di antara rev%d dan rev%d\n", cov.FirstRevision, cov.LastRevision)
	}
	fmt.Printf("revisi pertama   : rev%d\n", cov.FirstRevision)
	fmt.Println("                   Riwayat yang TIDAK dimulai dari revisi terendah BUKAN")
	fmt.Println("                   lubang: DETECTED tidak pernah menjadi baris (§9.5).")

	if cov.RevisionsEvidenceBroken > 0 {
		fmt.Printf("evidence RUSAK   : %d revisi. Barisnya nyata dan terhitung; keanggotaannya\n",
			cov.RevisionsEvidenceBroken)
		fmt.Println("                   tidak dapat diuji. Bukan nol kontributor — TIDAK DIKETAHUI.")
	}

	if cov.LedgerDropsKnown > 0 {
		fmt.Printf("ledger_drops     : %d (DIISI OPERATOR dari log). Pembacaan ini INCOMPLETE.\n", cov.LedgerDropsKnown)
	} else {
		fmt.Println("ledger_drops     : TIDAK DIKETAHUI. ledger_drops_total HANYA masuk log")
		fmt.Println("                   (D17/D30), jadi observasi dapat hilang tanpa jejak di")
		fmt.Println("                   dalam tabel. Nol di sini bukan 'tidak ada'.")
	}
	fmt.Println("retensi          : TIDAK ADA job retensi atau purge pada sensor_observations,")
	fmt.Println("                   event_state_log, alert_emissions atau earthquake_events.")
	fmt.Println("                   Riwayat tidak dipangkas di bawah pembacaan ini.")

	fmt.Printf("algo_ver baris   : %v\n", cov.AlgoVersRow)
	if len(cov.AlgoVersRow) > 1 {
		fmt.Println("                   LEBIH DARI SATU: riwayat ini melintasi perubahan versi")
		fmt.Println("                   algoritma. Baris lampau TIDAK ditulis ulang (D-006/V4).")
	}
	fmt.Printf("algo_ver biner   : %s\n", cov.AlgoVerBinary)
	fmt.Printf("toleransi tautan : %d ms  asal=%s  (jendela korelasi %d ms)\n",
		cov.LinkToleranceMs, cov.ToleranceProvenance, cov.CorrelationWindowMs)

	if cov.TerminalState != "" {
		fmt.Printf("state terminal   : %s tercatat. Riwayat ini SELESAI.\n", cov.TerminalState)
	} else {
		fmt.Println("state terminal   : TIDAK ADA baris RESOLVED atau CANCELLED. Event ini masih")
		fmt.Println("                   terbuka, ATAU transisi terminalnya tidak sampai ke disk.")
		fmt.Println("                   Kedua bacaan itu sah dan tabelnya tidak membedakannya.")
	}

	fmt.Println("")
	fmt.Println("Alat ini TIDAK menyatakan P4-M6' SATISFIED dan TIDAK memberi VALIDATED. Itu")
	fmt.Println("penilaian pemilik atas angka-angka di atas (PROJECT_RULES.md §8/§9), bukan")
	fmt.Println("boolean yang dihitung alat ini.")
}

// ---------------------------------------------------------------------------
// Pemformat
// ---------------------------------------------------------------------------

func fromState(p *string) string {
	if p == nil {
		return "(NULL)"
	}
	return *p
}

// orUnknown membedakan kolom NULL/kosong dari nilai bernama. String kosong
// ditulis sebagai NULL oleh UpsertEvent lewat NULLIF justru supaya keduanya tidak
// pernah dikonflasikan.
func orUnknown(s string) string {
	if s == "" {
		return "TIDAK DIKETAHUI (NULL)"
	}
	return s
}

func peakPGA(p *float64) string {
	if p == nil {
		return "TIDAK DILAPORKAN (NULL, bukan nol)"
	}
	return strconv.FormatFloat(*p, 'f', 4, 64)
}

// obsSeqVal membedakan obs_seq yang TIDAK ADA dari obs_seq nol. Kolomnya nullable
// karena protokol v1 tidak membawanya sama sekali (migrasi 000007).
func obsSeqVal(p *int64) string {
	if p == nil {
		return "TIDAK ADA (v1)"
	}
	return strconv.FormatInt(*p, 10)
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
	fmt.Fprintf(os.Stderr, "event_timeline: "+format+"\n", args...)
	os.Exit(1)
}
