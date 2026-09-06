# CURRENT_STATE.md

**Baseline commit:** `1ad1777` (`feat(event): Phase 3.x global-spatial hardening + observability`)

This is the authoritative record of what actually exists. It supersedes
`docs/GAP_ANALYSIS.md`, which is a historical snapshot of an older commit.

## Status legend

| Status | Meaning |
| --- | --- |
| **IMPLEMENTED** | Code exists; unit and integration tests pass. |
| **VALIDATED** | Behaviour demonstrated by evidence outside unit tests. |
| **RELEASED** | Deployed to the target environment and verified there. |

These are **not** the same thing (`PROJECT_RULES.md` §8). Unit tests grant
IMPLEMENTED only.

---

## Component status

| Component | Implemented | Validated | Released | Notes |
| --- | --- | --- | --- | --- |
| Ingest (MQTTS, HMAC, observation ledger) | yes | partial — single node | yes, private VPS | One node observed end-to-end. |
| Phase 3 event tracker | yes | no | yes, private VPS — **enabled** | No confirmed event has occurred in production. |
| Phase 2 consensus engine | yes | yes (Phase 2) | retained as rollback path | Removal is **U-008**. |
| Global spatial correctness (Phase 3.x) | yes | no | yes, private VPS | Proven by test at every latitude; no field data outside one location. |
| Near-confirmation observability log | yes | **yes — restart survival demonstrated 2026-09-03 against a real PostgreSQL database** | in-memory version: yes, private VPS · durable version (P4-M2′): **committed, not deployed** | Durable since P4-M2′/D-012: crossings are written to `event_near_confirmed` (migration `000009`) through the bounded drop-oldest ledger queue, and the read path answers with an explicit coverage envelope. Silent crossings (no state transition) are recorded too. Restart survival, terminal state outliving its parent row, persistence failure not blocking emission, and recorded `algo_ver`/`min_independent_cells` surviving a **different** running configuration are all demonstrated — see *Demonstrated*. The in-memory map remains the authority (§9.5); the table is a follower. |
| Admin tracker endpoints | yes | no | yes, private VPS | Behind the admin API key. |
| Deterministic replay (P4-M4′ forensics) | yes | **yes — recorded window reproduced 2026-09-03 against a real PostgreSQL database** | **not deployed** — operator tooling, `//go:build ignore` | Read-only by construction (D-013): two `SELECT`s feed a fresh `Tracker` with no persister and no ledger, never reconciled; no migration, no contract change. Identity is compared as an observation-grouping **bijection** (F2) and `decided_at` as an elapsed **delta** within tolerance (F3); revision, states, reason, `node_count` and `independent_cells` are compared exactly. One event on one node, and parameters other than `INDEPENDENCE_CELL_KM` are operator-asserted — see *Demonstrated* and *NOT demonstrated*. |
| Simulation harnesses in CI (P4-M5′) | yes | **yes — executed in GitHub Actions CI #22 with archived evidence, 2026-09-03** | n/a — CI only, nothing deployed | **Software evidence only.** A fourth CI job runs `sim_multi_node.sh` then `sim_dual_event.sh` serially and each harness emits its own `schema_version 1` artifact from an EXIT trap; both upload and are re-validated on the runner. The nodes are database rows with hand-picked coordinates (S9, D-011 constraint 2), so this is **not** field validation, production validation, real multi-node sensor performance, or real multi-node correlation — each artifact names those four in `not_claimed`. |
| WebSocket delivery | yes | partial | yes, private VPS | Advisory frames observed; alert frames not observed in production. |
| Push delivery | yes | no | yes, private VPS | Never triggered in production; one device vendor only in testing. |
| Firmware detection | yes | partial | one node | One board, one location, one firmware build. |
| Android client | yes | partial — drill-validated on one device | sideloaded, not published | Advisory-never-wakes enforced in three independent places. Locked-screen alarm, Doze wake, cross-channel dedup and all-clear teardown demonstrated on hardware 2026-08-31 (drill path). Delivery to an **unlocked, in-use** device shows nothing — **U-012**. |

**Production status, stated precisely:** Phase 3 is activated on a **private
VPS**. That is a single-operator deployment serving a single-node network. It is
**not** a public production service, and nothing here should be read as public
production readiness.

---

## Active confirmation gate

An event is CONFIRMED only when **all three** hold:

1. Peak PGA ≥ **16.6 gal** (compile-time constant, ≈ MMI IV).
2. **≥ 3** unique verified contributing nodes (compile-time constant).
3. **≥ 2** mutually independent contributors, independence being **geodesic
   distance ≥ 5 km** between registered coordinates (both configurable).

The independence count is a deterministic **lower bound** on the true maximum
independent set (D-004). It may undercount; it never overcounts.

`algo_ver` written by the current binary: `phase3-1.1/ic=<independence_km>`.

**No threshold change is currently approved.** Changing any of the three
requires an accepted decision plus an `algo_ver` bump (D-006, D-007, S4).

---

## Active configuration

Defaults as of `1ad1777`. Values are current decisions, not permanent
principles.

