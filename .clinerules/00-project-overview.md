# 00 — Project Overview & Golden Rules

QuakeAlert adalah **Community-Based Earthquake Early Warning System** (life-safety). Monorepo dengan 4 komponen: `android/`, `server/` (Go), `firmware/` (ESP32), + `contracts/`, `deploy/`, `docs/`.

## Aturan Emas (mengikat semua tugas)

1. **Contract-first.** `/contracts` adalah sumber kebenaran. Bila prosa spec dan `/contracts` berbeda, `/contracts` menang. Ubah kontrak lebih dulu, baru kode. Lihat ADR-0004.
2. **Satuan kanonik.** PGA = **gal** (`cm/s²`), timestamp = **ms epoch UTC** (`int64`), jarak = **km**, RSSI = **dBm**, durasi = **ms**. Konversi ke `g` HANYA untuk tampilan. Lihat `docs/SYSTEM_SPEC.md#0`.
3. **Life-safety mindset.** False positive (peringatan palsu) dan false negative (peringatan gagal sampai) sama-sama berbahaya. Utamakan integritas (HMAC), keandalan (QoS 1, WakeLock), dan verifikasi jarak lokal sebelum alarm.
4. **TLS everywhere.** MQTTS 8883, HTTPS, WSS. Plaintext dilarang di produksi. Lihat ADR-0003.
5. **Baca dulu, baru ubah.** Baca `docs/SYSTEM_SPEC.md`, `docs/GAP_ANALYSIS.md`, dan ADR terkait sebelum menulis kode lintas komponen.
6. **Tanpa stub/hallucinated deps.** Implementasi lengkap, dependency nyata dan ter-pin. Tulis test.

## Dokumen rujukan

- `docs/SYSTEM_SPEC.md` — blueprint arsitektur target.
- `docs/GAP_ANALYSIS.md` — kondisi aktual vs target.
- `docs/adr/` — keputusan arsitektural (0001–0004).
