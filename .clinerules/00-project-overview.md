# 00 — Baca ini lebih dulu

QuakeAlert adalah **Earthquake Early Warning System berbasis komunitas yang
global dan mandiri** (life-safety). Monorepo: `android/`, `server/` (Go),
`firmware/` (ESP32), `contracts/`, `deploy/`, `docs/`.

## Urutan baca — lakukan SEBELUM menyentuh apa pun

1. **`PROJECT_RULES.md`** — aturan permanen. Jangan dilanggar.
2. **`ROADMAP.md`**, baris **ACTIVE PHASE** paling atas. Itu batas ruang lingkup
   kerja Anda.
3. **`docs/CURRENT_STATE.md`** — apa yang benar-benar ada, dan apa yang baru
   IMPLEMENTED tetapi belum VALIDATED. Perhatikan daftar *NOT demonstrated*.
4. **`docs/DECISIONS.md`** — keputusan yang diterima dan pertanyaan yang belum
   terjawab. **Pertanyaan UNRESOLVED tidak boleh diselesaikan dengan
   mengimplementasikan salah satu tafsirannya.**
5. Baru kemudian aturan per komponen: `10-server-go.md`,
   `20-android-kotlin.md`, `30-firmware-esp32.md`.

## Hierarki otoritas

```
/contracts > PROJECT_RULES.md > docs/DECISIONS.md (accepted)
  > docs/CURRENT_STATE.md > ROADMAP.md > docs/*.md (prosa) > .hermes/plans/*
```

Bila dua sumber bertentangan, yang lebih tinggi menang dan yang lebih rendah
adalah **cacat yang harus dilaporkan** — bukan diselaraskan sendiri. Lihat
ADR-0004.

**`.hermes/plans/*` adalah artefak perencanaan HISTORIS dan TIDAK PERNAH
otoritatif.** Isinya memuat keputusan yang sudah digantikan (mis. probe 3×3
tetap, pita lintang, Phase 4 berbasis katalog eksternal). Jangan jadikan dasar
implementasi.

`docs/SYSTEM_SPEC.md` dan `docs/GAP_ANALYSIS.md` juga historis: keduanya
menjelaskan arsitektur konsensus Fase 2 yang sudah digantikan.

## Aturan Emas (mengikat semua tugas)

1. **Contract-first.** `/contracts` adalah sumber kebenaran. Ubah kontrak lebih
   dulu, baru kode. Lihat ADR-0004.
2. **Satuan kanonik.** PGA = **gal** (`cm/s²`), timestamp = **ms epoch UTC**
   (`int64`), jarak = **km**, RSSI = **dBm**, durasi = **ms**. Konversi ke `g`
   HANYA untuk tampilan.
3. **Life-safety mindset.** False positive dan false negative sama-sama
   berbahaya. Utamakan integritas (HMAC) dan verifikasi bukti independen sebelum
   alarm. **Trigger firmware dipublikasikan pada QoS 0**, dengan retry firmware
   dan dedup `obs_seq` + `phase` di server — BUKAN at-least-once. Lihat D-008 dan
   `contracts/mqtt/trigger.schema.json`.
4. **TLS everywhere.** MQTTS 8883, HTTPS, WSS. Plaintext dilarang di produksi.
   Lihat ADR-0003.
5. **Global secara baku.** Tidak ada asumsi khusus satu negara di komponen
   global. Tidak ada pita lintang, tidak ada lingkungan sel tetap, bujur selalu
   dilipat di antimeridian.
6. **Tanpa stub/hallucinated deps.** Implementasi lengkap, dependency nyata dan
   ter-pin. Tulis test.

## Kendali ruang lingkup

Implementasikan HANYA yang tercakup ACTIVE PHASE, atau yang disetujui eksplisit
sebagai prasyaratnya. Pekerjaan berguna di luar itu = **PROPOSAL**, bukan commit.

- Sebelum mengubah kode: sebutkan invarian apa yang dilindungi perubahan itu.
- Sesudahnya: pastikan tidak ada ruang lingkup lain yang masuk, dan laporkan
  `UNRELATED CHANGES: NONE` atau daftarnya.
- Jangan pernah menandai pekerjaan Anda sendiri sebagai VALIDATED. Unit test
  membuktikan IMPLEMENTED, bukan validasi lapangan.
- Jangan pernah mengubah ambang keselamatan agar exit criteria sebuah fase lulus.

## Beri label pada pernyataan Anda

REQUIREMENT / FACT (dengan `file:line`) / ASSUMPTION / PROPOSAL / UNRESOLVED.
Klaim tentang perilaku sistem tanpa label adalah cacat dalam jawaban Anda.

## Berhenti dan tanya

Berhenti — jangan pilih satu tafsiran lalu lanjut — bila pertanyaan yang belum
terjawab menyangkut **keselamatan, perilaku alert, semantik data, atau kontrak
publik**.