| Setting | Default | Configurable | Notes |
| --- | --- | --- | --- |
| `EVENT_TRACKER_ENABLED` | `false` | yes | **Enabled on the private VPS.** `false` selects the Phase 2 rollback path. |
| `CORRELATION_WINDOW_MS` | `20000` | yes | Matching tolerance, not a hold-open timer. |
| `ATTACH_RADIUS_KM` | `50` | yes | Bounded by a formula-validity ceiling. |
| `INDEPENDENCE_CELL_KM` | `5` | yes | Appears in `algo_ver`. |
| `MIN_INDEPENDENT_CELLS` | `2` | yes | Appears in the gate. |
| `MAX_EVENT_DIAMETER_KM` | `120` | yes | |
| `EVENT_RESOLVE_AFTER_MS` | `90000` | yes | |
| `EVENT_SWEEP_INTERVAL_MS` | `5000` | yes | |
| `TERMINAL_RETENTION_MS` | `900000` | yes | Tombstone retention. |
| `EVENT_TRACKER_MAX_OPEN` | `256` | yes | |
| `EVENT_TRACKER_MAX_TOMBSTONES` | `512` | yes | |
| `SINGLE_NODE_GEO_TOPIC_GUARD` | `true` | yes | |
| Alert radius | `200 km` | no | Fixed constant. See **U-004**. |
| PGA floor | `16.6 gal` | no | Compile-time (D-007). |
| Contributor quorum | `3` | no | Compile-time (D-007). |

---

## Delivery behaviour

| Event state | Frame type | WebSocket | Push | Wakes device |
| --- | --- | --- | --- | --- |
| UNCONFIRMED | advisory | yes | **no** | **no** |
| CONFIRMED | alert | yes | yes, high priority | yes |
| RESOLVED / CANCELLED | resolution | yes | only if the event was ever CONFIRMED | no |

Push recipients: tokens within the alert radius of the centroid. A severe event
(high intensity or high PGA) broadcasts to a geographic topic without a distance
filter.

The client enforces advisory-never-wakes independently of the server, in three
places. This is deliberate redundancy (D-009).

Trigger transport is **QoS 0** with firmware retry and server-side
de-duplication — **not** at-least-once (D-008).

---

## Demonstrated

- Global spatial coverage invariant, at every latitude, both axes, across the
  antimeridian — by test, including a sensitivity test proving the superseded
  fixed neighbourhood would fail at high latitude.
- Illegal state transitions rejected; retraction of a confirmed event when all
  its evidence is revoked — by integration test through the real HTTP path.
- Persistence failure does not block emission — by test.
- Determinism of the independence count under map-iteration order — by test.
- One firmware node ingesting through MQTTS with HMAC into the observation
  ledger, on a private VPS.
- **Server-side stage latency, reported from real events** (`ca8262b`, deployed
  2026-09-01; owner-approved SATISFIED for `P4-M3′` on 2026-09-01).
  `GET /api/v1/admin/tracker/stats` reports two server stages as p50/p95 over a
  256-sample ring, each alongside a cumulative `observed` count, with
  `onset_ts → decided_at` split by onset provenance:
  `event_latency_onset_to_decided_sensor_ms` **n=2** (measured onset, firmware
  7.0.0 on `NODE-52960B47`), `event_latency_onset_to_decided_publish_bound_ms`
  **n=4** (onset inferred as `publish_ts - dur_ms`, an upper bound, from the
  earlier v1 binary on the same node), and `event_latency_decided_to_emit_ms`
  **n=12**. The two onset series are never merged: a bound and a measurement are
  not comparable quantities. **Server stages only** — every timestamp is either
  the signed sensor onset or a server clock reading, so no client wake, heads-up,
  or siren timing can enter the number. Two properties were confirmed against
  live counters rather than argued: terminal transitions stay out of the onset
  series (the RESOLVED sweep on `3adf752d-48f1-4f81-b98e-d31e3775c923` would have
  contributed a 98138 ms sample; SENSOR `observed` did not move), and the sample
  accounting closes exactly — 6 onset samples equal
  `event_transitions_to_unconfirmed_total = 6`, and 12 decided→emit samples equal
  the 6 `UNCONFIRMED` plus 6 `RESOLVED` transitions. See *NOT demonstrated* for
  what these numbers do **not** support.
- **On a physical device, drill path only** (POCO F1, Android 16 / API 36, PixelOS
  `BP3A.250905.014`, 2026-08-31): a full-screen alarm launching over the lock
  screen from Doze without user action (`Displayed WarningActivity +467ms`); the
  same after the app was swiped away, revived by FCM (+960ms cold start); one
  alarm per earthquake across two independent transports (`AlertDedup` suppressed
  the FCM copy of a WebSocket frame); and the all-clear removing notification
  4301. These used `POST /api/v1/admin/test-alert`, which writes no
  `earthquake_events` row and carries no `event_state` — so they say nothing about
  History, about CANCELLED wording, or about a release build.
