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
