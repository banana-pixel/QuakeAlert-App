#!/usr/bin/env bash
# =============================================================================
# Preflight untuk firmware/src/secrets.h — dijalankan SEBELUM `pio run -t upload`.
#
# Yang diperiksa adalah hal-hal yang gagal secara diam-diam di lapangan: node
# menyala, tersambung Wi-Fi, dan tidak ada satu pun trigger yang masuk. Tidak ada
# pesan yang mengarah ke penyebabnya, karena penyebabnya ada di broker, bukan di
# node.
#
#   ./scripts/check-secrets.sh          # dari direktori firmware/
#
# Keluar 0 bila siap di-flash, 1 bila ada temuan. Tidak pernah mencetak nilai
# password: yang dicetak hanya nama kunci dan hasil pemeriksaan.
# =============================================================================
set -uo pipefail

cd "$(dirname "$0")/.."

# SECRETS_FILE dapat menunjuk berkas lain agar skrip ini dapat diuji tanpa
# menyentuh secrets.h milik siapa pun.
SECRETS=${SECRETS_FILE:-src/secrets.h}
ACL=../deploy/mosquitto/aclfile
findings=0

fail() { echo "  GAGAL: $*" >&2; findings=$((findings + 1)); }
ok()   { echo "  ok: $*"; }

if [ ! -f "$SECRETS" ]; then
    echo "GAGAL: $SECRETS tidak ada — copy dari src/secrets.h.example" >&2
    exit 1
fi

# `#define NAMA "nilai"` → nilai. Hanya untuk kunci non-rahasia (user, server,
# port); password tidak pernah dibaca oleh skrip ini.
value_of() {
    grep -oP "^\s*#define\s+$1\s+\"\K[^\"]*" "$SECRETS" | tail -n1
}
defined() {
    grep -qE "^\s*#define\s+$1\b" "$SECRETS"
}

echo "[preflight] peran MQTT"
mqtt_user=$(value_of SECRET_MQTT_USER)
if [ -z "$mqtt_user" ]; then
    fail "SECRET_MQTT_USER tidak di-set"
elif [ ! -f "$ACL" ]; then
    echo "  lewat: $ACL tidak ditemukan, peran tidak dapat diverifikasi"
elif ! grep -qE "^user[[:space:]]+${mqtt_user}\$" "$ACL"; then
    # Broker produksi memakai allow_anonymous false dan menolak semua topik untuk
    # user yang tidak ada di aclfile, jadi ini bukan peringatan gaya: node akan
    # terhubung (atau tidak) tanpa pernah bisa mem-publish apa pun.
    fail "SECRET_MQTT_USER='$mqtt_user' bukan salah satu peran di $ACL." \
         "Node harus memakai 'quakealert-node'."
elif [ "$mqtt_user" != "quakealert-node" ]; then
    # Peran server boleh membaca sensor/# dan menulis perintah. Sebuah node yang
    # memegangnya memberi siapa pun yang membongkar casing kemampuan seluruh
    # jaringan, bukan kemampuan satu node.
    fail "SECRET_MQTT_USER='$mqtt_user' adalah peran non-node; pakai 'quakealert-node'."
else
    ok "peran node ($mqtt_user) ada di aclfile"
fi

echo "[preflight] port & TLS"
port=$(grep -oP '^\s*#define\s+SECRET_MQTT_PORT\s+\K[0-9]+' "$SECRETS" | tail -n1)
case "$port" in
    8883) ok "port 8883 (MQTTS)" ;;
    "")   fail "SECRET_MQTT_PORT tidak di-set" ;;
    1883) fail "SECRET_MQTT_PORT=1883 plaintext; produksi hanya membuka 8883" ;;
    *)    echo "  catatan: SECRET_MQTT_PORT=$port bukan 8883 — pastikan itu memang broker Anda" ;;
esac

if defined SECRET_MQTT_ALLOW_INSECURE_TLS; then
    # Build seperti ini masih mengenkripsi, tetapi menerima sertifikat siapa pun,
    # jadi kredensial node yang dibagi seluruh fleet dapat dipanen oleh apa pun
    # yang bisa menjawab untuk nama broker.
    fail "SECRET_MQTT_ALLOW_INSECURE_TLS ter-set: validasi sertifikat mati." \
         "Hapus sebelum mem-flash node lapangan."
elif defined SECRET_MQTT_CA_CERT; then
    ok "CA kustom dari secrets.h dipakai"
else
    ok "akar ISRG ter-pin (src/mqtt_ca.h)"
fi

echo "[preflight] kunci wajib"
for key in SECRET_MQTT_SERVER SECRET_MQTT_PASS; do
    if defined "$key"; then ok "$key ada"; else fail "$key tidak di-set"; fi
done

echo
if [ "$findings" -eq 0 ]; then
    echo "[preflight] siap di-flash"
    exit 0
fi
echo "[preflight] $findings temuan — perbaiki $SECRETS sebelum upload" >&2
exit 1
