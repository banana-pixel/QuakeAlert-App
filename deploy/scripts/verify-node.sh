#!/usr/bin/env bash
#
# verify-node.sh — verifikasi node sensor lewat rute admin (migrasi 000005).
#
# Node yang baru di-provision lahir verified = false: heartbeat-nya diterima
# sehingga tampak di /sensors sebagai pending, tetapi trigger-nya ditolak dan
# tidak pernah ikut voting menuju ambang 3-node CONFIRMED. Skrip ini adalah
# sisi operator dari gerbang itu, untuk dipakai sampai wizard "Add a Sensor"
# memiliki langkah konfirmasi di dalam aplikasi.
#
# Kunci dibaca dari environment (ADMIN_API_KEY), bukan dari argumen — alasan
# sama dengan broadcast.sh: argumen muncul di `ps` dan riwayat shell.
#
#   export ADMIN_API_KEY="$(sed -n 's/^ADMIN_API_KEY=//p' /path/.env.prod)"
#   ./verify-node.sh --list                          # daftar node pending
#   ./verify-node.sh NODE-163A149F                   # setujui satu node
#   ./verify-node.sh --revoke NODE-163A149F          # tarik kepercayaan
#
# API_BASE default ke produksi; timpa untuk menguji ke server lokal:
#   API_BASE=http://localhost:8080 ./verify-node.sh --list
set -euo pipefail

API_BASE="${API_BASE:-https://api.quakealert.id}"

mode="verify"
station_id=""

usage() {
    cat >&2 <<'USAGE'
Pemakaian:
  verify-node.sh --list
  verify-node.sh <NODE-XXXXXXXX>          setujui node
  verify-node.sh --revoke <NODE-XXXXXXXX> tarik verifikasi node

Environment:
  ADMIN_API_KEY  wajib, kunci operator (header X-Admin-Key)
  API_BASE       opsional, default https://api.quakealert.id
USAGE
    exit 2
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --list)   mode="list"; shift ;;
        --revoke) mode="revoke"; shift; station_id="${1:-}"; shift ;;
        NODE-*)   station_id="$1"; shift ;;
        -h|--help) usage ;;
        *) echo "argumen tidak dikenal: $1" >&2; usage ;;
    esac
done

[[ -n "${ADMIN_API_KEY:-}" ]] || { echo "ADMIN_API_KEY belum di-set" >&2; exit 1; }

status="$(mktemp)"
trap 'rm -f "$status"' EXIT

case "$mode" in
    list)
        http_code="$(curl -sS -o "$status" -w '%{http_code}' \
            "${API_BASE}/api/v1/admin/nodes/pending" \
            -H "X-Admin-Key: ${ADMIN_API_KEY}")" || { echo "permintaan gagal" >&2; exit 1; }
        jq . <"$status" 2>/dev/null || cat "$status"
        [[ "$http_code" == "200" ]] || { echo "gagal: HTTP $http_code" >&2; exit 1; }
        echo "selesai." >&2
        ;;
    verify|revoke)
        [[ "$station_id" =~ ^NODE-[0-9A-F]{8}$ ]] || { echo "station_id harus berpola NODE-XXXXXXXX (hex kapital)" >&2; exit 1; }

        # Konfirmasi sebelum mengirim untuk revoke: node yang ditarik berhenti
        # voting seketika, jadi salah tekan di sini adalah kebisingan jaringan
        # sensor yang hilang, bukan perubahan kosmetik.
        if [[ "$mode" == "revoke" ]]; then
            read -r -p "Tarik verifikasi ${station_id}? [y/N] " confirm
            [[ "$confirm" == "y" || "$confirm" == "Y" ]] || { echo "dibatalkan" >&2; exit 1; }
            payload='{"verified":false}'
        else
            payload=''
        fi

        http_code="$(curl -sS -o "$status" -w '%{http_code}' \
            -X POST "${API_BASE}/api/v1/admin/nodes/${station_id}/verify" \
            -H "Content-Type: application/json" \
            -H "X-Admin-Key: ${ADMIN_API_KEY}" \
            ${payload:+--data-binary "$payload"})" || { echo "permintaan gagal" >&2; exit 1; }

        jq . <"$status" 2>/dev/null || cat "$status"

        if [[ "$http_code" != "200" ]]; then
            echo "gagal: HTTP $http_code" >&2
            exit 1
        fi
        if [[ "$mode" == "revoke" ]]; then
            echo "verifikasi ditarik — trigger node ini tidak lagi dihitung konsensus." >&2
        else
            echo "terverifikasi — trigger node ini sekarang ikut konsensus." >&2
        fi
        ;;
esac