- **Near-confirmation durability across a real process restart — P4-M2′, isolated
  PostgreSQL, 2026-09-03.** Owner-approved SATISFIED. Migration `000009` applied to
  a throwaway PostGIS container reached only over loopback; 14 integration tests
  that had never executed before now pass (11 in
  `server/internal/store/near_confirmed_test.go`, 3 in
  `server/internal/event/nearconfirmed_pg_test.go`), with 19 pre-existing Postgres
  tests re-run green on the same schema. Restart was then reproduced through the
  HTTP surface across four service runs of a locally built binary: two signed MQTT
  triggers 12 km apart produced a real **silent** crossing — `source=RECORDED`, a
  durable row, and **no** `event_state_log` row for the crossing itself, only
  `DETECTED→UNCONFIRMED` (cells=1) and `UNCONFIRMED→RESOLVED` (cells=2) — after
  which the process was terminated and the same entries came back `source=LOADED`.
  A run configured `ic=5`/threshold 3, different from every stored row, returned
  per-entry `algo_ver` `[ic=5, ic=5, ic=9]` and per-entry `min_independent_cells` 2
  against `coverage.algo_ver = ic=5` and `coverage.min_independent_cells = 3`, and
  the durable rows were byte-identical after boot: the running configuration does
  not rewrite recorded history (V3/V6, D-006). On an empty database the endpoint
  answered `entries: []` with `durable_read_attempted: true`,
  `durable_read_ok: true`, `durable_rows_loaded: 0` — the honest empty answer this
  criterion asks for on a one-node fleet. All five captured 200 bodies validated
  against `contracts/openapi/openapi.yaml` with zero undocumented fields; both 401
  bodies validated against `Error`; `EVENT_TRACKER_ENABLED=false` still returns 503.
  **This says nothing about production:** the code is committed and not deployed,
  and the runs used a locally built binary against a test database, never the
  production stack.
- **Deterministic replay of a recorded window reproduces its decisions — P4-M4′,
  isolated PostgreSQL, 2026-09-03.** Owner-approved SATISFIED / `VALIDATED`,
  authorized by **D-013**. Two read-only `SELECT`s
  (`ListObservationsForReplay`, `ListStateLogForReplay`) feed recorded
  observations, in canonical `received_ts, observation_id` order, through a
  **fresh** `Tracker` that holds no persister, no ledger, and is never
  reconciled; the decisions it makes are compared against the recorded
  `event_state_log`. 3 new PG-gated tests in
  `server/internal/store/replay_read_test.go` exercised those two queries against
  a real schema for the first time — canonical order including the
  `observation_id` tie-break on an identical `received_ts`, interval closed at
  both ends, and the **non-filtering** property (a failed-verification row and a
  NULL `node_location` row both come back, NULL arriving as NULL rather than as
  the perfectly valid coordinate `(0,0)`). 34 M4′ tests are green in total; the
  full suite ran 272 passed / 0 skipped / 0 failed serially, and `go test -race`
  was clean across all 10 packages. The real recorded window was then seeded and
  replayed: event `3adf752d-48f1-4f81-b98e-d31e3775c923` on `NODE-52960B47`,
  observations 28 (PRELIM) and 29 (FINAL), exit code 0, **bijective** under the
  grouping signature `NODE-52960B47#1507330`, both revisions reproduced
  (`DETECTED→UNCONFIRMED FLOOR_MET`, then `UNCONFIRMED→RESOLVED
  NO_NEW_EVIDENCE`) with `independent_cells` matching, and re-feeding the same
  window produced no second event. **Read-only was proven three ways rather than
  argued:** per-row `xmin`, row counts and sequence `last_value` all unchanged;
  `pg_stat_user_tables` insert/update/delete/hot-update counters all unchanged;
  and the same run under an enforced `default_transaction_read_only = on` session
  producing byte-identical output at exit 0. A deliberately divergent fixture
  reported `independent_cells: historis=2 replay=1` at exit 1, so a pass is
  distinguishable from a comparison that cannot fail; operator exit codes
  0 / 1 / 2 / 1 were confirmed on a built binary, the rejected-profile path
  included. What this does **not** claim, stated as limits and not as caveats:
  **no production or field validation** (S9) — the code is committed, nothing is
  deployed, the production stack was untouched, and every run used a locally
  built binary against a test database; **replay parameters are
  operator-asserted** — `algo_ver` records `INDEPENDENCE_CELL_KM` and nothing
  else, so correlation window, attach radius, resolve-after, sweep interval, max
  diameter and `MIN_INDEPENDENT_CELLS` come from the operator, are printed as an
  assertion before any result, and a match proves *these observations under
  **these** parameters produced those decisions*, never that those parameters were
  in force; **`decided_at` agreement is relative, not absolute** — compared as an
  elapsed delta from the event's first decision within a
  `EVENT_SWEEP_INTERVAL_MS + 1000` ms tolerance (0 ms and 2194 ms against 6000 ms
  here), because the sweep tick phase was never recorded; the real-sensor
  fixture's **`evidence_summary` is reconstructed** — that session captured only
  the scalars, so evidence-field agreement there is tautological and only the
  recorded scalars are independent evidence; and **`ledger_drops_total` is not
  historically recoverable** — drops are logged, not stored, so replay cannot
  distinguish an observation the tracker never saw from one that was never
  recorded, and a divergence caused by a historical drop is indistinguishable
  from one caused by a defect. One event on one node, so the `CONFIRMED` path
  stays unexercised by replay (S2).
