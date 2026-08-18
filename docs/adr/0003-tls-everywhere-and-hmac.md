# ADR-0003: TLS Everywhere & Manajemen Secret HMAC

- **Status:** Accepted
- **Tanggal:** 2026-08-18

## Konteks

Ini adalah sistem *life-safety*. Peringatan palsu (spoofed trigger) atau peringatan yang tidak sampai bisa berakibat fatal atau memicu kepanikan massal. Draf awal menyimpan secret sensor sebagai `secret_key_hash` dan menyebut port MQTT plaintext, keduanya salah untuk kebutuhan integritas & kerahasiaan.

## Keputusan

1. **Enkripsi transport wajib di semua jalur:**
   - MQTT → MQTTS port **8883** dengan validasi CA. Port 1883 plaintext dilarang di produksi.
   - REST → **HTTPS** saja.
   - WebSocket → **WSS** saja.
2. **HMAC untuk integritas & autentikasi payload sensor** (bukan kerahasiaan):
   - `HMAC-SHA256(node_id|pga|dur_ms|ts, secret_key)`, output 64-hex.
   - Kanonikalisasi string identik antara firmware & server (lihat `/contracts/mqtt`).
3. **Penyimpanan secret di server: terenkripsi-reversibel, BUKAN hash.**
   - Kolom `secret_key_enc` (ciphertext AES-256-GCM) + `secret_key_nonce` + `key_version`.
   - Master key dari secret manager/ENV (`HMAC_MASTER_KEY`), tidak pernah di-commit.
   - Alasan: verifikasi HMAC memerlukan key mentah untuk menghitung ulang signature; hash satu arah tak bisa dipakai.
4. **Anti-replay:**
   - Tolak trigger dengan `ts` menyimpang > 30s dari waktu server.
   - Simpan `last_seen_ts` per node; tolak `ts` ≤ nilai terakhir (mencegah replay & duplikat).
5. **Provisioning secret** ditampilkan **sekali** di response API, tidak pernah dikembalikan lagi.

## Konsekuensi

- (+) Trigger tidak bisa dipalsukan tanpa secret; payload tak bisa di-replay.
- (+) Transport terenkripsi mencegah sniffing & MITM.
- (−) Perlu manajemen sertifikat (broker) & master key rotation (`key_version` sudah disiapkan).
- (−) Sinkronisasi jam node penting; toleransi 30s + `ts` monotonic membantu.

## Alternatif ditolak

- **Menyimpan hash secret:** tidak bisa memverifikasi HMAC (fatal, kontradiksi draf awal).
- **Plaintext MQTT + HMAC saja:** payload & metadata terekspos, rawan analisis lalu lintas.
