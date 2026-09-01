// Tracker adalah otoritas siklus hidup event: satu mutex, satu map event
// terlacak, satu indeks sel/ember. Ia tidak pernah melakukan I/O di bawah kunci —
// disiplin yang sama yang membuat consensus.Engine tahan `-race`, dipertahankan
// karena alasannya tidak berubah.
package event

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/consensus"
	"github.com/banana-pixel/quakealert/server/internal/store"
)

// nodeSource adalah satu-satunya hal yang dibutuhkan Tracker dari basis data pada
// jalur masuk: koordinat node. Interface sempit, mengikuti pola ingest.nodeSource
// dan ledger.ledgerStore, supaya Tracker dapat diuji tanpa Postgres.
type nodeSource interface {
	GetNodeLocation(ctx context.Context, stationID string) (*store.NodeLocation, error)
}

// emitter menerima satu transisi state dan mengubahnya menjadi frame klien.
// Pemetaan snapshot -> dispatch.AlertMessage hidup di emit.go; di sini yang
// dibutuhkan hanya bahwa transisi keluar dari Tracker setelah kunci dilepas.
type emitter interface {
	EmitTransition(ctx context.Context, s Snapshot)
}

// Options adalah parameter korelasi yang berlaku, disalin dari config sekali saat
// konstruksi. Disalin dan bukan dibaca ulang: sebuah ambang yang berubah di tengah
// hidup sebuah event akan membuat keputusan yang tidak dapat dijelaskan oleh satu
// pun nilai konfigurasi.
type Options struct {
	CorrelationWindowMs int64
	AttachRadiusKm      float64
	IndependenceCellKm  float64
	MinIndependentCells int
	MaxEventDiameterKm  float64
	ResolveAfterMs      int64
	SweepIntervalMs     int64
	MaxOpen             int
	TerminalRetentionMs int64
	MaxTombstones       int
}

// indexKey adalah kunci indeks kandidat: sel lookup event ditambah ember onset-nya.
// Keduanya integer, tidak pernah string (§6.3).
type indexKey struct {
	cell   cellKey
	bucket int64
}

// Tracker melacak setiap event terbuka dan setiap event terminal yang masih dalam
// masa retensi tombstone (§6.8).
type Tracker struct {
	loc  nodeSource
	emit emitter
	log  *slog.Logger
	opt  Options

	// persist menerima satuan upsert+state-log. Nil = tidak dipersistensi, yang
	// sah dan tidak mengubah satu pun keputusan: kewenangan ada di memori dan
	// basis data adalah pengikut (§9.5).
	persist persister

	// now dan newID disuntikkan supaya seluruh perilaku Tracker deterministik di
	// bawah jam palsu; §18.2 bergantung padanya.
	now   func() time.Time
	newID func() string

	mu     sync.Mutex
	events map[string]*Event
	index  map[indexKey]map[string]*Event

	counters counters

	// latency memegang sampel latensi tahap server (P4-M3′, D-011). Dilindungi
	// t.mu bersama counters — bukan kunci kedua. Lihat latency.go.
	latency latency

	// nearConfirmed mencatat setiap event yang pernah mencapai >= minCells
	// kontributor independen. Tidak dihapus saat event terminal atau dievakuasi:
	// tujuannya rekonstruksi pasca-kejadian. Lihat nearconfirmed.go.
	nearConfirmed map[string]*NearConfirmedEntry
}

// NewTracker membuat Tracker. opt dianggap sudah divalidasi oleh config; nilai yang
// tidak masuk akal hanya diberi lantai di sini agar Tracker tetap aman dipakai
// dalam uji yang membangun Options-nya sendiri.
func NewTracker(loc nodeSource, opt Options, log *slog.Logger) *Tracker {
	if opt.CorrelationWindowMs <= 0 {
		opt.CorrelationWindowMs = 20000
	}
	if opt.IndependenceCellKm <= 0 {
		opt.IndependenceCellKm = 5
	}
	if opt.MinIndependentCells < 1 {
		opt.MinIndependentCells = 1
	}
	if opt.MaxOpen < 1 {
		opt.MaxOpen = 256
	}
	if opt.MaxTombstones < 1 {
		opt.MaxTombstones = 512
	}
	return &Tracker{
		loc:           loc,
		log:           log,
		opt:           opt,
		now:           time.Now,
		newID:         newEventID,
		events:        make(map[string]*Event),
		index:         make(map[indexKey]map[string]*Event),
		counters:      newCounters(),
		nearConfirmed: make(map[string]*NearConfirmedEntry),
	}
}

