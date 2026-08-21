#!/usr/bin/env bash
# =============================================================================
# Membuat deploy/mosquitto/passwd dari kredensial di .env.prod.
#
# Dijalankan SEKALI sebelum `docker compose up` pertama, dan lagi setiap kali
# salah satu password MQTT dirotasi. File passwd berisi hash PBKDF2 (bukan
# password), tetapi tetap tidak masuk git: ia cukup untuk serangan offline.
#
# Memakai container eclipse-mosquitto sekali pakai agar `mosquitto_passwd` tidak
# perlu diinstal di host — versi hash-nya lalu dijamin sama dengan versi broker
# yang membacanya.
#
#   ./scripts/init-mqtt-users.sh
# =============================================================================
set -euo pipefail

cd "$(dirname "$0")/.."

ENV_FILE=${ENV_FILE:-.env.prod}
[ -f "$ENV_FILE" ] || { echo "$ENV_FILE tidak ada — copy dari .env.prod.example dulu" >&2; exit 1; }

# shellcheck disable=SC1090
set -a; . "./$ENV_FILE"; set +a

: "${MQTT_SERVER_USER:=quakealert-server}"
: "${MQTT_NODE_USER:=quakealert-node}"
: "${MQTT_MONITOR_USER:=quakealert-monitor}"
: "${MQTT_SERVER_PASSWORD:?belum di-set di $ENV_FILE}"
: "${MQTT_NODE_PASSWORD:?belum di-set di $ENV_FILE}"
: "${MQTT_MONITOR_PASSWORD:?belum di-set di $ENV_FILE}"

# Nama user WAJIB sama dengan yang ada di mosquitto/aclfile: ACL dicocokkan per
# nama, dan user yang tidak disebut di aclfile ditolak untuk semua topik — gagal
# yang muncul sebagai "node terhubung tapi tidak ada trigger yang masuk".
for user in "$MQTT_SERVER_USER" "$MQTT_NODE_USER" "$MQTT_MONITOR_USER"; do
    grep -qE "^user[[:space:]]+${user}\$" mosquitto/aclfile || {
        echo "GAGAL: user '$user' tidak ada di mosquitto/aclfile" >&2
        exit 1
    }
done

OUT=mosquitto/passwd
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
: > "$TMP/passwd"

echo "[mqtt-users] membuat hash untuk 3 user"
docker run --rm -i \
    -v "$TMP:/work:z" \
    -e SU="$MQTT_SERVER_USER" -e SP="$MQTT_SERVER_PASSWORD" \
    -e NU="$MQTT_NODE_USER"   -e NP="$MQTT_NODE_PASSWORD" \
    -e MU="$MQTT_MONITOR_USER" -e MP="$MQTT_MONITOR_PASSWORD" \
    eclipse-mosquitto:2 sh -c '
        set -e
        # -b: password sebagai argumen. Ia terlihat di daftar proses container
        # sekali-pakai ini dan tidak pernah menyentuh disk host selain sebagai
        # hash — alternatifnya (stdin interaktif) tidak bisa diotomatiskan.
        mosquitto_passwd -b -c /work/passwd "$SU" "$SP"
        mosquitto_passwd -b    /work/passwd "$NU" "$NP"
        mosquitto_passwd -b    /work/passwd "$MU" "$MP"
    '

install -m 0640 "$TMP/passwd" "$OUT"
# uid 1883 = user mosquitto di dalam image; tanpa ini broker tidak bisa membaca
# file-nya dan menolak SEMUA koneksi (allow_anonymous false).
chown 1883:1883 "$OUT" 2>/dev/null ||
    echo "[mqtt-users] catatan: chown gagal (butuh root); pastikan uid 1883 dapat membaca $OUT" >&2

echo "[mqtt-users] $OUT ditulis ($(grep -c : "$OUT") user)"
echo "[mqtt-users] jalankan 'docker compose -f docker-compose.prod.yml kill -s HUP mosquitto' bila broker sudah berjalan"