- **Every qualifying trigger in a bounded production window traces to a
  transition and an advisory — P4-M1′, real production database, 2026-09-04.**
  Owner-approved SATISFIED / `VALIDATED`, on the archived evidence at
  `docs/evidence/p4-m1/2026-09-03-production-trace/` (verbatim stdout,
  before/after database metadata, tracker-counter body, provenance,
  `SHA256SUMS.txt`). That archive is a **durable reconstruction** committed
  2026-09-04 after the original temporary bundle `/tmp/m1-prod-20260903T125908Z`
  was destroyed by a tmpfs reboot; it was rebuilt from the session transcript
  with every recorded value preserved exactly, is **not** a new validation run,
  and changes no number below (**D-016**). The measurement
  tool is `server/scripts/trace_triggers.go` over three read-only `SELECT`s
  (`ListLastNObservations`, `ListStateLogForReplay`, `ListEmissionsForTrace`); it
  **measures and never enforces** — no exit code means “P4-M1′ passed”, and the
  persistence counters do not affect the exit code. Because production is
  `aarch64` and carries no Go toolchain, the tool was cross-compiled
  (`GOOS=linux GOARCH=arm64 CGO_ENABLED=0 -trimpath`) and its SHA-256 verified
  identical on both hosts before it ran. Run 2026-09-03T20:24:00Z with
  `LAST_N=200` requested and **51 rows read** — the whole ledger — spanning
  `2026-08-29T09:07:42Z .. 2026-09-03T15:36:05Z`, against `event_state_log` 50
  raw rows and `alert_emissions` 50 raw rows. The three denominators sum to 51:
  **25 below the `MinPGAGal` floor** (not triggers, not failures), **0
  excluded**, **26 qualifying**. Of those 26: **TRACED 26**, AMBIGUOUS 0,
  NO_UNCONFIRMED_TRANSITION 0, over **25** distinct `UNCONFIRMED` transitions —
  N:1, because observations inside one correlation window share one transition
  (`UNCONFIRMED→UNCONFIRMED` is not a legal transition), so shared rows are
  traced rather than counted twice, and the one negative lag in the report
  (`obs=41`, `-4018 ms`) is that documented sibling and not an anomaly. The
  advisory leg matched **`MATCHED_BY_EVENT_ID_AND_REVISION` 26** — the exact
  proof, `event_id` + `event_revision` — with `TIME_ONLY` 0, `MISSING` 0,
  `NOT_APPLICABLE` 0; `ws_clients` was 1 on 11 frames and 0 on 15, reported and
  never required non-zero, and no unattributed `UNCONFIRMED` transition remained.
  All 26 chains carry `algo_ver=phase3-1.1/ic=5`. The three `algo_ver` axes
  diverge in production and that is not a defect: `alert_emissions` carries
  `phase1-1.0` (the ledger schema version stamped at
  `server/internal/ledger/writer.go:42`) and `earthquake_events` carries 25
  `phase3-1.1/ic=5` plus 6 pre-Phase-3 `NULL`s, while the emission link joins
  `event_id` + `event_revision` and never `algo_ver`. **Reported alongside, never
  as a zero-valued acceptance requirement:** `event_persist_dropped_total` 0,
  `event_upsert_failures_total` 0, `event_state_log_failures_total` 0,
  `event_state_log_skipped_total` 0 — cumulative since process start and
  untimestamped, so **not attributable to this window**; byte-identical when
  re-read after the run, which is itself evidence the trace drove no tracker
  activity. **Read-only was server-enforced rather than asserted:** the session
  carried `default_transaction_read_only = on`, confirmed by
  `show transaction_read_only` immediately before the run, so PostgreSQL would
  have rejected any write; `pg_stat_user_tables` for `alert_emissions`,
  `event_state_log` and `earthquake_events` was byte-identical before and after.
  Live ingest did add two `sensor_observations` rows (`pga` 4.9489, below the
  floor) and node-heartbeat `iot_nodes` updates during the run; both are recorded
  as **production ingest, not trace writes**, because a frozen database was never
  claimed. No control write was attempted against production, no database role
  was created (one pre-existing login role before and after), and the temporary
  binary and scripts were removed from the host afterwards. What this does
  **not** claim: the observation→transition link is **membership-and-time, not
  causal** — `correlation_key` is computed and never stored (D-012),
  `event_state_log` has no `observation_id`, `sensor_observations` has no
  `event_id`, and there is no FK either way, so the only path back is
  `evidence_summary.contributors[].node_id` over a time window; `ledger_drops` is
  **UNKNOWN** — `ledger_drops_total` reaches only the log (D-017/D-030), so this
  window **may** be missing observations with no trace in any table, and a zero
  there would be a number that lies; **one node only** (`NODE-52960B47`), so this
  is production validation and **not** multi-node field validation, with the
  `CONFIRMED` path unreachable by density (S9, S2); production runs
  `schema_version = 8`, so migration `000009` is **not deployed**; and M1′ has no
  evidence emitter of its own, so the provenance in the bundle was captured by
  hand rather than written by the tool.
