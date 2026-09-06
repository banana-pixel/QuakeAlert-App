package store

// --- Pembacaan HANYA-BACA per-event_id untuk garis waktu forensik (P4-M6′, D-015) ---
//
// Tiga pembaca, semuanya SELECT, semuanya bertumpu pada satu event_id. Berkas
// TERPISAH dari event_lifecycle.go dan itu disengaja: pembaca berjendela di sana
// (ListObservationsForReplay, ListStateLogForReplay, ListEmissionsForTrace,
// ListLastNObservations, ListNearConfirmed) adalah bagian dari jalur M1′/M2′ yang
// sudah divalidasi pemilik, dan D-015 mengizinkan MEMAKAI-nya, bukan
// mengubahnya. Menaruh yang baru di berkas sendiri membuat berkas lama tidak
// tersentuh satu bita pun.
//
// Tidak ada pembaca di sini yang menyentuh event_near_confirmed (migrasi 000009,
// TIDAK terpasang pada schema_version = 8). Bukan didegradasi — memang tidak
// pernah dibaca, sehingga tidak ada keluaran wajib M6′ yang dapat terhalang
// olehnya (D-015 batasan 4).

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// EventByID mengembalikan SATU baris earthquake_events menurut event_id.
//
// ErrEventNotFound dipakai ulang di sini — komentarnya di store.go menyebut
// ResolveEvent karena itu pemanggil pertamanya, tetapi artinya sama persis:
// tidak ada baris dengan event_id itu.
//
// TIDAK memakai LEFT JOIN LATERAL seperti LoadOpenEvents, dan itu bukan
// kelalaian: LoadOpenEvents butuh revisi TERAKHIR karena ia merekonstruksi
// Tracker, sedangkan M6′ membaca SELURUH riwayat sebagai keluaran kedua yang
// berdiri sendiri. Mengisi LatestEvidence di sini akan menyalin satu revisi ke
// tempat kedua dan mengundang pembaca menyangka itu ringkasan seluruh riwayat.
// Karena itu LatestEvidence tetap nil dan LatestDecidedAt tetap 0.
//
// started_at di-COALESCE ke 0: kolomnya NULLABLE (000001, DEFAULT NOW() tanpa
// NOT NULL), dan sebuah alat forensik tidak boleh mati pada baris yang justru
// paling perlu dilihat. 0 berarti TIDAK DIKETAHUI, bukan epoch.
func (s *Store) EventByID(ctx context.Context, eventID string) (*EarthquakeEvent, error) {
	const q = `
		SELECT event_id,
		       COALESCE(status, ''),
		       ST_Y(estimated_centroid::geometry) AS lat,
		       ST_X(estimated_centroid::geometry) AS lon,
		       location_name, mmi_scale, intensity_label,
		       max_pga::double precision AS max_pga,
		       triggered_nodes_count,
		       COALESCE((EXTRACT(EPOCH FROM started_at) * 1000)::bigint, 0) AS started_at_ms,
		       COALESCE(event_state, ''),
		       revision,
		       COALESCE(origin_ts, 0),
		       COALESCE(origin_ts_source, ''),
		       COALESCE(independent_cell_count, 0),
		       COALESCE(algo_ver, '')
		FROM earthquake_events
		WHERE event_id = $1::uuid`

	var e EarthquakeEvent
	err := s.pool.QueryRow(ctx, q, eventID).Scan(
		&e.EventID, &e.Status, &e.CentroidLat, &e.CentroidLon,
		&e.LocationName, &e.MMIScale, &e.IntensityLabel,
		&e.MaxPGA, &e.TriggeredNodes, &e.StartedAtMs,
		&e.EventState, &e.Revision, &e.OriginTS, &e.OriginTSSource,
		&e.IndependentCellCount, &e.AlgoVer,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEventNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan event by id: %w", err)
	}
	return &e, nil
}