// SetEmitter memasang tujuan emisi. Dipisahkan dari konstruktor karena dispatcher
// dan Tracker dibangun pada titik yang berbeda di main.go, dan Tracker tanpa
// emitter tetap sah: ia melacak, tetapi tidak ada yang mendengar.
func (t *Tracker) SetEmitter(e emitter) { t.emit = e }

// SetLedger memasang antrean persistensi (pola Set* yang sama dengan
// Dispatcher.SetLedger). Nil menonaktifkan penulisan durable.
func (t *Tracker) SetLedger(p persister) { t.persist = p }

// Ingest adalah jalur masuk satu observasi terverifikasi.
//
// Bentuknya mengikuti §6.2 tepat: pencarian lokasi di LUAR kunci, seluruh
// keputusan di DALAM kunci tanpa satu pun I/O, emisi di luar kunci lagi.
func (t *Tracker) Ingest(ctx context.Context, in Input) {
	if in.NodeID == "" {
		return
	}

	// Gagal tertutup, seperti consensus.Engine hari ini: node tanpa koordinat
	// tidak dapat menyumbang ke geometri apa pun, jadi ia bukan observasi yang
	// dapat dipakai — bukan observasi yang dipaksa masuk dengan lokasi nol.
	nl, err := t.loc.GetNodeLocation(ctx, in.NodeID)
	if err != nil {
		t.log.Warn("event: lokasi node tak ditemukan, observasi diabaikan",
			"node_id", in.NodeID, "err", err)
		return
	}
	in.Lat, in.Lon, in.LocationName = nl.Lat, nl.Lon, nl.LocationName

	t.mu.Lock()
	transitions := t.ingestLocked(in)
	t.mu.Unlock()

	t.publish(ctx, transitions)
}

// publish mengirimkan transisi ke luar Tracker: EMISI lebih dulu, persistensi
// sesudahnya. Selalu di luar kunci, dan selalu dalam urutan itu.
//
// Urutannya adalah keseluruhan §9.5 dan bukan selera. Draf sebelumnya memanggil
// UpsertEvent secara sinkron dan MENGGULUNG BALIK transisinya bila penulisan itu
// gagal, yang artinya sebuah gangguan basis data MENEKAN peringatan gempa. Itu
// membalik satu-satunya kontrak yang Fase 1 tuliskan hitam di atas putih
// (internal/ledger: "pencatatan boleh gagal, jalur peringatan tidak") dan
// kegagalannya jauh lebih berbahaya: baris audit yang hilang ditemukan nanti oleh
// sebuah query, peringatan yang hilang ditemukan oleh sebuah gempa.
func (t *Tracker) publish(ctx context.Context, ts []Snapshot) {
	if len(ts) == 0 {
		return
	}

	if t.emit != nil {
		for _, s := range ts {
			t.emit.EmitTransition(ctx, s)
		}
	}

	// Pengukuran latensi P4-M3′ (D-011) DI SINI, setelah emisi, sebelum
	// persistensi. Posisinya adalah keseluruhan alasannya:
	//
	//   - SETELAH emisi, jadi tidak ada satu pun jam yang dibaca, kunci yang
	//     diambil, atau cabang yang dievaluasi sebelum frame keluar. Sebuah
	//     peringatan tidak boleh menunggu instrumentasinya sendiri (S1).
	//   - emit_at karena itu berarti "saat frame sudah diserahkan ke sink", yang
	//     memang batas tahap yang diukur. Ia BUKAN waktu tiba di perangkat: tidak
	//     ada tahap sisi klien di angka ini.
	//   - Kegagalan tidak mungkin di sini — tidak ada I/O — dan tetap begitu:
	//     apa pun yang ditambahkan ke blok ini nanti harus tetap tidak dapat
	//     mengembalikan galat, karena tidak ada pemanggil yang dapat
	//     menanganinya tanpa mempengaruhi jalur peringatan.
	t.observeLatency(ts)

	// Persistensi menyusul, per transisi, satu satuan masing-masing. Tidak ada
	// satu pun jalur di sini yang dapat mengembalikan galat ke pemanggil: satuan
	// yang dibuang atau gagal hanya menjadi counter (§15.5).
	if t.persist == nil {
		return
	}
	for _, s := range ts {
		t.persist.RecordEventUnit(t.unitFor(s))
	}
}