- **The two simulation harnesses execute in CI and archive their own evidence —
  P4-M5′, software simulation, 2026-09-03.** IMPLEMENTED under **D-014**;
  **owner-approved SATISFIED / `VALIDATED` 2026-09-03**, on the archived evidence
  from GitHub Actions **CI #22** (`PROJECT_RULES.md` §8, S9). A
  fourth CI job, `simulation`, runs `sim_multi_node.sh` then `sim_dual_event.sh`
  **serially** on one runner — mandatory, because both publish the same fixed host
  ports (5432/6379/1883/8080), name their containers by a fixed compose prefix,
  and assert on absolute deltas in shared tracker counters, so concurrency would
  turn `delta == 2` into a coin toss. Each harness emits
  `.sim-evidence/<script>.evidence.json` (`schema_version 1`) **itself**, from an
  EXIT trap that fires before teardown and re-raises the original exit code —
  never reconstructed from CI logs — carrying `run_id`, `git_sha`, `git_dirty`,
  `checkpoint`, `status`, `exit_code`, `tracker_counters_before` / `_after`, the
  observed scalars, the D-012 coverage envelope, and the assertion list its
  verdict rested on. `status` is three-valued: `PASS` / `FAIL` / **`ERROR`**, the
  last meaning the run never reached a verdict, because a broken runner is not a
  broken detector. Both files upload as `simulation-evidence` under
  `if: always()` with `if-no-files-found: error`, then a following step
  re-validates each for valid JSON, its required fields, and
  `evidence_class == "SOFTWARE_SIMULATION"`. Local evidence: checkpoint 3.1 exit 0
  with 9 PASS / 0 FAIL, checkpoint 3.2 exit 0 with 11 PASS / 0 FAIL (STEP 9, the
  Android `WarningNotifierStandDownTest`, retained and now recorded as an
  assertion), both artifacts valid JSON whose assertion text is **byte-identical**
  to stdout, counters reconciled against the observed scalars. A `BASE_URL`
  pointed at an unbound port failed the job non-zero with both artifacts still
  written `status=ERROR` and still uploaded, so a pass is distinguishable from a
  gate that cannot fail; no pass/fail logic was weakened to produce it. Two
  harness configuration names were corrected at **unchanged values**:
  `RESOLVE_AFTER_MS` → the real `EVENT_RESOLVE_AFTER_MS` (the built-in default
  90000 had been silently in force), and the `MIN_PGA_GAL` /
  `MIN_NODES_CONFIRMED` exports were **deleted** — `config.go` reads neither, they
  are compile-time constants (D-007), and a variable that looks like a knob and
  moves nothing is a lie about the gate (§8). `go build ./...`, `go vet ./...` and
  `go test -race -count=1 ./...` clean across all 10 packages.
  **Not claimed, and this is the whole point of the entry:** M5′ demonstrates that
  the multi-node and dual-event simulation harnesses execute successfully in CI
  and produce archived software evidence. It does not validate field correlation,
  production behavior, or real multi-node sensor performance. The harnesses drive
  **virtual nodes that are database rows** with hand-picked coordinates (S9, D-011
  constraint 2); the fleet is still one physical ESP32, so no real multi-node
  correlation occurred and none may be inferred from a green job.
  **Validation evidence — GitHub Actions CI #22**, run id `33737869128`, attempt 1,
  `workflow_dispatch` on `development` at head
  `84d46d52856cc97b69f45164041a2070ea5aada1`, 2026-09-03T09:15:51Z → 09:22:14Z:
  conclusion **success** with all four jobs green (Go server, Firmware host tests,
  Android app, and *Simulation harnesses (software evidence)* — all 13 steps of the
  simulation job succeeded). The `simulation-evidence` artifact (3278 B, SHA-256
  `62605c4f9e4737c982769c80f55b9f0ab364f119bfe0f6e46c822f7f00fd0bac`) was
  downloaded and re-read from the archive, not transcribed from the log: it holds
  exactly the two files, `sim_multi_node.evidence.json` (checkpoint 3.1,
  `status=PASS`, `exit_code=0`, 9 assertions passed / 0 failed) and
  `sim_dual_event.evidence.json` (checkpoint 3.2, `status=PASS`, `exit_code=0`, 11
  passed / 0 failed), both `schema_version 1`, `evidence_class:
  "SOFTWARE_SIMULATION"`, `git_dirty=false`, and `git_sha` equal to the run head —
  so the archived evidence provably describes the commit that was tested. The
  ephemeral `ADMIN_API_KEY` shows as `***` in every `env:` block of the run log and
  appears in neither artifact. The **first** real run belongs in this record too:
  CI #21 on `1e09a6b` passed every simulation assertion and archived **nothing**,
  because `actions/glob` skips dotted path components unless `include-hidden-files`
  is set, and it printed the generated key unmasked five times. Both were
  delivery-path defects around the harnesses, not detector defects, and were fixed
  in `acd25d4` — `include-hidden-files: true`, `::add-mask::` registered before the
  `$GITHUB_ENV` write, and `sim_evidence_selftest.sh` (24 assertions, each
  mutation-tested red) pinning the delivery contract that no assertion inside the
  harnesses was watching. No simulation semantics, threshold, coordinate, or
  confirmation rule changed. Not deployed.
