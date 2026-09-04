package store

// Persistensi lifecycle event (Fase 3, migrasi 000008).
//
// Dipisahkan dari store.go bukan karena ukuran, melainkan karena kontraknya
// berbeda dari repository lain di paket ini: kedua penulis di bawah dipanggil
// dari antrean ledger yang boleh gagal, SETELAH frame peringatan terkirim, dan
// karena itu tidak boleh mengasumsikan ada pemanggil yang menunggu hasilnya
// (§9.5). Identitas event dibuat di Go sebelum penulisan mana pun — basis data
// tidak pernah lagi menjadi otoritas identitas (§4.1), dan itulah sebabnya
// UpsertEvent menyebut event_id sebagai kolom INSERT eksplisit alih-alih
// memakai RETURNING seperti SaveEvent.

import (
	"context"
	"fmt"
)

// Nilai kolom earthquake_events.event_state. Cermin dari event.State di paket
// internal/event; didefinisikan ulang di sini agar store tidak mengimpor paket
// yang mengimpornya.
const (
	EventStateDetected    = "DETECTED"
	EventStateUnconfirmed = "UNCONFIRMED"
	EventStateConfirmed   = "CONFIRMED"
	EventStateResolved    = "RESOLVED"
	EventStateCancelled   = "CANCELLED"
)

// EventStateLog adalah satu baris event_state_log: satu TRANSISI state.
//
// FromState bertipe pointer karena kolomnya boleh NULL, dan NULL di sini punya
// arti yang sempit: tidak ada state sebelumnya sama sekali. Untuk event yang
// menjadi publik, FromState berisi 'DETECTED' — state itu memang pernah dipegang
// di memori meski tidak pernah menjadi baris (persistensi DETECTED lazy, §9.5).
//
// EvidenceSummary adalah JSON mentah yang sudah di-marshal pemanggil. Snapshot,
// bukan join: baris yang akan dijoin (sensor_observations) ditulis melalui
// antrean yang sengaja boleh membuang, sehingga jejak audit sebuah keputusan
// tidak boleh bergantung padanya (§9.3).
type EventStateLog struct {
	EventID          string
	Revision         int
	FromState        *string
	ToState          string
	Reason           string
	DecidedAt        int64 // ms epoch UTC, jam server
	NodeCount        int
	IndependentCells int
	PeakPGA          *float64
	EvidenceSummary  []byte
	AlgoVer          string
}

// UpsertEvent menulis atau memperbarui satu baris earthquake_events.
//
// ON CONFLICT dijaga oleh `WHERE earthquake_events.revision < EXCLUDED.revision`.
// Penjaga itu bukan optimasi: satuan persistensi diantre dan didrain oleh satu
// goroutine, tetapi antrean itu boleh MEMBUANG yang tertua (D17), sehingga dua
// satuan untuk event yang sama harus aman dalam urutan apa pun. Dengan penjaga
// ini sebuah penulisan yang datang terlambat tidak dapat memundurkan baris ke
// state yang lebih tua, dan drop-oldest hanya kehilangan satu langkah antara —
// bukan kebenaran baris terakhir.
//
// event_state ditulis lewat NULLIF: string kosong berarti "tidak diketahui" dan
// harus tiba sebagai NULL, bukan sebagai string kosong yang terlihat seperti
// state bernama.
//
// status TIDAK dihitung di sini melainkan diambil dari e.Status: ia proyeksi dari
// event_state (§9.1) dan pemilik proyeksi itu adalah pemanggil, yang juga yang
// tahu apakah baris ini berasal dari jalur Fase 3 sama sekali.
func (s *Store) UpsertEvent(ctx context.Context, e *EarthquakeEvent) error {
	const q = `
		INSERT INTO earthquake_events (
			event_id, status, estimated_centroid, location_name,
			mmi_scale, intensity_label, max_pga, triggered_nodes_count,
			started_at,
			event_state, revision, origin_ts, origin_ts_source,
			independent_cell_count, algo_ver
		) VALUES (
			$1::uuid, $2,
			ST_SetSRID(ST_MakePoint($3, $4), 4326)::geography,
			$5, $6, $7, $8, $9,
			to_timestamp($10::double precision / 1000.0),
			NULLIF($11, ''), $12, $13, NULLIF($14, ''),
			$15, NULLIF($16, '')
		)
		ON CONFLICT (event_id) DO UPDATE SET
			status                 = EXCLUDED.status,
			estimated_centroid     = EXCLUDED.estimated_centroid,
			location_name          = EXCLUDED.location_name,
			mmi_scale              = EXCLUDED.mmi_scale,
			intensity_label        = EXCLUDED.intensity_label,
			max_pga                = EXCLUDED.max_pga,
			triggered_nodes_count  = EXCLUDED.triggered_nodes_count,
			event_state            = EXCLUDED.event_state,
			revision               = EXCLUDED.revision,
			origin_ts              = EXCLUDED.origin_ts,
			origin_ts_source       = EXCLUDED.origin_ts_source,
			independent_cell_count = EXCLUDED.independent_cell_count,
			algo_ver               = EXCLUDED.algo_ver,
			resolved_at            = CASE WHEN EXCLUDED.status = 'RESOLVED'
			                              THEN COALESCE(earthquake_events.resolved_at, NOW())
			                              ELSE earthquake_events.resolved_at END
		WHERE earthquake_events.revision < EXCLUDED.revision`
	_, err := s.pool.Exec(ctx, q,
		e.EventID, e.Status, e.CentroidLon, e.CentroidLat, e.LocationName,
		e.MMIScale, e.IntensityLabel, e.MaxPGA, e.TriggeredNodes, e.StartedAtMs,
		e.EventState, e.Revision, e.OriginTS, e.OriginTSSource,
		e.IndependentCellCount, e.AlgoVer,
	)
	if err != nil {
		return fmt.Errorf("upsert earthquake_event: %w", err)
	}
	return nil
}

