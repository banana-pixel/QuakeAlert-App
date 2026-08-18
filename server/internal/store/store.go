// Package store menyediakan repository berbasis pgx/v5 (tanpa ORM, ADR-0002).
// Semua query memakai prepared statement implisit pgx + context timeout.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool config sesuai .clinerules/10 #1.
const (
	maxConns        = 8
	minConns        = 2
	maxConnIdleTime = 5 * time.Minute
)

// Store membungkus pool koneksi pgx.
type Store struct {
	pool *pgxpool.Pool
}

// New membuat pool pgx tervalidasi dari DATABASE_URL.
func New(ctx context.Context, databaseURL string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.MaxConns = maxConns
	cfg.MinConns = minConns
	cfg.MaxConnIdleTime = maxConnIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("buat pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close menutup pool (dipanggil saat graceful shutdown).
func (s *Store) Close() { s.pool.Close() }

// NodeSecret memuat data yang dibutuhkan untuk verifikasi HMAC & anti-replay.
type NodeSecret struct {
	StationID   string
	SecretEnc   []byte
	SecretNonce []byte
	LastSeenTS  int64
	IsActive    bool
}

var ErrNodeNotFound = errors.New("node tidak ditemukan")

// GetNodeSecret mengambil secret terenkripsi + last_seen_ts untuk sebuah node.
func (s *Store) GetNodeSecret(ctx context.Context, stationID string) (*NodeSecret, error) {
	const q = `
		SELECT station_id, secret_key_enc, secret_key_nonce, last_seen_ts, is_active
		FROM iot_nodes
		WHERE station_id = $1`
	row := s.pool.QueryRow(ctx, q, stationID)

	var n NodeSecret
	err := row.Scan(&n.StationID, &n.SecretEnc, &n.SecretNonce, &n.LastSeenTS, &n.IsActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNodeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan node secret: %w", err)
	}
	return &n, nil
}

// UpdateLastSeen memajukan last_seen_ts secara atomik HANYA jika ts lebih baru
// (anti-replay pada level DB). Mengembalikan true bila baris ter-update.
func (s *Store) UpdateLastSeen(ctx context.Context, stationID string, ts int64) (bool, error) {
	const q = `
		UPDATE iot_nodes
		SET last_seen_ts = $2, last_heartbeat = NOW()
		WHERE station_id = $1 AND last_seen_ts < $2`
	tag, err := s.pool.Exec(ctx, q, stationID, ts)
	if err != nil {
		return false, fmt.Errorf("update last_seen: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// NodeLocation adalah koordinat + nama lokasi sebuah node, dipakai konsensus
// untuk pengelompokan spasial & weighted centroid.
type NodeLocation struct {
	StationID    string
	Lat          float64
	Lon          float64
	LocationName string
}

// GetNodeLocation mengambil koordinat node dari kolom GEOGRAPHY(Point,4326).
// ST_Y = latitude, ST_X = longitude (urutan lon/lat pada tipe geometry).
func (s *Store) GetNodeLocation(ctx context.Context, stationID string) (*NodeLocation, error) {
	const q = `
		SELECT station_id,
		       ST_Y(location::geometry) AS lat,
		       ST_X(location::geometry) AS lon,
		       location_name
		FROM iot_nodes
		WHERE station_id = $1`
	row := s.pool.QueryRow(ctx, q, stationID)

	var n NodeLocation
	err := row.Scan(&n.StationID, &n.Lat, &n.Lon, &n.LocationName)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNodeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan node location: %w", err)
	}
	return &n, nil
}

// EarthquakeEvent adalah data event yang akan dipersistensikan ke
// earthquake_events setelah konsensus CONFIRMED.
type EarthquakeEvent struct {
	EventID          string
	Status           string  // 'HAPPENING' | 'RESOLVED'
	CentroidLat      float64 // estimated_centroid (BUKAN episenter)
	CentroidLon      float64
	LocationName     string
	MMIScale         string
	IntensityLabel   string
	MaxPGA           float64 // gal (satuan kanonik)
	TriggeredNodes   int
	StartedAtMs      int64 // ms epoch UTC
}

// SaveEvent menyimpan event gempa ke earthquake_events dan mengembalikan
// event_id yang di-generate DB. estimated_centroid dibangun via ST_MakePoint
// (lon, lat) lalu di-cast ke GEOGRAPHY(4326).
func (s *Store) SaveEvent(ctx context.Context, e *EarthquakeEvent) (string, error) {
	const q = `
		INSERT INTO earthquake_events (
			status, estimated_centroid, location_name,
			mmi_scale, intensity_label, max_pga,
			triggered_nodes_count, started_at
		) VALUES (
			$1,
			ST_SetSRID(ST_MakePoint($2, $3), 4326)::geography,
			$4, $5, $6, $7,
			to_timestamp($8::double precision / 1000.0)
		)
		RETURNING event_id`
	var id string
	err := s.pool.QueryRow(ctx, q,
		e.Status, e.CentroidLon, e.CentroidLat, e.LocationName,
		e.MMIScale, e.IntensityLabel, e.MaxPGA,
		e.TriggeredNodes, e.StartedAtMs,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert earthquake_event: %w", err)
	}
	return id, nil
}

