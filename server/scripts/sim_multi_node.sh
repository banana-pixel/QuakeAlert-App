#!/usr/bin/env bash
# =============================================================================
# sim_multi_node.sh — Checkpoint 3.1: multi-node CONFIRMED simulation
#
# Proves the real QuakeAlert pipeline can process 3 independent virtual nodes
# and reach CONFIRMED through the full path:
#   provision → verify → MQTT → ingest → Tracker → classify → emit → dispatch
#
# Runs against LOCAL DEV STACK only (server/docker-compose.yml).
# Never connects to the production VPS.
# Never reads or modifies production credentials.
#
# EVIDENCE CLASS: SOFTWARE SIMULATION (S9, D-011 constraint 2, D-014).
# The three nodes are database rows with hand-picked coordinates. A green run
# says the pipeline processes three independent virtual contributors and reaches
# CONFIRMED. It does NOT validate field correlation, production behaviour, or
# real multi-node sensor performance — those belong to Phase F.
#
# Writes a structured JSON evidence artifact (P4-M5', D-014). The artifact is
# written by this script while it still holds the values it asserted on, not
# reconstructed afterwards from this output.
#
# Usage:
#   ADMIN_API_KEY="<key>" ./server/scripts/sim_multi_node.sh
#
# Optional env:
#   BASE_URL           (default: http://localhost:8080)
#   HTTP_ADDR          (default: :8080)
#   SIM_WAIT_SECS      (default: 12)  — wait for Tracker to classify after triggers
#   SIM_EVIDENCE_DIR   (default: <repo>/.sim-evidence) — artifact output directory
#   SIM_RUN_ID         (default: derived) — run identity recorded in the artifact
# =============================================================================
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SERVER_DIR="$ROOT/server"
COMPOSE_FILE="$SERVER_DIR/docker-compose.yml"
COMPOSE_OVERRIDE="$SERVER_DIR/scripts/docker-compose.sim.override.yml"
COMPOSE_PROJECT="sim31"
COMPOSE="docker compose -f $COMPOSE_FILE -f $COMPOSE_OVERRIDE -p $COMPOSE_PROJECT"

BASE_URL="${BASE_URL:-http://localhost:8080}"
HTTP_ADDR="${HTTP_ADDR:-:8080}"
SIM_WAIT_SECS="${SIM_WAIT_SECS:-12}"

if [ -z "${ADMIN_API_KEY:-}" ]; then
  echo "ERROR: ADMIN_API_KEY env var required (used to verify sim nodes via /admin/nodes/{id}/verify)" >&2
  echo "  Generate: ADMIN_API_KEY=\"\$(openssl rand -base64 32)\" ./server/scripts/sim_multi_node.sh" >&2
  exit 1
fi

# Node definitions — must match sim_confirm_scenario.go simNodes exactly.
SIM_A_ID="NODE-53494D41"
SIM_B_ID="NODE-53494D42"
SIM_C_ID="NODE-53494D43"

PASS=0
FAIL=0

# The artifact contract lives in one place for both harnesses (D-014).
# shellcheck source=server/scripts/sim_evidence.sh
. "$SERVER_DIR/scripts/sim_evidence.sh"

# ok/bad keep printing exactly what they printed before; they additionally
# record the assertion so the artifact and this transcript cannot disagree.
ok()  { PASS=$((PASS + 1)); echo "  PASS  $1"; sim_record_assertion PASS "$1"; }
bad() { FAIL=$((FAIL + 1)); echo "  FAIL  $1"; sim_record_assertion FAIL "$1"; }
# die() records why the run aborted, so an ERROR artifact names its cause
# instead of leaving the reader to guess from a missing field.
die() { SIM_ERROR="$1"; echo "ERROR: $1" >&2; exit 1; }

SERVER_PID=""
TMPDIR_SIM=""
AUTH=""        # JWT bearer for auth-required endpoints
SIM_SECRET_A=""
SIM_SECRET_B=""
SIM_SECRET_C=""
SIM_STATION_A=""
SIM_STATION_B=""
SIM_STATION_C=""

STATUS=""
BODY=""

api() {
  local method="$1" path="$2" body="${3:-}"
  local args=(-sS -w '\n%{http_code}' -X "$method" "$BASE_URL$path" -H 'Content-Type: application/json')
  [ -n "$AUTH" ] && args+=(-H "Authorization: Bearer $AUTH")
  [ -n "$body" ] && args+=(-d "$body")
  local out
  out="$(curl "${args[@]}" 2>&1)" || die "curl failed: $method $path"
  STATUS="$(printf '%s\n' "$out" | tail -n1)"
  BODY="$(printf '%s\n' "$out" | sed '$d')"
}

