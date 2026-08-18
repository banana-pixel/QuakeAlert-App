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
}

// client membungkus satu koneksi WebSocket dengan channel kirim ber-buffer.
type client struct {
	conn *websocket.Conn
	send chan []byte
}

// Hub mengelola set klien aktif dan broadcast non-blocking.
type Hub struct {
	mu       sync.RWMutex
	clients  map[*client]struct{}
	log      *slog.Logger
	upgrader websocket.Upgrader
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

// ServeWS meng-upgrade koneksi HTTP menjadi WebSocket dan mendaftarkannya.
// Autentikasi (JWT) diasumsikan sudah divalidasi di middleware sebelum handler
// ini (life-safety: endpoint jangan dibiarkan tanpa auth di produksi).
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Warn("gagal upgrade websocket", "err", err)
		return
	}
	c := &client{conn: conn, send: make(chan []byte, clientSendBuffer)}

	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	h.log.Info("klien ws terhubung", "total", h.Count())

	go h.writePump(c)
	go h.readPump(c) // membaca untuk mendeteksi close/ping; pesan masuk diabaikan.
}

// Broadcast mengirim AlertMessage ke semua klien secara non-blocking.
func (h *Hub) Broadcast(msg *AlertMessage) {
	payload, err := json.Marshal(msg)
	if err != nil {
		h.log.Error("gagal marshal alert ws", "err", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		select {
		case c.send <- payload:
		default:
			// Buffer penuh -> klien lambat; drop pesan untuk klien ini.
			h.log.Warn("buffer klien ws penuh, pesan di-drop untuk 1 klien")
		}
	}
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
	for payload := range c.send {
		_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
		if err := c.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			h.log.Debug("write ws gagal, tutup klien", "err", err)
			break
		}
	}
	h.remove(c)
}

func (h *Hub) readPump(c *client) {
	defer h.remove(c)
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}
