package dispatch

import (
	"context"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/ledger"
)

// DispatchEventFrame adalah jalur emisi Fase 3: SATU frame yang sudah selesai
// diputuskan oleh event.Tracker, dikirim ke kanal klien apa adanya.
//
// Perbedaannya dari Dispatch bukan kosmetik. Dispatch memutuskan sendiri (status
// -> tipe), mempersistensi event secara SINKRON untuk mendapatkan event_id, dan
// memasang timer resolusinya sendiri. Ketiganya pindah ke Tracker pada Fase 3:
// identitas dibuat di Go sebelum penulisan mana pun (§4.1), persistensi mengikuti
// emisi dan boleh gagal (§9.5), dan resolusi dimiliki satu sweeper (§5.4). Yang
// tersisa untuk dispatcher adalah pengiriman — dan itulah seluruh isi fungsi ini.
//
// push memisahkan kanal, bukan tipe: UNCONFIRMED disiarkan lewat WebSocket dan
// TIDAK mendorong FCM (D10), sedangkan RESOLVED/CANCELLED mendorong hanya bila
// event-nya pernah CONFIRMED (§8.1). Keputusan itu dibuat pemanggil dari
// snapshot; di sini ia hanya dijalankan.
func (d *Dispatcher) DispatchEventFrame(ctx context.Context, msg *AlertMessage, push bool) {
	if msg == nil {
		return
	}

	// 1. WebSocket lebih dulu, selalu, untuk setiap transisi. Non-blocking.
	wsCount := d.hub.Broadcast(msg)

	// 2. FCM hanya bila transisi ini berhak atasnya.
	if push {
		d.dispatchFCM(msg, wsCount)
		return
	}

	// Tanpa FCM, baris alert_emissions tetap ditulis: §8.5 mengharuskan setiap
	// frame yang MUNGKIN diterima klien dapat direkonstruksi, dan sebuah frame
	// WebSocket-saja adalah frame yang diterima klien.
	//
	// fcmConfigured mengikuti apakah FCM ADA, bukan apakah ia dipakai. Dengan
	// begitu "dikonfigurasi tetapi sengaja tidak mengirim" tercatat sebagai
	// fcm_attempted = 0 — nol yang teramati, sama seperti guard satu-node —
	// sementara instalasi tanpa kredensial tetap NULL.
	d.recordEmission(msg, ledger.AudienceNone, time.Now().UnixMilli(), delivery{
		wsClients: wsCount, fcmConfigured: d.fcm != nil,
	})
}
