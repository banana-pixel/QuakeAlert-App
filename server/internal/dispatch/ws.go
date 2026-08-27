// Package dispatch menyebarkan event konsensus ke klien: WebSocket Hub (WSS)
// untuk klien foreground dan FCM Admin SDK v1 untuk delivery background.
// Lihat docs/SYSTEM_SPEC.md Bab 3 (Dispatch Tier) & .clinerules/10.
package dispatch

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// clientSendBuffer adalah kapasitas buffer per klien. Non-blocking: bila penuh,
// klien lambat di-drop agar broadcast tidak memblokir seluruh hub (life-safety:
// satu klien macet tak boleh menahan peringatan ke klien lain).
const clientSendBuffer = 16

// writeWait adalah deadline penulisan ke soket klien.
const writeWait = 5 * time.Second

// pongWait adalah deadline membaca data/pong dari klien. Bila klien tidak
// mengirim apa pun dalam pongWait (koneksi mati di jaringan seluler, switch
// network, dsb.), readPump menutup koneksi sehingga klien zombie tidak menumpuk.
const pongWait = 30 * time.Second

// pingPeriod adalah interval pengiriman PingMessage oleh writePump. Harus lebih
// kecil dari pongWait agar klien yang sehat selalu balas pong sebelum deadline.
const pingPeriod = (pongWait * 9) / 10 // 27s

// maxMessageSize membatasi ukuran frame yang dibaca klien. Klien tidak mengirim
// pesan berarti (pesan masuk dibuang), jadi limit kecil mencegah penyalahgunaan
// memori sambil tetap menerima ping/pong/close control frame.
const maxMessageSize = 1024

// AlertMessage adalah payload broadcast WebSocket. Bentuk JSON disederhanakan
// dan konsisten dengan kontrak FCM (satuan: pga gal, timestamp ms epoch UTC).
type AlertMessage struct {
	Type           string  `json:"type"` // EARTHQUAKE_ALERT | EARTHQUAKE_ADVISORY | EVENT_RESOLVED
	EventID        string  `json:"event_id"`
	MMI            string  `json:"mmi"`
	IntensityLabel string  `json:"intensity_label"`
	PGAGal         float64 `json:"pga_gal"`
	CentroidLat    float64 `json:"centroid_lat"`
	CentroidLon    float64 `json:"centroid_lon"`
	LocationName   string  `json:"location_name"`
	Timestamp      int64   `json:"timestamp"` // ms epoch UTC
	NodeCount      int     `json:"node_count"`
	// IsTest menandai peringatan LATIHAN. omitempty, jadi payload gempa
	// sungguhan tetap sama persis seperti sebelum flag ini ada — dan sebuah
	// alert tanpa field ini tidak mungkin ditafsirkan sebagai drill.
	IsTest bool `json:"is_test,omitempty"`

	// Enam field siklus hidup Fase 3 (§8.3). SELURUHNYA aditif dan seluruhnya
	// omitempty: jalur Fase 2 tidak mengisi satu pun, sehingga frame yang
	// dihasilkannya tetap BYTE-IDENTIK dengan frame hari ini dan flag
	// EVENT_TRACKER_ENABLED tidak dapat mengubah kontrak secara diam-diam.
	//
	// type TIDAK bertambah nilainya (D11): state siklus hidup dibawa
	// event_state, sehingga klien terpasang yang belum tahu apa-apa soal Fase 3
	// tetap memahami setiap frame.
	EventState           string `json:"event_state,omitempty"`      // UNCONFIRMED|CONFIRMED|RESOLVED|CANCELLED
	EventRevision        int    `json:"event_revision,omitempty"`   // monoton per event
	OriginTS             int64  `json:"origin_ts,omitempty"`        // ms epoch UTC, jangkar onset (§4.3)
	OriginTSSource       string `json:"origin_ts_source,omitempty"` // SENSOR|PUBLISH_BOUND — kejujuran tentang apa origin_ts itu
	IndependentCellCount int    `json:"independent_cell_count,omitempty"`
}

// chatBufferCeiling menjaga separuh buffer per-klien tetap kosong untuk alert.
// Frame chat hanya masuk bila antrean masih di bawah ambang ini, sehingga
// percakapan yang ramai tidak pernah memakan tempat yang dibutuhkan sebuah
// peringatan gempa (life-safety: chat boleh hilang, alert tidak).
const chatBufferCeiling = clientSendBuffer / 2