// observeLatency mencatat kedua tahap latensi server untuk sekumpulan transisi
// yang BARU SAJA diemisikan.
//
// Satu pengambilan kunci untuk seluruh batch, dan satu pembacaan jam untuk
// seluruh batch: transisi-transisi ini diemisikan dalam satu loop tanpa I/O di
// antaranya, jadi memberi masing-masing emit_at sendiri akan mengukur biaya loop
// alih-alih biaya jalurnya.
//
// OriginTS == 0 dilewati. Nol bukan "onset pada epoch"; ia berarti event ini tidak
// punya jangkar onset yang dapat dipertanggungjawabkan (baris pra-Fase-3 yang
// direkonsiliasi, misalnya), dan latensi terhadap jangkar yang tidak ada bukan
// nol — ia tidak terdefinisi.
//
// KEDUA tahap tidak memakai himpunan transisi yang sama, dan itu disengaja:
//
//   - onset->decided HANYA untuk transisi ke UNCONFIRMED dan CONFIRMED. Tahap ini
//     menjawab "berapa lama sejak tanah bergerak sampai sistem memutuskan sesuatu
//     yang dikirim ke orang", dan hanya kedua state itu yang melakukannya.
//     RESOLVED dan CANCELLED dibuang karena waktunya BUKAN latensi yang diukur:
//     sweep menaikkannya ResolveAfterMs setelah bukti terakhir, secara konfigurasi
//     (§9.4). Memasukkannya akan membuat p95 melaporkan timer resolve — puluhan
//     detik, sesuai desain — sebagai latensi deteksi, yaitu satu angka buruk yang
//     tidak menggambarkan apa pun yang rusak.
//   - decided->emit untuk SETIAP transisi. Tahap ini biaya jalur penyerahan frame,
//     dan biaya itu tidak bergantung pada state tujuan.
func (t *Tracker) observeLatency(ts []Snapshot) {
	emitAt := t.now().UnixMilli()

	t.mu.Lock()
	defer t.mu.Unlock()

	for _, s := range ts {
		if s.OriginTS > 0 && isDetectionDecision(s.To) {
			t.latency.observeOnsetToDecided(s.OriginTSSource, s.DecidedAt-s.OriginTS)
		}
		t.latency.observeDecidedToEmit(emitAt - s.DecidedAt)
	}
}

// isDetectionDecision benar untuk state yang keputusannya digerakkan BUKTI, jadi
// jarak dari onset ke keputusan itu adalah latensi sistem. State terminal
// digerakkan waktu, bukan bukti — lihat catatan di observeLatency.
func isDetectionDecision(to State) bool {
	return to == StateUnconfirmed || to == StateConfirmed
}

// Keempat callback ledger.EventPersistObserver. Counter-nya dimiliki Tracker
// karena §15.5 mendaftarkannya bersama counter event lain, sedangkan
// kegagalannya terjadi di goroutine drain — jadi ia dilaporkan kembali ke sini
// alih-alih dihitung di dua tempat yang dapat saling menyimpang.
func (t *Tracker) EventPersistDropped() { t.bumpCounter(func(c *counters) { c.persistDropped++ }) }
func (t *Tracker) EventUpsertFailed()   { t.bumpCounter(func(c *counters) { c.upsertFailures++ }) }
func (t *Tracker) EventStateLogSkipped() {
	t.bumpCounter(func(c *counters) { c.stateLogSkipped++ })
}
func (t *Tracker) EventStateLogFailed() {
	t.bumpCounter(func(c *counters) { c.stateLogFailures++ })
}

