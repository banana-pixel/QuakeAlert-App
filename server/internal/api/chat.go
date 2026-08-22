package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// Batas chat yang ditegakkan di tepi HTTP (cermin store.MaxChatBodyLen dan
// store.MaxChatLimit, diulang di sini agar klien mendapat 400 yang jelas
// alih-alih galat kolom dari Postgres).
const (
	maxChatBodyLen = store.MaxChatBodyLen

	// chatSendWindow adalah jeda minimum antar pesan per user. Chat adalah
	// permukaan user-generated pertama aplikasi ini, dan tanpa moderasi
	// (docs/CHAT_DESIGN.md §7) laju kirim adalah satu-satunya rem yang ada.
	chatSendWindow = 2 * time.Second
)

// ChatEvent adalah pesan yang sudah tersimpan dan siap disiarkan.
//
// Didefinisikan di paket api, bukan diambil dari dispatch: kedua paket itu tidak
// saling impor, dan cmd/quakealert yang menjembatani keduanya. Menambah impor di
// antara mereka hanya demi satu tipe payload akan menciptakan siklus.
type ChatEvent struct {
	MessageID       string
	ChannelID       string
	SenderID        string
	SenderPseudonym string
	LocationTag     string
	Body            string
	IsAdmin         bool
	CreatedAt       time.Time
}

// ChatFanout menyiarkan pesan yang SUDAH tersimpan ke klien WebSocket kanal itu.
//
// Pengiriman tetap lewat REST (durable, bisa diulang) dan socket hanya
// memfanout apa yang sudah ada di basis data — jadi kegagalan fanout tidak
// pernah menghilangkan pesan, hanya menundanya sampai klien memuat ulang.
type ChatFanout interface {
	BroadcastChat(ChatEvent)
}

// SetChatFanout memasang jalur siaran chat. Opsional: tanpa itu, chat tetap
// berfungsi lewat REST (klien melihat pesannya saat memuat halaman), hanya tanpa
// pembaruan langsung. Setter alih-alih parameter NewServer karena hub WebSocket
// dibangun setelah Server di cmd/quakealert.
func (s *Server) SetChatFanout(f ChatFanout) {
	s.chat = f
}

// --- GET /api/v1/chat/channels ---

type chatChannelDTO struct {
	ChannelID   string `json:"channel_id"`
	Kind        string `json:"kind"` // GLOBAL | REGIONAL
	DisplayName string `json:"display_name"`
}

type listChatChannelsResponse struct {
	Channels []chatChannelDTO `json:"channels"`
}

// HandleListChatChannels menjawab kanal mana yang boleh diakses pemanggil.
//
// Server yang menjawab, bukan klien yang menebak: kunci kanal regional adalah
// turunan region_code yang tersimpan, dan klien yang menyusun kuncinya sendiri
// akan meminta ruang yang tidak ada begitu normalisasi di server berubah.
func (s *Server) HandleListChatChannels(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "user tidak terautentikasi")
		return
	}

	channels, err := s.repo.ListChatChannels(r.Context(), userID)
	if errors.Is(err, store.ErrUserNotFound) {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "profil user tidak ditemukan")
		return
	}
	if err != nil {
		s.log.Error("gagal membaca kanal chat", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal membaca daftar kanal")
		return
	}

	out := make([]chatChannelDTO, 0, len(channels))
	for _, c := range channels {
		out = append(out, chatChannelDTO{
			ChannelID:   c.ChannelID,
			Kind:        c.Kind,
			DisplayName: c.DisplayName,
		})
	}
	writeJSON(w, http.StatusOK, listChatChannelsResponse{Channels: out})
}

// --- GET /api/v1/chat/messages ---

type chatMessageDTO struct {
	MessageID       string `json:"message_id"`
	ChannelID       string `json:"channel_id"`
	SenderID        string `json:"sender_id"`
	SenderPseudonym string `json:"sender_pseudonym"`
	SenderLocation  string `json:"sender_location_tag,omitempty"`
	Message         string `json:"message"`
	IsAdmin         bool   `json:"is_admin"`
	CreatedAt       string `json:"created_at"` // RFC3339 UTC
}

