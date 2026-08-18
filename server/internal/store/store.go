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

// --- REST API repositories (Fase 4) ---

// NewNode adalah data untuk membuat node baru saat provisioning. Secret HMAC
// disimpan terenkripsi AES-256-GCM (secret_key_enc + nonce), BUKAN hash.
type NewNode struct {
	StationID    string
	SensorModel  string
	LocationName string
	Lat          float64
	Lon          float64
	SecretEnc    []byte
	SecretNonce  []byte
}

// CreateNode menyisipkan node baru. location dibangun via ST_MakePoint(lon, lat).
func (s *Store) CreateNode(ctx context.Context, n *NewNode) error {
	const q = `
		INSERT INTO iot_nodes (
			station_id, sensor_model, location_name, location,
			secret_key_enc, secret_key_nonce
		) VALUES (
			$1, $2, $3,
			ST_SetSRID(ST_MakePoint($4, $5), 4326)::geography,
			$6, $7
		)`
	_, err := s.pool.Exec(ctx, q,
		n.StationID, n.SensorModel, n.LocationName,
		n.Lon, n.Lat, n.SecretEnc, n.SecretNonce,
	)
	if err != nil {
		return fmt.Errorf("insert iot_node: %w", err)
	}
	return nil
}

// SensorStatus merepresentasikan satu baris status sensor untuk endpoint /sensors.
type SensorStatus struct {
	StationID     string
	SensorModel   string
	LocationName  string
	Lat           float64
	Lon           float64
	IsActive      bool
	LastRSSI      int
	LastLatencyMs int
	// SecondsSincePing = detik sejak last_heartbeat; dipakai untuk status
	// Online/Offline & label "Ns ago".
	SecondsSincePing int64
}

// ListSensorsWithin mengembalikan sensor dalam radius rangeKm dari (lat, lon)
// user memakai ST_DWithin pada GEOGRAPHY (satuan meter → rangeKm * 1000).
// Diurutkan dari yang terdekat.
func (s *Store) ListSensorsWithin(ctx context.Context, lat, lon float64, rangeKm int) ([]SensorStatus, error) {
	const q = `
		SELECT station_id, sensor_model, location_name,
		       ST_Y(location::geometry) AS lat,
		       ST_X(location::geometry) AS lon,
		       is_active, last_rssi, last_latency_ms,
		       EXTRACT(EPOCH FROM (NOW() - last_heartbeat))::bigint AS secs_since_ping
		FROM iot_nodes
		WHERE ST_DWithin(location, ST_SetSRID(ST_MakePoint($2, $1), 4326)::geography, $3)
		ORDER BY location <-> ST_SetSRID(ST_MakePoint($2, $1), 4326)::geography`
	rows, err := s.pool.Query(ctx, q, lat, lon, float64(rangeKm)*1000.0)
	if err != nil {
		return nil, fmt.Errorf("query sensors: %w", err)
	}
	defer rows.Close()

	var out []SensorStatus
	for rows.Next() {
		var s SensorStatus
		if err := rows.Scan(
			&s.StationID, &s.SensorModel, &s.LocationName,
			&s.Lat, &s.Lon, &s.IsActive, &s.LastRSSI, &s.LastLatencyMs,
			&s.SecondsSincePing,
		); err != nil {
			return nil, fmt.Errorf("scan sensor: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sensors: %w", err)
	}
	return out, nil
}

// ErrUserNotFound dikembalikan bila user_id tidak ada di user_profiles.
var ErrUserNotFound = errors.New("user tidak ditemukan")

// UserLocation memuat lokasi terakhir user + radius coverage default.
// HasLocation=false bila last_location NULL (user belum pernah kirim lokasi).
type UserLocation struct {
	UserID           string
	Lat              float64
	Lon              float64
	HasLocation      bool
	CoverageRadiusKm int
}

// GetUserLocation mengambil last_location + coverage_radius_km user.
func (s *Store) GetUserLocation(ctx context.Context, userID string) (*UserLocation, error) {
	const q = `
		SELECT ST_Y(last_location::geometry) AS lat,
		       ST_X(last_location::geometry) AS lon,
		       last_location IS NOT NULL AS has_loc,
		       coverage_radius_km
		FROM user_profiles
		WHERE user_id = $1`
	var (
		lat, lon    *float64
		hasLoc      bool
		coverageKm  int
	)
	err := s.pool.QueryRow(ctx, q, userID).Scan(&lat, &lon, &hasLoc, &coverageKm)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan user location: %w", err)
	}
	u := &UserLocation{UserID: userID, HasLocation: hasLoc, CoverageRadiusKm: coverageKm}
	if hasLoc && lat != nil && lon != nil {
		u.Lat, u.Lon = *lat, *lon
	}
	return u, nil
}


// UpdatePseudonym menyetel pseudonym baru untuk user dan mengembalikan waktu
// last_active yang di-set NOW(). Mengembalikan ErrUserNotFound bila user absen.
func (s *Store) UpdatePseudonym(ctx context.Context, userID, pseudonym string) (time.Time, error) {
	const q = `
		UPDATE user_profiles
		SET pseudonym = $2, last_active = NOW()
		WHERE user_id = $1
		RETURNING last_active`
	var updatedAt time.Time
	err := s.pool.QueryRow(ctx, q, userID, pseudonym).Scan(&updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, ErrUserNotFound
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("update pseudonym: %w", err)
	}
	return updatedAt, nil
}


