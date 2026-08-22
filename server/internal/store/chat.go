package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// GlobalChannelID adalah satu-satunya kanal yang selalu ada dan selalu boleh
// diakses siapa pun — termasuk user yang belum pernah mengirim lokasi. Lihat
// docs/CHAT_DESIGN.md §2.
const GlobalChannelID = "global"

// Jenis kanal pada chat_channels.kind (dibatasi CHECK constraint di migrasi 000003).
const (
	ChannelKindGlobal   = "GLOBAL"
	ChannelKindRegional = "REGIONAL"
)

// Batas paginasi ListChatMessages (cermin kontrak REST di CHAT_DESIGN §4).
const (
	DefaultChatLimit = 30
	MaxChatLimit     = 100
)

// MaxChatBodyLen membatasi panjang isi pesan. Ditegakkan di handler agar klien
// mendapat 400 yang jelas, dan diulang di sini sebagai pertahanan berlapis
// karena kolom message bertipe TEXT (tanpa batas dari sisi Postgres).
const MaxChatBodyLen = 500

// ChatChannel adalah satu kanal yang boleh diakses seorang user.
type ChatChannel struct {
	ChannelID   string
	Kind        string // ChannelKindGlobal | ChannelKindRegional
	DisplayName string
}

// ChatMessage adalah satu baris chat_messages.
//
// SenderPseudonym adalah SNAPSHOT, bukan hasil join ke user_profiles: pseudonym
// bisa di-reroll, dan riwayat tidak boleh berganti nama secara retroaktif.
type ChatMessage struct {
	MessageID       string
	ChannelID       string
	SenderID        string
	SenderPseudonym string
	LocationTag     string
	Body            string
	IsAdmin         bool
	CreatedAt       time.Time
}

// ErrChannelForbidden dikembalikan bila user menulis ke kanal yang bukan
// keanggotaannya. Keanggotaan diturunkan dari region_code dan ditegakkan di
// server — klien tidak dipercaya soal kanal mana yang boleh ia tulis.
var ErrChannelForbidden = errors.New("kanal bukan keanggotaan user")

// UserChatIdentity memuat apa yang dibutuhkan untuk mengarsipkan satu pesan:
// pseudonym untuk snapshot pengirim, region untuk keanggotaan kanal, dan label
// lokasi opsional yang ditampilkan di bawah nama pengirim.
type UserChatIdentity struct {
	Pseudonym    string
	RegionCode   string // kosong bila user belum punya fix lokasi
	LocationName string
}

// GetUserChatIdentity membaca identitas chat user. ErrUserNotFound bila absen.
func (s *Store) GetUserChatIdentity(ctx context.Context, userID string) (*UserChatIdentity, error) {
	const q = `
		SELECT pseudonym, COALESCE(region_code, ''), COALESCE(location_name, '')
		FROM user_profiles
		WHERE user_id = $1`
	var identity UserChatIdentity
	err := s.pool.QueryRow(ctx, q, userID).
		Scan(&identity.Pseudonym, &identity.RegionCode, &identity.LocationName)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan chat identity: %w", err)
	}
	return &identity, nil
}