// bumpCounter menaikkan satu counter di bawah kunci Tracker. Kunci yang SAMA,
// bukan kunci kedua maupun atomics: setiap kenaikan counter lain sudah terjadi di
// bawahnya, dan dua mekanisme perlindungan untuk satu struct adalah dua tempat
// yang dapat saling menyimpang.
func (t *Tracker) bumpCounter(f func(*counters)) {
	t.mu.Lock()
	f(&t.counters)
	t.mu.Unlock()
}

// ingestLocked adalah keseluruhan §6.2 langkah 3-11. Wajib dipanggil dengan t.mu
// terkunci dan tidak boleh melakukan I/O.
func (t *Tracker) ingestLocked(in Input) []Snapshot {
	now := t.now().UnixMilli()

	e, split := t.selectTargetLocked(in)

	// Tombstone (§6.8): bukti yang terlambat MENEMPEL pada event yang
	// dijelaskannya, lalu berhenti di situ. classify tidak dijalankan, jadi tidak
	// ada revisi, tidak ada baris log, tidak ada frame.
	if e != nil && e.isTerminal() {
		t.upsertContributorLocked(e, in)
		t.counters.staleAbsorbed++
		return nil
	}

	var out []Snapshot
	if e == nil {
		created, forced := t.newEventLocked(in, now)
		out = append(out, forced...)
		e = created
		if split {
			t.counters.reonsetSplits++
		}
	} else {
		t.upsertContributorLocked(e, in)
	}

	e.LastEvidenceTS = now
	t.reindexLocked(e)

	if s := t.transitionLocked(e, now, ""); s != nil {
		out = append(out, *s)
	}
	return out
}

// selectTargetLocked mengembalikan event yang harus menerima observasi ini, atau
// nil bila observasi harus memulai event baru. split menandai bahwa nil berasal
// dari aturan re-onset §6.5 nomor 4, bukan dari tiadanya kandidat.
func (t *Tracker) selectTargetLocked(in Input) (target *Event, split bool) {
	open, tombstones := t.candidatesLocked(in)

	// Event terbuka lebih dulu: bila sebuah event yang masih hidup dan sebuah
	// tombstone sama-sama cocok, menyerap bukti ke dalam tombstone akan mencabutnya
	// dari event yang justru masih dapat mengambil keputusan. §4.3 menyebut
	// "beberapa event TERBUKA" dan urutan ini yang membuatnya benar.
	for _, cand := range [][]*Event{open, tombstones} {
		e := bestMatch(cand, in.OnsetTS)
		if e == nil {
			continue
		}
		if e.isTerminal() {
			return e, false
		}
		if t.isSecondEpisodeLocked(e, in) {
			// Episode kedua di node yang sudah menyumbang: diskriminator terkuat
			// yang dapat dihasilkan fleet ini (§6.6). Jangan menempel.
			return nil, true
		}
		if t.wouldExceedDiameterLocked(e, in) {
			t.counters.diameterRejections++
			return nil, false
		}
		return e, false
	}
	return nil, false
}

