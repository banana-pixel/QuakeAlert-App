#!/usr/bin/env bash
# =============================================================================
# sim_evidence.sh — structured JSON evidence for the P4-M5' simulation harnesses
#
# Sourced by sim_multi_node.sh and sim_dual_event.sh. It exists because P4-M5'
# asks those harnesses to "archive their tracker counters and evidence
# snapshots" (ROADMAP.md § Phase 4), and an artifact reconstructed afterwards
# from CI logs is not that: a log is a rendering of the run, while this is the
# run's own record, written by the run itself while it still holds the values.
#
# One implementation, two callers. The field names ARE the contract (D-014), and
# a contract copied into two scripts is a contract that drifts.
#
# EVERY ARTIFACT IS SOFTWARE EVIDENCE ONLY. The simulation drives virtual nodes
# that are database rows carrying hand-picked coordinates. It says nothing about
# field correlation, production behaviour, or real multi-node sensor performance
# (S9; D-011 constraint 2). The artifact carries that boundary inside its own
# body, in `evidence.evidence_class` and `evidence.not_claimed`, so the numbers
# can never be read apart from what qualifies them.
#
# Contract, schema_version 1 — top level:
#   schema_version, run_id, git_sha, git_dirty, script, checkpoint, status,
#   exit_code, started_at, finished_at, error,
#   tracker_counters_before, tracker_counters_after, evidence
# and inside `evidence`:
#   evidence_class, not_claimed, stack, compose_project, algo_ver,
#   min_independent_cells, assertions_passed, assertions_failed, observed,
#   assertions
#
# `status` is PASS, FAIL or ERROR. PASS and FAIL mean the run reached its own
# summary and counted its assertions; ERROR means it aborted before that, so
# neither a pass nor a failure was established. Those are three different
# things and collapsing ERROR into FAIL would report a broken runner as a
# broken system.
#
# The caller supplies: $ROOT, $PASS, $FAIL, and optionally a function
# sim_observed_json echoing one JSON object of the scalars it actually asserted
# on. Output path: $SIM_EVIDENCE_DIR (default $ROOT/.sim-evidence).
# =============================================================================

SIM_EVIDENCE_SCHEMA_VERSION=1

# Declared at load time, not only in sim_evidence_init: under `set -u` a caller
# that records or emits before init would abort on an unset array, and the
# emitter must never be the thing that kills the run it is documenting.
SIM_ASSERTIONS=()
SIM_SUMMARY_REACHED=0
SIM_ERROR=""

# _sim_json_or_null echoes $1 when it parses as JSON, else `null`. A snapshot
# never taken must be visibly absent rather than silently empty.
_sim_json_or_null() {
  local s="${1:-}"
  if [ -z "$s" ]; then printf 'null'; return 0; fi
  if printf '%s' "$s" | jq -e . >/dev/null 2>&1; then printf '%s' "$s"; else printf 'null'; fi
}

# _sim_num echoes $1 when it is an integer, else `null` — so a value the run
# never reached is absent, not zero. Zero is a measurement; absence is not.
_sim_num() {
  local v="${1:-}"
  case "$v" in
    ''|*[!0-9-]*) printf 'null' ;;
    *)            printf '%s' "$v" ;;
  esac
}