// ListStateLogForEvent mengembalikan SELURUH baris event_state_log milik satu
// event, diurutkan revision ASC.
//
// Tanpa jendela waktu dan tanpa LIMIT, dan keduanya disengaja. Pertanyaan M6′
// adalah "seluruh riwayat event ini"; sebuah jawaban yang dipotong tidak dapat
// membedakan "revisi itu tidak pernah ada" dari "revisi itu di luar potongan" —
// dan perbedaan itu tepatnya yang dibayar dengan kesimpulan yang salah.
// Kardinalitasnya adalah jumlah transisi satu event, satuan, bukan ribuan.
//
// ORDER BY revision, bukan decided_at: revision adalah urutan KEPUTUSAN dan
// unik per event menurut UNIQUE (event_id, revision), sementara dua transisi
// dapat berbagi milidetik yang sama. Indeks utamanya sudah menaruh event_id di
// depan, jadi ini pembacaan berjangkauan, bukan pemindaian.
func (s *Store) ListStateLogForEvent(ctx context.Context, eventID string) ([]EventStateLog, error) {
	const q = `
		SELECT event_id, revision, from_state, to_state, reason,
		       decided_at, node_count, independent_cells,
		       peak_pga::double precision AS peak_pga,
		       evidence_summary, algo_ver
		FROM event_state_log
		WHERE event_id = $1::uuid
		ORDER BY revision ASC`
	rows, err := s.pool.Query(ctx, q, eventID)
	if err != nil {
		return nil, fmt.Errorf("query state log for event: %w", err)
	}
	defer rows.Close()

	out := make([]EventStateLog, 0, 8)
	for rows.Next() {
		var l EventStateLog
		if err := rows.Scan(
			&l.EventID, &l.Revision, &l.FromState, &l.ToState, &l.Reason,
			&l.DecidedAt, &l.NodeCount, &l.IndependentCells, &l.PeakPGA,
			&l.EvidenceSummary, &l.AlgoVer,
		); err != nil {
			return nil, fmt.Errorf("scan state log for event: %w", err)
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate state log for event: %w", err)
	}
	return out, nil
}

// ListObservationsForNodesInWindow mengembalikan observasi milik HIMPUNAN node
// tertentu dengan received_ts di dalam [fromTS, toTS] (kedua ujung tertutup),
// diurutkan KANONIK: received_ts lalu observation_id.
//
// Penyaringnya (node_id, received_ts) dan hanya itu — inilah relasi
// KEANGGOTAAN-DAN-WAKTU dalam bentuk kueri. node_id berasal dari
// evidence_summary.contributors[], satu-satunya jalan pulang yang dimiliki skema
// (D12, tidak ada observation_id di event_state_log, tidak ada FK). Kueri ini
// karena itu mengembalikan KANDIDAT, bukan sebab.
//
// verify_result dan node_location TIDAK disaring di sini, mengikuti alasan yang
// sama dengan ListObservationsForReplay: pemanggil harus dapat MENGHITUNG baris
// yang ia buang. Kueri yang menyaring sendiri membuat laporan tampak lengkap
// padahal masukannya tidak.
//
// Himpunan node kosong mengembalikan nol baris tanpa galat: sebuah event yang
// seluruh evidence_summary-nya tak terbaca memang tidak punya kandidat, dan itu
// hasil yang benar untuk dilaporkan, bukan galat untuk dimatikan.
func (s *Store) ListObservationsForNodesInWindow(
	ctx context.Context, nodeIDs []string, fromTS, toTS int64,
) ([]ReplayObservation, error) {
	if fromTS > toTS {
		return nil, fmt.Errorf("list observations for nodes: jendela terbalik, fromTS=%d > toTS=%d", fromTS, toTS)
	}
	out := make([]ReplayObservation, 0, 32)
	if len(nodeIDs) == 0 {
		return out, nil
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
		FROM sensor_observations
		WHERE node_id = ANY($1) AND received_ts >= $2 AND received_ts <= $3
		ORDER BY received_ts ASC, observation_id ASC`
	rows, err := s.pool.Query(ctx, q, nodeIDs, fromTS, toTS)
	if err != nil {
		return nil, fmt.Errorf("query observations for nodes: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var o ReplayObservation
		if err := rows.Scan(
			&o.ObservationID, &o.NodeID, &o.Phase, &o.ProtoVer, &o.ObsSeq,
			&o.PGAGal, &o.DurMs, &o.PublishTS, &o.ReceivedTS,
			&o.OnsetTS, &o.OnsetTSUpperBound, &o.OnsetTSSource,
			&o.AttemptNo, &o.DetriggerTS,
			&o.Lat, &o.Lon, &o.VerifyResult,
		); err != nil {
			return nil, fmt.Errorf("scan observation for nodes: %w", err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate observations for nodes: %w", err)
	}
	return out, nil
}