// ChatMessage adalah frame chat yang difanout ke klien satu kanal. Bentuknya
// mengikuti AlertMessage: satu socket, satu envelope ber-"type", sehingga klien
// tidak perlu koneksi kedua dan jalur reconnect kedua.
type ChatMessage struct {
	Type            string `json:"type"` // CHAT_MESSAGE
	MessageID       string `json:"message_id"`
	ChannelID       string `json:"channel_id"`
	SenderID        string `json:"sender_id"`
	SenderPseudonym string `json:"sender_pseudonym"`
	SenderLocation  string `json:"sender_location_tag,omitempty"`
	Message         string `json:"message"`
	IsAdmin         bool   `json:"is_admin"`
	Timestamp       int64  `json:"timestamp"` // ms epoch UTC, seperti AlertMessage
}

// BroadcastMessage adalah pengumuman operator yang difanout ke klien.
//
// Envelope yang sama dengan AlertMessage dan ChatMessage — satu socket, satu
// bentuk ber-"type" — sehingga klien tidak perlu koneksi ketiga hanya untuk
// membaca pengumuman.
type BroadcastMessage struct {
	Type        string `json:"type"` // ADMIN_BROADCAST
	BroadcastID string `json:"broadcast_id"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	// RegionCode kosong berarti nasional; bila terisi, hanya anggota kanal
	// dengan kunci itu yang menerimanya.
	RegionCode string `json:"region_code,omitempty"`
	Timestamp  int64  `json:"timestamp"` // ms epoch UTC, seperti AlertMessage
}

// ChannelResolver menjawab kanal chat mana yang boleh diterima sebuah koneksi.
//
// Sebuah fungsi yang di-inject, bukan lookup di dalam paket ini: dispatch tidak
// tahu apa-apa soal autentikasi maupun basis data, dan cmd/quakealert yang
// menjembatani keduanya. Mengembalikan nil berarti koneksi hanya menerima alert.
type ChannelResolver func(r *http.Request) []string

// client membungkus satu koneksi WebSocket dengan channel kirim ber-buffer.
type client struct {
	conn *websocket.Conn
	send chan []byte
	// channels adalah keanggotaan kanal chat yang diambil SEKALI saat upgrade.
	// Snapshot, bukan langganan hidup: koneksi ini berumur pendek dan klien yang
	// wilayahnya berubah akan menyambung ulang, jadi keanggotaan yang menyegarkan
	// dirinya sendiri hanya menambah state yang bisa basi tanpa manfaat.
	channels map[string]struct{}
}

// Hub mengelola set klien aktif dan broadcast non-blocking.
type Hub struct {
	mu       sync.RWMutex
	clients  map[*client]struct{}
	log      *slog.Logger
	upgrader websocket.Upgrader
	// resolveChannels boleh nil: hub tetap menyiarkan alert, hanya tanpa chat.
	resolveChannels ChannelResolver
}

// NewHub membuat hub kosong. checkOrigin membatasi origin yang diizinkan
// (WSS; produksi harus memvalidasi origin—di sini default menolak lintas-origin
// kecuali di-set eksplisit oleh caller).
func NewHub(log *slog.Logger, allowOrigin func(r *http.Request) bool) *Hub {
	if allowOrigin == nil {
		allowOrigin = func(_ *http.Request) bool { return false }
	}
	return &Hub{
		clients: make(map[*client]struct{}),
		log:     log,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     allowOrigin,
		},
	}
}

// SetChannelResolver memasang penentu keanggotaan kanal chat. Dipanggil dari
// cmd/quakealert setelah store tersedia; tanpa itu hub berperilaku seperti
// sebelumnya, yaitu hanya menyiarkan alert.
func (h *Hub) SetChannelResolver(resolver ChannelResolver) {
	h.mu.Lock()
	h.resolveChannels = resolver
	h.mu.Unlock()
}

// ServeWS meng-upgrade koneksi HTTP menjadi WebSocket dan mendaftarkannya.
// Autentikasi (JWT) diasumsikan sudah divalidasi di middleware sebelum handler
// ini (life-safety: endpoint jangan dibiarkan tanpa auth di produksi).
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	// Keanggotaan dibaca SEBELUM upgrade: sesudahnya request sudah dibajak dan
	// context-nya (tempat userID berada) tidak lagi dapat diandalkan.
	h.mu.RLock()
	resolver := h.resolveChannels
	h.mu.RUnlock()
	var channels map[string]struct{}
	if resolver != nil {
		if ids := resolver(r); len(ids) > 0 {
			channels = make(map[string]struct{}, len(ids))
			for _, id := range ids {
				channels[id] = struct{}{}
			}
		}
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Warn("gagal upgrade websocket", "err", err)
		return
	}
	c := &client{conn: conn, send: make(chan []byte, clientSendBuffer), channels: channels}

	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	h.log.Info("klien ws terhubung", "total", h.Count())

	go h.writePump(c)
	go h.readPump(c) // membaca untuk mendeteksi close/ping; pesan masuk diabaikan.
}

// Broadcast mengirim AlertMessage ke semua klien secara non-blocking dan
// mengembalikan jumlah klien yang benar-benar menerima frame ini.
//
// Yang dihitung adalah ENQUEUE yang berhasil, bukan len(h.clients): klien lambat
// yang buffer-nya penuh tidak mendapatkan peringatan ini, dan menghitungnya
// sebagai penerima akan membuat ledger melaporkan jangkauan yang tidak pernah
// terjadi. Nilai kembali boleh diabaikan (jalur uji/operator melakukannya), jadi
// penambahannya tidak mengubah satu pun pemanggil yang ada.
func (h *Hub) Broadcast(msg *AlertMessage) int {
	payload, err := json.Marshal(msg)
	if err != nil {
		h.log.Error("gagal marshal alert ws", "err", err)
		return 0
	}

	sent := 0
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		select {
		case c.send <- payload:
			sent++
		default:
			// Buffer penuh -> klien lambat; drop pesan untuk klien ini.
			h.log.Warn("buffer klien ws penuh, pesan di-drop untuk 1 klien")
		}
	}
	return sent
}

// BroadcastChat mengirim satu frame chat hanya ke klien yang menjadi anggota
// kanalnya.
//
// Berbeda dari Broadcast dalam satu hal yang penting: frame chat dilewati bila
// antrean klien sudah melewati chatBufferCeiling, dan klien TIDAK ditutup.
// Pesan chat yang hilang akan muncul kembali saat klien memuat riwayat lewat
// REST; peringatan yang hilang tidak punya jalan kembali.
func (h *Hub) BroadcastChat(msg *ChatMessage) {
	payload, err := json.Marshal(msg)
	if err != nil {
		h.log.Error("gagal marshal chat ws", "err", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		if _, ok := c.channels[msg.ChannelID]; !ok {
			continue
		}
		if len(c.send) >= chatBufferCeiling {
			// Sisa buffer disimpan untuk alert; chat dilewati tanpa menutup klien.
			h.log.Debug("buffer klien ws menipis, frame chat dilewati")
			continue
		}
		select {
		case c.send <- payload:
		default:
			h.log.Debug("buffer klien ws penuh, frame chat di-drop")
		}
	}
}

// BroadcastAdmin mengirim satu pengumuman operator. RegionCode kosong berarti
// seluruh klien; bila terisi, hanya klien yang menjadi anggota kanal dengan
// kunci itu — kunci kanal chat regional dan kunci penargetan siaran adalah
// nilai yang sama (user_profiles.region_code), jadi keanggotaan yang sudah
// diambil saat upgrade sekaligus menjawab siapa yang berhak menerima siaran.
//
// Diperlakukan seperti chat, bukan seperti alert, dalam satu hal yang penting:
// frame dilewati bila antrean klien sudah melewati chatBufferCeiling, dan klien
// tidak ditutup. Pengumuman yang hilang muncul kembali saat klien membuka daftar
// Pembaruan lewat REST; sebuah peringatan gempa tidak punya jalan kembali, jadi
// sisa buffer selalu miliknya.
func (h *Hub) BroadcastAdmin(msg *BroadcastMessage) {
	payload, err := json.Marshal(msg)
	if err != nil {
		h.log.Error("gagal marshal siaran ws", "err", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	sent := 0
	for c := range h.clients {
		if msg.RegionCode != "" {
			if _, ok := c.channels[msg.RegionCode]; !ok {
				continue
			}
		}
		if len(c.send) >= chatBufferCeiling {
			h.log.Debug("buffer klien ws menipis, frame siaran dilewati")
			continue
		}
		select {
		case c.send <- payload:
			sent++
		default:
			h.log.Debug("buffer klien ws penuh, frame siaran di-drop")
		}
	}
	h.log.Info("siaran admin difanout via ws",
		"broadcast_id", msg.BroadcastID, "region", msg.RegionCode, "klien", sent)
}

// Count mengembalikan jumlah klien aktif.
func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (h *Hub) remove(c *client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
	}
	h.mu.Unlock()
	_ = c.conn.Close()
}

func (h *Hub) writePump(c *client) {
	defer h.remove(c)
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	for {
		select {
		case payload, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Channel ditutup oleh remove(): kirim CloseMessage lalu bersihkan.
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				h.log.Debug("write ws gagal, tutup klien", "err", err)
				return
			}
		case <-ticker.C:
			// Ping rutin: memaksa klien balas pong sehingga readPump dapat
			// mendeteksi koneksi mati melalui read deadline.
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				h.log.Debug("ping ws gagal, tutup klien", "err", err)
				return
			}
		}
	}
}

func (h *Hub) readPump(c *client) {
	defer h.remove(c)
	// Deadline baca diperpanjang setiap kali data/pong diterima. Tanpa ini,
	// koneksi TCP yang diam (matanya tak terlihat) tidak akan pernah ditutup.
	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}
