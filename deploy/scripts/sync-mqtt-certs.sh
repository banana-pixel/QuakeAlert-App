#!/usr/bin/env bash
# =============================================================================
# Menyalin sertifikat ACME milik hostname broker dari volume Caddy ke direktori
# yang dibaca Mosquitto, lalu memuat ulang broker.
#
# Mengapa menyalin dan tidak memount volume Caddy langsung: Caddy menulis kunci
# privat sebagai 0600 milik root, sementara Mosquitto berjalan sebagai uid 1883
# dan akan gagal start ketika tidak bisa membacanya. Menyamakannya berarti
# melonggarkan izin kunci milik reverse proxy — memindahkan satu salinan dengan
# pemilik yang tepat lebih murah daripada itu.
#
# Jalankan setelah `docker compose up` pertama, lalu sekali sebulan dari cron —
# Caddy memperpanjang pada 30 hari terakhir, jadi jadwal harian pun tidak
# berlebihan:
#   17 4 * * * /opt/quakealert/deploy/scripts/sync-mqtt-certs.sh >> /var/log/quakealert-certsync.log 2>&1
#
# Idempoten: keluar 0 tanpa menyentuh apa pun bila sertifikatnya belum berubah,
# sehingga cron tidak me-reload broker setiap hari tanpa alasan.
# =============================================================================
set -euo pipefail

cd "$(dirname "$0")/.."

COMPOSE=${COMPOSE:-docker compose -f docker-compose.prod.yml}
ENV_FILE=${ENV_FILE:-.env.prod}

# MQTT_DOMAIN dibaca dari .env.prod agar hanya ada satu tempat yang menyebut
# hostname broker (Caddyfile membacanya dari env yang sama).
if [ -f "$ENV_FILE" ]; then
    MQTT_DOMAIN=$(grep -E '^MQTT_DOMAIN=' "$ENV_FILE" | tail -n1 | cut -d= -f2- | tr -d '"'"'"'')
fi
: "${MQTT_DOMAIN:?MQTT_DOMAIN belum di-set (di $ENV_FILE atau environment)}"

CERT_DIR=mosquitto/certs
STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT

# Path penyimpanan Caddy. Direktori acme-* diglob karena namanya memuat
# direktori ACME yang dipakai (produksi vs staging Let's Encrypt).
REMOTE="/data/caddy/certificates/acme-*/${MQTT_DOMAIN}"

echo "[certsync] menarik sertifikat untuk ${MQTT_DOMAIN} dari volume caddy"
# `cat` di dalam container Caddy: file-nya milik root di sana, dan container ini
# sudah punya akses sah ke penyimpanannya sendiri.
$COMPOSE exec -T caddy sh -c "cat ${REMOTE}/${MQTT_DOMAIN}.crt" > "$STAGE/server.crt"
$COMPOSE exec -T caddy sh -c "cat ${REMOTE}/${MQTT_DOMAIN}.key" > "$STAGE/server.key"

# Sertifikat kosong berarti Caddy belum menerbitkannya (DNS belum mengarah, atau
# port 80 tertutup). Berhenti SEBELUM menimpa sertifikat lama yang masih sah:
# broker yang jalan dengan sertifikat kedaluwarsa masih menerima node, broker
# dengan sertifikat kosong tidak menerima siapa pun.
for f in server.crt server.key; do
    if [ ! -s "$STAGE/$f" ]; then
        echo "[certsync] GAGAL: $f kosong — Caddy belum menerbitkan sertifikat untuk ${MQTT_DOMAIN}" >&2
        exit 1
    fi
done

mkdir -p "$CERT_DIR"
if cmp -s "$STAGE/server.crt" "$CERT_DIR/server.crt" &&
   cmp -s "$STAGE/server.key" "$CERT_DIR/server.key"; then
    echo "[certsync] tidak ada perubahan; broker tidak di-reload"
    exit 0
fi

install -m 0644 "$STAGE/server.crt" "$CERT_DIR/server.crt"
install -m 0640 "$STAGE/server.key" "$CERT_DIR/server.key"
# uid/gid 1883 adalah user `mosquitto` di dalam image eclipse-mosquitto.
chown 1883:1883 "$CERT_DIR/server.crt" "$CERT_DIR/server.key" 2>/dev/null ||
    echo "[certsync] catatan: chown gagal (butuh root); pastikan uid 1883 dapat membaca $CERT_DIR" >&2

echo "[certsync] sertifikat diperbarui; memuat ulang mosquitto"
# SIGHUP memuat ulang konfigurasi DAN sertifikat tanpa memutus koneksi yang ada.
$COMPOSE kill -s HUP mosquitto
echo "[certsync] selesai"
