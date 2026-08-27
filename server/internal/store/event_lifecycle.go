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