// candidatesLocked menyelidiki lingkungan sel lookup di sekitar observasi kali
// TIGA ember onset — b-1, b, b+1.
//
// Lebar lingkungannya DITURUNKAN, bukan dipatok. Bentuk 3x3 yang lama menutupi
// AttachRadiusKm hanya selama LookupCellDeg * KmPerDegree * cos(lat) >= radius,
// yang berhenti berlaku di |lat| ~= 41,5°: di atas itu sebuah event yang benar
// berada di dalam radius attach dapat berada di luar lingkungan yang diselidiki,
// tidak pernah diperiksa matches(), dan menjadi event_id KEDUA — PEMBELAHAN,
// dua alert untuk satu gempa, atau dua belahan yang keduanya gagal mencapai
// kuorum sehingga TIDAK ADA alert sama sekali. Lebar kini dihitung dari lintang
// observasi itu sendiri (probeSpan), sehingga invarian I-COV berlaku di setiap
// lintang. Di bawah 41,5° kedua lebar tetap 1 dan lingkungan yang diselidiki
// IDENTIK dengan 3x3 yang lama.
//
// Sumbu bujur DILIPAT (wrapCellX): sel bujur adalah lingkaran, dan probe yang
// tidak melipat memperlakukan 179,9° dan -179,9° sebagai 600 sel berjauhan
// meski keduanya bertetangga.
//
// Tiga ember, bukan dua: buktinya di §4.3, dan dua ember mengandaikan jangkar
// sebuah event tidak pernah lebih baru dari onset yang sedang datang, yang gagal
// setiap kali observasi tak berurut menyeberangi batas ember. Kegagalannya juga
// PEMBELAHAN.
//
// Indeks hanya MENYEMPITKAN; matches() tetap satu-satunya penentu. Karena itu
// lingkungan yang terlalu lebar hanya biaya, bukan kesalahan: entri yang dibaca
// berlebih adalah kunci map yang kosong, karena indeks memegang paling banyak
// MaxOpen + MaxTombstones event apa pun lebarnya.
func (t *Tracker) candidatesLocked(in Input) (open, tombstones []*Event) {
	c := lookupCell(in.Lat, in.Lon)
	b := onsetBucket(in.OnsetTS, t.opt.CorrelationWindowMs)
	nx, ny, allLon := probeSpan(in.Lat, t.opt.AttachRadiusKm)

	seen := make(map[string]struct{})
	consider := func(k indexKey) {
		for id, e := range t.index[k] {
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			if !matches(e, in.OnsetTS, in.Lat, in.Lon, t.opt.CorrelationWindowMs, t.opt.AttachRadiusKm) {
				continue
			}
			if e.isTerminal() {
				tombstones = append(tombstones, e)
			} else {
				open = append(open, e)
			}
		}
	}

	// Kasus polar (allLon): lingkaran radius attach memuat sebuah kutub, jadi
	// SETIAP bujur berada di dalamnya dan tidak ada bound bujur yang bermakna.
	// Ditangani eksplisit — seluruh cincin bujur diselidiki untuk setiap baris
	// lintang — alih-alih dibiarkan menjadi pembagian oleh cos(lat) yang menuju
	// nol. Lebarnya paling banyak 600 kunci per baris per ember, dan hanya di
	// dalam ~50 km dari kutub.
	xs := make([]int32, 0, 2*int(nx)+1)
	if allLon {
		half := lonCellCount / 2
		for x := -half; x < half; x++ {
			xs = append(xs, x)
		}
	} else {
		for dx := -nx; dx <= nx; dx++ {
			xs = append(xs, wrapCellX(c.X+dx))
		}
	}

	for dy := -ny; dy <= ny; dy++ {
		y := c.Y + dy
		for _, x := range xs {
			for db := int64(-1); db <= 1; db++ {
				consider(indexKey{cell: cellKey{X: x, Y: y}, bucket: b + db})
			}
		}
	}
	return open, tombstones
}

// bestMatch memilih kandidat dengan |onset - origin_ts| terkecil; seri diputus
// oleh urutan leksikografis event_id, sehingga hasilnya tidak pernah bergantung
// pada urutan iterasi map.
func bestMatch(cands []*Event, onsetTS int64) *Event {
	var best *Event
	var bestD int64
	for _, e := range cands {
		d := onsetTS - e.OriginTS
		if d < 0 {
			d = -d
		}
		switch {
		case best == nil, d < bestD, d == bestD && e.ID < best.ID:
			best, bestD = e, d
		}
	}
	return best
}