// AppendStateLog menulis satu baris transisi.
//
// ON CONFLICT (event_id, revision) DO NOTHING membuat pemutaran ulang sebuah
// transisi menjadi no-op di tingkat basis data, yang diandalkan §15: satu-satunya
// hal yang menentukan sebuah transisi adalah (event, revisi), dan menulisnya dua
// kali tidak boleh menghasilkan dua baris riwayat.
//
// Pemanggil WAJIB memanggil UpsertEvent untuk event yang sama lebih dahulu dan
// TIDAK memanggil fungsi ini bila upsert itu gagal — FK ke earthquake_events
// dipenuhi by construction, bukan ditangkap sebagai error (§9.5).
func (s *Store) AppendStateLog(ctx context.Context, l *EventStateLog) error {
	const q = `
		INSERT INTO event_state_log (
			event_id, revision, from_state, to_state, reason,
			decided_at, node_count, independent_cells, peak_pga,
			evidence_summary, algo_ver
		) VALUES (
			$1::uuid, $2, $3, $4, $5,
			$6, $7, $8, $9,
			$10::jsonb, $11
		)
		ON CONFLICT (event_id, revision) DO NOTHING`
	_, err := s.pool.Exec(ctx, q,
		l.EventID, l.Revision, l.FromState, l.ToState, l.Reason,
		l.DecidedAt, l.NodeCount, l.IndependentCells, l.PeakPGA,
		string(l.EvidenceSummary), l.AlgoVer,
	)
	if err != nil {
		return fmt.Errorf("append event_state_log: %w", err)
	}
	return nil
}

// LoadOpenEvents mengembalikan seluruh event yang masih HAPPENING, beserta
// evidence_summary dari revisi TERTINGGI-nya, untuk rekonsiliasi saat boot
// (§15.3).
//
// Predikatnya `status = 'HAPPENING'`, bukan `event_state IN (...)`: baris
// pra-Fase-3 punya status tetapi tidak punya event_state, dan justru baris
// itulah yang paling mungkin menggantung setelah sebuah restart. Baris seperti
// itu tiba dengan EventState kosong, dan pemanggil yang memutuskan apa artinya.
//
// Ringkasan bukti diambil lewat LEFT JOIN LATERAL, bukan query kedua per event:
// jumlah event terbuka kecil, tetapi query per baris pada jalur boot adalah
// pola yang tumbuh diam-diam. LEFT, bukan INNER, karena event yang satuan
// persistensinya dibuang (D30) tetap punya baris induk tanpa baris log — dan
// event seperti itu justru yang paling perlu direkonsiliasi.
//
// decided_at baris yang sama ikut terbaca, dan bukan sebagai kemudahan: §15.3
// harus memutuskan apakah event yang dimuat ulang sudah kedaluwarsa, dan waktu
// bukti terakhir tidak ada di earthquake_events — transisi terakhir adalah
// satu-satunya jejak durable kapan event ini terakhir bergerak. COALESCE ke 0
// karena LEFT JOIN yang tak menemukan apa pun mengembalikan NULL, dan 0 di sini
// berarti "tidak ada transisi durable", bukan "epoch".
func (s *Store) LoadOpenEvents(ctx context.Context) ([]*EarthquakeEvent, error) {
	const q = `
		SELECT e.event_id,
		       e.status,
		       ST_Y(e.estimated_centroid::geometry) AS lat,
		       ST_X(e.estimated_centroid::geometry) AS lon,
		       e.location_name, e.mmi_scale, e.intensity_label,
		       e.max_pga::double precision AS max_pga,
		       e.triggered_nodes_count,
		       (EXTRACT(EPOCH FROM e.started_at) * 1000)::bigint AS started_at_ms,
		       COALESCE(e.event_state, ''),
		       e.revision,
		       COALESCE(e.origin_ts, 0),
		       COALESCE(e.origin_ts_source, ''),
		       COALESCE(e.independent_cell_count, 0),
		       COALESCE(e.algo_ver, ''),
		       l.evidence_summary,
		       COALESCE(l.decided_at, 0)
		FROM earthquake_events e
		LEFT JOIN LATERAL (
			SELECT evidence_summary, decided_at
			FROM event_state_log
			WHERE event_id = e.event_id
			ORDER BY revision DESC
			LIMIT 1
		) l ON TRUE
		WHERE e.status = 'HAPPENING'
		ORDER BY e.started_at ASC`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query open events: %w", err)
	}
	defer rows.Close()

	out := make([]*EarthquakeEvent, 0, 8)
	for rows.Next() {
		var e EarthquakeEvent
		if err := rows.Scan(
			&e.EventID, &e.Status, &e.CentroidLat, &e.CentroidLon,
			&e.LocationName, &e.MMIScale, &e.IntensityLabel,
			&e.MaxPGA, &e.TriggeredNodes, &e.StartedAtMs,
			&e.EventState, &e.Revision, &e.OriginTS, &e.OriginTSSource,
			&e.IndependentCellCount, &e.AlgoVer,
			&e.LatestEvidence, &e.LatestDecidedAt,
		); err != nil {
			return nil, fmt.Errorf("scan open event: %w", err)
		}
		out = append(out, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate open events: %w", err)
	}
	return out, nil
}

