// Package store menyediakan repository berbasis pgx/v5 (tanpa ORM, ADR-0002).
// Semua query memakai prepared statement implisit pgx + context timeout.
package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pgUniqueViolation adalah SQLSTATE unique_violation Postgres. Dipakai untuk
// membedakan "sudah ada" (konflik klien, 409) dari kegagalan server (500).
const pgUniqueViolation = "23505"

// isUniqueViolation melaporkan apakah err berasal dari pelanggaran UNIQUE /
// PRIMARY KEY di Postgres.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation
}

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

// UpdateHeartbeat memutakhirkan telemetri liveness node dari heartbeat MQTT:
// last_rssi (dBm), last_latency_ms (ms), dan last_heartbeat = NOW() yang dipakai
// endpoint /sensors untuk menentukan Online/Offline serta label "Ns ago".
// Sengaja TIDAK menyentuh last_seen_ts: kolom itu milik anti-replay trigger
// ber-HMAC, sedangkan heartbeat tidak terautentikasi.
// Mengembalikan false bila station_id tidak dikenal (0 baris ter-update).
func (s *Store) UpdateHeartbeat(ctx context.Context, stationID string, rssi, latencyMs int) (bool, error) {
	const q = `
		UPDATE iot_nodes
		SET last_rssi = $2, last_latency_ms = $3, last_heartbeat = NOW()
		WHERE station_id = $1`
	tag, err := s.pool.Exec(ctx, q, stationID, rssi, latencyMs)
	if err != nil {
		return false, fmt.Errorf("update heartbeat: %w", err)
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
	EventID        string
	Status         string  // 'HAPPENING' | 'RESOLVED'
	CentroidLat    float64 // estimated_centroid (BUKAN episenter)
	CentroidLon    float64
	LocationName   string
	MMIScale       string
	IntensityLabel string
	MaxPGA         float64 // gal (satuan kanonik)
	TriggeredNodes int
	StartedAtMs    int64 // ms epoch UTC
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
			$4, $5, $6, $7, $8,
			to_timestamp($9::double precision / 1000.0)
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

// ErrEventNotFound dikembalikan ResolveEvent bila event_id tidak ada atau sudah
// bukan berstatus HAPPENING (idempoten: tidak mengubah event yang sudah RESOLVED).
var ErrEventNotFound = errors.New("event tidak ditemukan")

// ResolveEvent menandai event CONFIRMED menjadi RESOLVED (all-clear) dan
// mencatat resolved_at = NOW(). Hanya baris berstatus HAPPENING yang di-update,
// sehingga pemanggilan ganda (timer ganda) bersifat idempoten.
func (s *Store) ResolveEvent(ctx context.Context, eventID string) error {
	const q = `
		UPDATE earthquake_events
		SET status = 'RESOLVED', resolved_at = NOW()
		WHERE event_id = $1 AND status = 'HAPPENING'`
	tag, err := s.pool.Exec(ctx, q, eventID)
	if err != nil {
		return fmt.Errorf("resolve event: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrEventNotFound
	}
	return nil
}

// ResolveStaleEvents menandai seluruh event HAPPENING yang mulai sebelum `before`
// menjadi RESOLVED. Dipakai saat startup untuk merekonsiliasi event yang masih
// aktif ketika proses mati sebelum state machine resolusi (in-memory di
// dispatcher) sempat mengeksekusi. Mengembalikan jumlah baris yang ter-update.
func (s *Store) ResolveStaleEvents(ctx context.Context, before time.Time) (int64, error) {
	const q = `
		UPDATE earthquake_events
		SET status = 'RESOLVED', resolved_at = NOW()
		WHERE status = 'HAPPENING' AND started_at < $1`
	tag, err := s.pool.Exec(ctx, q, before)
	if err != nil {
		return 0, fmt.Errorf("resolve stale events: %w", err)
	}
	return tag.RowsAffected(), nil
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

// ErrNodeAlreadyExists dikembalikan CreateNode bila station_id sudah terdaftar
// (PRIMARY KEY iot_nodes.station_id). Node yang sudah pernah di-provision harus
// memakai secret di NVS-nya, bukan meminta yang baru — API memetakan ini ke
// HTTP 409 alih-alih 500.
var ErrNodeAlreadyExists = errors.New("station_id sudah terdaftar")

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
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: %s", ErrNodeAlreadyExists, n.StationID)
		}
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
		lat, lon   *float64
		hasLoc     bool
		coverageKm int
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

// --- Android-facing repositories (auth anonim, lokasi, FCM, riwayat event) ---

// ErrUserAlreadyExists dikembalikan CreateUserProfile bila user_id sudah dipakai
// (tabrakan UUID v4 — praktis mustahil, tetapi tetap ditangani eksplisit alih-alih
// dibiarkan menjadi 500 yang membingungkan).
var ErrUserAlreadyExists = errors.New("user_id sudah terdaftar")

// CreateUserProfile menyisipkan profil anonim baru dan mengembalikan created_at
// yang di-set DB. userID harus UUID v4 yang di-generate caller agar dapat langsung
// dipakai sebagai klaim `sub` JWT tanpa round-trip kedua.
func (s *Store) CreateUserProfile(ctx context.Context, userID, pseudonym string) (time.Time, error) {
	const q = `
		INSERT INTO user_profiles (user_id, pseudonym)
		VALUES ($1, $2)
		RETURNING created_at`
	var createdAt time.Time
	if err := s.pool.QueryRow(ctx, q, userID, pseudonym).Scan(&createdAt); err != nil {
		if isUniqueViolation(err) {
			return time.Time{}, fmt.Errorf("%w: %s", ErrUserAlreadyExists, userID)
		}
		return time.Time{}, fmt.Errorf("insert user_profile: %w", err)
	}
	return createdAt, nil
}

// DefaultCoverageRadiusKm mencerminkan DEFAULT kolom
// user_profiles.coverage_radius_km (migrasi 000001). Dipakai sebagai nilai
// pengganti saat kolom masih NULL — baris lama yang dibuat sebelum klien
// menyinkronkan radius.
const DefaultCoverageRadiusKm = 50

// UpdateUserLocation menyetel last_location (GEOGRAPHY(Point,4326)) user dari
// (lat, lon) — perhatikan ST_MakePoint memakai urutan (lon, lat) — beserta label
// lokasi opsional dan radius coverage pilihan user, lalu mengembalikan
// last_active yang di-set NOW() bersama coverage_radius_km yang BERLAKU setelah
// update (nilai baru bila dikirim, nilai tersimpan bila tidak).
//
// Semantik PUT (replace): locationName kosong disimpan sebagai NULL, jadi klien
// yang mengirim body tanpa location_name memang MENGOSONGKAN label lama.
//
// coverageRadiusKm justru TIDAK ikut semantik replace itu: nil berarti
// "jangan ubah", bukan "kosongkan". Radius adalah preferensi yang dipakai
// dispatch untuk memutuskan apakah sebuah gempa perlu membangunkan perangkat
// ini — mengosongkannya karena klien versi lama tidak mengirim field baru akan
// mengubah jangkauan alert seseorang tanpa ia meminta.
// Mengembalikan ErrUserNotFound bila user_id tidak ada.
func (s *Store) UpdateUserLocation(ctx context.Context, userID string, lat, lon float64, locationName string, coverageRadiusKm *int) (time.Time, int, error) {
	const q = `
		UPDATE user_profiles
		SET last_location = ST_SetSRID(ST_MakePoint($2, $3), 4326)::geography,
		    location_name = NULLIF($4, ''),
		    coverage_radius_km = COALESCE($5, coverage_radius_km, $6),
		    last_active   = NOW()
		WHERE user_id = $1
		RETURNING last_active, coverage_radius_km`
	var (
		updatedAt time.Time
		radiusKm  int
	)
	err := s.pool.QueryRow(ctx, q, userID, lon, lat, locationName, coverageRadiusKm, DefaultCoverageRadiusKm).
		Scan(&updatedAt, &radiusKm)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, 0, ErrUserNotFound
	}
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("update user location: %w", err)
	}
	return updatedAt, radiusKm, nil
}

// UpdateUserFCMToken menyimpan registration token FCM perangkat user (dipakai
// dispatch untuk delivery background) dan mengembalikan last_active baru.
// Mengembalikan ErrUserNotFound bila user_id tidak ada.
func (s *Store) UpdateUserFCMToken(ctx context.Context, userID, token string) (time.Time, error) {
	const q = `
		UPDATE user_profiles
		SET fcm_token = $2, last_active = NOW()
		WHERE user_id = $1
		RETURNING last_active`
	var updatedAt time.Time
	err := s.pool.QueryRow(ctx, q, userID, token).Scan(&updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, ErrUserNotFound
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("update fcm token: %w", err)
	}
	return updatedAt, nil
}

// maxFCMTokensPerEvent membatasi jumlah token yang dikembalikan satu query
// dispatch. FCM HTTP v1 mengirim satu request per token, jadi batas ini menjaga
// fan-out satu gempa tetap terikat; sisanya tetap terjangkau lewat topic
// broadcast yang dipakai dispatch sebagai fallback.
const maxFCMTokensPerEvent = 2000

// fcmTokenMaxIdle membuang token instalasi mati: perangkat yang tidak pernah
// menyentuh API selama ini hampir pasti sudah uninstall, dan token-nya hanya
// menghasilkan UNREGISTERED dari FCM.
const fcmTokenMaxIdle = 60 // hari

// FCMTokensWithin mengembalikan registration token FCM milik user yang perlu
// dibangunkan oleh sebuah gempa di (lat, lon) — dasar delivery bertarget,
// menggantikan broadcast nasional ke satu topic.
//
// Dua radius bekerja bersamaan, dan keduanya perlu:
//
//   - rangeKm adalah BATAS ATAS dispatch, bukan radius alert. Ia satu-satunya
//     yang masuk ST_DWithin sehingga indeks GiST idx_users_spatial memangkas
//     tabel sebelum jarak per-baris dihitung.
//   - coverage_radius_km adalah radius pilihan user (slider Settings, kini
//     disinkronkan klien lewat PUT /users/location). Inilah yang menentukan
//     apakah perangkat ini benar-benar ingin dibangunkan: dulu semua yang
//     berada dalam rangeKm dikirimi notifikasi lalu di-drop oleh gate Haversine
//     di klien, artinya sebuah gempa 340 km jauhnya tetap menyalakan layar
//     seseorang yang memilih radius 50 km.
//
// Radius efektif dijepit ke [1, rangeKm]: NULL/0 dari baris lama jatuh ke
// DefaultCoverageRadiusKm alih-alih membisukan perangkat, dan nilai di atas
// batas dispatch tidak dapat memperlebar query melewati prefilter indeks.
//
// Agregasi per token (bukan DISTINCT ON) menjaga satu token dikirim sekali:
// satu perangkat dapat meninggalkan token yang sama pada beberapa baris user
// setelah "Reset Profile" mencetak identitas anonim baru. MIN(dist) dengan
// MAX(radius) memilih pembacaan yang paling longgar di antara baris duplikat —
// pada kanal life-safety, satu notifikasi berlebih lebih baik daripada satu
// perangkat yang seharusnya berbunyi tetapi diam. Urutan terdekat-dulu membuat
// pemotongan pada maxFCMTokensPerEvent membuang yang terjauh lebih dahulu.
func (s *Store) FCMTokensWithin(ctx context.Context, lat, lon float64, rangeKm int) ([]string, error) {
	const centroid = `ST_SetSRID(ST_MakePoint($2, $1), 4326)::geography`
	q := `
		SELECT fcm_token FROM (
			SELECT fcm_token,
			       MIN(last_location <-> ` + centroid + `) AS dist_m,
			       MAX(
			           LEAST(
			               GREATEST(COALESCE(NULLIF(coverage_radius_km, 0), $5), 1),
			               $6
			           )
			       ) * 1000.0 AS radius_m
			FROM user_profiles
			WHERE fcm_token IS NOT NULL
			  AND fcm_token <> ''
			  AND last_location IS NOT NULL
			  AND last_active > NOW() - ($4 || ' days')::interval
			  AND ST_DWithin(last_location, ` + centroid + `, $3)
			GROUP BY fcm_token
		) t
		WHERE dist_m <= radius_m
		ORDER BY dist_m
		LIMIT ` + strconv.Itoa(maxFCMTokensPerEvent)

	rows, err := s.pool.Query(ctx, q,
		lat, lon, float64(rangeKm)*1000.0, fcmTokenMaxIdle,
		DefaultCoverageRadiusKm, rangeKm,
	)
	if err != nil {
		return nil, fmt.Errorf("query fcm tokens: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var token string
		if err := rows.Scan(&token); err != nil {
			return nil, fmt.Errorf("scan fcm token: %w", err)
		}
		out = append(out, token)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate fcm tokens: %w", err)
	}
	return out, nil
}

// EventFilter membatasi ListEvents secara spasial: hanya event yang
// estimated_centroid-nya berada dalam RangeKm dari (Lat, Lon). nil = tanpa filter.
type EventFilter struct {
	Lat     float64
	Lon     float64
	RangeKm int
}

// Event adalah satu baris earthquake_events untuk endpoint GET /api/v1/events.
// Semua baris di tabel ini sudah CONFIRMED (dispatcher hanya mempersistensikan
// event >= 3 node unik; ADVISORY tidak disimpan).
type Event struct {
	EventID        string
	Status         string  // 'HAPPENING' | 'RESOLVED'
	Lat            float64 // ST_Y(estimated_centroid) — centroid, BUKAN episenter
	Lon            float64 // ST_X(estimated_centroid)
	LocationName   string
	MMIScale       string
	IntensityLabel string
	MaxPGA         float64 // gal
	TriggeredNodes int
	StartedAt      time.Time
	ResolvedAt     *time.Time // nil selama status masih HAPPENING
}

// Batas paginasi ListEvents (cermin kontrak OpenAPI GET /api/v1/events).
const (
	DefaultEventsLimit = 20
	MaxEventsLimit     = 100
)

// eventSelect adalah proyeksi bersama kedua varian query ListEvents.
// max_pga di-cast ke double precision agar NUMERIC(8,4) tiba di pgx sebagai
// float8 dan dapat di-scan langsung ke float64.
const eventSelect = `
	SELECT event_id, status,
	       ST_Y(estimated_centroid::geometry) AS lat,
	       ST_X(estimated_centroid::geometry) AS lon,
	       location_name, mmi_scale, intensity_label,
	       max_pga::double precision AS max_pga,
	       triggered_nodes_count, started_at, resolved_at
	FROM earthquake_events`

const (
	listEventsQ = eventSelect + `
		ORDER BY started_at DESC
		LIMIT $1 OFFSET $2`

	listEventsWithinQ = eventSelect + `
		WHERE ST_DWithin(estimated_centroid, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography, $3)
		ORDER BY started_at DESC
		LIMIT $4 OFFSET $5`
)

// ListEvents mengembalikan riwayat event terurut started_at DESC (terbaru dulu).
// filter nil = seluruh wilayah; filter non-nil = ST_DWithin pada GEOGRAPHY
// (satuan meter → RangeKm * 1000). limit di-clamp ke [1, MaxEventsLimit] dan
// offset ke >= 0 sebagai pertahanan berlapis di samping validasi handler.
func (s *Store) ListEvents(ctx context.Context, limit, offset int, filter *EventFilter) ([]Event, error) {
	if limit <= 0 {
		limit = DefaultEventsLimit
	}
	if limit > MaxEventsLimit {
		limit = MaxEventsLimit
	}
	if offset < 0 {
		offset = 0
	}

	var (
		rows pgx.Rows
		err  error
	)
	if filter != nil {
		rows, err = s.pool.Query(ctx, listEventsWithinQ,
			filter.Lon, filter.Lat, float64(filter.RangeKm)*1000.0, limit, offset)
	} else {
		rows, err = s.pool.Query(ctx, listEventsQ, limit, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	out := make([]Event, 0, limit)
	for rows.Next() {
		var e Event
		if err := rows.Scan(
			&e.EventID, &e.Status, &e.Lat, &e.Lon,
			&e.LocationName, &e.MMIScale, &e.IntensityLabel,
			&e.MaxPGA, &e.TriggeredNodes, &e.StartedAt, &e.ResolvedAt,
		); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}
	return out, nil
}