// isSecondEpisodeLocked menerapkan §6.5 aturan 1/4/5.
//
// Episode yang SAMA (obs_seq cocok) selalu sebuah revisi. Selain itu, onset di
// LUAR CorrelationWindowMs dari onset kontributor yang tercatat berarti node itu
// melaporkan guncangan kedua yang berbeda di lokasi yang diketahui. Untuk node v1
// obs_seq tidak ada di kedua sisi, jadi yang tersisa hanyalah uji jendela — tepat
// seperti §6.6 menyatakannya.
func (t *Tracker) isSecondEpisodeLocked(e *Event, in Input) bool {
	c, ok := e.Contributors[in.NodeID]
	if !ok {
		return false
	}
	if c.ObsSeq != nil && in.ObsSeq != nil && *c.ObsSeq == *in.ObsSeq {
		return false
	}
	d := in.OnsetTS - c.OnsetTS
	if d < 0 {
		d = -d
	}
	return d > t.opt.CorrelationWindowMs
}

// wouldExceedDiameterLocked menguji tutup diameter §6.4 SEBELUM kontributor
// dikomit: centroid berbobot PGA dapat melayang ke arah anggota ber-PGA tinggi dan
// menarik batas efektif bersamanya, jadi radius menempel saja tidak cukup.
func (t *Tracker) wouldExceedDiameterLocked(e *Event, in Input) bool {
	if t.opt.MaxEventDiameterKm <= 0 {
		return false
	}
	limit := t.opt.MaxEventDiameterKm / 2

	rs := e.readings()
	found := false
	for i := range rs {
		if rs[i].NodeID == in.NodeID {
			found = true
			if in.PGA > rs[i].PGA {
				rs[i].PGA = in.PGA
			}
			break
		}
	}
	if !found {
		rs = append(rs, consensus.Reading{
			NodeID: in.NodeID, Lat: in.Lat, Lon: in.Lon,
			PGA: in.PGA, TS: in.OnsetTS, LocationName: in.LocationName,
		})
	}

	c := consensus.WeightedCentroidGlobal(rs)
	for _, r := range rs {
		if consensus.HaversineKm(r.Lat, r.Lon, c.Lat, c.Lon) > limit {
			return true
		}
	}
	return false
}

// newEventLocked membuat event baru dan mendaftarkannya di indeks. Batas §15.4
// ditegakkan di sini, karena pembuatan adalah satu-satunya tempat map dapat
// bertumbuh: transisi yang dipaksakan olehnya ikut dikembalikan supaya ia juga
// sampai ke klien — dipaksa bukan berarti diam-diam.
func (t *Tracker) newEventLocked(in Input, now int64) (*Event, []Snapshot) {
	forced := t.enforceBoundsLocked(now)

	e := &Event{
		ID:             t.newID(),
		State:          StateDetected,
		OriginTS:       in.OnsetTS,
		OriginTSSource: in.OnsetSource,
		DecidedAt:      now,
		LastEvidenceTS: now,
		CreatedAt:      now,
		Contributors:   make(map[string]*Contributor, 4),
		minCells:       t.opt.MinIndependentCells,
		minSepKm:       t.opt.IndependenceCellKm,
	}
	t.upsertContributorLocked(e, in)
	t.events[e.ID] = e
	t.indexLocked(e, t.keyOf(e))
	t.counters.created++
	return e, forced
}

// keyOf menghitung kunci indeks event dari centroid-nya SAAT INI. Indeks
// mengikuti centroid, bukan node pembuat: predikat §4.3 mengukur jarak ke
// centroid, jadi pembuktian kecukupan 3x3 hanya berlaku bila titik yang
// diindeks adalah titik yang sama yang diukur.
func (t *Tracker) keyOf(e *Event) indexKey {
	c := e.centroid()
	return indexKey{
		cell:   lookupCell(c.Lat, c.Lon),
		bucket: onsetBucket(e.OriginTS, t.opt.CorrelationWindowMs),
	}
}

func (t *Tracker) indexLocked(e *Event, k indexKey) {
	m := t.index[k]
	if m == nil {
		m = make(map[string]*Event, 1)
		t.index[k] = m
	}
	m[e.ID] = e
	e.lookupKey, e.bucket = k.cell, k.bucket
}

