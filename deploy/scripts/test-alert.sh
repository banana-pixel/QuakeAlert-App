#!/usr/bin/env bash
#
# test-alert.sh — mengirim satu peringatan LATIHAN lewat POST /api/v1/admin/test-alert.
#
# Drill ini tidak dapat mencapai pengguna sungguhan, dan itu dijaga dua pagar
# yang saling bebas: server hanya mengirimnya ke topic FCM `test_alerts` (yang
# hanya dilanggani build debug), dan build rilis menjatuhkan frame ber-is_test
# apa pun yang berhasil sampai kepadanya. Satu kekeliruan konfigurasi tidak
# cukup untuk melewati keduanya.
#
# Tidak menulis satu baris pun ke earthquake_events dan tidak melewati
# consensus engine, jadi drill tidak akan muncul di riwayat aktivitas siapa pun
# maupun menahan all-clear gempa sungguhan.
#
# Kunci dibaca dari environment (ADMIN_API_KEY), bukan dari argumen: argumen
# muncul di `ps` dan tersimpan di riwayat shell. Contoh:
#
#   export ADMIN_API_KEY="$(sed -n 's/^ADMIN_API_KEY=//p' /path/.env.prod)"
#   API_BASE=http://localhost:8080 ./test-alert.sh --pga 300
#   ./test-alert.sh --pga 60 --lat -6.9175 --lon 107.6191 --place "Bandung, Jawa Barat"
#
# MMI dan intensity_label TIDAK dikirim: server menurunkannya dari --pga dengan
# fungsi yang sama dengan jalur konsensus, sehingga drill tidak dapat membawa
# kombinasi yang gempa sungguhan tidak pernah menghasilkan.
set -euo pipefail

API_BASE="${API_BASE:-https://api.quakealert.id}"

# Default di Bandung: posisi uji yang sama dengan emulator pengembangan, jadi
# gate jarak di klien (200 km dari centroid) meloloskannya alih-alih diam.
pga=""
lat="-6.9175"
lon="107.6191"
place=""
nodes=""

usage() {
    cat >&2 <<'USAGE'
Pemakaian: test-alert.sh --pga <gal> [--lat <lat>] [--lon <lon>] [--place <nama>] [--nodes <n>]

  --pga    wajib, PGA dalam gal (1..2000). MMI diturunkan server dari nilai ini.
           < 16.6 -> light, < 137.2 -> moderate, >= 137.2 -> strong;
           >= 250 gal juga melewati ambang "parah" di klien.
  --lat    opsional, default -6.9175 (Bandung)
  --lon    opsional, default 107.6191
  --place  opsional; tanpa ini teksnya sendiri mengaku latihan
  --nodes  opsional, default 3 (ambang CONFIRMED)

Environment:
  ADMIN_API_KEY  wajib, kunci operator (header X-Admin-Key)
  API_BASE       opsional, default https://api.quakealert.id
USAGE
    exit 2
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --pga)   pga="${2:-}"; shift 2 ;;
        --lat)   lat="${2:-}"; shift 2 ;;
        --lon)   lon="${2:-}"; shift 2 ;;
        --place) place="${2:-}"; shift 2 ;;
        --nodes) nodes="${2:-}"; shift 2 ;;
        -h|--help) usage ;;
        *) echo "argumen tidak dikenal: $1" >&2; usage ;;
    esac
done

[[ -n "${ADMIN_API_KEY:-}" ]] || { echo "ADMIN_API_KEY belum di-set" >&2; exit 1; }
[[ -n "$pga" ]] || { echo "--pga wajib" >&2; usage; }

command -v jq >/dev/null || { echo "jq dibutuhkan untuk menyusun JSON dengan aman" >&2; exit 1; }

# jq --argjson untuk angka (bukan --arg) agar pga_gal/latitude tidak terkirim
# sebagai string dan ditolak decoder Go.
payload="$(jq -nc \
    --argjson pga "$pga" \
    --argjson lat "$lat" \
    --argjson lon "$lon" \
    --arg place "$place" \
    --arg nodes "$nodes" \
    '{pga_gal: $pga, latitude: $lat, longitude: $lon}
     + (if $place == "" then {} else {location_name: $place} end)
     + (if $nodes == "" then {} else {node_count: ($nodes | tonumber)} end)')"

# Tanpa konfirmasi, tidak seperti broadcast.sh: drill tidak dapat menjangkau
# perangkat produksi, jadi mengirimnya berulang kali dari sebuah loop pengujian
# adalah pemakaian yang wajar dan sebuah prompt hanya akan menghalanginya.
echo "Drill ke ${API_BASE}:" >&2
echo "$payload" | jq . >&2

status="$(mktemp)"
trap 'rm -f "$status"' EXIT

http_code="$(curl -sS -o "$status" -w '%{http_code}' \
    -X POST "${API_BASE}/api/v1/admin/test-alert" \
    -H "Content-Type: application/json" \
    -H "X-Admin-Key: ${ADMIN_API_KEY}" \
    --data-binary "$payload")" || { echo "permintaan gagal" >&2; exit 1; }

jq . <"$status" 2>/dev/null || cat "$status"

if [[ "$http_code" != "202" ]]; then
    echo "gagal: HTTP $http_code" >&2
    exit 1
fi
echo "drill terkirim; all-clear otomatis 20 detik kemudian." >&2
