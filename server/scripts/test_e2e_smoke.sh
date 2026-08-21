#!/usr/bin/env bash
# ============================================================================
# QuakeAlert — E2E Smoke Test (REST)
#
# Memverifikasi alur integrasi Android client:
#   Register Anonymous -> Set Location -> Fetch Events
#   (+ FCM token, + filter spasial dari lokasi tersimpan)
#
# Prasyarat: docker, go, curl, jq, openssl.
# Menjalankan stack dev (postgis + migrate + redis + mosquitto) via
# server/docker-compose.yml, membangun & menjalankan server Go, lalu
# menguji 5 langkah alur. Teardown otomatis pada exit (docker compose down).
#
# Usage:  server/scripts/test_e2e_smoke.sh
# Env:    BASE_URL (default http://localhost:8080), HTTP_ADDR (default :8080)
# ============================================================================
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSE_FILE="$ROOT/server/docker-compose.yml"
BASE_URL="${BASE_URL:-http://localhost:8080}"
HTTP_ADDR="${HTTP_ADDR:-:8080}"

DB_USER="quakealert"
DB_PASS="devpassword"
DB_NAME="quakealert"

SERVER_PID=""
TMPDIR_SMOKE=""
AUTH="" # Bearer token aktif untuk request ber-auth

PASS=0
FAIL=0
ok()  { PASS=$((PASS + 1)); echo "  PASS   $1"; }
bad() { FAIL=$((FAIL + 1)); echo "  FAIL   $1"; }
die() { echo "ERROR: $1" >&2; exit 1; }

cleanup() {
  if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
    echo "[teardown] stop server (pid $SERVER_PID)"
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  echo "[teardown] docker compose down"
  docker compose -f "$COMPOSE_FILE" down >/dev/null 2>&1 || true
  [ -n "$TMPDIR_SMOKE" ] && rm -rf "$TMPDIR_SMOKE"
}
trap cleanup EXIT

for bin in docker go curl jq openssl; do
  command -v "$bin" >/dev/null 2>&1 || die "'$bin' tidak ditemukan di PATH"
done

# --- Helper: request API, hasil di globals STATUS/BODY ----------------------
STATUS=""
BODY=""
api() { # api METHOD PATH [JSON_BODY]
  local method="$1" path="$2" body="${3:-}"
  local args=(-sS -w '\n%{http_code}' -X "$method" "$BASE_URL$path" -H 'Content-Type: application/json')
  [ -n "$AUTH" ] && args+=(-H "Authorization: Bearer $AUTH")
  [ -n "$body" ] && args+=(-d "$body")
  local out
  out="$(curl "${args[@]}" 2>&1)" || die "curl gagal: $method $path"
  STATUS="$(printf '%s\n' "$out" | tail -n1)"
  BODY="$(printf '%s\n' "$out" | sed '$d')"
}

# --- Step 0: stack dev ------------------------------------------------------
echo "==> [0] Menyiapkan stack dev (postgis, migrate, redis, mosquitto)"
docker compose -f "$COMPOSE_FILE" up -d >/dev/null 2>&1 || die "docker compose up gagal"

for i in $(seq 1 90); do
  state="$(docker inspect -f '{{.State.Status}}' quakealert-migrate 2>/dev/null || echo missing)"
  if [ "$state" = "exited" ]; then
    code="$(docker inspect -f '{{.State.ExitCode}}' quakealert-migrate)"
    [ "$code" -eq 0 ] || die "migrate exit code $code (cek contracts/db/migrations)"
    break
  fi
  [ "$i" -eq 90 ] && die "migrate tidak selesai dalam 180s"
  sleep 2
done

for svc in quakealert-postgis quakealert-mosquitto; do
  for i in $(seq 1 60); do
    status="$(docker inspect -f '{{.State.Health.Status}}' "$svc" 2>/dev/null || echo missing)"
    [ "$status" = "healthy" ] && break
    [ "$i" -eq 60 ] && die "$svc tidak healthy dalam 120s (status: $status)"
    sleep 2
  done
done
echo "  OK    stack siap"

# --- Step 1: build & jalankan server ----------------------------------------
TMPDIR_SMOKE="$(mktemp -d)"
MASTER_KEY_HEX="$(openssl rand -hex 32)"
JWT_SECRET="$(openssl rand -hex 32)"

echo "==> [1] Build & jalankan server Go"
(
  cd "$ROOT/server"
  go build -o "$TMPDIR_SMOKE/quakealert" ./cmd/quakealert
) || die "go build gagal"