func (t *Tracker) unindexLocked(e *Event) {
	k := indexKey{cell: e.lookupKey, bucket: e.bucket}
	m := t.index[k]
	if m == nil {
		return
	}
	delete(m, e.ID)
	if len(m) == 0 {
		delete(t.index, k)
	}
}

// reindexLocked memindahkan entri indeks bila centroid sudah menyeberang batas sel.
// Ember tidak pernah berubah: ia berasal dari OriginTS, yang tidak pernah bergerak.
func (t *Tracker) reindexLocked(e *Event) {
	k := t.keyOf(e)
	if k.cell == e.lookupKey && k.bucket == e.bucket {
		return
	}
	t.unindexLocked(e)
	t.indexLocked(e, k)
}

// upsertContributorLocked menerapkan §6.5 aturan 0-5 pada satu kontributor.
func (t *Tracker) upsertContributorLocked(e *Event, in Input) {
	c, ok := e.Contributors[in.NodeID]
	if !ok {
		e.Contributors[in.NodeID] = &Contributor{
			NodeID:        in.NodeID,
			Lat:           in.Lat,
			Lon:           in.Lon,
			LocationName:  in.LocationName,
			Cell:          independenceCell(in.Lat, in.Lon, t.opt.IndependenceCellKm),
			ObsSeq:        in.ObsSeq,
			Phase:         in.Phase,
			PeakPGA:       in.PGA,
			OnsetTS:       in.OnsetTS,
			OnsetSource:   in.OnsetSource,
			LastPublishTS: in.PublishTS,
			DetriggerTS:   in.DetriggerTS,
			Revisions:     1,
		}
		return
	}

	// PGA hanya boleh naik. Regresi fase (FINAL lalu PRELIM yang tertunda) adalah
	// kejadian normal — dedup verifier bekerja per fase — dan puncak yang menyusut
	// karenanya akan menurunkan intensitas yang sudah disiarkan.
	if in.PGA > c.PeakPGA {
		c.PeakPGA = in.PGA
	}
	if in.Phase == PhaseFinal {
		c.Phase = PhaseFinal
	}
	if in.ObsSeq != nil && (c.ObsSeq == nil || *in.ObsSeq > *c.ObsSeq) {
		c.ObsSeq = in.ObsSeq
	}
	if in.DetriggerTS != nil {
		c.DetriggerTS = in.DetriggerTS
	}
	if in.PublishTS > c.LastPublishTS {
		c.LastPublishTS = in.PublishTS
	}
	c.Revisions++
	// OnsetTS dan OnsetSource TIDAK disentuh: first-bound-wins (D29).
}

// transitionLocked menjalankan §6.2 langkah 10-11. reason kosong berarti
// "turunkan dari state tujuan"; pemanggil yang punya alasan lebih spesifik
// (invalidasi, resolusi paksa) memberikannya sendiri.
func (t *Tracker) transitionLocked(e *Event, now int64, reason string) *Snapshot {
	next := classify(e)
	// Observability B: rekam near-confirmed bahkan bila state tidak berubah
	// (misalnya kontributor ke-2 tiba saat event sudah UNCONFIRMED). Pemanggil
	// forceTransitionLocked sudah memanggil recordNearConfirmedLocked di dalam
	// transisi nyata; panggilan ini menangkap kasus "lebih banyak bukti,
	// state sama".
	// DETECTED dikecualikan: event yang belum pernah publik tidak boleh masuk log.
	if e.State != StateDetected {
		t.recordNearConfirmedLocked(e, now)
	}
	return t.forceTransitionLocked(e, next, now, reason)
}

func (t *Tracker) forceTransitionLocked(e *Event, next State, now int64, reason string) *Snapshot {
	if next == e.State || !legal(e.State, next) {
		return nil
	}
	if reason == "" {
		reason = reasonFor(next)
	}

	from := e.State
	e.State = next
	e.Revision++
	e.DecidedAt = now
	if next == StateConfirmed {
		e.EverConfirmed = true
	}
	if isTerminal(next) {
		e.TerminalAt = now
	}
	t.counters.transitions[next]++

	// Observability B: rekam kondisi near-confirmed setelah state baru diterapkan.
	t.recordNearConfirmedLocked(e, now)

	s := e.snapshot(from, reason)
	return &s
}