type listChatMessagesResponse struct {
	ChannelID string           `json:"channel_id"`
	Limit     int              `json:"limit"`
	Count     int              `json:"count"`
	Messages  []chatMessageDTO `json:"messages"`
}

// HandleListChatMessages mengembalikan satu halaman pesan, terbaru lebih dulu.
//
// Kursor `before` (RFC3339), bukan offset: ruang yang aktif menggeser offset di
// antara dua permintaan, sehingga halaman kedua akan melewatkan atau menggandakan
// baris tepat saat percakapan sedang ramai.
func (s *Server) HandleListChatMessages(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "user tidak terautentikasi")
		return
	}

	channelID := strings.TrimSpace(r.URL.Query().Get("channel_id"))
	if channelID == "" {
		channelID = store.GlobalChannelID
	}
	if err := s.assertChatMember(r, userID, channelID); err != nil {
		s.writeChatMembershipError(w, err)
		return
	}

	limit := store.DefaultChatLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "limit harus bilangan positif")
			return
		}
		limit = parsed
		if limit > store.MaxChatLimit {
			limit = store.MaxChatLimit
		}
	}

	var before *time.Time
	if raw := r.URL.Query().Get("before"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "before harus RFC3339")
			return
		}
		before = &parsed
	}

	messages, err := s.repo.ListChatMessages(r.Context(), channelID, limit, before)
	if err != nil {
		s.log.Error("gagal membaca pesan chat", "err", err, "channel_id", channelID)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal membaca pesan")
		return
	}

	out := make([]chatMessageDTO, 0, len(messages))
	for _, m := range messages {
		out = append(out, toChatMessageDTO(m))
	}
	writeJSON(w, http.StatusOK, listChatMessagesResponse{
		ChannelID: channelID,
		Limit:     limit,
		Count:     len(out),
		Messages:  out,
	})
}

func toChatMessageDTO(m store.ChatMessage) chatMessageDTO {
	return chatMessageDTO{
		MessageID:       m.MessageID,
		ChannelID:       m.ChannelID,
		SenderID:        m.SenderID,
		SenderPseudonym: m.SenderPseudonym,
		SenderLocation:  m.LocationTag,
		Message:         m.Body,
		IsAdmin:         m.IsAdmin,
		CreatedAt:       m.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// --- POST /api/v1/chat/messages ---

type createChatMessageRequest struct {
	ChannelID string `json:"channel_id"`
	Message   string `json:"message"`
	// ClientMessageID membuat pengiriman idempoten. Klien yang timeout tidak tahu
	// apakah percobaan pertamanya sampai, dan pesan ganda di ruang publik tidak
	// bisa ditarik kembali.
	ClientMessageID string `json:"client_message_id"`
}

// HandleCreateChatMessage menyimpan satu pesan lalu memfanout-nya.
//
// Urutannya penting: simpan dulu, siarkan sesudah. Siaran yang mendahului
// penyimpanan akan menampilkan pesan yang bisa hilang saat halaman dimuat ulang.
func (s *Server) HandleCreateChatMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "user tidak terautentikasi")
		return
	}

	var req createChatMessageRequest
	if !s.decodeBody(w, r, &req) {
		return
	}

	body := strings.TrimSpace(req.Message)
	if body == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "message wajib diisi")
		return
	}
	// Dihitung dalam rune, bukan byte: 500 byte memotong teks non-Latin jauh
	// lebih pendek daripada 500 karakter yang dijanjikan ke user.
	if len([]rune(body)) > maxChatBodyLen {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "message maksimal 500 karakter")
		return
	}

	channelID := strings.TrimSpace(req.ChannelID)
	if channelID == "" {
		channelID = store.GlobalChannelID
	}
	identity, err := s.chatIdentity(r, userID)
	if err != nil {
		s.writeChatMembershipError(w, err)
		return
	}
	if !chatChannelAllowed(identity, channelID) {
		writeError(w, http.StatusForbidden, "PERMISSION_DENIED", "kanal bukan keanggotaan user")
		return
	}

	// Rate limit setelah validasi, sebelum penulisan: permintaan yang cacat
	// bentuknya tidak boleh menghabiskan kuota kirim user, tapi permintaan yang
	// valid harus melewati rem sebelum menyentuh basis data.
	//
	// Idempotency retry ikut terkena rem — itu disengaja: klien mengulang setelah
	// timeout, bukan dalam dua detik.
	allowed, err := s.limiter.Allow(r.Context(), "chat_send:"+userID, chatSendWindow)
	if err != nil {
		s.log.Error("gagal memeriksa rate limit chat", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal memeriksa batas kirim")
		return
	}
	if !allowed {
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "tunggu sebentar sebelum mengirim lagi")
		return
	}

	saved, err := s.repo.InsertChatMessage(
		r.Context(), channelID, userID,
		identity.Pseudonym, identity.LocationName, body, strings.TrimSpace(req.ClientMessageID),
	)
	if err != nil {
		s.log.Error("gagal menyimpan pesan chat", "err", err, "channel_id", channelID)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal menyimpan pesan")
		return
	}

	if s.chat != nil {
		s.chat.BroadcastChat(ChatEvent{
			MessageID:       saved.MessageID,
			ChannelID:       saved.ChannelID,
			SenderID:        saved.SenderID,
			SenderPseudonym: saved.SenderPseudonym,
			LocationTag:     saved.LocationTag,
			Body:            saved.Body,
			IsAdmin:         saved.IsAdmin,
			CreatedAt:       saved.CreatedAt,
		})
	}

	writeJSON(w, http.StatusCreated, toChatMessageDTO(*saved))
}

