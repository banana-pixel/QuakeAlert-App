// Package store menyediakan repository berbasis pgx/v5 (tanpa ORM, ADR-0002).
// Semua query memakai prepared statement implisit pgx + context timeout.
package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
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

// Ping memeriksa koneksi basis data untuk /healthz. Pemanggil membawa context
// ber-timeout sendiri (500 ms di internal/api), jadi probe yang menggantung tidak
// pernah menahan slot reverse proxy.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// NodeSecret memuat data yang dibutuhkan untuk verifikasi HMAC & anti-replay.
type NodeSecret struct {
	StationID   string
	SecretEnc   []byte
	SecretNonce []byte
	LastSeenTS  int64
	IsActive    bool
	// Verified adalah gerbang konsensus (migrasi 000005): node yang belum
	// dikonfirmasi operator boleh heartbeat (tampak di /sensors), tetapi
	// trigger-nya ditolak sehingga tidak pernah ikut voting menuju CONFIRMED.
	Verified bool
}

var ErrNodeNotFound = errors.New("node tidak ditemukan")

// GetNodeSecret mengambil secret terenkripsi + last_seen_ts untuk sebuah node.
func (s *Store) GetNodeSecret(ctx context.Context, stationID string) (*NodeSecret, error) {
	const q = `
		SELECT station_id, secret_key_enc, secret_key_nonce, last_seen_ts, is_active, verified
		FROM iot_nodes
		WHERE station_id = $1`
	row := s.pool.QueryRow(ctx, q, stationID)

	var n NodeSecret
	err := row.Scan(&n.StationID, &n.SecretEnc, &n.SecretNonce, &n.LastSeenTS, &n.IsActive, &n.Verified)
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
//
// latencyMs nil berarti latency tidak terukur pada heartbeat ini (node tanpa jam
// tersinkronisasi, O4). Nilai LAMA dipertahankan lewat COALESCE, bukan ditimpa
// dengan 0: nol akan terbaca di /sensors sebagai latency sempurna, yaitu
// kebalikan dari keadaan sebenarnya. last_heartbeat tetap dimutakhirkan — itulah
// yang membuat node tersebut tampak hidup alih-alih hilang.
func (s *Store) UpdateHeartbeat(ctx context.Context, stationID string, rssi int, latencyMs *int) (bool, error) {
	const q = `
		UPDATE iot_nodes
		SET last_rssi = $2,
		    last_latency_ms = COALESCE($3, last_latency_ms),
		    last_heartbeat = NOW()
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

// PendingNode adalah satu baris daftar verifikasi operator: node yang sudah
// terdaftar tetapi belum dikonfirmasi (migrasi 000005).
type PendingNode struct {
	StationID    string
	SensorModel  string
	LocationName string
	Lat          float64
	Lon          float64
	CreatedAt    time.Time
}

// ListUnverifiedNodes mengembalikan seluruh node yang menunggu konfirmasi
// operator, terbaru dulu. Dipakai endpoint admin GET /api/v1/admin/nodes/pending.
func (s *Store) ListUnverifiedNodes(ctx context.Context) ([]PendingNode, error) {
	const q = `
		SELECT station_id, sensor_model, location_name,
		       ST_Y(location::geometry) AS lat,
		       ST_X(location::geometry) AS lon,
		       created_at
		FROM iot_nodes
		WHERE NOT verified
		ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query unverified nodes: %w", err)
	}
	defer rows.Close()

	var out []PendingNode
	for rows.Next() {
		var n PendingNode
		if err := rows.Scan(&n.StationID, &n.SensorModel, &n.LocationName, &n.Lat, &n.Lon, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan unverified node: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// DeleteUnverifiedNode menghapus baris node yang BELUM terverifikasi, dan hanya
// itu. Predikat `verified = FALSE` ada DI DALAM SQL, bukan di handler: satu
// statement DELETE bersyarat adalah atomik terhadap UPDATE verifikasi operator
// (Postgres menserialisasi keduanya pada lock baris), sehingga node yang sudah
// sah TIDAK MUNGKIN terhapus lewat jalur ini — meski balapan dengan klik verify
// di admin, dan berapa pun keadaan kode pemanggil. Mengembalikan false bila
// station_id tidak dikenal ATAU sudah terverifikasi (pemanggil API membedakan
// keduanya via GetNodeSecret sebelum memanggil).
func (s *Store) DeleteUnverifiedNode(ctx context.Context, stationID string) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM iot_nodes WHERE station_id = $1 AND verified = FALSE`, stationID)
	if err != nil {
		return false, fmt.Errorf("delete unverified node: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// PurgeAbandonedPendingNodes menghapus node yang terdaftar tetapi TIDAK PERNAH
// mengirim heartbeat dalam masa retensi — kelas zombie provisioning: baris
// dicetak wizard lalu sesi dibuang (proses mati, respons jaringan hilang)
// sebelum ESP32 sempat dikonfigurasi.
//
// Dua predikat, keduanya wajib, dan itulah seluruh keselamatan loop ini:
//   - `verified = FALSE`  → node produksi (terverifikasi operator) mustahil
//     tersentuh, berapa pun umurnya;
//   - `last_heartbeat < created_at + interval '90 seconds'` → hanya node yang
//     tidak pernah "melapor tugas": instalasi sah yang menunggu konfirmasi
//     operator tetap berdenyut (UpdateHeartbeat menulis last_heartbeat = NOW()),
//     jadi usia saja tidak pernah menjadi alasan penghapusan. Ambang 90 s adalah
//     onlineThresholdSec di internal/api — batas yang sama yang membedakan
//     Online dari Offline.
//
// DELETE tanpa ORDER/LIMIT bersifat set-at-a-time dan idempoten: sweep berulang
// pada basis data yang sudah bersih menghapus 0 baris. Mengembalikan jumlah
// baris terhapus untuk observability; tidak ada field secret yang disentuh.
func (s *Store) PurgeAbandonedPendingNodes(ctx context.Context, olderThan time.Duration) (int64, error) {
	const q = `
		DELETE FROM iot_nodes
		WHERE verified = FALSE
		  AND created_at < NOW() - $1::interval
		  AND last_heartbeat < created_at + interval '90 seconds'`
	tag, err := s.pool.Exec(ctx, q, olderThan.String())
	if err != nil {
		return 0, fmt.Errorf("purge abandoned pending nodes: %w", err)
	}
	return tag.RowsAffected(), nil
}

// SetNodeVerified mengubah status verifikasi node. Mengembalikan false bila
// station_id tidak dikenal — pemanggil API memetakannya ke 404, bukan 500,
// karena ID yang salah adalah kesalahan operator yang dapat ditindaklanjuti.
func (s *Store) SetNodeVerified(ctx context.Context, stationID string, verified bool) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE iot_nodes SET verified = $2 WHERE station_id = $1`, stationID, verified)
	if err != nil {
		return false, fmt.Errorf("update node verified: %w", err)
	}
	return tag.RowsAffected() == 1, nil
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
	// Verified (migrasi 000005): node pending tetap muncul di daftar, tetapi
	// API menandainya sebagai status tersendiri dan tidak menghitungnya ke
	// active_sensors_count — kepercayaan tidak boleh terlihat sama dengan
	// kesehatan.
	Verified bool
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
		       is_active, verified, last_rssi, last_latency_ms,
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
			&s.Lat, &s.Lon, &s.IsActive, &s.Verified, &s.LastRSSI, &s.LastLatencyMs,
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
	UserID      string
	Lat         float64
	Lon         float64
	HasLocation bool
}

// GetUserLocation mengambil last_location user. coverage_radius_km tidak dibaca:
// radius peringatan tetap (dispatch.AlertRadiusKm) dan filter "NEAR" pada
// events/sensors dikirim klien sebagai range_km, jadi tidak ada pembaca lagi.
func (s *Store) GetUserLocation(ctx context.Context, userID string) (*UserLocation, error) {
	const q = `
		SELECT ST_Y(last_location::geometry) AS lat,
		       ST_X(last_location::geometry) AS lon,
		       last_location IS NOT NULL AS has_loc
		FROM user_profiles
		WHERE user_id = $1`
	var (
		lat, lon *float64
		hasLoc   bool
	)
	err := s.pool.QueryRow(ctx, q, userID).Scan(&lat, &lon, &hasLoc)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan user location: %w", err)
	}
	u := &UserLocation{UserID: userID, HasLocation: hasLoc}
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

// LocationUpdate adalah hasil UpdateUserLocation.
type LocationUpdate struct {
	// UpdatedAt adalah last_active yang baru di-set NOW().
	UpdatedAt time.Time

	// MovedKm adalah jarak dari posisi SEBELUMNYA ke posisi baru, nil bila user
	// belum pernah punya posisi. Dibaca dalam statement yang sama karena
	// nilainya hanya ada sesaat sebelum UPDATE menimpanya, dan pemanggil butuh
	// itu untuk memutuskan apakah region_code lama masih layak dipertahankan.
	MovedKm *float64
}

// UpdateUserLocation menyetel last_location (GEOGRAPHY(Point,4326)) user dari
// (lat, lon) — perhatikan ST_MakePoint memakai urutan (lon, lat) — beserta label
// lokasi opsional, lalu mengembalikan last_active yang di-set NOW() dan jarak
// perpindahannya.
//
// Semantik PUT (replace): locationName kosong disimpan sebagai NULL, jadi klien
// yang mengirim body tanpa location_name memang MENGOSONGKAN label lama.
//
// Kolom coverage_radius_km TIDAK disentuh: radius peringatan kini tetap dan
// ditentukan sistem (dispatch.AlertRadiusKm), bukan preferensi per user, jadi
// tidak ada nilai dari klien yang perlu disimpan di sini.
// Mengembalikan ErrUserNotFound bila user_id tidak ada.
func (s *Store) UpdateUserLocation(ctx context.Context, userID string, lat, lon float64, locationName string) (LocationUpdate, error) {
	// CTE prev membaca last_location pada snapshot SEBELUM UPDATE, jadi jaraknya
	// terhitung tanpa round trip kedua dan tanpa celah di antara dua statement.
	const q = `
		WITH prev AS (
		    SELECT user_id, last_location FROM user_profiles WHERE user_id = $1
		)
		UPDATE user_profiles p
		SET last_location = ST_SetSRID(ST_MakePoint($2, $3), 4326)::geography,
		    location_name = NULLIF($4, ''),
		    last_active   = NOW()
		FROM prev
		WHERE p.user_id = prev.user_id
		RETURNING p.last_active,
		          ST_Distance(
		              prev.last_location,
		              ST_SetSRID(ST_MakePoint($2, $3), 4326)::geography
		          ) / 1000.0`
	var out LocationUpdate
	err := s.pool.QueryRow(ctx, q, userID, lon, lat, locationName).
		Scan(&out.UpdatedAt, &out.MovedKm)
	if errors.Is(err, pgx.ErrNoRows) {
		return LocationUpdate{}, ErrUserNotFound
	}
	if err != nil {
		return LocationUpdate{}, fmt.Errorf("update user location: %w", err)
	}
	return out, nil
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
// rangeKm adalah radius peringatan itu sendiri (dispatch.AlertRadiusKm, 200 km),
// bukan lagi sekadar batas atas prefilter: sejak radius menjadi tetap dan
// ditentukan sistem, tidak ada radius kedua per user yang perlu dijepit di sini.
// Satu-satunya predikat jarak adalah ST_DWithin, sehingga indeks GiST
// idx_users_spatial memangkas tabel sebelum jarak per-baris dihitung.
//
// Kejadian berintensitas tinggi TIDAK melewati fungsi ini sama sekali: dispatch
// menyiarkannya ke topic tanpa filter jarak (lihat dispatch.IsSevere).
//
// DISTINCT ON per token menjaga satu perangkat dikirimi sekali: satu perangkat
// dapat meninggalkan token yang sama pada beberapa baris user setelah "Reset
// Profile" mencetak identitas anonim baru. Urutan terdekat-dulu membuat
// pemotongan pada maxFCMTokensPerEvent membuang yang terjauh lebih dahulu.
func (s *Store) FCMTokensWithin(ctx context.Context, lat, lon float64, rangeKm int) ([]string, error) {
	const centroid = `ST_SetSRID(ST_MakePoint($2, $1), 4326)::geography`
	q := `
		SELECT fcm_token FROM (
			SELECT DISTINCT ON (fcm_token)
			       fcm_token,
			       last_location <-> ` + centroid + ` AS dist_m
			FROM user_profiles
			WHERE fcm_token IS NOT NULL
			  AND fcm_token <> ''
			  AND last_location IS NOT NULL
			  AND last_active > NOW() - make_interval(days => ` + strconv.Itoa(fcmTokenMaxIdle) + `)
			  AND ST_DWithin(last_location, ` + centroid + `, $3)
			ORDER BY fcm_token, dist_m
		) t
		ORDER BY dist_m
		LIMIT ` + strconv.Itoa(maxFCMTokensPerEvent)

	rows, err := s.pool.Query(ctx, q, lat, lon, float64(rangeKm)*1000.0)
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

// EventFilter membatasi ListEvents. nil = tanpa filter sama sekali.
//
// Keempat kriteria saling independen dan digabung dengan AND:
//   - spasial: aktif hanya bila RangeKm > 0 (ST_DWithin pada estimated_centroid
//     terhadap Lat/Lon). RangeKm == 0 berarti seluruh wilayah, sehingga filter
//     waktu atau intensitas dapat dipakai tanpa memaksa klien mengirim koordinat.
//   - MinPGA: ambang bawah max_pga dalam gal. Perbandingan dilakukan pada PGA,
//     bukan mmi_scale, karena mmi_scale adalah angka Romawi (VARCHAR) sementara
//     max_pga NUMERIC dapat dibandingkan langsung tanpa kolom turunan baru.
//   - Since/Until: batas inklusif pada started_at.
//
// Semua kriteria dievaluasi di SQL, bukan setelah paginasi: menyaring hasil di
// klien membuat halaman menjadi pendek dan membuat klien menyimpulkan data sudah
// habis padahal server masih menyimpan kecocokan.
type EventFilter struct {
	Lat     float64
	Lon     float64
	RangeKm int
	MinPGA  *float64
	Since   *time.Time
	Until   *time.Time
}

// HasCriteria melaporkan apakah filter benar-benar membatasi sesuatu. Filter
// non-nil yang semua fieldnya kosong diperlakukan sama dengan nil.
func (f *EventFilter) HasCriteria() bool {
	return f != nil && (f.RangeKm > 0 || f.MinPGA != nil || f.Since != nil || f.Until != nil)
}

// HasSpatial melaporkan apakah filter radius aktif. Dipakai handler untuk
// memutuskan apakah `range_km` ikut dikembalikan pada envelope respons.
func (f *EventFilter) HasSpatial() bool {
	return f != nil && f.RangeKm > 0
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

// listEventsQuery menyusun query ListEvents beserta argumennya.
//
// Query dibangun dinamis karena kriteria filter opsional dan saling bebas, jadi
// jumlah kombinasinya (spasial x intensitas x waktu) tidak layak ditulis sebagai
// konstanta terpisah. Nilai tetap masuk sebagai placeholder $n — tidak ada nilai
// yang pernah diinterpolasi ke dalam SQL — sehingga pgx tetap memakai prepared
// statement dan tidak ada permukaan injeksi.
func listEventsQuery(filter *EventFilter, limit, offset int) (string, []any) {
	args := make([]any, 0, 7)
	// next menitipkan satu nilai ke args dan mengembalikan placeholder-nya.
	next := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}

	conds := make([]string, 0, 4)
	if filter != nil {
		if filter.RangeKm > 0 {
			lon, lat := next(filter.Lon), next(filter.Lat)
			meters := next(float64(filter.RangeKm) * 1000.0)
			conds = append(conds, fmt.Sprintf(
				"ST_DWithin(estimated_centroid, ST_SetSRID(ST_MakePoint(%s, %s), 4326)::geography, %s)",
				lon, lat, meters))
		}
		if filter.MinPGA != nil {
			conds = append(conds, "max_pga >= "+next(*filter.MinPGA))
		}
		if filter.Since != nil {
			conds = append(conds, "started_at >= "+next(*filter.Since))
		}
		if filter.Until != nil {
			conds = append(conds, "started_at <= "+next(*filter.Until))
		}
	}

	q := eventSelect
	if len(conds) > 0 {
		q += "\n\t\tWHERE " + strings.Join(conds, " AND ")
	}
	q += "\n\t\tORDER BY started_at DESC\n\t\tLIMIT " + next(limit) + " OFFSET " + next(offset)
	return q, args
}

// ListEvents mengembalikan riwayat event terurut started_at DESC (terbaru dulu).
// filter nil (atau tanpa kriteria) = seluruh riwayat; selain itu kriteria pada
// [EventFilter] digabung dengan AND — radius memakai ST_DWithin pada GEOGRAPHY
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

	if !filter.HasCriteria() {
		filter = nil
	}
	q, args := listEventsQuery(filter, limit, offset)
	rows, err := s.pool.Query(ctx, q, args...)
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

// ---------------------------------------------------------------------------
// Observation ledger (migrasi 000006)
//
// Dua fungsi tulis di bawah ini adalah SATU-SATUNYA jalur tulis ke
// sensor_observations dan alert_emissions, dan keduanya HANYA dipanggil dari
// goroutine drain internal/ledger — bukan dari jalur peringatan. Keduanya
// tidak mengembalikan id: tidak ada pemanggil yang membutuhkannya, dan
// mengembalikannya akan mengundang pembacaan sinkron di masa depan.
// ---------------------------------------------------------------------------

// Observation adalah satu baris sensor_observations: satu trigger yang sampai ke
// server, lolos verifikasi atau tidak.
//
// Field bertipe pointer adalah field yang memang boleh NULL di kolomnya. Lat/Lon
// nil berarti lokasi node tidak diketahui saat ingest (node dihapus, atau tidak
// pernah dikenal) — bukan kegagalan verifikasi.
type Observation struct {
	NodeID               string
	SourceClass          string // 'FIXED_ESP32'
	Phase                string // 'PRELIM' | 'FINAL'
	ProtoVer             *int16 // NULL pada v1
	ObsSeq               *int64 // NULL pada v1
	PGAGal               float64
	DurMs                int64
	PublishTS            int64 // ts payload (ms epoch UTC)
	ReceivedTS           int64 // jam server (ms epoch UTC)
	OnsetTS              *int64
	OnsetTSUpperBound    *int64
	OnsetTSSource        string
	AttemptNo            *int16   // NULL pada v1 (migrasi 000007)
	DetriggerTS          *int64   // NULL pada v1 dan pada PRELIM (migrasi 000007)
	Lat                  *float64 // node_location, boleh NULL
	Lon                  *float64
	Signature            string
	VerifyResult         string // 'OK' atau nama Err* verifier
	SuppressedRejections int
}

// InsertObservation menulis satu baris sensor_observations.
//
// node_location dibangun lewat CASE: ST_MakePoint akan menolak NULL, sedangkan
// observasi tanpa lokasi justru kasus yang wajib tetap tercatat (A16), jadi
// koordinat nil harus melewati ST_MakePoint sepenuhnya, bukan masuk ke dalamnya.
func (s *Store) InsertObservation(ctx context.Context, o *Observation) error {
	const q = `
		INSERT INTO sensor_observations (
			node_id, source_class, phase, proto_ver, obs_seq,
			pga_gal, dur_ms, publish_ts, received_ts,
			onset_ts, onset_ts_upper_bound, onset_ts_source,
			attempt_no, detrigger_ts,
			node_location, signature, verify_result, suppressed_rejections
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9,
			$10, $11, $12,
			$13, $14,
			CASE WHEN $15::double precision IS NULL OR $16::double precision IS NULL
			     THEN NULL
			     ELSE ST_SetSRID(ST_MakePoint($15::double precision, $16::double precision), 4326)::geography
			END,
			NULLIF($17, ''), $18, $19
		)`
	_, err := s.pool.Exec(ctx, q,
		o.NodeID, o.SourceClass, o.Phase, o.ProtoVer, o.ObsSeq,
		o.PGAGal, o.DurMs, o.PublishTS, o.ReceivedTS,
		o.OnsetTS, o.OnsetTSUpperBound, o.OnsetTSSource,
		o.AttemptNo, o.DetriggerTS,
		o.Lon, o.Lat,
		o.Signature, o.VerifyResult, o.SuppressedRejections,
	)
	if err != nil {
		return fmt.Errorf("insert sensor_observation: %w", err)
	}
	return nil
}

// AlertEmission adalah satu baris alert_emissions: satu KEPUTUSAN dispatch,
// beserta hasil pengirimannya bila hasil itu memang dapat diobservasi.
//
// DecidedAt dan DeliveryAt adalah dua waktu yang berbeda dan keduanya perlu:
// yang pertama adalah kapan keputusan dibuat, yang kedua kapan pengiriman
// selesai. Selisih keduanya adalah satu-satunya ukuran biaya fan-out yang
// dimiliki sistem ini.
//
// Keempat kolom hasil kirim bernilai NULL berarti "hasil tidak pernah
// dilaporkan", BUKAN nol. Nol yang ditulis untuk jalur yang tidak dapat
// mengamati pengiriman akan terbaca sebagai pengiriman yang gagal total.
type AlertEmission struct {
	EventID     *string // NULL untuk ADVISORY (tidak punya identitas event)
	AlertType   string
	Status      string
	MMI         *string
	PGAGal      *float64
	NodeCount   int
	CentroidLat *float64
	CentroidLon *float64
	IsSevere    bool
	Audience    string // TOKENS_RADIUS_200KM | GEO_TOPIC_ALL | NONE
	DecidedAt   int64  // ms epoch UTC, jam server
	AlgoVer     string

	// Hasil pengiriman (migrasi 000007). NULL = tidak diobservasi.
	WSClientCount *int
	FCMAttempted  *int
	FCMSucceeded  *int
	DeliveryAt    *int64
}

// InsertAlertEmission menulis satu baris alert_emissions.
//
// event_id di-cast eksplisit ke uuid: parameter dikirim sebagai *string, dan
// tanpa cast Postgres tidak dapat menyimpulkan tipe untuk NULL.
func (s *Store) InsertAlertEmission(ctx context.Context, e *AlertEmission) error {
	const q = `
		INSERT INTO alert_emissions (
			event_id, alert_type, status, mmi, pga_gal, node_count,
			centroid, is_severe, audience, decided_at, algo_ver,
			ws_client_count, fcm_attempted, fcm_succeeded, delivery_at
		) VALUES (
			$1::uuid, $2, $3, $4, $5, $6,
			CASE WHEN $7::double precision IS NULL OR $8::double precision IS NULL
			     THEN NULL
			     ELSE ST_SetSRID(ST_MakePoint($7::double precision, $8::double precision), 4326)::geography
			END,
			$9, $10, $11, $12,
			$13, $14, $15, $16
		)`
	_, err := s.pool.Exec(ctx, q,
		e.EventID, e.AlertType, e.Status, e.MMI, e.PGAGal, e.NodeCount,
		e.CentroidLon, e.CentroidLat,
		e.IsSevere, e.Audience, e.DecidedAt, e.AlgoVer,
		e.WSClientCount, e.FCMAttempted, e.FCMSucceeded, e.DeliveryAt,
	)
	if err != nil {
		return fmt.Errorf("insert alert_emission: %w", err)
	}
	return nil
}
