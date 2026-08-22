# CHAT DESIGN — Community Chat (server-backed)

Desain kanal chat komunitas QuakeAlert: model kanal, kontrak REST, jalur realtime,
dan batasannya. Dokumen ini mengikat implementasi `server/internal/api` +
`server/internal/dispatch` dan klien `android/.../ui/chat`.

**Status:** desain untuk potongan pertama (migrasi `000003`). Ditulis sebelum kode
agar keputusan yang sulit dibalik — terutama kunci kanal — tercatat lebih dulu.

---

## 1. Bukan mesh

Layar Chat di desain menyebut "West Java Mesh" dan KDoc `ChatViewModel` menyebut
"mesh-network transport". Keduanya sisa draf awal. `SYSTEM_SPEC.md` Bab 4 sudah
mengganti nama tabel `mesh_chat_messages` → `chat_messages` dengan alasan istilah
"mesh" menyesatkan: arsitekturnya **server-backed**.

Mesh (BLE/Wi-Fi Aware peer-to-peer) ditolak untuk potongan ini, bukan karena tidak
menarik tetapi karena tiga hal yang tidak bisa dipenuhi:

* **Jangkauan.** BLE ~10–100 m. Gempa dirasakan puluhan kilometer; kanal yang hanya
  menjangkau satu blok tidak menjawab "apa yang terjadi di kota saya".
* **Ketersediaan.** Mesh butuh massa perangkat yang saling dekat *dan* aplikasinya
  terpasang. Pada peluncuran, jumlah itu nol.
* **Biaya rawat.** Routing, dedup, dan keamanan store-and-forward adalah subsistem
  tersendiri — tanpa moderasi, tanpa retensi, tanpa identitas.

Yang tetap benar dari ide mesh: pesan harus berguna saat jaringan buruk. Itu dijawab
dengan pengiriman idempoten + retry di klien (§6), bukan dengan transport P2P.

---

## 2. Model kanal — dua tingkat tetap

| Tingkat | `channel_id` | Keanggotaan | Kapan berguna |
|---|---|---|---|
| Global | `global` | semua user | selalu; satu-satunya kanal yang jalan sebelum ada fix lokasi |
| Regional | `<ISO2>-<admin1-slug>` mis. `ID-jawa-barat` | region dari posisi tersinkron terakhir | "gempa di dekat saya" |

Keanggotaan **diturunkan, tidak pernah di-join eksplisit**: kanal regional Anda adalah
region dari `last_location` yang tersimpan di server. Tidak ada fix, atau region yang
tidak bisa dipetakan → hanya Global, dan tab mengatakan alasannya alih-alih menampilkan
ruang kosong.

Kunci regional diturunkan dari reverse-geocode yang **sudah** mengisi `location_name`
(migrasi `000002`): klien mengirim `country_iso` + `admin_area` pada
`PUT /api/v1/users/location`, server menormalkan ke slug dan menyimpannya di
`user_profiles.region_code`. Tidak ada dataset baru, tidak ada izin baru.

**Granularitas admin1 (provinsi/negara bagian) disengaja:** cukup kasar sehingga
keanggotaan kanal bukan pengungkapan lokasi, cukup besar untuk berisi orang, dan cukup
spesifik agar "gempa tadi" masih relevan.

**Ditolak:**

* *Kanal berbasis radius* ("semua dalam 50 km") — keanggotaan berbeda per user, jadi
  tidak ada riwayat bersama untuk dipaginasi dan jumlah anggota tak terjawab.
* *Sel geohash* — stabil tapi tak bisa dinamai, dan presisi yang cukup berguna sudah
  cukup untuk membocorkan tempat tinggal.

**Ditunda (layak menyusul):** kanal per-event untuk gempa aktif — tempat sebenarnya
lalu lintas "kamu aman?" bermuara. Butuh siklus hidup kanal (buka saat confirm, tutup
setelah resolve) yang tidak dibutuhkan dua tingkat statis di atas.

---

## 3. Skema

`chat_messages` **sudah ada** sejak migrasi `000001` (`message_id`, `channel_id`,
`sender_id`, `sender_pseudonym`, `sender_location_tag`, `message`, `is_admin`,
`created_at`, indeks `(channel_id, created_at DESC)`). Migrasi `000003` karena itu
hanya bersifat aditif:

```sql
ALTER TABLE user_profiles ADD COLUMN IF NOT EXISTS region_code VARCHAR(64);

CREATE TABLE IF NOT EXISTS chat_channels (
    channel_id   VARCHAR(50) PRIMARY KEY,
    kind         VARCHAR(16) NOT NULL,   -- 'GLOBAL' | 'REGIONAL'
    display_name VARCHAR(80) NOT NULL,
    created_at   TIMESTAMPTZ DEFAULT NOW()
);
INSERT INTO chat_channels (channel_id, kind, display_name)
VALUES ('global', 'GLOBAL', 'Global') ON CONFLICT DO NOTHING;
```

Catatan desain:

* **`sender_pseudonym` adalah snapshot, bukan join.** Pseudonym bisa di-reroll
  (`POST /users/pseudonym/reroll`); riwayat tidak boleh berubah nama secara retroaktif.
* **Baris `chat_channels` regional dibuat saat pertama dipakai**, menyimpan
  `display_name` dari penulis pertama, sehingga semua anggota melihat judul yang sama
  alih-alih ejaan masing-masing ponsel.