- **Forensic timeline for one event — P4-M6′, one-node production leg plus
  separate synthetic leg, 2026-09-06.** Owner-approved SATISFIED /
  `VALIDATED` per **D-015**. Production leg: single v2 CLI invocation for
  locked event `4fcc3374-032a-440b-9d6f-609d8a4096ce` against the live
  production database (schema `8|f`), exit 0, stderr 0 bytes, stdout 9141
  bytes / 143 lines archived verbatim at
  `docs/evidence/p4-m6/2026-09-06-production-event-4fcc3374/` with
  `SHA256SUMS.txt` 18/18. In-CLI banner from the same `pgxpool` as the
  forensic reads: `default_transaction_read_only=on/client`,
  `transaction_read_only=on/override` (effective `override` is correct on
  PostgreSQL by design — owner-approved correction), `application_name`
  correlation only, pool-wide limitation stated. `pg_stat_user_tables`
  before→after: all write counters unchanged on all 47 tables
  (`alert_emissions` 52, `earthquake_events` 32/32, `event_state_log` 52,
  `sensor_observations` 53), only read counters advanced. All four required
  outputs OBSERVED: event row (`RESOLVED` rev2, SENSOR origin, 77.8888 gal, 1
  node / 1 cell, `phase3-1.1/ic=5`), 2-row history (`FLOOR_MET` then
  `NO_NEW_EVIDENCE`), 2 parsed evidence summaries (single `NODE-52960B47`
  contributor), 1 `TRACED` candidate (`obs=45`, lag +3 ms, `EXACT`; 2 rows
  read, 1 below floor, 0 excluded/unattributed/ambiguous). Tolerance 2000 ms
  `M1_DEFAULT` over correlation window 20000 ms; terminal `RESOLVED`
  recorded; `ledger_drops` UNKNOWN (log-only); absence never proof of
  absence. Synthetic leg (`server/internal/event/timeline_fixture_test.go`,
  committed, software evidence only) covers `CONFIRMED`, multi-contributor,
  `independent_cells >= 2`, `mixed_provenance`, terminal and ambiguity
  shapes the fleet cannot produce. Prior v1 bundle stays preserved as
  INCOMPLETE and is never cited as success. What this does **not** claim:
  no `CONFIRMED` production path, no multi-node field correlation, no
  lead-time, population, or reliability claim. Phase F remains `BLOCKED`.

## NOT demonstrated

Read this list as authoritative. **Absence from it is not proof of validation;
only presence in *Demonstrated* is** (`PROJECT_RULES.md` §8).

- **No CONFIRMED event has ever occurred in production.** With one node, the
  gate is unreachable — this is a network-density fact, not a gate defect (S2).
- **No population-level latency performance.** The stage latencies in
  *Demonstrated* rest on **n=2** SENSOR, **n=4** PUBLISH_BOUND, and **n=12**
  decided→emit samples. At those counts `ceil(0.95 × n)` selects the largest
  observation, so the reported `p95_ms` **is** the maximum of a handful of
  events, not a percentile of a population. The numbers demonstrate that the
  measurement works on real data; they support no claim about how fast the system
  is, and must not be quoted as a latency figure. Small n here follows from a
  one-node fleet and a handful of deliberate shakes; D-011 sets no sample-count
  target, so this is a limit on what may be claimed, not a defect.
- **CONFIRMED-path stage latency is unvalidated.**
  `event_transitions_to_confirmed_total = 0`, so every onset→decided sample came
  from an `UNCONFIRMED` decision. That `CONFIRMED` transitions also feed the
  onset series is covered by unit test only — and it stays unvalidated for as
  long as the physical fleet has one node (S2), since the gate is unreachable by
  density.
- **`onset_ts → decided_at` is not server processing time.** It structurally
  contains the shake whenever PRELIM's peak-so-far is below `MinPGAGal`, because
  the decision then waits for the FINAL observation at de-trigger. On the
  2026-09-01 SENSOR event the sample was 6166 ms against `dur_ms` 6138: the
  server's own share was 28 ms, of which 17 ms was network transit. Reporting
  this stage as server latency would misstate it in the system's favour.
- **PUBLISH_BOUND onset→decided is a bound, not a measurement.** Its onset is
  `publish_ts - dur_ms`, whose error is the unbounded publish delay, so those
  four samples are upper bounds and may never be averaged with, compared against,
  or presented as the SENSOR series.
- **`decided_at → emit` is reported below its own resolution.** p50 and p95 both
  read 0 ms because the stage completes in under a millisecond. `observed = 12`
  is what distinguishes "measured, and fast" from "nothing measured"; the
  percentiles cannot distinguish 0 ms from 0.9 ms.