admin_api() {
  local method="$1" path="$2" body="${3:-}"
  local args=(-sS -w '\n%{http_code}' -X "$method" "$BASE_URL$path"
    -H 'Content-Type: application/json'
    -H "X-Admin-Key: $ADMIN_API_KEY")
  [ -n "$body" ] && args+=(-d "$body")
  local out
  out="$(curl "${args[@]}" 2>&1)" || die "curl failed: $method $path"
  STATUS="$(printf '%s\n' "$out" | tail -n1)"
  BODY="$(printf '%s\n' "$out" | sed '$d')"
}

# sim_observed_json echoes the scalars this run's own assertions rested on, for
# the artifact's evidence.observed. Every read is guarded with ${VAR:-} so a run
# that aborts halfway still archives what it did reach, and a value never
# measured appears as null instead of aborting the emitter under `set -u`.
sim_observed_json() {
  jq -nc \
    --argjson sim_wait_secs "$(_sim_num "${SIM_WAIT_SECS:-}")" \
    --argjson onset_ts_ms "$(_sim_num "${ONSET_TS:-}")" \
    --argjson baseline_created "$(_sim_num "${BASELINE_CREATED:-}")" \
    --argjson baseline_confirmed "$(_sim_num "${BASELINE_CONFIRMED:-}")" \
    --argjson created_total "$(_sim_num "${CREATED:-}")" \
    --argjson created_delta "$(_sim_num "${NEW_CREATED:-}")" \
    --argjson unconfirmed_total "$(_sim_num "${UNCONFIRMED:-}")" \
    --argjson confirmed_total "$(_sim_num "${CONFIRMED:-}")" \
    --argjson confirmed_delta "$(_sim_num "${NEW_CONFIRMED:-}")" \
    --argjson tombstone_evictions "$(_sim_num "${EVICTIONS:-}")" \
    --argjson forced_resolutions "$(_sim_num "${FORCED:-}")" \
    --argjson near_confirmed_entries "$(_sim_num "${NC_COUNT:-}")" \
    --argjson near_confirmed_independent_at_peak "$(_sim_num "${NC_INDEP:-}")" \
    --argjson near_confirmed_confirmed_at_ms "$(_sim_num "${NC_CONFIRMED_AT:-}")" \
    --argjson rest_events_count "$(_sim_num "${EVENTS_COUNT:-}")" \
    --arg rest_event_state "${EVENT_STATE:-}" \
    --arg rest_event_id "${EVENT_ID_REST:-}" \
    '{
      sim_wait_secs: $sim_wait_secs,
      onset_ts_ms: $onset_ts_ms,
      nodes_triggered: 3,
      baseline_created: $baseline_created,
      baseline_confirmed: $baseline_confirmed,
      created_total: $created_total,
      created_delta: $created_delta,
      unconfirmed_total: $unconfirmed_total,
      confirmed_total: $confirmed_total,
      confirmed_delta: $confirmed_delta,
      tombstone_evictions: $tombstone_evictions,
      forced_resolutions: $forced_resolutions,
      near_confirmed_entries: $near_confirmed_entries,
      near_confirmed_independent_at_peak: $near_confirmed_independent_at_peak,
      near_confirmed_confirmed_at_ms: $near_confirmed_confirmed_at_ms,
      rest_events_count: $rest_events_count,
      rest_event_state: (if $rest_event_state == "" then null else $rest_event_state end),
      rest_event_id: (if $rest_event_id == "" then null else $rest_event_id end)
    }'
}