# -----------------------------------------------------------------------------
# sim_evidence_init — call once, after $ROOT is known and before STEP 0.
#
# Fixes the artifact path BEFORE the run starts, because the emitter is called
# from the EXIT trap and a run that dies in STEP 0 still owes an artifact.
#
# The path deliberately sits OUTSIDE the run's mktemp dir: both harnesses'
# cleanup() ends in `rm -rf "$TMPDIR_SIM"`, so an artifact written there would
# be deleted by the very trap that produced it.
# -----------------------------------------------------------------------------
sim_evidence_init() {
  SIM_SCRIPT_NAME="$1"     # e.g. sim_multi_node.sh
  SIM_CHECKPOINT="$2"      # e.g. 3.1
  SIM_COMPOSE_PROJECT="$3" # e.g. sim31

  SIM_EVIDENCE_DIR="${SIM_EVIDENCE_DIR:-$ROOT/.sim-evidence}"
  SIM_EVIDENCE_PATH="$SIM_EVIDENCE_DIR/${SIM_SCRIPT_NAME%.sh}.evidence.json"

  SIM_STARTED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  # run_id is overridable so CI can carry its own run/attempt identity; locally
  # the timestamp plus pid keeps two runs on one machine distinguishable.
  SIM_RUN_ID="${SIM_RUN_ID:-${SIM_SCRIPT_NAME%.sh}-$(date -u +%Y%m%dT%H%M%SZ)-$$}"

  SIM_GIT_SHA="$(git -C "$ROOT" rev-parse HEAD 2>/dev/null || echo unknown)"
  # git_dirty is not decoration: an artifact produced from a modified tree is
  # not reproducible from its own git_sha, and a reader must be able to see that
  # from the artifact rather than infer it (V7).
  if [ -n "$(git -C "$ROOT" status --porcelain 2>/dev/null)" ]; then
    SIM_GIT_DIRTY=true
  else
    SIM_GIT_DIRTY=false
  fi

  SIM_ASSERTIONS=()
  SIM_SUMMARY_REACHED=0
  SIM_ERROR=""
  SIM_COUNTERS_BEFORE=""
  SIM_COUNTERS_AFTER=""
  SIM_NEAR_CONFIRMED=""
  SIM_ALGO_VER=""
  SIM_MIN_INDEP_CELLS=""

  mkdir -p "$SIM_EVIDENCE_DIR"
  echo "  evidence artifact: $SIM_EVIDENCE_PATH"
}

# sim_record_assertion <PASS|FAIL> <text> — one entry per ok()/bad() call, in
# call order. The artifact carries the assertion texts the run actually printed,
# so the archived evidence and the human-readable stdout cannot disagree.
sim_record_assertion() {
  local st="$1" text="$2" idx
  idx="${#SIM_ASSERTIONS[@]}"
  SIM_ASSERTIONS+=("$(jq -nc --argjson index "$idx" --arg status "$st" --arg assertion "$text" \
    '{index: $index, status: $status, assertion: $assertion}')")
}