// chatIdentity membaca identitas chat pemanggil (pseudonym untuk snapshot
// pengirim, region untuk keanggotaan kanal).
func (s *Server) chatIdentity(r *http.Request, userID string) (*store.UserChatIdentity, error) {
	return s.repo.GetUserChatIdentity(r.Context(), userID)
}

// chatChannelAllowed adalah aturan keanggotaan, seluruhnya: global untuk semua,
// dan tepat satu kanal regional yaitu region dari posisi terakhir yang disinkron.
// Keanggotaan DITURUNKAN, tidak pernah di-join secara eksplisit, sehingga tidak
// ada state keanggotaan yang bisa basi terhadap lokasi user.
func chatChannelAllowed(identity *store.UserChatIdentity, channelID string) bool {
	if channelID == store.GlobalChannelID {
		return true
	}
	return identity.RegionCode != "" && identity.RegionCode == channelID
}

// assertChatMember menolak akses baca ke kanal yang bukan keanggotaan pemanggil.
// Riwayat kanal regional dibatasi seperti hak tulisnya: ruang yang bisa dibaca
// siapa saja bukan lagi ruang bagi wilayah itu.
func (s *Server) assertChatMember(r *http.Request, userID, channelID string) error {
	identity, err := s.repo.GetUserChatIdentity(r.Context(), userID)
	if err != nil {
		return err
	}
	if !chatChannelAllowed(identity, channelID) {
		return store.ErrChannelForbidden
	}
	return nil
}

// writeChatMembershipError memetakan galat identitas/keanggotaan ke respons HTTP,
// agar ketiga handler chat menjawab kasus yang sama dengan kode yang sama.
func (s *Server) writeChatMembershipError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrChannelForbidden):
		writeError(w, http.StatusForbidden, "PERMISSION_DENIED", "kanal bukan keanggotaan user")
	case errors.Is(err, store.ErrUserNotFound):
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "profil user tidak ditemukan")
	default:
		s.log.Error("gagal memeriksa keanggotaan kanal", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal memeriksa kanal")
	}
}
