# ADR-0004: Contract-First Development

- **Status:** Accepted
- **Tanggal:** 2026-08-18

## Konteks

Empat komponen (Android, Go, ESP32, dokumentasi) harus sepakat pada payload yang sama persis: field, tipe, satuan, urutan kanonik HMAC, dan skema DB. Draf awal menaruh contoh payload tersebar di prosa spec, menyebabkan drift (mis. `epicenter` vs `centroid`, `pga` g vs gal, signature 32-hex vs 64-hex).

## Keputusan

1. **`/contracts` adalah sumber kebenaran tunggal.** Bila spec prosa dan `/contracts` berbeda, `/contracts` menang.
2. **Isi `/contracts`:**
   - `openapi/quakealert.yaml` — REST API (OpenAPI 3.1).
   - `mqtt/*.schema.json` — JSON Schema untuk payload `trigger` & `heartbeat`, plus definisi kanonikalisasi string HMAC.
   - `fcm/*.schema.json` — skema data-only FCM message.
   - `db/schema.sql` + `db/migrations/` — DDL kanonik & migrasi berversi.
3. **Ditulis manual lebih dulu** (bukan di-generate dari kode). Kode Go & Kotlin menyesuaikan kontrak, bukan sebaliknya.
4. **Code generation opsional belakangan** (oapi-codegen untuk Go, dsb.) — kontrak tetap ditulis tangan sebagai otoritas.
5. **Perubahan kontrak = PR terpisah** yang di-review lintas komponen sebelum implementasi menyesuaikan.

## Konsekuensi

- (+) Menghilangkan sumber utama bug integrasi lintas komponen.
- (+) Reviewer bisa memeriksa satu tempat untuk menyetujui perubahan API.
- (+) Firmware & server menghitung HMAC dari definisi kanonikalisasi yang sama.
- (−) Disiplin ekstra: perubahan tidak boleh langsung di kode tanpa update kontrak.

## Alternatif ditolak

- **Code-first (generate kontrak dari anotasi Go):** membuat firmware C++ dan Kotlin bergantung pada implementasi Go; drift satuan sulit dicegah lintas bahasa.
