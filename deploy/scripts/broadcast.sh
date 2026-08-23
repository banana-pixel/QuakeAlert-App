#!/usr/bin/env bash
#
# broadcast.sh — mengirim satu pengumuman operator lewat POST /api/v1/admin/broadcasts.
#
# Kunci dibaca dari environment (ADMIN_API_KEY), bukan dari argumen: argumen
# muncul di `ps` dan tersimpan di riwayat shell, dan satu-satunya cara kunci ini
# dicuri adalah dengan tertulis di tempat yang salah. Contoh:
#
#   export ADMIN_API_KEY="$(sed -n 's/^ADMIN_API_KEY=//p' /path/.env.prod)"
#   ./broadcast.sh --title "Uji sistem" --body "Ini pengumuman uji."
#   ./broadcast.sh --title "Gladi resik" --body "Pukul 10.00 WIB." --region "ID-jawa-barat"
#
# --region opsional; tanpa itu pengumuman bersifat nasional. Bentuknya
# <ISO2>-<slug-admin1> dan dinormalisasi ulang oleh server, jadi "ID-Jawa Barat"
# juga diterima.
#
# API_BASE default ke produksi; timpa untuk menguji ke server lokal:
#   API_BASE=http://localhost:8080 ./broadcast.sh --title ... --body ...
set -euo pipefail

API_BASE="${API_BASE:-https://api.quakealert.id}"

title=""
body=""
region=""

usage() {
    cat >&2 <<'USAGE'
Pemakaian: broadcast.sh --title <judul> --body <isi> [--region <ISO2>-<slug>]

  --title    wajib, maksimal 120 karakter
  --body     wajib, maksimal 500 karakter
  --region   opsional; tanpa ini pengumuman nasional

Environment:
  ADMIN_API_KEY  wajib, kunci operator (header X-Admin-Key)
  API_BASE       opsional, default https://api.quakealert.id
USAGE
    exit 2
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --title)  title="${2:-}"; shift 2 ;;
        --body)   body="${2:-}"; shift 2 ;;
        --region) region="${2:-}"; shift 2 ;;
        -h|--help) usage ;;
        *) echo "argumen tidak dikenal: $1" >&2; usage ;;
    esac
done

[[ -n "${ADMIN_API_KEY:-}" ]] || { echo "ADMIN_API_KEY belum di-set" >&2; exit 1; }
[[ -n "$title" ]] || { echo "--title wajib" >&2; usage; }
[[ -n "$body"  ]] || { echo "--body wajib" >&2; usage; }

command -v jq >/dev/null || { echo "jq dibutuhkan untuk menyusun JSON dengan aman" >&2; exit 1; }

# JSON disusun jq, bukan printf: judul yang memuat tanda kutip atau newline akan
# menghasilkan payload rusak (atau lebih buruk, payload yang berbeda dari yang
# dimaksud) bila dirangkai sebagai string.
payload="$(jq -nc \
    --arg title "$title" \
    --arg body "$body" \
    --arg region "$region" \
    '{title: $title, body: $body} + (if $region == "" then {} else {region_code: $region} end)')"

# Konfirmasi sebelum mengirim: pengumuman ini muncul di shade setiap perangkat
# yang menjadi sasarannya dan tidak bisa ditarik kembali.
echo "Akan dikirim ke ${API_BASE}:" >&2
echo "$payload" | jq . >&2
read -r -p "Lanjut? [y/N] " confirm
[[ "$confirm" == "y" || "$confirm" == "Y" ]] || { echo "dibatalkan" >&2; exit 1; }

status="$(mktemp)"
trap 'rm -f "$status"' EXIT

http_code="$(curl -sS -o "$status" -w '%{http_code}' \
    -X POST "${API_BASE}/api/v1/admin/broadcasts" \
    -H "Content-Type: application/json" \
    -H "X-Admin-Key: ${ADMIN_API_KEY}" \
    --data-binary "$payload")" || { echo "permintaan gagal" >&2; exit 1; }

jq . <"$status" 2>/dev/null || cat "$status"

if [[ "$http_code" != "201" ]]; then
    echo "gagal: HTTP $http_code" >&2
    exit 1
fi
echo "terkirim." >&2