cleanup() {
  # --- Remove simulation nodes from DB (idempotent) ---
  if [ -n "${TMPDIR_SIM:-}" ] && command -v psql >/dev/null 2>&1; then
    psql "postgres://quakealert:devpassword@localhost:5432/quakealert?sslmode=disable" \
      -c "DELETE FROM iot_nodes WHERE station_id LIKE 'NODE-53494D4_';" \
      >/dev/null 2>&1 || true
    echo "[cleanup] deleted sim nodes from DB"
  fi

  # --- Stop server process ---
  if [ -n "${SERVER_PID:-}" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
    echo "[cleanup] server stopped"
  fi

  # --- Tear down dev stack ---
  echo "[cleanup] docker compose down"
  $COMPOSE down -v >/dev/null 2>&1 || true

  # --- Remove temp files ---
  # NOTE: the evidence artifact lives in $SIM_EVIDENCE_DIR, deliberately outside
  # this directory, so this rm cannot delete the run's own record (D-014).
  [ -n "${TMPDIR_SIM:-}" ] && rm -rf "$TMPDIR_SIM"
  return 0
}

# The artifact is emitted BEFORE teardown: it is this run's deliverable, and a
# `docker compose down` that hangs or fails must not be able to take it with it.
# $code is re-raised at the end so PASS/FAIL exit semantics stay exactly as they
# were before M5' (D-014).
on_exit() {
  local code=$?
  sim_evidence_emit "$code"
  cleanup || true
  exit "$code"
}

# Init BEFORE the trap: the emitter needs the path, so establishing it first is
# what makes the trap able to report a failure in STEP 0.
sim_evidence_init "sim_multi_node.sh" "3.1" "$COMPOSE_PROJECT"
trap on_exit EXIT

# Tool preflight runs AFTER the trap is armed: a missing tool is exactly the kind
# of runner problem CI needs told about, and once the trap exists that die()
# leaves behind an ERROR artifact naming the tool instead of nothing at all.
for bin in docker go curl jq openssl; do
  command -v "$bin" >/dev/null 2>&1 || die "'$bin' not found in PATH"
done

# =============================================================================
# STEP 0 — Start dev stack
# =============================================================================
echo "==> [0] Starting dev stack"
$COMPOSE up -d >/dev/null 2>&1 || die "docker compose up failed"

for i in $(seq 1 90); do
  state="$(docker inspect -f '{{.State.Status}}' sim31-migrate 2>/dev/null || echo missing)"
  if [ "$state" = "exited" ]; then
    code="$(docker inspect -f '{{.State.ExitCode}}' sim31-migrate 2>/dev/null || echo 1)"
    [ "$code" = "0" ] || die "migrate exited with code $code"
    break
  fi
  [ "$i" -lt 90 ] || die "migration timeout"
  sleep 1
done
echo "  migration OK"

# =============================================================================
# STEP 1 — Build and start server with EVENT_TRACKER_ENABLED=true
# =============================================================================
echo "==> [1] Building server"
TMPDIR_SIM="$(mktemp -d)"
(cd "$SERVER_DIR" && go build -o "$TMPDIR_SIM/quakealert" ./cmd/quakealert) || die "go build failed"

# The confirmation gate itself is NOT configurable and is deliberately not set
# here: consensus.MinNodesConfirmed (3) and consensus.MinPGAGal (16.6) are
# compiled-in constants. Earlier revisions exported MIN_NODES_CONFIRMED and
# MIN_PGA_GAL, which config.go never reads — they implied a knob that does not
# exist, so they are gone rather than kept as decoration (D-014).
# EVENT_RESOLVE_AFTER_MS is the real name of the sweep deadline; the former
# RESOLVE_AFTER_MS was silently ignored, leaving the default 90000 in force.
# The value stays 90000, so runtime behaviour is unchanged.
echo "==> [1] Starting server (EVENT_TRACKER_ENABLED=true)"
DATABASE_URL="postgres://quakealert:devpassword@localhost:5432/quakealert?sslmode=disable" \
REDIS_URL="redis://localhost:6379/0" \
MQTT_BROKER="tcp://localhost:1883" \
MQTT_USER="" \
MQTT_PASSWORD="" \
MQTT_CLIENT_ID="quakealert-sim-server" \
MQTT_PUBLIC_BROKER="localhost" \
MQTT_PUBLIC_PORT="1883" \
MQTT_PUBLIC_TLS="false" \
MASTER_KEY_HEX="0000000000000000000000000000000000000000000000000000000000000001" \
JWT_SECRET="sim-test-jwt-secret-not-production-32b" \
ADMIN_API_KEY="$ADMIN_API_KEY" \
EVENT_TRACKER_ENABLED="true" \
CORRELATION_WINDOW_MS="8000" \
ATTACH_RADIUS_KM="50" \
INDEPENDENCE_CELL_KM="5" \
MIN_INDEPENDENT_CELLS="2" \
EVENT_RESOLVE_AFTER_MS="90000" \
HTTP_ADDR="$HTTP_ADDR" \
FCM_PROJECT_ID="" \
FCM_CREDENTIALS_FILE="" \
  "$TMPDIR_SIM/quakealert" > "$TMPDIR_SIM/server.log" 2>&1 &
SERVER_PID=$!

for i in $(seq 1 30); do
  if curl -fsS "$BASE_URL/healthz" >/dev/null 2>&1; then
    break
  fi
  [ "$i" -lt 30 ] || die "server health timeout. log: $(tail -20 "$TMPDIR_SIM/server.log")"
  sleep 1
done
echo "  server healthy (pid=$SERVER_PID)"

# =============================================================================
# STEP 2 — Insert 3 sim nodes directly into DB (bypasses provisioning rate limit)
# Secrets are hardcoded in sim_setup_nodes.go — fixed, known, dev-only.
# =============================================================================
echo "==> [2] Setting up sim nodes in DB"
SIM_SECRET_A="sec_sim_alpha_checkpoint_3_1_aaaa"
SIM_SECRET_B="sec_sim_bravo_checkpoint_3_1_bbbb"
SIM_SECRET_C="sec_sim_charlie_checkpoint_3_1_cc"

(
  cd "$SERVER_DIR"
  DATABASE_URL="postgres://quakealert:devpassword@localhost:5432/quakealert?sslmode=disable" \
  MASTER_KEY_HEX="0000000000000000000000000000000000000000000000000000000000000001" \
  go run scripts/sim_setup_nodes.go
) || die "sim_setup_nodes.go failed"
echo "  $SIM_A_ID $SIM_B_ID $SIM_C_ID inserted and verified"

# =============================================================================
# STEP 3 — Record baseline stats before triggers
# =============================================================================
echo "==> [3] Baseline tracker stats"
admin_api GET /api/v1/admin/tracker/stats
# The whole counter set is snapshotted, not only the two the assertions read:
# "archive their tracker counters" (ROADMAP.md P4-M5') means the counters, and a
# reader chasing an unexpected delta later cannot go back and ask this run again.
SIM_COUNTERS_BEFORE="$BODY"
BASELINE_CREATED="$(printf '%s' "$BODY" | jq -r .event_created_total)"
BASELINE_CONFIRMED="$(printf '%s' "$BODY" | jq -r .event_transitions_to_confirmed_total)"
echo "  event_created_total=$BASELINE_CREATED event_transitions_to_confirmed_total=$BASELINE_CONFIRMED"

# =============================================================================
# STEP 4 — Publish 3 HMAC-signed triggers
# =============================================================================
echo "==> [4] Publishing 3 sim triggers"
ONSET_TS="$(date +%s%3N)"
(
  cd "$SERVER_DIR"
  go run scripts/sim_confirm_scenario.go \
    "$SIM_SECRET_A" "$SIM_SECRET_B" "$SIM_SECRET_C" "$ONSET_TS"
) || die "sim_confirm_scenario.go failed"

# =============================================================================
# STEP 5 — Wait for Tracker to classify
# =============================================================================
echo "==> [5] Waiting ${SIM_WAIT_SECS}s for classification"
sleep "$SIM_WAIT_SECS"

# =============================================================================
# STEP 6 — Assertions
# =============================================================================
echo "==> [6] Assertions"

admin_api GET /api/v1/admin/tracker/stats
STATS="$BODY"
SIM_COUNTERS_AFTER="$BODY"

CREATED="$(printf '%s' "$STATS" | jq -r .event_created_total)"
UNCONFIRMED="$(printf '%s' "$STATS" | jq -r .event_transitions_to_unconfirmed_total)"
CONFIRMED="$(printf '%s' "$STATS" | jq -r .event_transitions_to_confirmed_total)"
EVICTIONS="$(printf '%s' "$STATS" | jq -r .event_tombstone_evictions_total)"
FORCED="$(printf '%s' "$STATS" | jq -r .event_forced_resolutions_total)"

# D: exactly 1 new event created
NEW_CREATED=$(( CREATED - BASELINE_CREATED ))
if [ "$NEW_CREATED" -eq 1 ]; then
  ok "D: exactly 1 event created (event_created_total delta=$NEW_CREATED)"
else
  bad "D: expected 1 new event, got $NEW_CREATED (total=$CREATED baseline=$BASELINE_CREATED)"
fi

# E: UNCONFIRMED transition occurred
NEW_UNCONFIRMED=$(( UNCONFIRMED ))
if [ "$UNCONFIRMED" -ge 1 ]; then
  ok "E: UNCONFIRMED transition occurred (event_transitions_to_unconfirmed_total=$UNCONFIRMED)"
else
  bad "E: no UNCONFIRMED transition (event_transitions_to_unconfirmed_total=$UNCONFIRMED)"
fi

# F: CONFIRMED transition occurred
NEW_CONFIRMED=$(( CONFIRMED - BASELINE_CONFIRMED ))
if [ "$NEW_CONFIRMED" -eq 1 ]; then
  ok "F: CONFIRMED transition occurred (delta=$NEW_CONFIRMED)"
else
  bad "F: expected 1 CONFIRMED transition, got $NEW_CONFIRMED (total=$CONFIRMED baseline=$BASELINE_CONFIRMED)"
fi

# J: no extra events (no duplicate creation)
if [ "$NEW_CREATED" -le 1 ]; then
  ok "J: no extra events (event_created_total delta=$NEW_CREATED)"
else
  bad "J: extra events created (delta=$NEW_CREATED)"
fi

# tombstone evictions must be zero (plan §22)
if [ "$EVICTIONS" -eq 0 ]; then
  ok "tombstone evictions zero"
else
  bad "unexpected tombstone evictions: $EVICTIONS"
fi

# near-confirmed: independence >= 2, ConfirmedAt > 0
admin_api GET /api/v1/admin/tracker/near-confirmed
NC="$BODY"
# Archived whole, envelope included: an independence count is only readable
# next to the thresholds that produced it, and the M2' coverage envelope
# distinguishes "no crossing happened" from "nothing could be answered"
# (D-012). Both belong in the evidence snapshot.
SIM_NEAR_CONFIRMED="$BODY"
SIM_ALGO_VER="$(printf '%s' "$BODY" | jq -r '.coverage.algo_ver // .entries[0].algo_ver // ""')"
SIM_MIN_INDEP_CELLS="$(printf '%s' "$BODY" | jq -r '.coverage.min_independent_cells // .entries[0].min_independent_cells // ""')"
NC_COUNT="$(printf '%s' "$NC" | jq '.entries | length')"
NC_CONFIRMED_AT="$(printf '%s' "$NC" | jq -r '.entries[0].confirmed_at_ms // 0')"
NC_INDEP="$(printf '%s' "$NC" | jq -r '.entries[0].independent_count_at_peak // 0')"

if [ "$NC_COUNT" -ge 1 ]; then
  ok "near-confirmed: at least 1 entry recorded"
else
  bad "near-confirmed: no entries found"
fi

if [ "$NC_INDEP" -ge 2 ]; then
  ok "G/independence: independent_count_at_peak=$NC_INDEP >= 2"
else
  bad "G/independence: independent_count_at_peak=$NC_INDEP < 2"
fi

if [ "${NC_CONFIRMED_AT:-0}" -gt 0 ]; then
  ok "near-confirmed: ConfirmedAt>0 (event did reach CONFIRMED)"
else
  bad "near-confirmed: ConfirmedAt=0 (event did not reach CONFIRMED)"
fi

# I: REST events feed shows CONFIRMED event
api GET "/api/v1/events"
EVENTS_COUNT="$(printf '%s' "$BODY" | jq -r '.events | length')"
if [ "${EVENTS_COUNT:-0}" -ge 1 ]; then
  EVENT_STATE="$(printf '%s' "$BODY" | jq -r '.events[0].event_state // "null"')"
  EVENT_ID_REST="$(printf '%s' "$BODY" | jq -r '.events[0].event_id')"
  if [ "$EVENT_STATE" = "CONFIRMED" ] || [ "$EVENT_STATE" = "RESOLVED" ]; then
    ok "I: REST /events shows event state=$EVENT_STATE event_id=$EVENT_ID_REST"
  else
    bad "I: REST /events event_state=$EVENT_STATE (want CONFIRMED or RESOLVED)"
  fi
else
  bad "I: REST /events returned no events"
fi

# =============================================================================
# STEP 9 — Summary
# =============================================================================
echo ""
echo "============================================"
echo "Checkpoint 3.1 results: $PASS PASS, $FAIL FAIL"
echo "============================================"

# Reaching here means the run produced a verdict of its own. Below this line the
# artifact says PASS or FAIL; above it, ERROR — the difference between "the
# system misbehaved" and "the run never got far enough to tell" (D-014).
SIM_SUMMARY_REACHED=1

if [ "$FAIL" -gt 0 ]; then
  echo "--- server.log (tail 40) ---"
  tail -40 "$TMPDIR_SIM/server.log" || true
  exit 1
fi
echo "Simulation PASSED. All assertions green."
echo "EVIDENCE CLASS: software simulation only — not field validation, not"
echo "production validation, not real multi-node sensor performance (S9)."
exit 0
