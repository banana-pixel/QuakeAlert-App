# ROADMAP.md

> ## ACTIVE PHASE: **Phase 4 — Self-Measurement & Forensics** — status `IN_PROGRESS`
> Scope approved by the owner 2026-09-01 as acceptance criteria **P4-M1′ … P4-M6′**
> (see § Phase 4 below, and D-011 in `docs/DECISIONS.md`). Work is bounded to those
> six criteria: instrumentation and read-only forensics on a **one-node** fleet.
> Nothing outside them is in scope, and no threshold, quorum, radius, contract,
> event semantic, or notification policy changes under this phase.

Baseline commit: `1ad1777`. Authority: see `PROJECT_RULES.md` §5.

---

## Phase lifecycle

| Status | Meaning |
| --- | --- |
| `PLANNED` | Scope agreed, work not started. |
| `IN_PROGRESS` | Being implemented now. This is the ACTIVE PHASE. |
| `IMPLEMENTED` | Code exists, unit tests pass. No field evidence. |
| `VALIDATED` | Behaviour demonstrated by evidence outside unit tests. |
| `RELEASED` | Deployed to the target environment and verified there. |
| `BLOCKED` | Cannot proceed. **Must name the blocker and who can clear it.** |
| `SUPERSEDED` | Replaced by later work. Definition preserved, successor named. |
| `ABANDONED` | Deliberately not being done. Reason recorded. No successor. |

Rules:

- **Exactly one phase may be `IN_PROGRESS`.**
- `IMPLEMENTED → VALIDATED` cannot be granted by the agent that implemented it.
- **Unit tests do not equal field validation.** Passing tests grant
  `IMPLEMENTED`, never `VALIDATED`.
- `BLOCKED` without a named blocker is invalid.
- A `VALIDATION` or `RESEARCH` phase that produces no code is a complete,
  successful phase.

Phase kinds: `ENGINEERING` (produces code/contracts), `VALIDATION` (produces
evidence, no feature), `RELEASE` (produces a verified deployed state),
`RESEARCH` (produces a decision or a documented dead end).

---

## Phases

| Phase | Kind | Status | Depends on | Exit criteria |
| --- | --- | --- | --- | --- |
| 1 — Observation ledger & ingest | ENGINEERING | `RELEASED` | — | met |
| 2 — Consensus engine | ENGINEERING | `SUPERSEDED` by Phase 3 | 1 | met, superseded |
| 3 — Event architecture | ENGINEERING | `IMPLEMENTED` (`9752c5e`) | 1, 2 | see below |
| 3.x — Global spatial hardening + observability | ENGINEERING | `IMPLEMENTED` (`1ad1777`) | 3 | see below |
| 4 — Self-measurement & forensics | VALIDATION | `IN_PROGRESS` | 3.x | P4-M1′…P4-M6′, see below |
| F — Field validation | VALIDATION | `BLOCKED` | 3.x | see below |

### Phase 1 — Observation ledger & ingest — `RELEASED`
MQTTS ingest, HMAC-authenticated observations, immutable observation ledger.

### Phase 2 — Consensus engine — `SUPERSEDED`
Superseded by Phase 3. Retained behind `EVENT_TRACKER_ENABLED=false` as the
rollback path. Whether and when this code is deleted is **U-008 (unresolved)**.

Documents describing Phase 2 as the current architecture are historical:
`docs/SYSTEM_SPEC.md`, `docs/GAP_ANALYSIS.md`.

### Phase 3 — Event architecture — `IMPLEMENTED`
Five-state event lifecycle, in-memory tracker as authority with the database as
follower, one immutable row per transition, async persistence that never blocks
emission.

Exit criteria:
- [x] State machine implemented; illegal transitions rejected.
- [x] Persistence never blocks emission.
- [x] Unit and integration tests pass.
- [ ] Field-validated → requires Phase F.

### Phase 3.x — Global spatial hardening + observability — `IMPLEMENTED`
Candidate-search width derived per observation from its own latitude, longitude
folded at the antimeridian, independence measured as geodesic distance between
contributors, near-confirmation observability log, two read-only admin
endpoints.

Exit criteria:
- [x] Coverage invariant holds at every latitude, both axes, across the
      antimeridian, with a sensitivity test proving the old fixed neighbourhood
      would fail.
- [x] Polar case reported explicitly rather than dividing by `cos(lat) → 0`.
- [x] Phase 2 path unaffected (`EVENT_TRACKER_ENABLED=false`).
- [x] New endpoints documented in `contracts/openapi/openapi.yaml`.
- [ ] Field-validated → requires Phase F.

