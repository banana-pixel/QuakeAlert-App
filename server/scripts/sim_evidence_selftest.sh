#!/usr/bin/env bash
# =============================================================================
# sim_evidence_selftest.sh — pins the P4-M5' artifact-delivery contract
#
# Runs in about a second and touches nothing: no Docker, no database, no server,
# no network. It exists because the two things that broke the first real CI run
# of the simulation job were both invisible to the harnesses' own assertions —
# the harnesses PASSED, wrote both artifacts, and the workflow still archived
# nothing. A green simulation that archives no evidence is the one outcome that
# makes M5' meaningless (D-014), so the delivery path gets its own test.
#
# What this pins, in the order the requirements were stated:
#   1  the artifact directory is rooted at the repo root ($GITHUB_WORKSPACE on a
#      runner), not at $PWD and not inside a temp dir
#   2  the directory is created by sim_evidence_init, before any exit can happen
#   3  the EXIT trap always writes the final artifact — including on a run that
#      aborts before it ever reaches its assertion summary
#   4  cleanup cannot remove the final artifact
#   5  both harnesses use the same artifact contract, from one file
#   6  serial execution cannot overwrite the other harness's artifact
#   7  a failure in one harness does not stop the other from emitting its own
#   8  the artifact filenames are EXACTLY the two names the workflow uploads
#   9  the workflow's upload step can actually see a DOTTED directory
#
# Check 9 is the one that cost a run: `.sim-evidence` starts with a dot, and
# actions/glob skips any path component whose basename matches /^\./ unless
# hidden files are included. Bash and Python globs do not behave that way, which
# is precisely why a local reproduction could not catch it — so the coupling
# between the dotted path and `include-hidden-files: true` is asserted here as
# text, against the workflow file itself.
#
# Usage: server/scripts/sim_evidence_selftest.sh
# Exit 0 = every check held. Non-zero = the delivery path is broken.
# =============================================================================
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPTS="$ROOT/server/scripts"
WORKFLOW="$ROOT/.github/workflows/ci.yml"

PASS=0
FAIL=0
ok()  { PASS=$((PASS + 1)); echo "  PASS  $1"; }
bad() { FAIL=$((FAIL + 1)); echo "  FAIL  $1"; }

SELFTEST_TMP="$(mktemp -d)"
trap 'rm -rf "$SELFTEST_TMP"' EXIT

EXPECT_MULTI=".sim-evidence/sim_multi_node.evidence.json"
EXPECT_DUAL=".sim-evidence/sim_dual_event.evidence.json"

echo "==> [1] Both harnesses source ONE artifact contract"

for f in sim_multi_node.sh sim_dual_event.sh; do
  if grep -q '^\. "\$SERVER_DIR/scripts/sim_evidence.sh"' "$SCRIPTS/$f"; then
    ok "$f sources sim_evidence.sh (contract not copied)"
  else
    bad "$f does not source sim_evidence.sh — the field names can drift"
  fi
done

# A second definition of the emitter anywhere would defeat the shared contract.
# Anchored at line start, so this file's own mention of the name does not count
# as a definition of it.
emitters="$(grep -l '^sim_evidence_emit() {' "$SCRIPTS"/*.sh | wc -l)"
if [ "$emitters" -eq 1 ]; then
  ok "exactly one definition of sim_evidence_emit exists"
else
  bad "expected 1 definition of sim_evidence_emit, found $emitters"
fi

echo "==> [2] Artifact paths are derived, rooted, and exact"

# Derive the paths the way the harnesses do — by sourcing the real library and
# calling the real init — rather than by re-deriving them here. A test that
# recomputes the answer it is checking proves nothing.
derive() { # <script-name> -> echoes SIM_EVIDENCE_PATH
  ( set -euo pipefail
    ROOT="$ROOT"
    unset SIM_EVIDENCE_DIR
    . "$SCRIPTS/sim_evidence.sh"
    sim_evidence_init "$1" "0.0" "selftest" >/dev/null
    printf '%s' "$SIM_EVIDENCE_PATH" )
}

multi_path="$(derive sim_multi_node.sh)"
dual_path="$(derive sim_dual_event.sh)"