// reasonFor memetakan state tujuan ke kosakata reason §5.3. Tertutup dan kecil:
// tabel ini ada supaya kolomnya dapat diagregasi, bukan dibaca satu-satu.
func reasonFor(to State) string {
	switch to {
	case StateUnconfirmed:
		return ReasonFloorMet
	case StateConfirmed:
		return ReasonQuorumMet
	case StateResolved:
		return ReasonNoNewEvidence
	case StateCancelled:
		return ReasonEvidenceInvalid
	default:
		return ReasonFirstObservation
	}
}

// enforceBoundsLocked menegakkan kedua langit-langit §15.4 sebelum sebuah event
// baru masuk. Keduanya dihitung dan dievakuasi SECARA TERPISAH: sebuah tombstone
// tidak boleh menekan event yang masih hidup keluar dari map, dan sebaliknya.
func (t *Tracker) enforceBoundsLocked(now int64) []Snapshot {
	var out []Snapshot

	for len(t.tombstonesLocked()) >= t.opt.MaxTombstones {
		oldest := oldestBy(t.tombstonesLocked(), func(e *Event) int64 { return e.TerminalAt })
		if oldest == nil {
			break
		}
		t.dropLocked(oldest)
		// Nol dalam operasi normal: tombstone dievakuasi oleh USIA (§6.8), bukan
		// oleh tekanan. Bukan nol berarti jaminan "tidak ada alert ganda yang dapat
		// dilihat pengguna" sedang melemah, dan itu harus terlihat.
		t.counters.tombstoneEvictions++
	}

	for len(t.openLocked()) >= t.opt.MaxOpen {
		oldest := oldestBy(t.openLocked(), func(e *Event) int64 { return e.CreatedAt })
		if oldest == nil {
			break
		}
		// DETECTED tidak pernah "diselesaikan": ia tidak pernah publik, jadi
		// tidak ada apa pun untuk ditutup dan D->RESOLVED ilegal (§5.2). Ia
		// KEDALUWARSA.
		if s := t.forceTransitionLocked(oldest, StateResolved, now, ReasonNoNewEvidence); s != nil {
			out = append(out, *s)
		} else {
			t.dropLocked(oldest)
		}
		t.counters.forcedResolutions++
	}
	return out
}

// dropLocked mencabut event dari map dan indeks. Satu-satunya tempat penghapusan
// terjadi, sehingga entri indeks tidak dapat hidup lebih lama dari event-nya.
func (t *Tracker) dropLocked(e *Event) {
	t.unindexLocked(e)
	delete(t.events, e.ID)
}

// openLocked dan tombstonesLocked mengiterasi map. Map itu berbatas 256 + 512
// entri (§15.4), jadi iterasi jauh lebih murah daripada dua counter yang dapat
// menyimpang dari kebenaran tanpa ada yang menyadarinya.
func (t *Tracker) openLocked() []*Event {
	out := make([]*Event, 0, len(t.events))
	for _, e := range t.events {
		if !e.isTerminal() {
			out = append(out, e)
		}
	}
	return out
}

func (t *Tracker) tombstonesLocked() []*Event {
	out := make([]*Event, 0, len(t.events))
	for _, e := range t.events {
		if e.isTerminal() {
			out = append(out, e)
		}
	}
	return out
}

// oldestBy memilih event dengan stempel terkecil; seri diputus oleh event_id
// supaya evakuasi tidak bergantung pada urutan iterasi map.
func oldestBy(es []*Event, stamp func(*Event) int64) *Event {
	if len(es) == 0 {
		return nil
	}
	sort.Slice(es, func(i, j int) bool {
		si, sj := stamp(es[i]), stamp(es[j])
		if si != sj {
			return si < sj
		}
		return es[i].ID < es[j].ID
	})
	return es[0]
}