### Phase 4 — Self-measurement & forensics — `IN_PROGRESS`
**Scope approved by the owner 2026-09-01.** Recorded as **D-011** in
`docs/DECISIONS.md`. Phase 4 measures **this system against itself**. It is a
`VALIDATION` phase and produces instrumentation plus read-only forensics, no
feature and no change to any decision the system makes.

Intended concept (unchanged):
- Measure whether CONFIRMED is reachable by the current network, and what stops
  it when it is not.
- Measure stage latency, split rate, network geometry ceiling, per-node trigger
  rate.
- Deterministic replay of recorded observations, grouped by `algo_ver`
  (`PROJECT_RULES.md` §11).
- Manual, post-event forensic reconstruction of individual events.

**Fleet reality bounding this phase (FACT).** One physical ESP32, and no
additional nodes are planned during Phase 4. The confirmation gate needs ≥3
verified contributors in ≥2 independence cells, so **CONFIRMED is unreachable in
production for the whole of Phase 4**, by network density and not by defect (S2).
Every criterion below is therefore written to be satisfiable, or explicitly
reported as unsatisfiable, on a one-node fleet. The instrumentation must be
N-node-correct on the day a second node appears, which means it records
`node_id`, per-node onset provenance, `event_id`, `algo_ver` and cell identity
from the outset — all of which the Phase 3/3.x rows already carry.

#### Exit criteria — owner-approved P4-M1′ … P4-M6′

Each is bound to `algo_ver = phase3-1.1/ic=<independence_km>`. Unit tests grant
`IMPLEMENTED` only; `VALIDATED` needs the archived evidence named in each row and
may not be granted by the agent that implemented it (`PROJECT_RULES.md` §8, S9).

- [ ] **P4-M1′ — Trigger durability, measured not asserted.** Over a bounded
      window (last N ledger observations, **not** a fixed calendar period), every
      `sensor_observations` row with `pga ≥ MinPGAGal` has exactly one
      `event_state_log` transition into `UNCONFIRMED` and one advisory WebSocket
      frame; `event_persist_dropped_total` and `event_upsert_failures_total` are
      **reported alongside**, never required to be zero.
- [x] **P4-M2′ — Near-confirmation log is queryable and survives a restart.**
      With one node the correct answer is **empty**, and it must still be empty —
      and answerable — after the process restarts, rather than merely absent
      because the in-memory map was rebuilt.
      `IMPLEMENTED` 2026-09-02, authorized by **D-012** (A2 + B1): durable
      `event_near_confirmed` (migration `000009`, additive) written asynchronously
      through the existing bounded drop-oldest ledger queue, plus an additive
      `coverage` envelope on `GET /api/v1/admin/tracker/near-confirmed` so that an
      empty list states the window it covers and its provenance. Silent crossings —
      those that produce no state transition, hence no `event_state_log` row — are
      recorded too.
      **Owner-approved SATISFIED / `VALIDATED` 2026-09-03**, on archived evidence
      from a real PostgreSQL database (`TEST_DATABASE_URL`, isolated container,
      migration `000009` applied): 14 integration tests that had never run before
      now pass — 11 in `internal/store/near_confirmed_test.go` (merge monotonicity,
      first-wins terminal columns, recorded parameters never rewritten, one row per
      event, list order, `000009` up/down idempotence, no parent-event requirement,
      NULL ≠ zero) and 3 in `internal/event/nearconfirmed_pg_test.go` (restart
      survival, terminal state outliving its parent row, persistence failure not
      blocking emission), alongside 19 pre-existing Postgres tests re-run green on
      the same schema (11 event-lifecycle, 5 ledger, 3 pending-node purge). Restart was then reproduced end-to-end through the HTTP
      surface across four service runs against that database: a real silent crossing
      driven by two signed MQTT triggers produced `source=RECORDED` plus a durable
      row while `event_state_log` gained **no** row for the crossing; after process
      termination and reload the same entries returned `source=LOADED`; and a run
      configured with `ic=5`/threshold 3 — different from every stored row —
      returned per-entry `algo_ver` `[ic=5, ic=5, ic=9]` and per-entry
      `min_independent_cells` 2 against `coverage.algo_ver = ic=5` and
      `coverage.min_independent_cells = 3`, with the durable rows byte-identical
      after boot: the running configuration does not overwrite recorded history.
      On an empty database the endpoint answered `entries: []` with
      `durable_read_attempted: true`, `durable_read_ok: true`,
      `durable_rows_loaded: 0` — the empty answer states its own coverage, which is
      what this criterion asks for on a one-node fleet. All five captured 200 bodies
      validated against `contracts/openapi/openapi.yaml` with zero undocumented
      fields; both 401 bodies validated against `Error`.
      **Still not demonstrated:** production deployment of this code, concurrent
      multi-process writers against one row (merge order-independence is proven at
      SQL level only), and `ListNearConfirmed` full-scan cost at large table size.
      One **unrelated pre-existing** contract gap was found and deliberately left
      unfixed: the 503 body's `code` value `TRACKER_DISABLED`
      (`server/internal/api/admin.go:297,309`) is absent from the `Error.code` enum
      in `contracts/openapi/openapi.yaml`. It was introduced by `1ad1777` with the
      Phase 3.x stats endpoint, affects both admin tracker endpoints equally, and is
      outside D-012's scope.