* **Retensi 7 hari** sudah dicatat di `SYSTEM_SPEC.md` Bab 4 (pg_cron / partisi /
  goroutine terjadwal). Potongan pertama memakai goroutine terjadwal di backend,
  karena pg_cron belum tentu ada di host produksi.

---

## 4. Kontrak REST

Keduanya di bawah grup terautentikasi yang sudah ada di `internal/api/router.go`, jadi
identitas anonim (JWT) dipakai ulang; tidak ada jalur auth baru.

```
GET  /api/v1/chat/channels
GET  /api/v1/chat/messages?channel_id=<id>&limit=<n>&before=<RFC3339>
POST /api/v1/chat/messages   { "channel_id": "...", "message": "...", "client_message_id": "<uuid>" }
```

* `GET /chat/channels` menjawab kanal yang **boleh** diakses pemanggil: selalu
  `global`, plus kanal regionalnya bila `region_code` terisi. Klien tidak menebak kunci.
* `GET /chat/messages` paginasi DESC dengan kursor `before` (waktu, bukan offset:
  ruang yang aktif menggeser offset di antara halaman). `limit` default 30, maks 100.
* `POST` menolak kanal yang bukan keanggotaan pemanggil dengan **403** — keanggotaan
  ditegakkan di server, tidak dipercayakan ke klien.
* `client_message_id` membuat pengiriman **idempoten**: retry setelah jaringan putus
  mengembalikan pesan yang sama, bukan duplikat. Disimpan dengan indeks unik
  `(sender_id, client_message_id)`.
* Batas isi **500 karakter** (rune, bukan byte) (`INVALID_ARGUMENT` bila lebih), dan body kosong/whitespace
  ditolak.
* Rate limit per user via `internal/api/ratelimit.go`: 1 pesan / 2 detik untuk `POST`.

---

## 5. Realtime

`dispatch/ws.go` hari ini broadcast-only dan membuang frame masuk. Hub **diperluas**,
bukan diduplikasi dengan soket kedua, sehingga alert dan chat berbagi satu koneksi,
satu jalur auth, dan satu logika reconnect:

```json
{"type":"CHAT_MESSAGE","channel_id":"ID-jawa-barat","message_id":"…",
 "sender_id":"…","sender_pseudonym":"Quakezen-7B9A","sender_location_tag":"Bandung",
 "message":"…","is_admin":false,"timestamp":1723891234120}
```

* **Kirim tetap REST**, socket hanya menyebar apa yang sudah tersimpan. Chat karena itu
  durabel dan bisa di-retry; socket boleh gagal tanpa pesan hilang.
* **Alert tetap prioritas.** Frame chat lewat buffer per-klien non-blocking yang sama;
  klien lambat kehilangan chat, **tidak pernah** kehilangan alert. Implementasi: kirim
  chat dengan `select`/`default` seperti sekarang, dan bila buffer penuh drop frame chat
  — bukan menutup klien.
* **Stempel waktu socket adalah ms epoch**, bukan RFC3339 seperti REST — sama seperti
  frame alert di soket yang sama, sehingga satu parser menangani keduanya.
* Frame masuk tetap dibuang (`maxMessageSize` kecil dipertahankan). Socket bukan jalur
  tulis, jadi tidak ada permukaan serangan baru di sisi baca.
* Fan-out disaring per kanal: klien menerima frame kanal yang ia ikuti. Karena itu
  koneksi WS kini menyimpan `userID` + daftar kanal saat upgrade.

---

## 6. Klien Android

* `ChatViewModel` kehilangan `mockConversation()` dan KDoc "mesh-network transport";
  memakai repository berbasis `QuakeApiClient`.
* **Optimistic send**: bubble muncul langsung dengan status `SENDING`, menjadi `SENT`
  saat POST berhasil (ditandai `client_message_id`, jadi frame WS yang datang untuk
  pesan sendiri menggantikan bubble optimistik alih-alih menambah yang kedua), dan
  `FAILED` dengan aksi ulang bila gagal.
* Paginasi ke atas dengan kursor `before`.
* `ChatChannelInfo` kehilangan "West Java Mesh"/12 users hardcoded; menjadi pemilih dua
  tingkat dengan nama dari `GET /chat/channels`.
* Tanpa fix lokasi: hanya Global, dengan alasan yang tertulis dan tautan ke sync lokasi.

---

## 7. Yang belum dibangun (jujur dicatat)

Chat adalah konten buatan pengguna pertama di aplikasi ini, artinya juga permukaan
penyalahgunaan pertama:

* **Moderasi & pelaporan belum ada.** Tidak ada block, report, mute, atau hapus oleh
  admin. Kolom `is_admin` ada tapi belum dipakai.
* **Tidak ada verifikasi identitas.** Pseudonym anonim bisa di-reroll; "Rescue Team"
  di mock bukan identitas terverifikasi dan tidak boleh dibuat seolah begitu.
* **Tidak ada moderasi konten otomatis** untuk misinformasi saat gempa — risiko nyata
  pada kanal darurat.

Konsekuensi: chat dirilis sebagai kanal komunitas, **bukan** kanal instruksi resmi.
Peringatan resmi tetap datang lewat alert (WS + FCM), yang tidak bisa ditulis pengguna.
