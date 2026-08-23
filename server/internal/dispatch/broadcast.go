package dispatch

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
)

// UpdatesTopic adalah topic FCM untuk siaran nasional — TERPISAH dari GeoTopic
// yang dipakai peringatan gempa.
//
// Dua topic, bukan satu, supaya kedua hal itu dapat dilepas satu per satu:
// seseorang yang lelah menerima pengumuman dapat berhenti berlangganan
// updates_all tanpa kehilangan sirene, dan tidak ada keadaan di mana usaha
// mengurangi kebisingan berakhir dengan mematikan peringatan gempa. Bila
// keduanya berbagi satu topic, "matikan berita" dan "matikan alert" menjadi
// tombol yang sama.
const UpdatesTopic = "updates_all"

// BuildBroadcastData membentuk map data-only untuk siaran admin. Sama seperti
// alert, SEMUA nilai bertipe string (batasan FCM), dan tidak ada blok
// "notification": klien yang memutuskan kanal dan tampilannya, sehingga sebuah
// pengumuman tidak pernah dirender oleh sistem seperti peringatan gempa.
func BuildBroadcastData(b *BroadcastMessage) map[string]string {
	return map[string]string{
		"type":         b.Type,
		"broadcast_id": b.BroadcastID,
		"title":        b.Title,
		"body":         b.Body,
		"region_code":  b.RegionCode,
		"timestamp":    strconv.FormatInt(b.Timestamp, 10),
	}
}

// DispatchBroadcast menyiarkan pengumuman operator yang SUDAH tersimpan ke dua
// kanal: klien WebSocket yang sedang terbuka, dan FCM untuk yang tidak.
//
// Prioritas NORMAL, bukan HIGH: HIGH ada untuk membangunkan perangkat dari Doze
// demi keselamatan hidup, dan memakainya untuk pengumuman adalah cara tercepat
// melatih pengguna — dan Android — memperlakukan kiriman aplikasi ini sebagai
// hal yang bisa diabaikan. Pengumuman yang tiba beberapa menit lebih lambat
// tidak merugikan siapa pun.
func (d *Dispatcher) DispatchBroadcast(msg *BroadcastMessage) {
	msg.Type = TypeBroadcast
	d.hub.BroadcastAdmin(msg)
	d.dispatchBroadcastFCM(msg)
}

// dispatchBroadcastFCM mengirim siaran lewat FCM, asinkron seperti jalur alert.
//
// Satu perbedaan yang disengaja dari dispatchFCM: siaran REGIONAL tidak pernah
// jatuh ke topic nasional. Bila tidak ada satu pun token di wilayah itu, siaran
// berhenti di WebSocket dan basis data. Fallback ke topic pada jalur gempa dapat
// dibenarkan karena membangunkan orang yang tidak perlu dibangunkan masih lebih
// baik daripada melewatkan yang perlu; pada pengumuman wilayah, mengirimkannya
// ke seluruh negeri hanya salah.
func (d *Dispatcher) dispatchBroadcastFCM(msg *BroadcastMessage) {
	if d.fcm == nil {
		return
	}
	data := BuildBroadcastData(msg)
	regional := msg.RegionCode != ""

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), fcmTimeout)
		defer cancel()

		if !regional {
			if err := d.fcm.Send(ctx, &FCMMessage{
				Topic:    UpdatesTopic,
				Data:     data,
				Priority: "NORMAL",
			}); err != nil {
				d.log.Error("gagal kirim FCM siaran nasional", "err", err,
					"broadcast_id", msg.BroadcastID)
			}
			return
		}

		tokens := d.regionTokens(ctx, msg.RegionCode)
		if len(tokens) == 0 {
			d.log.Info("siaran regional tanpa token FCM; hanya lewat ws dan basis data",
				"broadcast_id", msg.BroadcastID, "region", msg.RegionCode)
			return
		}
		d.sendBroadcastToTokens(ctx, tokens, data, msg)
	}()
}

// regionTokens mengembalikan token pada satu region_code, atau nil bila store
// tidak mendukung pencarian itu / query gagal.
func (d *Dispatcher) regionTokens(ctx context.Context, regionCode string) []string {
	finder, ok := d.saver.(regionTokenFinder)
	if !ok {
		return nil
	}
	tokens, err := finder.FCMTokensInRegion(ctx, regionCode)
	if err != nil {
		d.log.Error("gagal cari token FCM per wilayah", "err", err, "region", regionCode)
		return nil
	}
	return tokens
}

// sendBroadcastToTokens mengirim satu message per token dengan paralelisme yang
// sama dengan jalur alert. Kegagalan per token hanya dicatat: satu token mati
// tidak boleh menghentikan pengiriman ke perangkat lain pada siaran yang sama.
func (d *Dispatcher) sendBroadcastToTokens(
	ctx context.Context,
	tokens []string,
	data map[string]string,
	msg *BroadcastMessage,
) {
	sem := make(chan struct{}, maxFCMConcurrency)
	var wg sync.WaitGroup
	var failed atomic.Int64

	for _, token := range tokens {
		wg.Add(1)
		sem <- struct{}{}
		go func(token string) {
			defer wg.Done()
			defer func() { <-sem }()
			err := d.fcm.Send(ctx, &FCMMessage{
				Token:    token,
				Data:     data,
				Priority: "NORMAL",
			})
			if err != nil {
				failed.Add(1)
				// Token tidak ikut dicatat: itu identifier perangkat.
				d.log.Warn("gagal kirim FCM siaran ke token", "err", err,
					"broadcast_id", msg.BroadcastID)
			}
		}(token)
	}
	wg.Wait()

	d.log.Info("siaran regional terkirim via FCM",
		"broadcast_id", msg.BroadcastID, "region", msg.RegionCode,
		"tokens", len(tokens), "gagal", failed.Load())
}