for pair in "sim_multi_node.sh|$multi_path|$EXPECT_MULTI" "sim_dual_event.sh|$dual_path|$EXPECT_DUAL"; do
  IFS='|' read -r name got want <<<"$pair"
  if [ "$got" = "$ROOT/$want" ]; then
    ok "$name writes exactly $want, rooted at the repo root"
  else
    bad "$name writes $got, expected $ROOT/$want"
  fi
done

if [ "$multi_path" != "$dual_path" ]; then
  ok "the two harnesses cannot overwrite each other's artifact"
else
  bad "both harnesses resolve to the same artifact path"
fi

# On a runner the repo root IS $GITHUB_WORKSPACE, and the upload step's relative
# glob is resolved against it. If those two ever diverge the glob looks in the
# wrong place, so the equality is asserted where it can be seen.
if [ -n "${GITHUB_WORKSPACE:-}" ]; then
  if [ "$ROOT" = "$GITHUB_WORKSPACE" ]; then
    ok "repo root == \$GITHUB_WORKSPACE ($ROOT)"
  else
    bad "repo root $ROOT != \$GITHUB_WORKSPACE $GITHUB_WORKSPACE"
  fi
else
  echo "  note  \$GITHUB_WORKSPACE unset (local run); its equality with the repo"
  echo "        root is checked when this runs in CI"
fi

# The directory must exist the moment init returns: the emitter is called from
# an EXIT trap, and a run that dies in STEP 0 still owes an artifact.
# `if (...)` and not `(...)` followed by `$?`: under `set -e` a failing subshell
# outside a condition would abort this script instead of recording a FAIL.
if ( set -euo pipefail
     ROOT="$SELFTEST_TMP/rooted"
     mkdir -p "$ROOT"
     unset SIM_EVIDENCE_DIR
     . "$SCRIPTS/sim_evidence.sh"
     sim_evidence_init "sim_multi_node.sh" "3.1" "selftest" >/dev/null
     [ -d "$SIM_EVIDENCE_DIR" ] ); then
  ok "sim_evidence_init creates the evidence directory before returning"
else
  bad "sim_evidence_init returned without creating the evidence directory"
fi

echo "==> [3] The EXIT trap writes an artifact on every path out"

# A miniature harness with the same shape as the real ones: source the library,
# arm the trap, then leave by one of three doors. The library is the real one —
# only the simulation is fake, because what is under test is the delivery of the
# artifact and not the correctness of any assertion.
mini() { # <script-name> <mode: pass|fail|abort> <root>
  local name="$1" mode="$2" root="$3"
  ( set -euo pipefail
    ROOT="$root"
    unset SIM_EVIDENCE_DIR
    PASS=0
    FAIL=0
    . "$SCRIPTS/sim_evidence.sh"
    ok()  { PASS=$((PASS + 1)); sim_record_assertion PASS "$1"; }
    bad() { FAIL=$((FAIL + 1)); sim_record_assertion FAIL "$1"; }
    die() { SIM_ERROR="$1"; exit 1; }
    # The same three-line trap the harnesses use: artifact, then teardown, then
    # re-raise. cleanup() here does what theirs does to the filesystem — it
    # removes its temp dir — so the "cleanup cannot delete the artifact" claim
    # is tested against the real ordering.
    cleanup() { rm -rf "$root/tmp-of-run"; return 0; }
    on_exit() { local code=$?; sim_evidence_emit "$code"; cleanup || true; exit "$code"; }
    mkdir -p "$root"
    sim_evidence_init "$name" "9.9" "selftest" >/dev/null
    trap on_exit EXIT
    mkdir -p "$root/tmp-of-run"
    case "$mode" in
      abort) die "aborted in STEP 0 on purpose" ;;
      pass)  ok "a thing held"; SIM_SUMMARY_REACHED=1; exit 0 ;;
      fail)  ok "a thing held"; bad "a thing did not hold"; SIM_SUMMARY_REACHED=1; exit 1 ;;
    esac ) >/dev/null 2>&1
}