# -----------------------------------------------------------------------------
# sim_evidence_emit <exit_code> — write the artifact. Called from the EXIT trap
# so that it runs on every path out of the script: green summary, red summary,
# or a die() in the middle of STEP 0.
#
# It must never be the reason a run fails. The exit code belongs to the
# simulation's own PASS/FAIL assertions, and a JSON writer that could change it
# would make the artifact part of the thing being measured — so every failure
# here is reported on stderr and swallowed.
# -----------------------------------------------------------------------------
sim_evidence_emit() {
  local code="${1:-0}" status
  [ -n "${SIM_EVIDENCE_PATH:-}" ] || return 0

  # PASS/FAIL are claims about the simulation; ERROR says the run never got far
  # enough to make one. Folding ERROR into FAIL would report a broken runner as
  # a broken system, which is exactly the confusion this milestone exists to
  # avoid.
  if [ "${SIM_SUMMARY_REACHED:-0}" -eq 1 ]; then
    if [ "${FAIL:-0}" -gt 0 ]; then status="FAIL"; else status="PASS"; fi
  else
    status="ERROR"
    [ -n "${SIM_ERROR:-}" ] || SIM_ERROR="run aborted before the assertion summary was reached (exit $code)"
  fi

  # `|| assertions_json="[]"` matters under `set -e`: this runs inside an EXIT
  # trap, and an unguarded failure here would abandon the trap and lose the
  # artifact entirely rather than degrade one field.
  # SIM_ERROR can carry a server-log tail (die() embeds one on a health timeout),
  # which is content this emitter does not author. Capped so one unbounded field
  # cannot dominate the artifact, with the truncation stated rather than silent.
  local err="${SIM_ERROR:-}"
  if [ "${#err}" -gt 2000 ]; then
    err="${err:0:2000}… [truncated, ${#SIM_ERROR} chars total]"
  fi

  local assertions_json="[]"
  if [ "${#SIM_ASSERTIONS[@]}" -gt 0 ]; then
    assertions_json="$(printf '%s\n' "${SIM_ASSERTIONS[@]}" | jq -sc . 2>/dev/null)" \
      || assertions_json="[]"
  fi

  # The caller owns `observed`: only the harness knows which scalars its own
  # assertions rested on.
  local observed_json="null"
  if declare -F sim_observed_json >/dev/null 2>&1; then
    observed_json="$(sim_observed_json 2>/dev/null || printf 'null')"
    observed_json="$(_sim_json_or_null "$observed_json")"
  fi

  local tmp_out
  tmp_out="$(mktemp "${SIM_EVIDENCE_PATH}.XXXXXX")" || {
    echo "[evidence] WARN: cannot create temp file next to $SIM_EVIDENCE_PATH" >&2
    return 0
  }

  # NOT_CLAIMED is carried inside the artifact, not only in the docs. An
  # artifact travels: it is uploaded, downloaded, pasted into a report, quoted
  # months later. The boundary has to travel with the numbers (S9, D-011
  # constraint 2), because the numbers alone read like field evidence.
  if jq -n \
      --argjson schema_version "$SIM_EVIDENCE_SCHEMA_VERSION" \
      --arg run_id "${SIM_RUN_ID:-unknown}" \
      --arg git_sha "${SIM_GIT_SHA:-unknown}" \
      --argjson git_dirty "${SIM_GIT_DIRTY:-false}" \
      --arg script "${SIM_SCRIPT_NAME:-unknown}" \
      --arg checkpoint "${SIM_CHECKPOINT:-unknown}" \
      --arg status "$status" \
      --argjson exit_code "$code" \
      --arg started_at "${SIM_STARTED_AT:-}" \
      --arg finished_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
      --arg error "$err" \
      --argjson counters_before "$(_sim_json_or_null "${SIM_COUNTERS_BEFORE:-}")" \
      --argjson counters_after "$(_sim_json_or_null "${SIM_COUNTERS_AFTER:-}")" \
      --arg compose_project "${SIM_COMPOSE_PROJECT:-unknown}" \
      --arg algo_ver "${SIM_ALGO_VER:-}" \
      --argjson min_independent_cells "$(_sim_num "${SIM_MIN_INDEP_CELLS:-}")" \
      --argjson passed "$(_sim_num "${PASS:-0}")" \
      --argjson failed "$(_sim_num "${FAIL:-0}")" \
      --argjson observed "$observed_json" \
      --argjson near_confirmed "$(_sim_json_or_null "${SIM_NEAR_CONFIRMED:-}")" \
      --argjson assertions "$assertions_json" \
      '{
        schema_version: $schema_version,
        run_id: $run_id,
        git_sha: $git_sha,
        git_dirty: $git_dirty,
        script: $script,
        checkpoint: $checkpoint,
        status: $status,
        exit_code: $exit_code,
        started_at: $started_at,
        finished_at: $finished_at,
        error: (if $error == "" then null else $error end),
        tracker_counters_before: $counters_before,
        tracker_counters_after: $counters_after,
        evidence: {
          evidence_class: "SOFTWARE_SIMULATION",
          not_claimed: [
            "field validation",
            "production validation",
            "real multi-node sensor performance",
            "real multi-node correlation"
          ],
          stack: "local ephemeral docker compose (postgis, migrate, redis, mosquitto) + server built from this tree",
          compose_project: $compose_project,
          algo_ver: (if $algo_ver == "" then null else $algo_ver end),
          min_independent_cells: $min_independent_cells,
          assertions_passed: $passed,
          assertions_failed: $failed,
          observed: $observed,
          near_confirmed: $near_confirmed,
          assertions: $assertions
        }
      }' > "$tmp_out" 2>/dev/null
  then
    mv -f "$tmp_out" "$SIM_EVIDENCE_PATH" 2>/dev/null || true
    # mktemp makes the file 0600; an archived artifact is meant to be read, and
    # nothing in it is a secret.
    chmod 644 "$SIM_EVIDENCE_PATH" 2>/dev/null || true
    echo "[evidence] $status -> $SIM_EVIDENCE_PATH"
  else
    rm -f "$tmp_out" 2>/dev/null || true
    echo "[evidence] WARN: failed to render artifact JSON" >&2
  fi
}