- [x] **P4-M3′ — Server-side stage latency reported.** Per event:
      `onset_ts → decided_at` and `decided_at → emit`, as p50/p95 over the
      window. **Server stages only** — no client-side wake, heads-up, or siren
      timing enters this number.
      `IMPLEMENTED` `ca8262b` (deployed 2026-09-01); **owner-approved SATISFIED
      2026-09-01** on runtime evidence from real physical events on
      `NODE-52960B47`, firmware 7.0.0 — SENSOR onset→decided n=2,
      PUBLISH_BOUND onset→decided n=4, decided→emit n=12. Terminal-transition
      exclusion and onset-provenance separation both confirmed against live
      counters. **No population-level latency performance is claimed**: at n=2
      and n=4 the reported p95 is the maximum of a handful of observations.
      **CONFIRMED-path latency remains unvalidated** —
      `event_transitions_to_confirmed_total = 0` because the physical fleet has
      one node (S2), so every sample came from an `UNCONFIRMED` decision. See
      `docs/CURRENT_STATE.md` for the demonstrated/not-demonstrated split.
- [x] **P4-M4′ — Deterministic replay.** Replaying recorded observations grouped
      by `algo_ver` through a fresh tracker reproduces the same `event_id`,
      `revision`, and `independent_cell_count` decisions (V7).
      `IMPLEMENTED` 2026-09-03, authorized by **D-013**, which defines how each
      quantity is compared: **F2** — `event_id` is compared as an
      observation-grouping **bijection** (the sorted set of `node_id#obs_seq`
      behind each event's last revision), not as UUID equality, because the
      identifier is minted fresh at DETECTED and equality is unsatisfiable by
      construction; **F3** — `decided_at` is compared as an elapsed **delta** from
      the event's first decision, within a tolerance defaulting to
      `EVENT_SWEEP_INTERVAL_MS + 1000` ms, because the sweep tick phase was never
      recorded. `revision`, `from_state`, `to_state`, `reason`, `node_count` and
      `independent_cells` are compared **exactly**. Canonical input order is
      `received_ts, observation_id` — total, because `observation_id` is
      `BIGSERIAL`. Read-only by construction: two `SELECT`s
      (`ListObservationsForReplay`, `ListStateLogForReplay`), a fresh `Tracker`
      with no persister and no ledger, never reconciled.
      **Owner-approved SATISFIED / `VALIDATED` 2026-09-03**, on archived evidence
      from an isolated PostgreSQL/PostGIS database (`TEST_DATABASE_URL`,
      loopback-only container, all nine migrations applied): 3 new PG-gated tests
      in `internal/store/replay_read_test.go` exercised the two readers against a
      real schema for the first time (canonical order including the
      `observation_id` tie-break, interval closed at both ends, and the
      **non-filtering** property — a failed-verification row and a NULL
      `node_location` row both return, NULL arriving as NULL rather than `(0,0)`);
      34 M4′ tests green in total (27 `internal/event/replay_test.go`, 4
      `replay_realsensor_test.go`, 3 `replay_read_test.go`); the full suite 272
      passed / 0 skipped / 0 failed run serially, and `go test -race` clean across
      all 10 packages. The recorded real-sensor window was then seeded and
      replayed: event `3adf752d-48f1-4f81-b98e-d31e3775c923` on `NODE-52960B47`,
      observations 28 (PRELIM) and 29 (FINAL), exit code 0, **bijective** under
      the signature `NODE-52960B47#1507330`, both revisions reproduced
      (`DETECTED→UNCONFIRMED FLOOR_MET`, then `UNCONFIRMED→RESOLVED
      NO_NEW_EVIDENCE`) with `independent_cells` matching, F3 deltas 0 ms and
      2194 ms against a 6000 ms tolerance, and re-feeding the same window produced
      no second event. Read-only was **proven, not argued**, three independent
      ways: per-row `xmin`, row counts and sequence `last_value` unchanged;
      `pg_stat_user_tables` insert/update/delete/hot-update counters unchanged;
      and the same run under an enforced `default_transaction_read_only = on`
      session producing byte-identical output at exit 0. A deliberately divergent
      fixture reported `independent_cells: historis=2 replay=1` at exit 1, so a
      pass is distinguishable from a comparison that cannot fail; operator exit
      codes 0 / 1 / 2 / 1 were confirmed on a built binary, the rejected-profile
      path (`INDEPENDENCE_CELL_KM=9` against rows carrying `ic=5`) included.
      **Still not demonstrated:** production or field validation of any kind (S9)
      — nothing is deployed and the production stack was untouched; more than one
      event on more than one node, so the `CONFIRMED` path stays unexercised by
      replay for as long as the fleet has one node (S2); the real-sensor fixture's
      `evidence_summary`, which is **reconstructed** because that session captured
      only the scalars — evidence-field agreement there is tautological and only
      the recorded scalars are independent evidence; replay parameters other than
      `INDEPENDENCE_CELL_KM`, which are **operator-asserted** and not recoverable
      from the rows; absolute `decided_at` agreement, F3 being relative by design;
      `event_persist_dropped_total` / ledger drops, which are logged and not
      stored, so a divergence caused by a historical drop cannot be distinguished
      from one caused by a defect; and a durable operator-level regression test for
      the divergence path — the divergence fixture was manual and was deleted.
      One **unrelated pre-existing** test-isolation defect was found and
      deliberately left unfixed: `internal/store` and `internal/event` share one
      database and run concurrently by default, so the migration-down tests
      (`TestMigration000006DownRestoresSchema`, `TestMigration000009DownRestoresSchema`)
      and `pgBreakNearConfirmedTable` race each other. Reproduced 3/3 on a clean
      `git archive HEAD` tree containing no replay code; `-p 1` is green. It
      predates Phase 4 and is outside D-013's scope.
- [ ] **P4-M5′ — Simulated multi-node runs in CI.** `sim_multi_node.sh` and
      `sim_dual_event.sh` pass in CI and archive their tracker counters and
      evidence snapshots. **This is software validation, never field
      validation** (S9) — it may not be cited as multi-node correlation.
      **IMPLEMENTED 2026-09-03, authorized by D-014; awaiting owner sign-off — not
      `VALIDATED`.** `.github/workflows/ci.yml` gains a fourth job, `simulation`,
      which runs the two harnesses **serially** on one runner (both publish the
      same fixed host ports 5432/6379/1883/8080, both assert on absolute deltas
      in shared tracker counters, so concurrency would make `delta == 2` a coin
      toss) and uploads `simulation-evidence` —
      `.sim-evidence/sim_multi_node.evidence.json` and
      `.sim-evidence/sim_dual_event.evidence.json`, `schema_version 1` — under
      `if: always()` with `if-no-files-found: error`. Each artifact is emitted by
      the harness itself from an EXIT trap that fires **before** teardown and
      re-raises the original exit code, never reconstructed from CI logs, and
      carries `run_id`, `git_sha`, `git_dirty`, `checkpoint`, `status`
      (`PASS`/`FAIL`/**`ERROR`** — a broken runner is not a broken detector),
      `exit_code`, `tracker_counters_before` / `_after`, and the observed scalars,
      the D-012 coverage envelope, and the assertion list its verdict rested on.
      The boundary travels **inside** the artifact:
      `evidence_class: "SOFTWARE_SIMULATION"` plus an explicit `not_claimed` list.
      `sim_dual_event.sh` STEP 9 (Android `WarningNotifierStandDownTest`) is
      retained and its outcome recorded. Two harness config names were corrected
      at unchanged values: `RESOLVE_AFTER_MS` → the real `EVENT_RESOLVE_AFTER_MS`
      (the built-in default 90000 had been silently in force), and the
      `MIN_PGA_GAL` / `MIN_NODES_CONFIRMED` exports were **deleted** — `config.go`
      reads neither, they are compile-time constants (D-007), and a variable that
      looks like a knob and moves nothing is a lie about the gate (§8). Local
      evidence: 3.1 exit 0 with 9 PASS / 0 FAIL, 3.2 exit 0 with 11 PASS / 0 FAIL,
      both artifacts valid JSON whose assertion text is byte-identical to stdout;
      a `BASE_URL` pointed at an unbound port failed the job non-zero with both
      artifacts still `status=ERROR` and still uploaded, so a pass is
      distinguishable from a gate that cannot fail. **Not yet run on GitHub
      Actions** at the time of this commit — validated by a local reproduction
      executing the workflow's own step bodies; the first real run is the one this
      commit triggers, and its result is not yet evidence of anything.
      **Stated for the record:** M5′
      demonstrates that the multi-node and dual-event simulation harnesses execute
      successfully in CI and produce archived software evidence. It does not
      validate field correlation, production behavior, or real multi-node sensor
      performance. **Still not demonstrated:** the harnesses drive **virtual nodes
      that are database rows** with hand-picked coordinates (S9, D-011 constraint
      2); the fleet remains one physical ESP32, and Phase F owns the field
      evidence. Not deployed.
- [ ] **P4-M6′ — Forensic timeline for one event.** A read-only path returns, for
      one `event_id`, the event row, its ordered `event_state_log` history, the
      `evidence_summary` per revision, and the contributing observations.

#### Explicitly **out of scope** for Phase 4
- Any external catalogue as ground truth (`PROJECT_RULES.md` §2).
- Automatic false-positive classification; any claimed false-positive or
  false-negative **rate**.
- Automatic threshold calibration; any change to detection thresholds,
  confirmation semantics, quorum, alert radius, delivery behaviour, notification
  policy, or a published contract. Those require accepted decisions, not a phase.
- Manufacturing a CONFIRMED event in production.
- Any lead-time claim (S2), and any real-world multi-node validation claim.
- A hard reliability target such as "zero dropped persists", which the bounded
  drop-oldest writer can legitimately violate without suppressing a warning (S1).

An earlier plan defined a BMKG-comparison Phase 4. That definition is
`ABANDONED` — see § Superseded and abandoned phase definitions.

### Phase F — Field validation — `BLOCKED`
**Blocker:** the network is one node in one home. Independent confirmation
requires at least three verified nodes with sufficient mutual separation.
**Cleared by:** the owner deploying additional nodes. Not clearable by code.

Exit criteria (none met):
- [ ] Multi-node correlation observed in the field.
- [ ] CONFIRMED reached at least once in production.
- [ ] Push delivery verified across more than one device vendor.
- [ ] A measured false-positive rate exists over a population of events.

---

## Future research — NOT phases

These are open questions, not scheduled work. They are tracked in
`docs/DECISIONS.md` § Unresolved and **must not be resolved by implementation**.

- Background delivery for unconfirmed events (**U-001**).
- Non-geographic independence axes (**U-002**).
- Evidence required to change the contributor quorum (**U-003**).
- Alert radius (**U-004**).
- Chat feature status (**U-005**).
- MQTT alert-channel contract status (**U-006**).
- Reconciliation versus stored `algo_ver` (**U-007**).
- Removal of the Phase 2 consensus path (**U-008**).
- Legacy decision-ID migration (**U-009**).
- How a client knows an alert has stopped being actionable (**U-010**, formerly
  F-7). Blocks checkpoint 1.4 of `docs/TEMP_ANDROID_PHASE4_CHECKLIST.md`.
- Simultaneous confirmed events: coexist or replace (**U-011**, formerly F-5).
  Testable on the local stack today; blocked on the decision, not the fleet.
- Delivery to a phone that is unlocked and in use (**U-012**). Found on device
  2026-08-31: Android suppresses the full-screen intent when no keyguard is
  showing and no heads-up replaces it, so an in-use device shows nothing.
- Whether the alert-raising path logs at all (**U-013**). Success and a silently
  gated-out alert are currently indistinguishable in logcat.

Notes on U-012: root cause is a **device setting**, not an app defect —
`heads_up_notifications_enabled=0` on the test phone makes AOSP's
`PeekDisabledSuppressor` suppress heads-up for every app, and `couldHeadsUp=false`
then forces `NO_FSI_NO_HUN_OR_KEYGUARD`. **Retracted:** the earlier claim that the
silent notification channel was the confirmed cause — asserted from a filter's name
without reading its body. Not yet tested: whether QuakeAlert heads-ups with the
setting enabled. That one experiment decides whether any app change is warranted.

---

## Superseded and abandoned phase definitions

- **Phase 4 as "BMKG catalogue comparison / automatic false-alarm-rate
  calibration"** — `ABANDONED`. No successor carries this content. It conflicts
  with `PROJECT_RULES.md` §2. It still appears in
  `.hermes/plans/2026-08-27_quakealert-final-architecture-plan.md`, which is a
  historical artifact and not authoritative.
- **Phase 5 calibration** — folded into the abandoned definition above. Not a
  current phase.