check_artifact() { # <path> <want-status> <want-exit> <label>
  local f="$1" want_status="$2" want_exit="$3" label="$4" got_status got_exit
  if [ ! -f "$f" ]; then
    bad "$label: no artifact at $f"
    return 0
  fi
  if ! jq -e . "$f" >/dev/null 2>&1; then
    bad "$label: artifact is not valid JSON"
    return 0
  fi
  got_status="$(jq -r '.status' "$f")"
  got_exit="$(jq -r '.exit_code' "$f")"
  if [ "$got_status" = "$want_status" ] && [ "$got_exit" = "$want_exit" ]; then
    ok "$label: status=$got_status exit_code=$got_exit"
  else
    bad "$label: got status=$got_status exit_code=$got_exit, wanted $want_status/$want_exit"
  fi
}

R_PASS="$SELFTEST_TMP/pass"
R_FAIL="$SELFTEST_TMP/fail"
R_ABORT="$SELFTEST_TMP/abort"

mini sim_multi_node.sh pass  "$R_PASS"  || true
mini sim_multi_node.sh fail  "$R_FAIL"  || true
mini sim_multi_node.sh abort "$R_ABORT" || true

check_artifact "$R_PASS/$EXPECT_MULTI"  PASS  0 "clean run"
check_artifact "$R_FAIL/$EXPECT_MULTI"  FAIL  1 "failed assertions"
# ERROR, not FAIL: the run never reached a verdict, and a broken runner is not a
# broken detector. Collapsing the two is the confusion M5' exists to avoid.
check_artifact "$R_ABORT/$EXPECT_MULTI" ERROR 1 "aborted before the summary"

if [ -n "$(jq -r '.error // ""' "$R_ABORT/$EXPECT_MULTI")" ]; then
  ok "the aborted run's artifact names why it aborted"
else
  bad "the aborted run's artifact has no error text"
fi

echo "==> [4] Cleanup cannot remove the final artifact"

# The trap ran cleanup AFTER emitting, and cleanup deleted its temp dir. If the
# artifact were written inside that dir it would now be gone.
if [ -f "$R_PASS/$EXPECT_MULTI" ] && [ ! -d "$R_PASS/tmp-of-run" ]; then
  ok "artifact survives teardown that removed the run's temp dir"
else
  bad "artifact did not survive teardown"
fi

for f in sim_multi_node.sh sim_dual_event.sh; do
  if grep -q 'sim_evidence_emit "\$code"' "$SCRIPTS/$f" &&
     awk '/^on_exit\(\)/,/^}/' "$SCRIPTS/$f" |
       grep -n 'sim_evidence_emit\|cleanup' | head -2 |
       paste -sd' ' - | grep -q 'sim_evidence_emit.*cleanup'; then
    ok "$f emits the artifact BEFORE cleanup, in the EXIT trap"
  else
    bad "$f does not emit the artifact before cleanup"
  fi
done

echo "==> [5] Serial runs keep both artifacts, and one failure does not silence the other"

# The workflow runs 3.1 then 3.2 in one workspace. Emulated here with one shared
# root: a red 3.1 must not stop 3.2 from leaving its own record, and neither may
# overwrite the other — which is what `continue-on-error` on both steps buys, and
# what distinct filenames make safe.
R_SERIAL="$SELFTEST_TMP/serial"
mini sim_multi_node.sh fail "$R_SERIAL" || true
mini sim_dual_event.sh pass "$R_SERIAL" || true

check_artifact "$R_SERIAL/$EXPECT_MULTI" FAIL 1 "serial: 3.1 red"
check_artifact "$R_SERIAL/$EXPECT_DUAL"  PASS 0 "serial: 3.2 green after a red 3.1"

n="$(find "$R_SERIAL/.sim-evidence" -name '*.evidence.json' | wc -l)"
if [ "$n" -eq 2 ]; then
  ok "both artifacts coexist after a serial run (found $n)"
else
  bad "expected 2 artifacts after a serial run, found $n"
fi

echo "==> [6] The workflow can actually SEE a dotted directory"

# The defect this check exists for: the harnesses passed, both artifacts were on
# disk, and upload-artifact still reported "No files were found" — because
# actions/glob skips path components whose basename starts with `.` unless
# hidden files are included. Nothing in the harnesses can detect that, so the
# coupling is asserted against the workflow text.
if [ ! -f "$WORKFLOW" ]; then
  bad "cannot read $WORKFLOW"