- **Latency instrumentation unexercised at its edges.** Never observed in
  production: the 256-sample ring wrapping, rejection of a negative sample from a
  node clock ahead of the server's, a retry publish (`attempt_no > 1`), or the
  series across a process restart — the samples are in memory only, and today's
  counts begin at the 2026-09-01 deploy. All are unit-tested; none are field
  evidence (S9).
- **Deterministic replay proves reproduction, not the parameters and not the
  field.** The P4-M4′ result in *Demonstrated* rests on **one** event on **one**
  node, replayed against a test database by a locally built binary; nothing is
  deployed, so no production or field claim follows from it (S9). Four limits are
  structural, not incidental: replay parameters other than `INDEPENDENCE_CELL_KM`
  are **operator-asserted**, since `algo_ver` records only that one, so a match
  proves *these observations under **these** parameters produced those decisions*
  and never that those parameters were in force; **`decided_at` agreement is
  relative** (an elapsed delta within a `EVENT_SWEEP_INTERVAL_MS + 1000` ms
  tolerance), because the sweep tick phase was never recorded, so absolute
  timestamp reproduction is neither claimed nor testable; the real-sensor
  fixture's **`evidence_summary` is reconstructed** rather than recorded, making
  evidence-field agreement there tautological — only the recorded scalars are
  independent evidence; and **`event_persist_dropped_total` / ledger drops are not
  historically recoverable**, being logged and not stored, so a divergence caused
  by a historical drop cannot be distinguished from one caused by a defect. The
  `CONFIRMED` path is unexercised by replay for as long as the fleet has one node
  (S2). No durable regression test covers the operator-level divergence path: the
  divergent fixture was manual and was deleted after use.
- **No multi-node correlation in the field.** All multi-node behaviour is proven
  by test only.
- **No measured false-positive rate.** No population of events exists.
- **No measured false-negative rate.**
- **No measured lead time.** No end-to-end warning has preceded shaking for any
  real user. Do not claim EEW lead time.
- **Push delivery not verified across device vendors.** One vendor, in testing.
- **Delivery to an unlocked, in-use device does not happen on the one device
  tested.** Observed failing on hardware 2026-08-31 in both process states. Root
  cause is a **device setting**, not an app defect:
  `settings get global heads_up_notifications_enabled` returns `0`, which makes
  AOSP's `PeekDisabledSuppressor` suppress heads-up for every app on the phone;
  `couldHeadsUp=false` then drives `FullScreenIntentDecisionProvider` to
  `NO_FSI_NO_HUN_OR_KEYGUARD`. The keyguard is why the locked path works.
  Whether QuakeAlert would heads-up with that setting enabled is **not yet
  tested**, so how much of this gap is device-specific is unknown. Policy choice
  is **U-012**.
- **Alert delivery to a force-stopped app is impossible, by platform design.** Not
  a project gap: Android withholds broadcasts from a stopped app until the user
  launches it again.
- **No alert delivery has been observed for a real event on any device.** Every
  device observation to date is the drill path, which writes no row and carries
  no `event_state`.
- **The alert-raising path emits no logs**, so a foreground alarm that does not
  appear cannot be distinguished from one that appeared unobserved (**U-013**).
- **No instrumented (on-device) test suite exists.** `app/src/androidTest`
  contains only the Android Studio template `ExampleInstrumentedTest.kt`, so
  every OS-level behaviour above rests on manual observation, not on a suite that
  can be re-run.
- **Firmware not validated beyond one board in one location.**
- **Reconciliation across a real restart during a real event** — tested, not
  observed in production.
- **Behaviour at high latitude and across the antimeridian** — correct by proof,
  never exercised by real sensors.

---

## Active phase

