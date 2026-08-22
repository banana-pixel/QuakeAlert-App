package dispatch

import (
	"encoding/json"
	"io"
	"log/slog"
	"testing"
)

// addClient mendaftarkan klien tiruan langsung ke hub. BroadcastChat/Broadcast
// tidak menyentuh conn, hanya channel send, sehingga penyaringan kanal dan
// prioritas alert dapat diuji tanpa soket sungguhan.
func addClient(h *Hub, channels ...string) *client {
	c := &client{send: make(chan []byte, clientSendBuffer)}
	if len(channels) > 0 {
		c.channels = make(map[string]struct{}, len(channels))
		for _, id := range channels {
			c.channels[id] = struct{}{}
		}
	}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	return c
}

func testHub() *Hub {
	return NewHub(slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
}

func TestBroadcastChat_ReachesOnlyTheChannelMembers(t *testing.T) {
	h := testHub()
	member := addClient(h, "global", "ID-jawa-barat")
	outsider := addClient(h, "global")
	alertOnly := addClient(h) // koneksi tanpa resolver: hanya alert

	h.BroadcastChat(&ChatMessage{Type: "CHAT_MESSAGE", ChannelID: "ID-jawa-barat", Message: "aman"})

	if len(member.send) != 1 {
		t.Fatalf("anggota kanal menerima %d frame, mau 1", len(member.send))
	}
	if len(outsider.send) != 0 {
		t.Fatalf("bukan anggota menerima %d frame, mau 0", len(outsider.send))
	}
	if len(alertOnly.send) != 0 {
		t.Fatalf("klien tanpa kanal menerima %d frame, mau 0", len(alertOnly.send))
	}

	var got ChatMessage
	if err := json.Unmarshal(<-member.send, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Type != "CHAT_MESSAGE" || got.Message != "aman" {
		t.Fatalf("frame = %+v", got)
	}
}

func TestBroadcastChat_LeavesRoomForAlerts(t *testing.T) {
	h := testHub()
	c := addClient(h, "global")

	// Isi antrean sampai ambang chat, lalu buktikan chat berhenti tapi alert
	// masih masuk: chat boleh hilang, peringatan gempa tidak.
	for i := 0; i < chatBufferCeiling; i++ {
		c.send <- []byte("x")
	}
	h.BroadcastChat(&ChatMessage{Type: "CHAT_MESSAGE", ChannelID: "global", Message: "halo"})
	if len(c.send) != chatBufferCeiling {
		t.Fatalf("antrean = %d, mau tetap %d karena chat dilewati", len(c.send), chatBufferCeiling)
	}

	h.Broadcast(&AlertMessage{Type: "EARTHQUAKE_ALERT", EventID: "evt-1"})
	if len(c.send) != chatBufferCeiling+1 {
		t.Fatalf("antrean = %d, mau %d karena alert tetap masuk", len(c.send), chatBufferCeiling+1)
	}

	// Klien yang chat-nya dilewati TIDAK ditutup: pesan chat yang hilang muncul
	// kembali lewat REST, jadi memutus koneksinya hanya membuang jalur alert.
	if h.Count() != 1 {
		t.Fatalf("jumlah klien = %d, mau 1", h.Count())
	}
}