else
  sim_job="$(awk '/^  simulation:/,0' "$WORKFLOW")"
  archive_step="$(printf '%s\n' "$sim_job" | awk '/- name: Archive simulation evidence/,/^      - name: Verify/')"

  # The two artifact names in the upload glob must be the two names the library
  # derives. A rename on either side and the archive silently empties.
  glob_line="$(printf '%s\n' "$archive_step" | grep -E '^\s+path: ' | head -1 | sed 's/^\s*path: //')"
  if [ "$glob_line" = ".sim-evidence/*.evidence.json" ]; then
    ok "upload glob is .sim-evidence/*.evidence.json"
  else
    bad "upload glob is '$glob_line', not .sim-evidence/*.evidence.json"
  fi

  case "$glob_line" in
    .*)
      if printf '%s\n' "$archive_step" | grep -qE '^\s+include-hidden-files:\s*true\s*$'; then
        ok "dotted upload path is paired with include-hidden-files: true"
      else
        bad "upload path '$glob_line' is dotted but include-hidden-files is not true — actions/glob will find nothing"
      fi ;;
    *)
      ok "upload path is not dotted; include-hidden-files is not required" ;;
  esac

  if printf '%s\n' "$archive_step" | grep -qE '^\s+if-no-files-found:\s*error\s*$'; then
    ok "if-no-files-found: error retained (a silent empty archive stays impossible)"
  else
    bad "if-no-files-found is not error — an empty archive could pass unnoticed"
  fi

  if printf '%s\n' "$archive_step" | grep -qE '^\s+if: always\(\)\s*$'; then
    ok "if: always() retained (a red simulation still archives its evidence)"
  else
    bad "the archive step is not if: always()"
  fi
fi

echo "==> [7] The generated admin key is masked before anything can print it"

# The key is written to $GITHUB_ENV, which makes it a job-level env var, and the
# runner prints the whole env block at the head of every later step. Registering
# the mask AFTER the write is therefore too late — the ordering is the fix, so
# the ordering is what gets asserted.
if [ -f "$WORKFLOW" ]; then
  # Comments are stripped BEFORE the lines are numbered. The step's own comment
  # explains the ordering and so contains the word add-mask; numbering first and
  # filtering afterwards compares against that prose instead of the code, and the
  # check passes even when the two commands are swapped (it did).
  keystep="$(awk '/- name: Generate ephemeral ADMIN_API_KEY/,/^      # The two harnesses run SERIALLY/' "$WORKFLOW" \
    | grep -vE '^\s*#')"
  mask_at="$(printf '%s\n' "$keystep" | grep -n 'add-mask' | head -1 | cut -d: -f1)"
  env_at="$(printf '%s\n' "$keystep" | grep -n 'GITHUB_ENV' | tail -1 | cut -d: -f1)"
  if [ -n "$mask_at" ] && [ -n "$env_at" ] && [ "$mask_at" -lt "$env_at" ]; then
    ok "::add-mask:: is emitted before the key reaches \$GITHUB_ENV"
  else
    bad "the generated key reaches \$GITHUB_ENV without a mask registered first"
  fi
  if printf '%s\n' "$keystep" | grep -q 'openssl rand'; then
    ok "the key is still generated at runtime, not stored as a repository secret"
  else
    bad "the key no longer looks runtime-generated"
  fi
fi

echo "==> [8] No artifact carries the admin key"

# The artifact is uploaded and downloaded by people. Whatever else it contains,
# it must not contain the credential the harness authenticated with.
leaked=0
for f in "$R_PASS/$EXPECT_MULTI" "$R_SERIAL/$EXPECT_DUAL"; do
  [ -f "$f" ] || continue
  if grep -qiE 'admin[_-]?api[_-]?key|x-admin-key' "$f"; then
    leaked=1
    bad "$f mentions the admin key"
  fi
done
[ "$leaked" -eq 0 ] && ok "no artifact mentions the admin key"

echo
echo "============================================"
echo "sim_evidence selftest: $PASS PASS, $FAIL FAIL"
echo "============================================"
if [ "$FAIL" -gt 0 ]; then
  echo "The artifact DELIVERY path is broken. This is not a simulation failure:" >&2
  echo "the harnesses can pass every assertion and still archive nothing." >&2
  exit 1
fi
echo "Artifact delivery contract holds. This test says nothing about the"
echo "simulation itself — it pins only that evidence is produced, survives"
echo "teardown, and can be found by the workflow that uploads it."