`ROADMAP.md` ACTIVE PHASE is **Phase 4 — Self-Measurement & Forensics**, status
`IN_PROGRESS` as of 2026-09-01, scoped by **D-011** to acceptance criteria
P4-M1′ … P4-M6′. Phase 4 is instrumentation and read-only forensics on a
**one-node** fleet: it changes no threshold, quorum, radius, event semantic, or
notification policy — the single contract change it carries is the additive,
operator-only one the owner authorized in **D-012**, behind `X-Admin-Key`.
**P4-M3′** (server-side stage latency reported) is owner-approved SATISFIED as of
2026-09-01. **P4-M2′** (near-confirmation log queryable and surviving a restart) is
owner-approved SATISFIED / `VALIDATED` as of 2026-09-03, on the real-PostgreSQL
evidence in *Demonstrated*; committed, **not deployed**. **P4-M4′** (deterministic
replay) is owner-approved SATISFIED / `VALIDATED` as of 2026-09-03, authorized by
**D-013**, on the isolated-PostgreSQL evidence in *Demonstrated*; committed, **not
deployed**, and read-only — it adds no migration and no contract change, so D-012
remains the phase's only authorized contract exception. **P4-M5′** (simulated
multi-node runs in CI) is owner-approved SATISFIED / `VALIDATED` as of
2026-09-03 under **D-014**, on the archived GitHub Actions **CI #22** evidence in
*Demonstrated*; committed, **not deployed**. That `VALIDATED` covers exactly what
D-014 authorized — the harnesses execute in CI and archive their own evidence —
and **nothing beyond it**: the evidence is software simulation only (S9, D-011
constraint 2), and each artifact names what it does not claim (*field validation*,
*production validation*, *real multi-node sensor performance*, *real multi-node
correlation*). It adds no migration and no contract change, so D-012 remains the
phase's only authorized contract exception. **P4-M1′** (trigger durability,
measured not asserted) is owner-approved SATISFIED / `VALIDATED` as of
2026-09-04, on the archived **production** evidence in *Demonstrated*
(`docs/evidence/p4-m1/2026-09-03-production-trace/`, a durable reconstruction of
the lost temporary bundle — D-016) — the only Phase 4 criterion so far carrying
evidence from the real production database, read under a server-enforced
`default_transaction_read_only = on` session. That `VALIDATED` covers exactly the
bounded window it measured — 51 ledger rows, 26 qualifying, 26 traced, 26 exact
advisory matches, the four persistence counters reported alongside — and
**nothing beyond it**: the observation→transition link is membership-and-time and
not causal, `ledger_drops` is UNKNOWN, and the population is **one node**, so it
   is **not** multi-node field validation (S9, S2). It adds no migration and no
   contract change. **P4-M6′** (forensic timeline for one event) is
   owner-approved SATISFIED / `VALIDATED` as of 2026-09-06 per **D-015**, on
   the committed production evidence plus the separate synthetic fixture leg
   in *Demonstrated*
   (`docs/evidence/p4-m6/2026-09-06-production-event-4fcc3374/`): a one-node
   timeline only, read-only proved in-CLI
   (`default_transaction_read_only=on/client` plus effective
   `transaction_read_only=on`), attribution membership-and-time and NON-CAUSAL.
   That `VALIDATED` covers exactly the single locked event it measured and
   **nothing beyond it**: no `CONFIRMED` production path, no multi-node field
   correlation, and no lead-time/population/reliability claim. It adds no
   migration and no contract change.

Phase F remains `BLOCKED` on the owner deploying additional nodes. Every item in
*NOT demonstrated* that requires a confirmed event, multiple nodes, or a
population of events stays there for the duration of Phase 4 — that is expected,
not a Phase 4 failure.

---

## Known open defects

- `docs/SYSTEM_SPEC.md` and `docs/GAP_ANALYSIS.md` describe superseded Phase 2
  architecture. Both now carry historical banners. Not rewritten.
- `contracts/mqtt/alert.schema.json` documents a channel the server does not
  publish (**U-006**).
- `docs/CHAT_DESIGN.md` versus `docs/GAP_ANALYSIS.md` disagree on whether chat
  is in scope (**U-005**).
- The 503 body of both admin tracker endpoints returns `code: TRACKER_DISABLED`
  (`server/internal/api/admin.go:297,309`), a value absent from the `Error.code`
  enum in `contracts/openapi/openapi.yaml`. **Pre-existing and unrelated to Phase
  4:** introduced by `1ad1777` with the Phase 3.x stats endpoint, found during
  P4-M2′ validation on 2026-09-03, deliberately left unfixed because it lies
  outside D-012's scope. Either the enum gains the value or the handler uses an
  existing one; that is a Phase 3.x decision, not a Phase 4 one.
- The `internal/store` and `internal/event` test packages share one database and
  run concurrently by default, so `TestMigration000006DownRestoresSchema` and
  `TestMigration000009DownRestoresSchema` (which drop tables) race
  `pgBreakNearConfirmedTable` (which renames `event_near_confirmed` and back).
  Failures are non-deterministic and vary per run. **Pre-existing and unrelated to
  Phase 4:** reproduced 3/3 on a clean `git archive HEAD` tree containing no
  replay code, found during P4-M4′ validation on 2026-09-03, deliberately left
  unfixed as outside D-013's scope. `go test -p 1` is green (272 passed / 0
  failed). A fix belongs with the test harness — a `TestMain` guard or a
  per-package database — not with a Phase 4 criterion.
- ~~`server/scripts/sim_setup_nodes.go` is not `gofmt`-clean~~ — **closed
  2026-09-03 by `84d46d5`.** The `server` CI job's *Verify formatting* step
  (`gofmt -l .`, run from `server/`, which covers `scripts/`) failed on this file
  in CI #21. Pre-existing and unrelated to Phase 4: the blob was unformatted since
  `0822d35` introduced it with the checkpoint-3 harnesses, and it was found during
  P4-M5′ validation on 2026-09-03. Fixed as its own commit, deliberately kept out
  of the M5′ feature and CI-fix commits — `gofmt -w` only, every changed line a
  comment line, non-comment token stream identical. *Verify formatting* is green in
  CI #22.

## Maintenance

Update this file when the baseline commit changes, when a status column changes,
or when an item moves between *Demonstrated* and *NOT demonstrated*. Do not
record decisions here — those belong in `docs/DECISIONS.md`.