env \
  DATABASE_URL="postgres://$DB_USER:$DB_PASS@localhost:5432/$DB_NAME?sslmode=disable" \
  MQTT_BROKER="tcp://localhost:1883" \
  MQTT_PUBLIC_TLS="false" \
  HTTP_ADDR="$HTTP_ADDR" \
  MASTER_KEY_HEX="$MASTER_KEY_HEX" \
  JWT_SECRET="$JWT_SECRET" \
  "$TMPDIR_SMOKE/quakealert" >"$TMPDIR_SMOKE/server.log" 2>&1 &
SERVER_PID=$!

for i in $(seq 1 30); do
  if curl -fsS "$BASE_URL/healthz" >/dev/null 2>&1; then
    echo "  OK    server listen di $BASE_URL"
    break
  fi
  [ "$i" -eq 30 ] && { echo "--- server.log ---"; tail -n 20 "$TMPDIR_SMOKE/server.log"; die "server tidak siap dalam 60s"; }
  sleep 2
done

# --- Step 2: alur client ----------------------------------------------------
echo "==> [2] Alur: Register Anonymous -> Set Location -> Fetch Events"

echo "  [2.1] POST /api/v1/auth/anonymous"
api POST /api/v1/auth/anonymous
if [ "$STATUS" = "201" ] && printf '%s' "$BODY" | jq -e '
    (.token | length > 0) and (.token_type == "Bearer") and
    (.user_id | test("^[0-9a-f-]{36}$")) and (.pseudonym | startswith("Quakezen-")) and
    (.expires_at != null) and (.created_at != null)' >/dev/null 2>&1; then
  ok "201 + token/user_id/pseudonym/expires_at valid"
  AUTH="$(printf '%s' "$BODY" | jq -r .token)"
  USER_ID="$(printf '%s' "$BODY" | jq -r .user_id)"
  echo "      user_id=${USER_ID:0:8}... pseudonym=$(printf '%s' "$BODY" | jq -r .pseudonym)"
else
  bad "auth anonymous gagal (status=$STATUS, body=$BODY)"
fi

echo "  [2.2] PUT /api/v1/users/location"
api PUT /api/v1/users/location '{"latitude":-6.8721,"longitude":107.5422,"location_name":"Cimahi, West Java, ID"}'
# Koordinat inilah satu-satunya yang dipakai dispatch saat memilih token FCM:
# radiusnya tetap (dispatch.AlertRadiusKm), jadi posisi yang tidak sampai ke DB
# berarti perangkat itu tidak dibangunkan sama sekali.
if [ "$STATUS" = "200" ] && printf '%s' "$BODY" | jq -e '
    (.latitude == -6.8721) and (.longitude == 107.5422) and
    (.location_name == "Cimahi, West Java, ID") and
    (has("coverage_radius_km") | not) and
    (.user_id == "'"$USER_ID"'") and (.updated_at != null)' >/dev/null 2>&1; then
  ok "200 + koordinat/location_name tersimpan & echo benar (tanpa coverage_radius_km)"
else
  bad "set location gagal (status=$STATUS, body=$BODY)"
fi

echo "  [2.3] GET /api/v1/events?limit=5"
api GET "/api/v1/events?limit=5"
if [ "$STATUS" = "200" ] && printf '%s' "$BODY" | jq -e '
    (.limit == 5) and (.offset == 0) and (.count >= 0) and
    (.events | type == "array") and (.range_km == null)' >/dev/null 2>&1; then
  ok "200 + paginasi/shape benar (count=$(printf '%s' "$BODY" | jq -r .count))"
else
  bad "list events gagal (status=$STATUS, body=$BODY)"
fi

echo "  [2.4] PUT /api/v1/users/fcm-token"
api PUT /api/v1/users/fcm-token '{"fcm_token":"fMEP0vJq-smoke:APA91bH-test-token-2026"}'
if [ "$STATUS" = "200" ] && printf '%s' "$BODY" | jq -e '.updated_at != null' >/dev/null 2>&1; then
  ok "200 + updated_at diterima"
else
  bad "set fcm-token gagal (status=$STATUS, body=$BODY)"
fi

echo "  [2.5] GET /api/v1/events?range_km=300 (filter dari lokasi tersimpan)"
api GET "/api/v1/events?range_km=300"
if [ "$STATUS" = "200" ] && printf '%s' "$BODY" | jq -e '.range_km == 300' >/dev/null 2>&1; then
  ok "200 + range_km=300 (filter spasial aktif via lokasi user)"
else
  bad "filter spasial gagal (status=$STATUS, body=$BODY)"
fi

# --- Ringkasan --------------------------------------------------------------
echo ""
echo "============================================"
echo "Hasil: $PASS PASS, $FAIL FAIL"
echo "============================================"
if [ "$FAIL" -gt 0 ]; then
  echo "--- server.log (tail) ---"
  tail -n 30 "$TMPDIR_SMOKE/server.log" || true
  exit 1
fi
exit 0