// EventUnit adalah SATU satuan persistensi untuk satu transisi state: baris induk
// earthquake_events dan baris riwayat event_state_log-nya, dalam urutan itu.
//
// Satu satuan, bukan dua item antrean, dan itulah keseluruhan alasan tipe ini ada.
// FK event_state_log.event_id -> earthquake_events(event_id) dipenuhi BY
// CONSTRUCTION bila kedua penulisan selalu berpasangan dan selalu berurutan:
// upsert lebih dulu, log hanya bila upsert berhasil. Dua item terpisah di antrean
// yang boleh membuang yang tertua (D17) tidak dapat menjanjikan itu — yang dibuang
// bisa saja induknya.
//
// Log boleh nil (persistensi tanpa transisi, mis. rekonsiliasi), Event tidak
// boleh: sebuah baris riwayat tanpa induk adalah tepatnya hal yang tidak boleh
// dapat terjadi.
type EventUnit struct {
	Event *EarthquakeEvent
	Log   *EventStateLog
}

// ListActiveNodeLocations mengembalikan koordinat seluruh node yang AKTIF dan
// TERVERIFIKASI, tanpa titik pusat.
//
// Ada karena pemeriksaan-diri fleet (§7.3, §6.3.1) menanyakan hal yang berbeda
// dari ListSensorsWithin: bukan "node apa yang dekat pengguna ini", melainkan
// "apakah fleet secara keseluruhan dapat mencapai CONFIRMED sama sekali". Tidak
// ada titik pusat yang masuk akal untuk pertanyaan itu, jadi radius bukan
// parameter yang dapat dihilangkan — query-nya memang query lain.
//
// Predikatnya sama dengan yang menentukan apakah observasi sebuah node dihitung:
// is_active AND verified — tidak lebih. Node yang tidak dapat menyumbang bukti
// tidak boleh ikut menghitung sel independensi, karena menghitungnya akan membuat
// peringatan startup berbohong ke arah yang aman-terlihat. iot_nodes.location
// sendiri NOT NULL sejak migrasi 000001, jadi tidak ada penjaga koordinat di sini.
func (s *Store) ListActiveNodeLocations(ctx context.Context) ([]NodeLocation, error) {
	const q = `
		SELECT station_id,
		       ST_Y(location::geometry) AS lat,
		       ST_X(location::geometry) AS lon,
		       location_name
		FROM iot_nodes
		WHERE is_active AND verified
		ORDER BY station_id ASC`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query active node locations: %w", err)
	}
	defer rows.Close()

	out := make([]NodeLocation, 0, 16)
	for rows.Next() {
		var n NodeLocation
		if err := rows.Scan(&n.StationID, &n.Lat, &n.Lon, &n.LocationName); err != nil {
			return nil, fmt.Errorf("scan active node location: %w", err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active node locations: %w", err)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Pembacaan untuk replay deterministik (P4-M4′).
//
// Dua query di bawah adalah SATU-SATUNYA jalur baca ke sensor_observations dan
// ke riwayat event_state_log yang dimiliki paket ini, dan keduanya READ-ONLY:
// tidak ada INSERT, UPDATE, DELETE, tidak ada tabel bantu, tidak ada migrasi.
// V4 melarang menulis ulang baris historis, jadi replay hanya boleh MEMBACA
// ledger dan membandingkan — bukan memperbaikinya.
//
// Keduanya mengembalikan baris APA ADANYA, termasuk baris yang tidak akan
// dipakai konsensus (verify_result != 'OK', node_location NULL). Penyaringan
// dilakukan pemanggil supaya jumlah yang tersaring dapat DILAPORKAN; query yang
// menyaring sendiri akan membuat replay tampak lengkap padahal masukannya
// tidak.
// ---------------------------------------------------------------------------

// ReplayObservation adalah satu baris sensor_observations sebagaimana dibutuhkan
// pemutaran ulang: kolom mentah, tanpa turunan apa pun.
//
// Berbeda dari Observation (jalur tulis) dalam dua hal yang penting: ada
// ObservationID — kunci urut kedatangan dan tie-break kanonik — dan ada
// VerifyResult yang dibawa keluar apa adanya supaya pemanggil dapat menghitung
// baris yang ia saring.
//
// Lat/Lon adalah SNAPSHOT node_location saat ingest, bukan koordinat node
// sekarang. Replay wajib memakai snapshot ini: node yang pindah atau dihapus
// setelah kejadian tidak boleh mengubah keputusan historis.
type ReplayObservation struct {
	ObservationID     int64
	NodeID            string
	Phase             string
	ProtoVer          *int16
	ObsSeq            *int64
	PGAGal            float64
	DurMs             int64
	PublishTS         int64
	ReceivedTS        int64
	OnsetTS           *int64
	OnsetTSUpperBound *int64
	OnsetTSSource     string
	AttemptNo         *int16
	DetriggerTS       *int64
	Lat               *float64
	Lon               *float64
	VerifyResult      string
}

// ListObservationsForReplay mengembalikan seluruh observasi dengan
// received_ts di dalam [fromTS, toTS] (kedua ujung tertutup), diurutkan
// KANONIK: received_ts lalu observation_id.
//
// Urutan itu DIDEKLARASIKAN, bukan direkonstruksi. Handler MQTT produksi
// berjalan dengan SetOrderMatters(false), sehingga urutan pemrosesan historis
// yang sebenarnya tidak tersimpan di mana pun dan tidak dapat dipulihkan dari
// baris. observation_id (BIGSERIAL) adalah urutan PENULISAN ledger, yang juga
// bukan urutan pemrosesan. Jadi replay memutar satu urutan yang tertentu dan
// dapat diulang, dan divergensi yang muncul karenanya WAJIB dilaporkan, bukan
// disembunyikan.
//
// Batasnya received_ts, bukan publish_ts: publish_ts berasal dari jam node dan
// distempel ulang pada tiap retry, jadi ia bukan sumbu waktu yang monoton di
// sisi server.
func (s *Store) ListObservationsForReplay(ctx context.Context, fromTS, toTS int64) ([]ReplayObservation, error) {
	const q = `
		SELECT observation_id, node_id, phase, proto_ver, obs_seq,
		       pga_gal::double precision AS pga_gal,
		       dur_ms, publish_ts, received_ts,
		       onset_ts, onset_ts_upper_bound, onset_ts_source,
		       attempt_no, detrigger_ts,
		       ST_Y(node_location::geometry) AS lat,
		       ST_X(node_location::geometry) AS lon,
		       verify_result
		FROM sensor_observations
		WHERE received_ts >= $1 AND received_ts <= $2
		ORDER BY received_ts ASC, observation_id ASC`
	rows, err := s.pool.Query(ctx, q, fromTS, toTS)
	if err != nil {
		return nil, fmt.Errorf("query observations for replay: %w", err)
	}
	defer rows.Close()

	out := make([]ReplayObservation, 0, 64)
	for rows.Next() {
		var o ReplayObservation
		if err := rows.Scan(
			&o.ObservationID, &o.NodeID, &o.Phase, &o.ProtoVer, &o.ObsSeq,
			&o.PGAGal, &o.DurMs, &o.PublishTS, &o.ReceivedTS,
			&o.OnsetTS, &o.OnsetTSUpperBound, &o.OnsetTSSource,
			&o.AttemptNo, &o.DetriggerTS,
			&o.Lat, &o.Lon, &o.VerifyResult,
		); err != nil {
			return nil, fmt.Errorf("scan replay observation: %w", err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate replay observations: %w", err)
	}
	return out, nil
}

// ListStateLogForReplay mengembalikan seluruh baris event_state_log dengan
// decided_at di dalam [fromTS, toTS], diurutkan (event_id, revision).
//
// Urutan itu per-event dan disengaja: sweepLocked() mengiterasi map, sehingga
// urutan RESOLVED antar-event di dalam satu tik TIDAK terdefinisi. Satu aliran
// global karena itu tidak dapat dibandingkan; perbandingan hanya sah PER EVENT,
// dan bentuk hasil ini yang memaksa pemanggil melakukannya.
//
// Jendelanya decided_at, bukan started_at induknya: transisi terakhir sebuah
// event (RESOLVED lewat sweep) jatuh sampai ResolveAfterMs + SweepIntervalMs
// SETELAH observasi terakhirnya, jadi jendela log harus lebih panjang daripada
// jendela observasi. Pemanggil yang memilih panjangnya.
//
// AlgoVer dibawa apa adanya supaya pemanggil dapat mengelompokkan menurutnya
// (V5) dan MENOLAK memutar ulang baris yang basis algoritmanya tidak dikenal
// biner ini.
func (s *Store) ListStateLogForReplay(ctx context.Context, fromTS, toTS int64) ([]EventStateLog, error) {
	const q = `
		SELECT event_id, revision, from_state, to_state, reason,
		       decided_at, node_count, independent_cells,
		       peak_pga::double precision AS peak_pga,
		       evidence_summary, algo_ver
		FROM event_state_log
		WHERE decided_at >= $1 AND decided_at <= $2
		ORDER BY event_id ASC, revision ASC`
	rows, err := s.pool.Query(ctx, q, fromTS, toTS)
	if err != nil {
		return nil, fmt.Errorf("query state log for replay: %w", err)
	}
	defer rows.Close()

	out := make([]EventStateLog, 0, 32)
	for rows.Next() {
		var l EventStateLog
		if err := rows.Scan(
			&l.EventID, &l.Revision, &l.FromState, &l.ToState, &l.Reason,
			&l.DecidedAt, &l.NodeCount, &l.IndependentCells, &l.PeakPGA,
			&l.EvidenceSummary, &l.AlgoVer,
		); err != nil {
			return nil, fmt.Errorf("scan replay state log: %w", err)
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate replay state log: %w", err)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Pembacaan untuk penelusuran keterlacakan pemicu (P4-M1′).
//
// Dua query di bawah melengkapi dua lubang yang membuat kriteria P4-M1′ tidak
// dapat dijawab sama sekali sebelum ini:
//
//  1. JENDELA BERBATAS N-BARIS TERAKHIR. Kriterianya menyebut "last N ledger
//     observations, bukan periode kalender tetap", jadi jendela berbasis
//     received_ts (ListObservationsForReplay) tidak dapat memenuhinya: sebuah
//     rentang kalender pada fleet satu-node dapat berisi nol baris atau ribuan,
//     dan keduanya membuat angka hasilnya tidak dapat dibandingkan antar-jalan.
//
//  2. JALUR BACA alert_emissions. Sebelum ini paket ini hanya bisa MENULIS ke
//     tabel itu (InsertAlertEmission); tidak ada satu pun SELECT di seluruh
//     kode. Kaki "advisory WebSocket frame" pada kriteria karena itu durable
//     tetapi tidak terbaca — sebuah baris yang ada tetapi tidak dapat ditanya
//     sama saja dengan bukti yang tidak dimiliki.
//
// Keduanya READ-ONLY dan keduanya TIDAK MENYARING. Penyaringan (lantai PGA,
// verify_result, node_location, tipe frame) dilakukan pemanggil, dengan alasan
// yang sama seperti pada blok replay di atas: jumlah yang tersaring harus dapat
// DILAPORKAN. Query yang menyaring sendiri akan membuat sebuah jendela tampak
// terlacak seluruhnya padahal sebagian masukannya tidak pernah ikut dihitung.
// ---------------------------------------------------------------------------

// ListLastNObservations mengembalikan limit observasi TERBARU, lalu
// mengembalikannya dalam urutan KANONIK naik (received_ts, observation_id).
//
// Dua urutan dalam satu query, dan itu disengaja: batas jendela ditentukan dari
// ujung TERBARU (ORDER BY ... DESC LIMIT), sementara pemanggil membutuhkan baris
// dalam urutan kedatangan supaya jendela yang sama dapat dibandingkan dengan
// jendela riwayat yang juga naik. Membalik di Go akan mengharuskan pemanggil
// mengetahui bahwa urutannya terbalik, dan pengetahuan seperti itu adalah
// tepatnya yang hilang saat sebuah query dipakai ulang setahun kemudian.
//
// Batas berbasis JUMLAH BARIS, bukan waktu. Pada fleet satu-node sebuah jendela
// kalender dapat berisi nol baris (node diam) atau ribuan (satu badai retry),
// jadi "N observasi terakhir" adalah satu-satunya jendela yang membuat dua
// jalan berbeda menghasilkan penyebut yang sebanding.
//
// limit <= 0 dianggap galat, bukan "semua": pembacaan tanpa batas atas pada
// tabel ledger adalah hal yang tumbuh diam-diam sampai ia menjadi masalah
// produksi.
func (s *Store) ListLastNObservations(ctx context.Context, limit int) ([]ReplayObservation, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list last N observations: limit harus > 0, diberi %d", limit)
	}
	const q = `
		SELECT observation_id, node_id, phase, proto_ver, obs_seq,
		       pga_gal::double precision AS pga_gal,
		       dur_ms, publish_ts, received_ts,
		       onset_ts, onset_ts_upper_bound, onset_ts_source,
		       attempt_no, detrigger_ts,
		       ST_Y(node_location::geometry) AS lat,
		       ST_X(node_location::geometry) AS lon,
		       verify_result
		FROM (
			SELECT *
			FROM sensor_observations
			ORDER BY received_ts DESC, observation_id DESC
			LIMIT $1
		) w
		ORDER BY received_ts ASC, observation_id ASC`
	rows, err := s.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("query last N observations: %w", err)
	}
	defer rows.Close()

	out := make([]ReplayObservation, 0, limit)
	for rows.Next() {
		var o ReplayObservation
		if err := rows.Scan(
			&o.ObservationID, &o.NodeID, &o.Phase, &o.ProtoVer, &o.ObsSeq,
			&o.PGAGal, &o.DurMs, &o.PublishTS, &o.ReceivedTS,
			&o.OnsetTS, &o.OnsetTSUpperBound, &o.OnsetTSSource,
			&o.AttemptNo, &o.DetriggerTS,
			&o.Lat, &o.Lon, &o.VerifyResult,
		); err != nil {
			return nil, fmt.Errorf("scan last N observation: %w", err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate last N observations: %w", err)
	}
	return out, nil
}

// TraceEmission adalah satu baris alert_emissions sebagaimana dibaca untuk
// penelusuran P4-M1′. Bukan store.AlertEmission: yang itu adalah bentuk TULIS,
// tidak membawa emission_id, dan membawa kolom (mmi, centroid, fcm_*) yang tidak
// dipakai penelusuran. Bentuk baca yang lebih sempit dipilih supaya tidak ada
// pembaca yang menyangka satu baris hasil penelusuran dapat ditulis kembali.
//
// Pointer pada EventID/EventState/EventRevision/WSClientCount bukan kerapian:
//
//	EventID NULL       — ADVISORY Fase 1 tidak punya identitas event sama sekali.
//	EventState NULL    — baris ditulis SEBELUM migrasi 000008. Ia tetap bukti
//	                     bahwa sebuah frame diputuskan, hanya tanpa state.
//	WSClientCount NULL — hasil pengiriman TIDAK PERNAH DILAPORKAN (000007),
//	                     BUKAN nol klien. Membedakan keduanya adalah seluruh
//	                     alasan kolomnya nullable.
type TraceEmission struct {
	EmissionID    int64
	EventID       *string
	AlertType     string
	Status        string
	Audience      string
	DecidedAt     int64
	AlgoVer       string
	EventState    *string
	EventRevision *int
	WSClientCount *int
}

// ListEmissionsForTrace membaca alert_emissions pada satu jendela decided_at.
//
// Ini SELECT pertama terhadap tabel ini di seluruh kode. Sebelumnya tabel ini
// hanya ditulis (InsertAlertEmission), yang berarti kaki "satu frame advisory
// WebSocket" pada kriteria P4-M1′ durable tetapi tidak dapat ditanya.
//
// TIDAK menyaring alert_type maupun audience: penelusuran perlu MELIHAT baris
// bertetangga (mis. sebuah EARTHQUAKE_ALERT di jendela yang sama) untuk dapat
// mengatakan "tidak ada baris advisory" alih-alih "tidak ada baris apa pun".
// Keduanya kesimpulan yang berbeda.
//
// Jendela di sini berbasis WAKTU, bukan jumlah baris, dan itu benar: batas
// N-baris ditentukan di sisi observasi, lalu diterjemahkan menjadi batas waktu
// oleh pemanggil. Membatasi sisi emisi dengan LIMIT juga akan memotong justru
// baris yang dicari saat sebuah jendela padat.
func (s *Store) ListEmissionsForTrace(ctx context.Context, fromTS, toTS int64) ([]TraceEmission, error) {
	const q = `
		SELECT emission_id, event_id::text, alert_type, status, audience,
		       decided_at, algo_ver, event_state, event_revision, ws_client_count
		FROM alert_emissions
		WHERE decided_at >= $1 AND decided_at <= $2
		ORDER BY decided_at ASC, emission_id ASC`
	rows, err := s.pool.Query(ctx, q, fromTS, toTS)
	if err != nil {
		return nil, fmt.Errorf("query emissions for trace: %w", err)
	}
	defer rows.Close()

	out := make([]TraceEmission, 0, 32)
	for rows.Next() {
		var e TraceEmission
		if err := rows.Scan(
			&e.EmissionID, &e.EventID, &e.AlertType, &e.Status, &e.Audience,
			&e.DecidedAt, &e.AlgoVer, &e.EventState, &e.EventRevision, &e.WSClientCount,
		); err != nil {
			return nil, fmt.Errorf("scan trace emission: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trace emissions: %w", err)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Persistensi durable catatan near-confirmation (P4-M2′, migrasi 000009, D-012).
//
// Satu baris per EVENT yang pernah melampaui ambang independensi. Ia BUKAN baris
// riwayat transisi, dan itu sebabnya ia bukan baris event_state_log: sebuah
// persilangan ambang dapat terjadi TANPA transisi state sama sekali —
// UNCONFIRMED -> UNCONFIRMED ilegal (§5.2), sehingga kontributor independen
// kedua yang tiba pada event yang sudah UNCONFIRMED tidak menaikkan revisi,
// tidak menghasilkan baris riwayat, dan tidak menghasilkan frame. Persilangan
// sunyi itu tepat kasus yang paling umum pada fleet kecil, dan sebelum P4-M2′ ia
// hanya ada di sebuah map yang lenyap bersama prosesnya.
//
// Penulisnya menempuh antrean ledger yang sama dengan satuan event: asinkron,
// berbatas, dan boleh membuang yang tertua (D17/D-002/S1). Karena itu ia tunduk
// pada aturan yang sama seperti UpsertEvent — dua penulisan untuk satu event
// harus aman dalam URUTAN APA PUN — dan itulah yang dijawab bentuk ON CONFLICT
// di bawah.

// NearConfirmedRow adalah satu baris event_near_confirmed.
//
// ConfirmedAt, TerminalState dan TerminalAt bertipe pointer karena kolomnya boleh
// NULL, dan NULL di sini punya arti yang sempit: BELUM PERNAH TERJADI, bukan nol.
// Sebuah event yang tidak pernah CONFIRMED bukan event yang CONFIRMED pada epoch,
// dan perbedaan itu harus utuh sampai ke pemanggil.
//
// MinIndependentCells adalah ambang yang BERLAKU saat persilangan, dibawa apa
// adanya. Tanpanya "mencapai 2" tidak dapat ditafsirkan oleh pembaca yang
// MIN_INDEPENDENT_CELLS-nya sudah berbeda — dan menghitungnya ulang dari
// konfigurasi sekarang berarti menilai keputusan lampau dengan parameter yang
// tidak menghasilkannya.
type NearConfirmedRow struct {
	EventID                string
	FirstTwoIndependentAt  int64 // ms epoch UTC, jam server
	IndependentCountAtPeak int
	NodeCountAtPeak        int
	MinIndependentCells    int
	ConfirmedAt            *int64
	TerminalState          *string
	TerminalAt             *int64
	AlgoVer                string
}

// UpsertNearConfirmed menulis atau menggabungkan satu baris event_near_confirmed.
//
// ON CONFLICT-nya adalah penggabungan MONOTON, bukan penimpaan, dan itu bukan
// selera: satuan-satuannya diantre dan boleh dibuang, jadi baris ini dapat
// menerima pembaruan yang tiba TERLAMBAT atau TIDAK URUT, atau kehilangan salah
// satu pembaruan sama sekali. Setiap kolom karena itu digabung dengan aturan yang
// hasilnya tidak bergantung pada urutan kedatangan:
//
//	first_two_independent_at  LEAST     — "pertama kali" hanya bisa bergerak MAJU
//	                                      ke masa lalu; kedatangan yang lebih tua
//	                                      lebih dekat pada kebenaran.
//	independent_count_at_peak GREATEST  — PUNCAK, jadi ia tidak pernah turun.
//	                                      Independensi boleh turun setelah
//	                                      invalidasi kontributor; puncaknya tidak.
//	node_count_at_peak        mengikuti  — bergerak HANYA bersama puncak yang baru.
//	                                      Tiga node di satu atap bukan tiga bukti,
//	                                      jadi kedua angka hanya bermakna
//	                                      berpasangan dan tidak boleh berasal dari
//	                                      dua saat yang berbeda.
//	confirmed_at              COALESCE  — yang pertama non-NULL menang, cermin dari
//	                                      penjaga `== 0` di memori.
//	terminal_state/terminal_at COALESCE — keduanya bergerak BERSAMA; sebuah state
//	                                      terminal tanpa waktunya tidak dapat
//	                                      ditafsirkan.
//	min_independent_cells     yang ada  — parameter saat persilangan, tidak pernah
//	algo_ver                  yang ada    ditulis ulang (V3/V6, D-006). Ditulis
//	                                      eksplisit alih-alih dihilangkan supaya
//	                                      "yang pertama menang" terbaca di kueri.
func (s *Store) UpsertNearConfirmed(ctx context.Context, r *NearConfirmedRow) error {
	const q = `
		INSERT INTO event_near_confirmed (
			event_id, first_two_independent_at,
			independent_count_at_peak, node_count_at_peak,
			min_independent_cells,
			confirmed_at, terminal_state, terminal_at, algo_ver
		) VALUES ($1::uuid, $2, $3, $4, $5, $6, NULLIF($7,''), $8, $9)
		ON CONFLICT (event_id) DO UPDATE SET
			first_two_independent_at =
				LEAST(event_near_confirmed.first_two_independent_at,
				      EXCLUDED.first_two_independent_at),
			independent_count_at_peak =
				GREATEST(event_near_confirmed.independent_count_at_peak,
				         EXCLUDED.independent_count_at_peak),
			node_count_at_peak = CASE
				WHEN EXCLUDED.independent_count_at_peak
				     > event_near_confirmed.independent_count_at_peak
				THEN EXCLUDED.node_count_at_peak
				ELSE event_near_confirmed.node_count_at_peak
			END,
			min_independent_cells = event_near_confirmed.min_independent_cells,
			confirmed_at   = COALESCE(event_near_confirmed.confirmed_at,   EXCLUDED.confirmed_at),
			terminal_state = COALESCE(event_near_confirmed.terminal_state, EXCLUDED.terminal_state),
			terminal_at    = COALESCE(event_near_confirmed.terminal_at,    EXCLUDED.terminal_at),
			algo_ver = event_near_confirmed.algo_ver`
	if _, err := s.pool.Exec(ctx, q,
		r.EventID, r.FirstTwoIndependentAt,
		r.IndependentCountAtPeak, r.NodeCountAtPeak,
		r.MinIndependentCells,
		r.ConfirmedAt, derefString(r.TerminalState), r.TerminalAt, r.AlgoVer,
	); err != nil {
		return fmt.Errorf("upsert event_near_confirmed: %w", err)
	}
	return nil
}

// ListNearConfirmed mengembalikan SELURUH baris event_near_confirmed, diurutkan
// (first_two_independent_at, event_id).
//
// Tanpa jendela dan tanpa batas, dan keduanya disengaja. Jendela waktu akan
// mengalahkan maksud tabel ini: pertanyaannya "apakah pernah ada persilangan",
// dan sebuah jawaban yang dipotong pada 24 jam terakhir tidak dapat membedakan
// "tidak pernah ada" dari "ada, lebih lama dari itu". Batas baris akan melakukan
// hal yang sama secara diam-diam. Kardinalitasnya adalah jumlah event yang PERNAH
// melampaui ambang independensi — nol pada fleet satu-node, dan pada fleet mana
// pun beberapa urutan besaran lebih kecil daripada ledger observasi — dan ia
// dibaca SEKALI saat boot, bukan per permintaan.
//
// Urutannya sama dengan urutan yang dipakai Tracker di memori, sehingga sebuah
// jawaban yang dibangun ulang dari basis data tidak dapat dibedakan dari jawaban
// yang lahir di proses ini KECUALI oleh field provenance-nya — yang memang harus
// menjadi satu-satunya perbedaan yang terlihat.
func (s *Store) ListNearConfirmed(ctx context.Context) ([]NearConfirmedRow, error) {
	const q = `
		SELECT event_id, first_two_independent_at,
		       independent_count_at_peak, node_count_at_peak,
		       min_independent_cells,
		       confirmed_at, terminal_state, terminal_at, algo_ver
		FROM event_near_confirmed
		ORDER BY first_two_independent_at ASC, event_id ASC`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query event_near_confirmed: %w", err)
	}
	defer rows.Close()

	out := make([]NearConfirmedRow, 0, 32)
	for rows.Next() {
		var r NearConfirmedRow
		if err := rows.Scan(
			&r.EventID, &r.FirstTwoIndependentAt,
			&r.IndependentCountAtPeak, &r.NodeCountAtPeak,
			&r.MinIndependentCells,
			&r.ConfirmedAt, &r.TerminalState, &r.TerminalAt, &r.AlgoVer,
		); err != nil {
			return nil, fmt.Errorf("scan event_near_confirmed: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate event_near_confirmed: %w", err)
	}
	return out, nil
}

// derefString meratakan *string menjadi string kosong supaya NULLIF di kueri yang
// memutuskan NULL, bukan dua cabang parameter di Go. Pola yang sama dengan
// NULLIF($11,”) pada UpsertEvent.
func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