// SetUserRegion menyimpan kunci kanal regional user (kosong = NULL, artinya user
// kembali hanya punya kanal global). Dipanggil dari PUT /users/location, karena
// region adalah turunan dari posisi yang sama — menyimpannya di tempat lain akan
// membuat dua sumber kebenaran untuk satu fakta.
func (s *Store) SetUserRegion(ctx context.Context, userID, regionCode string) error {
	const q = `
		UPDATE user_profiles
		SET region_code = NULLIF($2, '')
		WHERE user_id = $1`
	tag, err := s.pool.Exec(ctx, q, userID, regionCode)
	if err != nil {
		return fmt.Errorf("update region_code: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// EnsureChatChannel membuat baris kanal bila belum ada, lalu mengembalikan nama
// tampilan yang BERLAKU — yaitu milik penulis pertama, bukan yang baru dikirim.
// Itu sengaja: dua ponsel bisa mengeja "Jawa Barat" dan "West Java" berbeda, dan
// anggota satu kanal harus melihat satu judul yang sama.
func (s *Store) EnsureChatChannel(
	ctx context.Context, channelID, kind, displayName string,
) (string, error) {
	const q = `
		INSERT INTO chat_channels (channel_id, kind, display_name)
		VALUES ($1, $2, $3)
		ON CONFLICT (channel_id) DO UPDATE SET channel_id = chat_channels.channel_id
		RETURNING display_name`
	var name string
	if err := s.pool.QueryRow(ctx, q, channelID, kind, displayName).Scan(&name); err != nil {
		return "", fmt.Errorf("ensure chat channel: %w", err)
	}
	return name, nil
}

// ListChatChannels mengembalikan kanal yang boleh diakses user: global selalu,
// plus kanal regionalnya bila region_code terisi. Klien tidak pernah menebak
// kunci kanal — ia memakai daftar ini.
func (s *Store) ListChatChannels(ctx context.Context, userID string) ([]ChatChannel, error) {
	identity, err := s.GetUserChatIdentity(ctx, userID)
	if err != nil {
		return nil, err
	}

	ids := []string{GlobalChannelID}
	if identity.RegionCode != "" {
		ids = append(ids, identity.RegionCode)
	}

	const q = `
		SELECT channel_id, kind, display_name
		FROM chat_channels
		WHERE channel_id = ANY($1)`
	rows, err := s.pool.Query(ctx, q, ids)
	if err != nil {
		return nil, fmt.Errorf("query chat channels: %w", err)
	}
	defer rows.Close()

	found := make(map[string]ChatChannel, len(ids))
	for rows.Next() {
		var c ChatChannel
		if err := rows.Scan(&c.ChannelID, &c.Kind, &c.DisplayName); err != nil {
			return nil, fmt.Errorf("scan chat channel: %w", err)
		}
		found[c.ChannelID] = c
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chat channels: %w", err)
	}

	// Urutan mengikuti ids (global lebih dulu), dan kanal regional yang belum
	// pernah ditulis siapa pun tetap dikembalikan — ia ada sebagai keanggotaan
	// walau belum punya baris katalog, dan ruang kosong adalah jawaban yang sah.
	out := make([]ChatChannel, 0, len(ids))
	for _, id := range ids {
		if c, ok := found[id]; ok {
			out = append(out, c)
			continue
		}
		kind := ChannelKindRegional
		if id == GlobalChannelID {
			kind = ChannelKindGlobal
		}
		out = append(out, ChatChannel{ChannelID: id, Kind: kind, DisplayName: id})
	}
	return out, nil
}

// ListChatMessages membaca satu halaman pesan, terbaru lebih dulu.
//
// Kursor waktu (before), bukan offset: ruang yang aktif menggeser offset di
// antara dua permintaan, sehingga paginasi berbasis offset akan melewatkan atau
// menggandakan baris. Tie-break pada message_id membuat urutan tetap total bila
// dua pesan punya created_at identik.
func (s *Store) ListChatMessages(
	ctx context.Context, channelID string, limit int, before *time.Time,
) ([]ChatMessage, error) {
	if limit <= 0 {
		limit = DefaultChatLimit
	}
	if limit > MaxChatLimit {
		limit = MaxChatLimit
	}

	q := `
		SELECT message_id, channel_id, COALESCE(sender_id::text, ''), sender_pseudonym,
		       COALESCE(sender_location_tag, ''), message, COALESCE(is_admin, FALSE), created_at
		FROM chat_messages
		WHERE channel_id = $1`
	args := []any{channelID}
	if before != nil {
		q += ` AND created_at < $2`
		args = append(args, *before)
	}
	q += fmt.Sprintf(` ORDER BY created_at DESC, message_id DESC LIMIT $%d`, len(args)+1)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query chat messages: %w", err)
	}
	defer rows.Close()

	messages := make([]ChatMessage, 0, limit)
	for rows.Next() {
		var m ChatMessage
		if err := rows.Scan(
			&m.MessageID, &m.ChannelID, &m.SenderID, &m.SenderPseudonym,
			&m.LocationTag, &m.Body, &m.IsAdmin, &m.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan chat message: %w", err)
		}
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chat messages: %w", err)
	}
	return messages, nil
}

// InsertChatMessage menyimpan satu pesan dan mengembalikan baris yang tersimpan.
//
// Idempoten terhadap clientMessageID: klien mengirim ulang setelah timeout tanpa
// tahu apakah percobaan pertama sampai, dan pesan ganda di ruang publik adalah
// kerusakan yang tidak bisa ditarik kembali. Indeks unik parsial di migrasi
// 000003 yang menegakkannya; ON CONFLICT DO NOTHING lalu SELECT membuat
// percobaan kedua mengembalikan baris yang SAMA, bukan galat.
//
// Predikat indeks (WHERE client_message_id IS NOT NULL) WAJIB diulang pada klausa
// ON CONFLICT: inferensi Postgres hanya mengenali indeks parsial bila predikatnya
// disebutkan, dan tanpa itu insert gagal 42P10 ("no unique or exclusion constraint
// matching the ON CONFLICT specification") — bukan galat kompilasi, jadi hanya
// muncul saat pesan pertama dikirim.
//
// clientMessageID kosong tetap diterima (disimpan NULL) supaya klien lama atau
// pengirim non-Android tidak dipaksa ikut skema idempotensi.
func (s *Store) InsertChatMessage(
	ctx context.Context,
	channelID, senderID, pseudonym, locationTag, body, clientMessageID string,
) (*ChatMessage, error) {
	// sender_location_tag adalah VARCHAR(50) sementara location_name bisa 150:
	// dipotong di sini agar label yang panjang tidak menggagalkan penyimpanan
	// pesan — nama tempat adalah hiasan, pesannya yang penting.
	if len(locationTag) > maxLocationTagLen {
		locationTag = locationTag[:maxLocationTagLen]
	}

	const insert = `
		INSERT INTO chat_messages (
			channel_id, sender_id, sender_pseudonym, sender_location_tag,
			message, client_message_id
		)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, NULLIF($6, '')::uuid)
		ON CONFLICT (sender_id, client_message_id) WHERE client_message_id IS NOT NULL
		    DO NOTHING
		RETURNING message_id, channel_id, COALESCE(sender_id::text, ''), sender_pseudonym,
		          COALESCE(sender_location_tag, ''), message, COALESCE(is_admin, FALSE), created_at`

	var m ChatMessage
	err := s.pool.QueryRow(ctx, insert, channelID, senderID, pseudonym, locationTag, body, clientMessageID).
		Scan(&m.MessageID, &m.ChannelID, &m.SenderID, &m.SenderPseudonym,
			&m.LocationTag, &m.Body, &m.IsAdmin, &m.CreatedAt)
	if err == nil {
		return &m, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("insert chat message: %w", err)
	}

	// Tidak ada baris kembali = konflik idempotensi. Ambil kiriman pertama.
	const existing = `
		SELECT message_id, channel_id, COALESCE(sender_id::text, ''), sender_pseudonym,
		       COALESCE(sender_location_tag, ''), message, COALESCE(is_admin, FALSE), created_at
		FROM chat_messages
		WHERE sender_id = $1 AND client_message_id = $2::uuid`
	if err := s.pool.QueryRow(ctx, existing, senderID, clientMessageID).
		Scan(&m.MessageID, &m.ChannelID, &m.SenderID, &m.SenderPseudonym,
			&m.LocationTag, &m.Body, &m.IsAdmin, &m.CreatedAt); err != nil {
		return nil, fmt.Errorf("baca pesan idempoten: %w", err)
	}
	return &m, nil
}

// PurgeChatMessages menghapus pesan yang lebih tua dari retensi dan
// mengembalikan jumlah baris terhapus.
//
// Dijalankan oleh goroutine terjadwal di cmd/quakealert, bukan pg_cron: ekstensi
// itu belum tentu ada di host produksi, dan retensi yang bergantung pada
// ekstensi opsional adalah retensi yang diam-diam tidak berjalan.
func (s *Store) PurgeChatMessages(ctx context.Context, olderThan time.Duration) (int64, error) {
	const q = `DELETE FROM chat_messages WHERE created_at < NOW() - $1::interval`
	tag, err := s.pool.Exec(ctx, q, olderThan.String())
	if err != nil {
		return 0, fmt.Errorf("purge chat messages: %w", err)
	}
	return tag.RowsAffected(), nil
}

// Batas kolom sender_location_tag pada migrasi 000001.
const maxLocationTagLen = 50
