package store

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// Batas kolom broadcasts (migrasi 000004). Ditegakkan juga di tepi HTTP agar
// pengirim mendapat 400 yang jelas alih-alih galat kolom dari Postgres.
const (
	MaxBroadcastTitleLen = 120
	MaxBroadcastBodyLen  = 500
)

// Batas paginasi daftar siaran yang dibaca aplikasi.
const (
	DefaultBroadcastLimit = 20
	MaxBroadcastLimit     = 50
)

// broadcastRetentionDays memotong daftar yang dibaca klien, bukan menghapus
// baris: pengumuman lama tetap ada untuk audit operator, tetapi sebuah "Pembaruan"
// dari tahun lalu bukan pembaruan lagi dan tidak perlu memenuhi layar.
const broadcastRetentionDays = 90

// Broadcast adalah satu pengumuman operator yang tersimpan.
type Broadcast struct {
	ID    string
	Title string
	Body  string
	// RegionCode kosong berarti nasional. Bila terisi, nilainya adalah
	// user_profiles.region_code — kunci yang sama dengan kanal chat regional.
	RegionCode string
	CreatedAt  time.Time
}

// InsertBroadcast menyimpan siaran dan mengembalikannya lengkap dengan id serta
// created_at dari basis data.
//
// Disimpan lebih dulu, sebelum fanout apa pun: id yang dikembalikan di sini
// adalah yang dipakai payload push, sehingga notifikasi yang diketuk selalu
// menemukan barisnya di dalam aplikasi. regionCode kosong disimpan sebagai NULL
// (nasional), bukan sebagai string kosong yang tidak akan cocok dengan kanal
// mana pun.
func (s *Store) InsertBroadcast(ctx context.Context, title, body, regionCode string) (*Broadcast, error) {
	const q = `
		INSERT INTO broadcasts (title, body, region_code)
		VALUES ($1, $2, NULLIF($3, ''))
		RETURNING broadcast_id, created_at`

	out := &Broadcast{Title: title, Body: body, RegionCode: regionCode}
	err := s.pool.QueryRow(ctx, q, title, body, regionCode).
		Scan(&out.ID, &out.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert broadcast: %w", err)
	}
	return out, nil
}

// ListBroadcastsForUser mengembalikan siaran yang berlaku bagi satu user,
// terbaru dahulu: yang nasional ditambah yang menyasar region_code-nya.
//
// Penyaringan dilakukan dari region_code yang TERSIMPAN, bukan dari parameter
// yang dikirim klien: kalau tidak, siapa pun dapat membaca pengumuman wilayah
// lain hanya dengan mengubah query string — dan lebih buruk, tidak akan pernah
// melihat pengumuman wilayahnya sendiri kalau klien salah menyusun kuncinya.
func (s *Store) ListBroadcastsForUser(ctx context.Context, userID string, limit int) ([]Broadcast, error) {
	if limit <= 0 {
		limit = DefaultBroadcastLimit
	}
	if limit > MaxBroadcastLimit {
		limit = MaxBroadcastLimit
	}

	// Retensi disisipkan sebagai konstanta, bukan parameter: pgx mengirim
	// argumen integer sebagai int4 dan `$n || ' days'` menuntut text, sehingga
	// bentuk berparameter gagal di runtime ("cannot find encode plan") — sebuah
	// kegagalan yang hanya muncul saat kueri benar-benar dijalankan. Nilainya
	// konstanta compile-time, jadi tidak ada masukan pengguna yang disatukan ke
	// dalam SQL di sini.
	//
	// LEFT JOIN, bukan subquery wajib: user tanpa baris profil (atau tanpa
	// region) tetap harus menerima seluruh siaran nasional.
	q := `
		SELECT b.broadcast_id, b.title, b.body, COALESCE(b.region_code, ''), b.created_at
		FROM broadcasts b
		LEFT JOIN user_profiles u ON u.user_id = $1
		WHERE b.created_at > NOW() - make_interval(days => ` + strconv.Itoa(broadcastRetentionDays) + `)
		  AND (b.region_code IS NULL OR b.region_code = u.region_code)
		ORDER BY b.created_at DESC
		LIMIT $2`

	rows, err := s.pool.Query(ctx, q, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("query broadcasts: %w", err)
	}
	defer rows.Close()

	out := make([]Broadcast, 0, limit)
	for rows.Next() {
		var b Broadcast
		if err := rows.Scan(&b.ID, &b.Title, &b.Body, &b.RegionCode, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan broadcast: %w", err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate broadcasts: %w", err)
	}
	return out, nil
}

// FCMTokensInRegion mengembalikan token FCM milik user pada satu region_code.
//
// Penargetan siaran regional memakai region_code, bukan radius: sebuah
// pengumuman ditujukan kepada wilayah administratif ("BMKG menutup sementara
// layanan di Jawa Barat"), sementara peringatan gempa ditujukan kepada jarak
// dari sebuah titik. Keduanya sengaja tidak memakai kueri yang sama.
//
// DISTINCT dan batas token sama dengan jalur gempa: satu perangkat dikirimi
// sekali, dan instalasi yang lama tak aktif dilewati karena token-nya hampir
// pasti sudah mati.
func (s *Store) FCMTokensInRegion(ctx context.Context, regionCode string) ([]string, error) {
	if regionCode == "" {
		return nil, fmt.Errorf("region_code wajib untuk pencarian token regional")
	}
	q := `
		SELECT DISTINCT fcm_token
		FROM user_profiles
		WHERE region_code = $1
		  AND fcm_token IS NOT NULL
		  AND fcm_token <> ''
		  AND last_active > NOW() - make_interval(days => ` + strconv.Itoa(fcmTokenMaxIdle) + `)
		LIMIT ` + strconv.Itoa(maxFCMTokensPerEvent)

	rows, err := s.pool.Query(ctx, q, regionCode)
	if err != nil {
		return nil, fmt.Errorf("query fcm tokens region: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var token string
		if err := rows.Scan(&token); err != nil {
			return nil, fmt.Errorf("scan fcm token region: %w", err)
		}
		out = append(out, token)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate fcm tokens region: %w", err)
	}
	return out, nil
}
