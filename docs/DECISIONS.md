# DECISIONS.md

Accepted decisions, unresolved questions, and superseded decisions.

Authority: `/contracts` > `PROJECT_RULES.md` > **this file (accepted)** >
`docs/CURRENT_STATE.md` > `ROADMAP.md` > prose docs > `.hermes/plans/*`.

**Legacy identifiers are preserved.** Existing code comments cite `D1`, `D10`,
`D11`, `D18`, `D30`, `A11`, `A17`, `O4`, `P2`, `R9`, `§7.3`, `§9.5` and similar.
Those identifiers are **not renumbered** — renumbering would make code comments
unfindable. New entries use the distinct `D-nnn` / `U-nnn` prefixes and
cross-reference the legacy IDs where they apply. Whether legacy IDs are ever
migrated is **U-009**.

---

## Architecture Decision Records (index, not duplicated)

| ADR | Title | Status |
| --- | --- | --- |
| [ADR-0001](adr/0001-monorepo-and-resource-budget.md) | Monorepo layout & resource budget (1 vCPU / 1 GB VPS) | Accepted |
| [ADR-0002](adr/0002-go-pgx-no-orm.md) | Go backend — pgx without ORM, slog, bounded concurrency | Accepted |
| [ADR-0003](adr/0003-tls-everywhere-and-hmac.md) | TLS everywhere & HMAC secret management | Accepted |
| [ADR-0004](adr/0004-contract-first.md) | Contract-first development | Accepted |

Read the ADRs for their reasoning. They are not restated here.

---

## Accepted decisions

Format: decision, reason, reversibility, what it affects.

### D-001 — `/contracts` outranks prose documentation
**Status:** ACCEPTED · **See:** ADR-0004
When a prose document and a contract disagree, the contract is correct and the
prose is a defect. **Affects:** public contracts. **Reversible:** no
(foundational).

### D-002 — In-memory tracker is the authority; the database is a follower
**Status:** ACCEPTED · **Commit:** `9752c5e` · **Legacy:** D30, §9.5
Event state lives in memory. Persistence is asynchronous, bounded, and
drop-oldest. Emission always happens, whether or not persistence succeeds.
**Because:** a database outage must never suppress a warning
(`PROJECT_RULES.md` §10 / S1). **Affects:** real-time path, data semantics.
**Reversible:** no, without redesign.

### D-003 — Five-state event lifecycle; downgrade is illegal
**Status:** ACCEPTED · **Commit:** `9752c5e` · **Legacy:** §5.2, §6.7
States: DETECTED → UNCONFIRMED → CONFIRMED → RESOLVED / CANCELLED. Confirmed
never downgrades to unconfirmed; retraction is an explicit separate transition.
Emission occurs only on transitions. `revision` increments only on transitions.
**Because:** monotonic public claims (S3). **Affects:** data semantics, public
contract. **Reversible:** no.

### D-004 — Independence is geodesic distance between registered coordinates
**Status:** ACCEPTED · **Commit:** `1ad1777` · **Legacy:** §7.3
Independence is measured as direct geodesic distance between contributors'
**registered** coordinates, not as occupancy of grid cells. The count is a
deterministic **lower bound** on the true maximum independent set (greedy over
sorted contributors — it may undercount, never overcount).
**Because:** a grid threshold depends on where cell boundaries happen to fall;
distance has no boundary to align with. Determinism matters because map
iteration order must not be able to move a safety gate.
**Known limit:** non-geographic independence axes (owner, network, power
source) are real but not representable in the current schema — see **U-002**.
**Affects:** confirmation semantics. **Reversible:** yes, with a version bump.

### D-005 — Candidate search width is derived per observation from its latitude
**Status:** ACCEPTED · **Commit:** `1ad1777` · **Legacy:** D1, invariant I-COV
Any tracked event whose centroid lies within the attach radius of an
observation must appear in that observation's candidate set — at every latitude,
on both axes, across the antimeridian. The probe may over-select; it must never
under-select. The polar case is reported explicitly rather than dividing by
`cos(lat) → 0`.
**Because:** a fixed neighbourhood is only sufficient inside the latitude band
it was derived from, and a test whose bounds match the assumption it tests can
never find the defect. **Affects:** confirmation semantics (globally).
**Reversible:** no — reverting reintroduces a known global defect.

### D-006 — `algo_ver` carries decision semantics; no migration rewrites history
**Status:** ACCEPTED · **Commit:** `1ad1777` · **See:** `PROJECT_RULES.md` §11
The persisted algorithm version is `<base>/<params>`. A version bump replaces a
migration. Rows written under an earlier version remain interpretable under the
rules that produced them.
**Because:** the same column meant "occupied grid cells" before D-004 and
"mutually separated contributors" after. Rewriting past rows would falsify
decisions actually taken. **Affects:** data semantics. **Reversible:** no.

### D-007 — Safety-critical thresholds are compile-time constants
**Status:** ACCEPTED
The PGA floor and contributor quorum are compile-time constants, not
environment variables. **Because:** they must follow the binary that made the
decision; a runtime-changeable threshold lets a deployment silently stop
warning without anyone deciding to. **Affects:** confirmation semantics.
**Reversible:** yes, but see S4.

### D-008 — MQTT trigger publication uses QoS 0, honestly named
**Status:** ACCEPTED · **Commit:** `942d74f` · **Contract:**
`contracts/mqtt/trigger.schema.json`
Firmware publishes at QoS 0. The client library does not wait for `PUBACK`, so
declaring QoS 1 would be an unenforced claim. Resilience comes from firmware
retry plus `obs_seq` + `phase` de-duplication server-side.
**Because:** naming must match behaviour (`PROJECT_RULES.md` §8). A schema
claiming at-least-once delivery that the transport does not provide is worse
than an honest QoS 0.
**Supersedes:** the "QoS 1" claim previously stated in
`.clinerules/00-project-overview.md` and `docs/SYSTEM_SPEC.md`. Those documents
were defects.
**Affects:** public contract, delivery reliability. **Reversible:** yes, but
only together with a client that actually enforces it.

### D-009 — Unconfirmed events are WebSocket-only; no push
**Status:** ACCEPTED · **Legacy:** D10, D11, §8.1
Unconfirmed events are published as advisory frames over WebSocket and never
trigger a push notification. Confirmed events additionally send a
high-priority push. The client enforces "advisory never wakes the device" in
three independent places.
**Because:** alarm-grade delivery requires alarm-grade evidence (S4).
**Consequence, stated honestly:** a backgrounded device learns nothing until
CONFIRMED, which means the part of the timeline containing lead time is
currently invisible to most of the realistic audience. That is the substance of
**U-001** and is **not** resolved here.
**Affects:** delivery. **Reversible:** yes, by owner decision only.

### D-010 — Boot-time self-checks warn; they never lower a threshold
**Status:** ACCEPTED · **Commit:** `1ad1777` · **Legacy:** §7.3, §15.3
On startup the server reports whether the current network can reach CONFIRMED
at all, and reconciles open events so an observation after a restart attaches
to the pre-restart identity rather than forming a second event. Failures log
and the server still starts.
**Because:** a server that refuses to start because it cannot read past events
is a server that cannot warn about the earthquake in progress. And lowering a
safety threshold to silence a startup warning is how a system stops working
without anyone deciding it should. **Affects:** real-time path.
**Reversible:** yes.

### D-011 — Phase 4 scope is fixed to six criteria measurable on a one-node fleet
**Status:** ACCEPTED · **Owner-approved:** 2026-09-01 · **See:** `ROADMAP.md`
§ Phase 4
Phase 4 (`VALIDATION`) is bounded to acceptance criteria **P4-M1′ … P4-M6′**:
trigger durability *reported* over a bounded ledger window, a near-confirmation
log that survives a restart, server-side stage latency (`onset_ts → decided_at`,
`decided_at → emit`), deterministic replay grouped by `algo_ver`, the existing
simulated multi-node harnesses running in CI, and a read-only per-event forensic
timeline. It produces instrumentation and read paths only.

**Because:** the fleet is one physical node and no further nodes are planned
during this phase, so CONFIRMED is unreachable in production throughout it (S2 —
a network-density fact, not a defect). A phase whose exit depends on a confirmed
event would be unpassable, and the temptation to pass it by loosening the gate is
exactly what S4 and §6 forbid. Fixing the scope in advance also stops Phase 4
from growing into the abandoned catalogue-comparison definition.

**Three constraints carried by this decision, stated so they cannot be read
away:**
1. **No hard reliability target.** `event_persist_dropped_total` is *reported*,
   never asserted zero. The ledger writer is bounded and drop-oldest by design
   (D-002/S1): a drop means a durability record was shed *after* the warning was
   emitted, which is the intended trade, so a zero-drop exit criterion would
   misrepresent the architecture as well as being unpassable.
2. **Simulation is software evidence only.** `sim_multi_node.sh` /
   `sim_dual_event.sh` passing in CI may never be cited as multi-node field
   correlation (S9). Field multi-node evidence belongs to Phase F.
3. **N-node compatibility is required now, extra nodes are not.** The
   instrumentation records `node_id`, per-node onset time and provenance,
   `event_id`, `algo_ver`, and cell identity from the start — all already present
   in Phase 3/3.x rows — so a second node participates correctly without
   redesigning the event or consensus model.

**Affects:** phase scope, observability surface. **Reversible:** yes — Phase 4
may be re-scoped by a later owner decision; nothing here changes what the system
decides.

**Does not decide:** U-001 … U-013 remain unresolved. In particular Phase 4 is
deliberately designed to need **none** of them: it adds no wire field, no
delivery tier, no notification-policy change, and no contract change, so it can
proceed while U-010 (alert validity), U-011 (concurrent alarms), U-012 (in-use
delivery) and U-013 (raise-path logging) stay open.

**Cross-reference (added 2026-09-02; nothing above is rewritten):** D-012 narrows
the "no contract change" clause of the paragraph above — it authorizes one
additive change to the **operator-only** admin contract for P4-M2′, plus the
`000009` migration. The scope of Phase 4, the three constraints, and every other
claim in this entry stand exactly as written; U-001 … U-013 remain unresolved.

### D-012 — The near-confirmation record is persisted, and the answer states its own coverage
**Status:** ACCEPTED · **Owner-approved:** 2026-09-02 · **See:** `ROADMAP.md`
§ Phase 4 (P4-M2′), D-011,
`contracts/db/migrations/000009_near_confirmation_durability.up.sql`

The near-confirmation crossing is written to a durable table, and the
near-confirmed read path answers with an explicit coverage envelope beside the
list. Two parts, decided together:

**A2 — persist the crossing.** A new additive table `event_near_confirmed`, one
row per event, records the crossing as it happened: `first_two_independent_at`,
`independent_count_at_peak` with its paired `node_count_at_peak`,
`min_independent_cells` and `algo_ver` *as they were at that moment*, plus
`confirmed_at` / `terminal_state` / `terminal_at` where they apply. Writes travel
the existing bounded drop-oldest ledger queue, asynchronously, outside the
Tracker lock.

**B1 — make the answer self-describing.** `GET /api/v1/admin/tracker/near-confirmed`
gains a `coverage` object stating the window the answer covers
(`process_started_at_ms`, `as_of_ms`), whether the durable read was attempted and
whether it succeeded, how many rows it loaded, and the provenance split of the
entries returned. `entries` stays a top-level array under the same name; the
envelope is additive.

**Because:** P4-M2′ requires the record to survive a restart, and re-deriving it
read-only cannot recover it faithfully. A crossing can happen with **no state
transition at all** — `UNCONFIRMED → UNCONFIRMED` is illegal (§5.2), so there is
no revision, no `event_state_log` row and no emitted frame — and those silent
crossings are the common case on a small fleet. `earthquake_events` cannot answer
either: it is mutable by design and holds only the latest independence count, so
an event that reached three independent contributors and then fell back to one
reads as never having come close. And on a one-node fleet the correct list is
**empty** (S2 — CONFIRMED is unreachable), so an empty list must be
distinguishable from "nothing could be answered"; without the envelope both ship
as identical bytes. Forensic correctness is worth more here than a smaller schema.

**This decision explicitly authorizes,** and nothing beyond:
1. **The migration.** `000009_near_confirmation_durability` — one new table,
   additive and idempotent, no `ALTER`, no `DROP`, no type change, no rewrite of
   existing rows. Pre-000009 binaries keep running against this schema.
2. **The durable record.** Writing near-confirmation rows from the alert path,
   including for crossings that produce no state transition.
3. **The contract change.** The additive `coverage` object and the four added
   entry fields (`min_independent_cells`, `algo_ver`, `source`,
   `updated_in_process`) in `contracts/openapi/openapi.yaml`, plus two new
   counters in the tracker stats response.

**Constraints carried by this decision, stated so they cannot be read away:**
1. **Persistence never blocks emission (S1, §9.5).** The queue stays bounded and
   drop-oldest. Drops and upsert failures are *reported* —
   `event_near_confirmed_persist_dropped_total`,
   `event_near_confirmed_upsert_failures_total` — and counted **separately** from
   the event-unit counters. Neither is ever asserted zero: D-011 constraint 1
   applies unchanged, and a zero-drop target here could only be met by blocking
   the warning path.
2. **In-memory Tracker stays the authority (§9.5, D-002).** The table is a
   follower. The boot-time read plants rows only for events not already in
   memory; it never overwrites a live entry.
3. **No recomputation from current state.** The recorded threshold and `algo_ver`
   are carried as written and frozen on conflict (V3/V6, D-006). Historical
   independence is never recomputed from current node coordinates — **U-007 is
   not reopened.** Judging a past decision with parameters that did not produce it
   is not a correction.
4. **`event_state_log` is untouched.** That table means one row per state
   *transition*; a threshold crossing is not a transition. No non-transition row
   is added to it, and its semantics are unchanged.
5. **`entries: []` remains an honest answer.** On the current one-node fleet the
   list may legitimately stay empty for all of Phase 4. The envelope reports
   coverage and provenance; it does not grade them. There is deliberately no
   `complete`, `healthy` or `valid` field.

**Affects:** schema (one new table), the near-confirmed read path, the admin
contract, observability counters. **Reversible:** yes — `000009` down is a single
`DROP TABLE`, and the code degrades to "since this process started". What is
**not** recoverable after that rollback is the crossing history itself, silent
crossings included: they exist in no other table and cannot be re-derived.

**Does not decide:** nothing about what the system decides. Quorum, thresholds
(`MinPGAGal`, `MinNodesConfirmed`, `MinIndependentCells`), attach radius, the
legal state transitions and event semantics are all unchanged; this is a record
and a read path. U-001 … U-013 **remain unresolved**, U-007 included. D-011 is
**not** superseded: its scope, its three constraints and its reasoning stand as
written. Its "Does not decide" paragraph says Phase 4 "adds no wire field, no
delivery tier, no notification-policy change, and no contract change" — the
first, second and third of those remain true, and this decision narrows the
fourth: it authorizes **one additive admin-contract change**, on an
operator-only endpoint behind `X-Admin-Key`, with no change to any client-facing
or wire contract. The four unresolved questions D-011 names (U-010, U-011, U-012,
U-013) stay open and untouched.

**Validation record (appended 2026-09-03; nothing above is rewritten).** P4-M2′ is
**owner-approved SATISFIED / `VALIDATED` 2026-09-03**. Evidence: migration `000009`
applied to an isolated PostgreSQL database (`TEST_DATABASE_URL`, loopback-only
container), 14 previously-never-executed integration tests green — 11 in
`server/internal/store/near_confirmed_test.go` and 3 in
`server/internal/event/nearconfirmed_pg_test.go` — plus 19 pre-existing Postgres
tests re-run green on the same schema, and restart reproduced end-to-end through
the HTTP surface: a real silent crossing recorded with **no** `event_state_log`
row for the crossing, `source` moving `RECORDED → LOADED` across a process
termination, and per-entry `algo_ver`/`min_independent_cells` preserved verbatim
under a running configuration that differed from every stored row. Constraint 5
was exercised directly: on an empty database the answer was `entries: []` with
`durable_read_attempted: true`, `durable_read_ok: true`, `durable_rows_loaded: 0`.
`event_state_log` semantics are unchanged, and **U-001 … U-013 remain unresolved,
U-007 included** — nothing in this validation reopened any of them. Not deployed:
the code is committed only. Full detail in `docs/CURRENT_STATE.md` § Demonstrated
and `ROADMAP.md` § Phase 4 (P4-M2′).

### D-013 — Deterministic replay proves the decisions; identity is compared by grouping and time by elapsed offset
**Status:** ACCEPTED · **Owner-approved:** 2026-09-03 · **See:** `ROADMAP.md`
§ Phase 4 (P4-M4′), D-011, `PROJECT_RULES.md` V5/V7 and §11,
`server/internal/event/replay.go`, `server/scripts/replay_window.go`

P4-M4′ replays recorded observations through a fresh tracker, read-only, and
compares the decisions it makes against the recorded `event_state_log`. Four
comparison boundaries are approved, and V7 is satisfied through them:

**F2 — event identity is compared as an observation-grouping bijection, not as
UUID equality.** `event_id` is a freshly generated UUID minted when DETECTED is
created, so a replayed run can never reproduce the historical value; requiring
equality would make V7 unsatisfiable by construction. Each historical event and
each replayed event is instead reduced to its **grouping signature** — the sorted
set of `node_id#obs_seq` contributing to its last revision — and the two sets of
events must stand in a one-to-one correspondence under that signature. That is
the property V7 asks for: the same observations grouped into the same events, one
for one, no merge and no split.

**F3 — `decided_at` is compared as a relative delta with tolerance, never as an
absolute timestamp.** Wall-clock equality is unreachable: `decided_at` is a
server clock reading taken at decision time, and the phase of the sweep tick that
produced a terminal transition was never recorded. Every revision after the first
is compared as its elapsed offset from that event's first decision — historical
against replayed — and must agree within a tolerance defaulting to
`EVENT_SWEEP_INTERVAL_MS + 1000` ms. A sweep-quantised difference is not a
divergence; a different decision is.

**Canonical input order is `received_ts, observation_id`.** That, and only that,
is the order in which recorded observations are fed to the tracker. It is a
*total* order because `observation_id` is `BIGSERIAL` and breaks ties on an
identical `received_ts`. Reproducibility under V7 is defined against this order;
a replay fed in any other order is a different experiment and its outcome is
evidence neither for nor against V7.

**Replay parameters are operator-asserted unless they were historically
recorded.** `algo_ver` records `INDEPENDENCE_CELL_KM`, and nothing else.
Correlation window, attach radius, resolve-after, sweep interval, max diameter
and `MIN_INDEPENDENT_CELLS` were never written beside the rows, so replay takes
them from the operator, prints them as an assertion **before** any result, and
rejects a profile whose `INDEPENDENCE_CELL_KM` contradicts the `algo_ver` on the
rows being replayed. A matching replay therefore proves *these observations,
under **these** parameters, produce those decisions* — never that those
parameters were the ones in force.

**Because:** V7 read as identifier equality is unsatisfiable, and an
unsatisfiable criterion is either quietly abandoned or "satisfied" by a replay
that reuses the historical `event_id` and proves nothing. The two quantities that
cannot be reproduced exactly are precisely the two that carry no decision
meaning — a random UUID and a clock reading — while everything that does carry
decision meaning is still compared exactly: revision count, `from_state`,
`to_state`, `reason`, `node_count`, `independent_cells`, and the grouping itself.
Recovering the unrecorded parameters is impossible without rewriting history
(V3/V6, D-006), and adding a schema change to record them retroactively is beyond
a read-only forensics phase (D-011). Asserting them out loud, with the one
contradiction the data *can* detect rejected outright, is the honest alternative
to a silent default.

**This decision explicitly authorizes,** and nothing beyond:
1. **Two read-only readers.** `ListObservationsForReplay` and
   `ListStateLogForReplay` in `server/internal/store/event_lifecycle.go`: two
   `SELECT`s, canonical order, interval closed at both ends, no filtering.
2. **The replay engine.** `server/internal/event/replay.go` — a fresh `Tracker`
   per call, no persister, no ledger, never reconciled.
3. **The operator CLI.** `server/scripts/replay_window.go` (`//go:build
   ignore`), with exit codes 0 success / 1 divergence or rejected profile /
   2 empty observation window.
4. **Grouping by `algo_ver` before comparison** (V5, `PROJECT_RULES.md` §11).
   Rows carrying different labels are never replayed as one stream.

**Constraints carried by this decision, stated so they cannot be read away:**
1. **Read-only by construction.** No migration, no schema change, no contract
   change, and no `INSERT`, `UPDATE` or `DELETE` on any path this decision
   authorizes. Replay builds its own tracker, holds no persister, and never
   reconciles.
2. **Comparison is per event, never as one global stream.** `sweepLocked()`
   iterates a map, so the order of terminal transitions *between* events inside
   one sweep tick is undefined; a global-stream comparison would report that
   undefined order as a divergence.
3. **A matching replay is software evidence, never field validation (S9).** It
   shows that recorded input reproduces recorded decisions. It says nothing about
   production, about the network, or about which parameters were actually in
   force.
4. **Rows the tracker cannot use are reported, not dropped silently.** Failed
   verification, NULL `node_location`, and no onset anchor are counted and named
   in the input report. The SQL deliberately does not filter them, so the count
   cannot be lost before the caller sees it.
5. **`event_persist_dropped_total` / ledger drops are not historically
   recoverable.** Drops are logged, not stored, so replay cannot distinguish an
   observation the tracker never saw from one that was never recorded. A
   divergence caused by a historical drop is indistinguishable from a divergence
   caused by a defect; the operator asserts what is known and the assertion is
   printed with the result.
6. **U-007 is not reopened.** Historical independence is never recomputed against
   current node coordinates. Replay uses each observation's own recorded
   location, and a stored `algo_ver` is a gate, never something to be corrected.

**Affects:** the forensics surface only — two read-only store readers, one
package-internal replay engine, one build-ignored operator script, and the
governance records of P4-M4′. No schema, no published contract, no counter, no
runtime behaviour of the deployed server binary. **Reversible:** yes, completely
— deleting the three new files and the two readers leaves no trace, because
nothing was written and nothing was migrated.

**Does not decide:** nothing about what the system decides. Thresholds
(`MinPGAGal`, `MinNodesConfirmed`, `MinIndependentCells`), quorum, attach radius,
the legal transitions and every event semantic are unchanged; this is a read path
and a comparison. **`event_state_log` semantics are untouched** — one row per
state transition, and replay adds none. **U-001 … U-013 remain unresolved, U-007
included.** D-011 is **not** superseded: its scope, its three constraints and its
reasoning stand as written, and this decision changes none of them — in
particular D-011's "no wire field, no delivery tier, no notification-policy
change, and no contract change" holds here in full, since replay adds no contract
change at all. D-012 is **not** superseded and is unaffected. The pre-existing
Phase 3.x `TRACKER_DISABLED` enum gap (`server/internal/api/admin.go:297,309`
versus `Error.code` in `contracts/openapi/openapi.yaml`) is **not** addressed
here and remains open as a Phase 3.x matter.

**Validation record (appended 2026-09-03; nothing above is rewritten).** P4-M4′ is
**owner-approved SATISFIED / `VALIDATED` 2026-09-03**. Evidence: an isolated
loopback-only PostGIS container with all nine migrations applied; 3 new
PG-gated tests in `server/internal/store/replay_read_test.go` proving the two
readers' canonical order, closed interval and non-filtering against a real
schema for the first time; 34 M4′ tests green in total; the whole suite 272
passed / 0 skipped / 0 failed run serially, and `go test -race` clean across all
10 packages. The recorded real-sensor window (`NODE-52960B47`, event
`3adf752d-48f1-4f81-b98e-d31e3775c923`, observations 28 and 29) was seeded and
replayed at exit code 0: bijective under the signature `NODE-52960B47#1507330`,
both revisions reproduced (`DETECTED→UNCONFIRMED FLOOR_MET`, then
`UNCONFIRMED→RESOLVED NO_NEW_EVIDENCE`) with `independent_cells` matching, F3
deltas 0 ms and 2194 ms against a 6000 ms tolerance, and re-feeding the same
window produced no second event. Read-only was proven three ways rather than
argued: per-row `xmin`, row counts and sequence `last_value` unchanged;
`pg_stat_user_tables` insert/update/delete/hot-update counters unchanged; and the
same run under an enforced `default_transaction_read_only = on` session producing
byte-identical output at exit 0. A deliberately divergent fixture reported
`independent_cells: historis=2 replay=1` and exit 1, so a passing result is
distinguishable from a comparison that cannot fail. Operator exit codes 0 / 1 /
2 / 1 were confirmed on a built binary, the rejected-profile path included.
**Not claimed:** production or field validation of any kind (S9) — nothing was
deployed and the production stack was untouched; one event on one node, so the
CONFIRMED path stayed unexercised (S2); the real-sensor fixture's
`evidence_summary` is **reconstructed**, that session having captured only the
scalars, so evidence-field agreement there is tautological and only the recorded
scalars are independent evidence; the replay parameters were operator-asserted;
`decided_at` agreement is relative, not absolute; and `ledger_drops_total` was
not historically recoverable. Full detail in `docs/CURRENT_STATE.md`
§ Demonstrated and `ROADMAP.md` § Phase 4 (P4-M4′).

### D-014 — The simulation harnesses run as one serial CI job that archives its own evidence, and that evidence is software only
**Status:** ACCEPTED · **Owner-approved:** 2026-09-03 · **See:** `ROADMAP.md`
§ Phase 4 (P4-M5′), D-011 constraint 2, `PROJECT_RULES.md` §8 and S9,
`.github/workflows/ci.yml` job `simulation`, `server/scripts/sim_evidence.sh`

P4-M5′ is satisfied by **one dedicated GitHub Actions job** that runs the two
existing harnesses and archives a structured JSON artifact per harness. Six
parts, decided together:

**A — a dedicated job, not a step inside an existing one.** `.github/workflows/ci.yml`
gains a fourth job, `simulation`, alongside `server`, `firmware` and `android`.
It declares no `needs` and nothing declares `needs: simulation`: those three jobs
answer *does the code hold together*, this one answers *does the pipeline reach
CONFIRMED end to end*, and folding either into the other would put a four-minute
Docker run in front of every unit test in the repo. A red simulation is still a
red workflow — the job's own result carries that — but it blocks no other job.

**B — serial execution, by construction.** `sim_multi_node.sh` then
`sim_dual_event.sh`, two sequential steps on one runner. This is a requirement,
not a preference: both harnesses publish the **same fixed host ports** (5432,
6379, 1883, 8080) and name their containers by a fixed compose prefix, and both
assert on **absolute deltas** in shared tracker counters. Run concurrently they
would contend for the ports and, worse, read each other's counters, turning
`delta == 2` into a coin toss. The first harness runs under `continue-on-error`
so that a red 3.1 cannot hide 3.2's result; the job's pass/fail is re-derived
from both recorded outcomes in a final gate step, so suppressing the step-level
exit code does not weaken the verdict.

**C — the artifact is emitted by the harness, not reconstructed from logs.** A
shared library, `server/scripts/sim_evidence.sh`, is sourced by both scripts and
owns the field names, so the two artifacts cannot drift apart. Each run writes
`.sim-evidence/sim_multi_node.evidence.json` and
`.sim-evidence/sim_dual_event.evidence.json` at `schema_version 1`, carrying
`run_id`, `git_sha`, `git_dirty`, `script`, `checkpoint`, `status`, `exit_code`,
`started_at`, `finished_at`, `error`, `tracker_counters_before`,
`tracker_counters_after`, and an `evidence` object holding the scalars and the
assertion list the run's verdict actually rested on. It is written from an EXIT
trap that fires **before** teardown and re-raises the original exit code, so a
run that dies in STEP 0 still leaves an artifact behind and PASS/FAIL semantics
are untouched. `status` is three-valued — `PASS` / `FAIL` / **`ERROR`** — because
*the system misbehaved* and *the run never reached a verdict* are different
findings, and reading a broken runner as a broken detector is the exact confusion
this milestone exists to prevent.

**D — the artifact is uploaded, and its absence is an error.** `actions/upload-artifact`
publishes both files as `simulation-evidence` under `if: always()`, because a
failing simulation is when its counters matter most, and with
`if-no-files-found: error`, because a job that reports success while archiving no
evidence makes M5′ meaningless. A following step re-validates each uploaded file
for valid JSON, the required top-level fields, a `status` in the enum, and
`evidence_class == "SOFTWARE_SIMULATION"`.

**E — the boundary travels inside the artifact.** Every artifact declares
`evidence_class: "SOFTWARE_SIMULATION"` and an explicit `not_claimed` list —
field validation, production validation, real multi-node sensor performance, real
multi-node correlation. An artifact travels: it is uploaded, downloaded, pasted
into a report, quoted months later. The numbers alone read like field evidence,
so the qualification is stored beside them rather than left in a document that
may not travel with them.

**F — STEP 9 of `sim_dual_event.sh` is retained.** The Android
`WarningNotifierStandDownTest` continues to run as part of checkpoint 3.2; the CI
job installs Temurin 21 and a read-only Gradle cache for it. Its outcome is now
recorded as an assertion in the artifact instead of only reaching stdout. No
Android behaviour is changed.

**Because:** the harnesses already existed and already passed locally, which is
precisely the problem — evidence that lives on one developer's machine and
survives only as scrollback is not archived evidence, and P4-M5′ asks for CI
execution *and* archival. Generating the JSON inside the script rather than
parsing it out of CI logs afterwards is what makes the artifact evidence at all:
a reconstruction is a claim about what the run did, while an emission is what the
run recorded about itself.

**Configuration-name corrections, carried by this decision.** Both harnesses
exported `RESOLVE_AFTER_MS`, which `config.go` never reads — the sweep deadline
is `EVENT_RESOLVE_AFTER_MS`, and the built-in default 90000 had been silently in
force all along. Corrected to the real name at the **same value**, so no timing
changes. Both also exported `MIN_PGA_GAL` and `MIN_NODES_CONFIRMED`, which are
**compile-time constants** (D-007) and were read by nothing: the exports are
deleted rather than kept as decoration, because a variable that looks like a knob
and moves nothing is a lie about the gate's tunability (§8 — naming must match
behaviour).

**Nothing about what the system decides is touched.** No threshold, quorum,
radius, correlation window, independence rule, confirmation semantic, simulation
coordinate, or event algorithm is changed. No migration, no contract change, no
new table, no metrics infrastructure, no firmware change, no deployment. The
harnesses remain runnable locally outside CI and keep their human-readable
stdout; the artifact directory is git-ignored, being per-run output rather than
tracked source.

**Cross-reference (nothing in D-011 or D-012 is rewritten).** This decision is
the mechanism for D-011's fifth criterion and is bound by **D-011 constraint 2**
exactly as written: `sim_multi_node.sh` / `sim_dual_event.sh` passing in CI may
never be cited as multi-node field correlation (S9), and field multi-node
evidence belongs to Phase F. D-011 constraint 3 also stands: the harnesses drive
virtual nodes that are database rows, which is what N-node compatibility lets
them do, and it is not a claim that a second physical node exists. **D-012**
supplies the read path the artifacts snapshot — the near-confirmed coverage
envelope is captured verbatim into `evidence.near_confirmed` — and D-012 remains
Phase 4's **only** authorized contract exception; M5′ adds none.

**Stated for the record, and not to be softened:** M5′ demonstrates that the
multi-node and dual-event simulation harnesses execute successfully in CI and
produce archived software evidence. It does not validate field correlation,
production behavior, or real multi-node sensor performance.

**Affects:** CI surface, `server/scripts/sim_*.sh`, `.gitignore`.
**Reversible:** yes — the job may be removed or re-scoped by a later owner
decision; nothing here changes what the system decides, and deleting it would
cost archival, not behaviour.

**Does not decide:** U-001 … U-013 remain unresolved, U-007 included; nothing
here reopens or reinterprets any of them. It does not decide P4-M1′ or P4-M6′, it
does not resolve the M4′ open items F2/F3, and it grants **no** `VALIDATED`
status: `IMPLEMENTED → VALIDATED` for P4-M5′ requires an owner ruling on the
archived evidence, and may not be granted by the agent that implemented it
(`PROJECT_RULES.md` §8, S9). One physical ESP32 remains the entire fleet.

**Validation record (appended 2026-09-03; nothing above is rewritten).** The owner
ruling this decision required has been given. P4-M5′ is **owner-approved SATISFIED
/ `VALIDATED` 2026-09-03**, on GitHub Actions **CI #22** — run id `33737869128`,
attempt 1, `workflow_dispatch` on `development` at head
`84d46d52856cc97b69f45164041a2070ea5aada1`, 2026-09-03T09:15:51Z → 09:22:14Z,
conclusion **success**, all four jobs green (Go server, Firmware host tests,
Android app, and *Simulation harnesses (software evidence)* with all 13 steps
successful). The `simulation-evidence` artifact (3278 B, SHA-256
`62605c4f9e4737c982769c80f55b9f0ab364f119bfe0f6e46c822f7f00fd0bac`) was downloaded
and re-read from the archive rather than transcribed from the log: it contains
exactly `.sim-evidence/sim_multi_node.evidence.json` (checkpoint 3.1, `status=PASS`,
`exit_code=0`, 9 assertions passed / 0 failed) and
`.sim-evidence/sim_dual_event.evidence.json` (checkpoint 3.2, `status=PASS`,
`exit_code=0`, 11 passed / 0 failed), both `schema_version 1`, both
`evidence_class: "SOFTWARE_SIMULATION"`, both with `git_dirty=false` and `git_sha`
equal to the run head, and both carrying the `not_claimed` list verbatim. The
ephemeral `ADMIN_API_KEY` shows as `***` in every `env:` block of the run log and
in neither artifact.

Two delivery-path defects surfaced in the first real run, CI #21 on `1e09a6b`, and
belong in this record: the upload archived **nothing** while every simulation
assertion passed, because `actions/glob` skips dotted path components unless
`include-hidden-files` is set, and the generated key printed unmasked five times.
Both were fixed in `acd25d4` (`include-hidden-files: true`; `::add-mask::`
registered before the `$GITHUB_ENV` write; `server/scripts/sim_evidence_selftest.sh`
pinning the delivery contract with 24 assertions, each mutation-tested red) with the
unrelated pre-existing `gofmt` baseline fixed separately in `84d46d5`. `if: always()`
and `if-no-files-found: error` were left intact — the files were made findable, not
the empty archive acceptable. No simulation semantics, threshold, coordinate,
confirmation rule, production runtime behaviour or Android behaviour was changed.

**What this `VALIDATED` covers, and what it must never be read as.** It covers
exactly what this decision authorized: the two harnesses execute in CI and produce
archived software evidence. It is **not** field validation, **not** production
validation, **not** real multi-node sensor performance, and **not** real multi-node
correlation — the four claims each artifact names in `not_claimed`. The harnesses
drive virtual nodes that are database rows with hand-picked coordinates (S9, D-011
constraint 2); the fleet is still one physical ESP32, and Phase F owns the field
evidence. **D-011 and D-012 are unchanged in meaning**: D-011 constraint 2 is what
bounds this evidence, and D-012 remains Phase 4's only authorized contract
exception — M5′ adds no migration and no contract change. **U-001 … U-013 remain
unresolved, U-007 included**; nothing in this validation reopens or reinterprets any
of them. Not deployed: the code is committed only. Full detail in
`docs/CURRENT_STATE.md` § Demonstrated and `ROADMAP.md` § Phase 4 (P4-M5′).

---

### D-016 — Milestone evidence lives in the repository, and the lost M1′ bundle is reconstructed rather than re-measured
**Status:** ACCEPTED · **Owner-approved:** 2026-09-04 · **See:** `ROADMAP.md`
§ Phase 4 (P4-M1′), `docs/CURRENT_STATE.md` § Demonstrated, `PROJECT_RULES.md` §8,
`docs/evidence/p4-m1/2026-09-03-production-trace/`

**The defect.** P4-M1′ was owner-approved SATISFIED / `VALIDATED` on 2026-09-04
citing archived production evidence at `/tmp/m1-prod-20260903T125908Z`. `/tmp` on
the workstation is a **tmpfs** mount (`findmnt -no FSTYPE,OPTIONS /tmp` →
`tmpfs rw,nosuid,nodev,…`), so it is memory-backed and empty after a boot. The
workstation rebooted at 2026-09-04T17:32:02 local and the entire bundle ceased to
exist, together with two sibling bundles from the same milestone
(`/tmp/m1-evidence-20260903T105023Z`, the throwaway-PostgreSQL software leg, and
`/tmp/m1-prod-attempt-20260903T123124Z`, the documented failed-access attempt).
Nothing deleted them deliberately; no cleanup command was run against them. This
is an **evidence-archive lifecycle defect**: `PROJECT_RULES.md` §8 makes the named
archived evidence part of what `VALIDATED` means, so a milestone whose cited path
has stopped existing has a broken evidence trail even when the measurement itself
was sound. Both `ROADMAP.md` and `docs/CURRENT_STATE.md` were pointing at nothing.

**A — evidence a governance document cites belongs in version control.** The
milestone evidence is now committed at
`docs/evidence/p4-m1/2026-09-03-production-trace/`: eight files —
`trace_triggers_stdout.txt`, `production_db_readonly_before.txt`,
`production_db_readonly_after.txt`, `production_algover_and_emissions.txt`,
`tracker_stats_prod.json`, `metadata.txt`, `RECONSTRUCTION_NOTE.txt` and
`SHA256SUMS.txt`. This does **not** retroactively re-file P4-M5′ evidence:
`.sim-evidence/` stays gitignored and per-run by D-014, because that evidence is
regenerated by CI on demand, whereas a one-shot read of live production is not
reproducible at will and therefore cannot survive as a rerun instruction.

**B — the archive is a reconstruction, and says so on every file.** The contents
were rebuilt from this session's own transcript, which had recorded the tool's
stdout, both database snapshots, the tracker-stats body, the algo_ver/emission
distribution, and every shell command that wrote the original `metadata.txt`.
Every file carries the banner `RECONSTRUCTED FROM SESSION TRANSCRIPT — ORIGINAL
/tmp BUNDLE LOST AFTER TMPFS REBOOT`, names the original path, states that the
original no longer exists, and separates **exact transcript-preserved
reproductions** from **reconstructed metadata**. Byte-identity with the lost
originals is **not** claimed and cannot be established once the originals are
gone; what is claimed is the same recorded values at the same recorded lengths.
One value the transcript does not contain —
`event_latency_decided_to_emit_ms` `p50_ms`/`p95_ms`, cut off by a `head -30` in
the only rendering — is recorded as **NOT RECOVERED** rather than filled in, and
the original `utc_finish_pass` reading is likewise marked NOT RECOVERED and merely
bounded by surrounding record timestamps. Nothing was invented, recomputed, or
improved. The transcript itself was **not** copied into the repository, and no
credential, DSN, API key, private key or secret was carried across.

**C — reconstruction is not validation, and the M1′ result is untouched.** No
production validation was rerun to produce this archive: no SSH session, no
database connection, no HTTP request, no cross-compilation, no deployment. The
substantive P4-M1′ result is **unchanged** — 51 ledger rows read of 200 requested,
25 below the `MinPGAGal` floor, 0 excluded, 26 qualifying, TRACED 26 over 25
distinct `UNCONFIRMED` transitions, `MATCHED_BY_EVENT_ID_AND_REVISION` 26, the
four persistence counters reported at 0 alongside and not as a target — and so are
its limits: the observation→transition link is **membership-and-time, not causal**,
`ledger_drops` is **UNKNOWN**, the population is **one node**, so this is
production validation and **not** multi-node field validation (S9, S2), and
production still runs `schema_version = 8`. This decision changes the evidence
*location* and nothing else. The M1′ acceptance criterion is not modified, and
**D-011, D-012, D-013 and D-014 are unchanged in meaning** — D-012 remains Phase
4's only authorized contract exception; this adds no migration and no contract
change. **U-001 … U-013 remain unresolved, U-007 included**; nothing here reopens,
reinterprets or resolves any of them. The corrective commit does not amend the
sign-off commit `c51c7cc`; it follows it. Not deployed: committed only.

---

## Unresolved questions

**Do not resolve any of these by implementation.** Each requires an explicit
owner decision recorded here first (`PROJECT_RULES.md` §9).

### U-001 — Does unconfirmed detection ever get a background delivery channel?
**Why it matters:** largest known coverage gap. Under D-009 the lead-time
portion of the timeline reaches nobody whose device is locked.
**What would settle it:** an owner decision on whether a non-alarming
background tier exists at all, plus evidence on whether low-priority push is
delivered reliably enough on real devices to be worth having.
**Blocks:** any change to delivery behaviour. **Affects:** D-009, U-004.

### U-002 — Does independence gain non-geographic axes?
**Why it matters:** three nodes 6 km apart, one owner, one network, one power
grid currently count as three independent contributors. Geographic separation is
a proxy for independence, not independence itself.
**What would settle it:** a registry schema for owner / network / power, and a
trust model for self-declared values. **Affects:** D-004.

### U-003 — What evidence would justify changing the contributor quorum?
**Why it matters:** with a single-node network, CONFIRMED is unreachable at any
threshold short of quorum 1, so loosening buys no true warnings and only false
alarms. But the criterion for ever changing it is not written down.
**What would settle it:** a defined evidence standard over a population of
events (S4, §6). **Blocks:** any quorum change. **Must not** be resolved to
make a phase exit criterion pass.

### U-004 — Is the fixed alert radius correct?
**Why it matters:** one fixed radius serves all magnitudes, and if U-001
introduces a second tier, that tier may need a different radius.
**What would settle it:** owner decision, informed by Phase 4 measurement.

### U-005 — Is the chat feature live or abandoned?
**Why it matters:** `docs/CHAT_DESIGN.md` designs it; `docs/GAP_ANALYSIS.md`
calls it out of scope. Nothing marks which is current.
**What would settle it:** owner decision. Then one of the two documents gets a
historical banner.

### U-006 — Does the MQTT alert-channel contract stay, or go?
**Why it matters:** `contracts/mqtt/alert.schema.json` describes a channel the
server does not publish. Under D-001 `/contracts` is authoritative, so a
contract for unimplemented behaviour sits in the authoritative tier.
**What would settle it:** owner decision — keep it as an explicitly
forward-looking contract with a marker convention, or remove it.

### U-007 — Should reconciliation judge by the row's `algo_ver` or by current config?
**Why it matters:** reconciliation currently recomputes independence from
*current* configuration, deliberately — a live event should be judged by the
binary judging it. But an event that survives a restart across a configuration
boundary can therefore change its count.
**What would settle it:** owner decision on whether that is correct.
**Affects:** D-004, D-006.

### U-008 — When is the Phase 2 consensus path removed?
**Why it matters:** it is the current rollback path (S8), retained "for one
release". Which release is not defined, so it is retained indefinitely by
default. **What would settle it:** owner decision naming the release.

### U-009 — Are legacy decision IDs migrated to `D-nnn`?
**Why it matters:** legacy IDs (`D1`, `D10`, `A17`, `O4`, `§7.3`, …) are cited
throughout code comments. Renumbering makes those citations unfindable.
**Recommendation on record:** do not migrate; cross-reference instead.
**What would settle it:** owner decision.

### U-010 — How does a client know an alert has stopped being actionable?
**Formerly F-7** in `docs/TEMP_ANDROID_PHASE4_CHECKLIST.md` §1.4, recorded here
because that file is temporary and deleting it would delete the question.

**Why it matters:** the client currently *guesses* the answer, and its guess
disagrees with the server by an order of magnitude. The server closes an event
after `EVENT_RESOLVE_AFTER_MS` = **90 s**
(`docs/CURRENT_STATE.md` § Active configuration). The client treats an alert as
still alarm-worthy for `RECENT_WINDOW_MS` = **15 minutes**
(`android/.../domain/WsAlertMessage.kt:134`). Inside that ~13.5-minute gap a
late-delivered copy of an alert can sound the siren for an event the server has
already ended or withdrawn.

Reachability is not hypothetical. `android/.../domain/AlertDedup.kt:73-74` keys
on `"${type}:$eventId"`, deliberately, so an all-clear is never mistaken for a
duplicate of the alarm it clears. The consequence is that the two keys never
consult each other: a device offline for the alert and online for the resolve
holds no `EARTHQUAKE_ALERT:<id>` entry, so the late alert reads as news.

**What is already settled and constrains any answer:** an `event_id` that has
reached RESOLVED or CANCELLED never becomes live again —
`server/internal/event/state_test.go:55` `TestTerminalStatesHaveNoExit` asserts
terminal states have no exit transition, and D-003 makes downgrade illegal. So
an alert bearing an `event_id` the client has already seen stood down is
**necessarily** stale, never a still-live quake. Aftershocks receive new
`event_id`s and are unaffected. This removes one half of the apparent trade-off:
suppressing on a *matched, already-stood-down* `event_id` cannot silence a live
event.

**Two candidate answers, and they are not equivalent:**

1. *Client-side stand-down memory.* Record stood-down `event_id`s in
   `AlertDedup` and suppress later alerts for them. One file, no contract
   change. **But** `AlertDedup` is process-lifetime by design
   (`AlertDedup.kt:12-14`), and the case that produces a late alert — a device
   offline long enough to miss one — is also the case where the OS has most
   likely killed the process. The guard is absent exactly when it is needed.
   It also adds a second piece of safety-path state that must stay consistent
   with the first, and leaves the 90 s / 15 min disagreement in place.
2. *Server-declared validity on the wire.* The alert carries how long it is
   valid; the client honours the sender rather than inferring. This is the
   CAP v1.2 model (`expires`, OASIS; used by FEMA and NOAA/NWS, recommended by
   UNDRR), where the sender states validity and undertakes to update or cancel
   by that time. Survives process death, a cold install, and out-of-order
   delivery, because the message is self-describing. It also closes the root
   cause rather than the symptom: the client stops holding a second, private
   copy of a server policy.
   **Cost:** this is a contract change, not a defect fix. Under D-001 /
   ADR-0004 `contracts/fcm/alert_payload.json` and
   `contracts/openapi/openapi.yaml` change first, then server, then Android —
   three components. The current payload carries 15 `data` keys and **none**
   expresses validity; the concept exists server-side only, as
   `TerminalRetention` (`server/internal/config/config.go:137`, validated
   against maximum accepted trigger age at `config.go:349-356`).

**What would settle it:** an owner decision naming which of the two the system
uses, and — if (2) — whether validity is an absolute instant or a duration, and
what a client does when the field is absent (every pre-change frame). Absence
must not read as "expired", or one deployment lag silences every alert.

**Blocks:** checkpoint 1.4 of `docs/TEMP_ANDROID_PHASE4_CHECKLIST.md`, which has
nothing to implement against until this exists. **Affects:** delivery behaviour,
public contract. **Related:** U-001 (delivery tiers), U-004 (radius).

### U-011 — Do two simultaneous confirmed events alarm simultaneously, or does the newer replace the older?
**Formerly F-5** in `docs/TEMP_ANDROID_PHASE4_CHECKLIST.md` §1.4, recorded here
for the same reason as U-010.

**Why it matters:** the server handles concurrency correctly and the client
discards half of it. This is the failure mode that damaged the JMA system's
credibility in Japan: on 2011-03-11 separate aftershocks were read as one quake
and warnings were issued at the wrong severity, and in January 2018 two
simultaneous events ~400 km apart (M4.5 and M4.0) produced an overpredicted
warning through incorrect event association — JMA added the Integrated Particle
Filter specifically to separate concurrent events.

QuakeAlert does **not** have JMA's association defect. `sim_dual_event.sh`
(committed `0822d35`) drives two disjoint clusters and observes two distinct
`event_id`s with `event_open_gauge=2` and no merging: the server keeps
concurrent quakes apart. The gap is entirely on the client, at the last hop —
`NOTIFICATION_ID = 4301` is one constant
(`android/.../service/WarningNotifier.kt:38`) and `activeAlertDetails` is one
nullable field (`android/.../ui/warning/WarningViewModel.kt:131`), so a second
concurrent event overwrites the first rather than coexisting. The user loses the
first quake with no indication that it happened.

**Already fixed regardless of which way this is decided:** an all-clear for one
event no longer stands down another (F-1, commit `47c95cc`). That was a defect
under either reading, and it is not what this question is about.

**Neither answer is free.** Two simultaneous full-screen alarms compete for one
screen, one siren stream, and one user's attention, and the app cannot say which
to act on first. One alarm replacing the other is coherent but silently discards
a live warning. A third reading exists — alarm once, and represent the second
event without a second alarm — and is not obviously worse than either.

**Testable now; not blocked on hardware.** An earlier note in this project
claimed this needed ≥3 physical sensors. That was wrong: `sim_dual_event.sh`
reaches two concurrent CONFIRMED events on the local stack today. Only the
on-device half (whether the OEM notification stack renders two emergency
notifications as intended) needs a phone, and none of it needs a second ESP32.
So this is blocked on the decision, not on the fleet.

**What would settle it:** an owner decision naming the intended behaviour, plus
— if concurrency is kept — how many concurrent alarms are allowed before the
policy changes, since one screen bounds it in practice.

**Must not** be resolved by implementing whichever reading the current single
`activeAlertDetails` field happens to produce. That field is an accident of
implementation, not a decision. **Affects:** delivery behaviour, UI.
**Related:** U-010.

### U-012 — Does an alert reach a user who is actively using the phone?
Found on device 2026-08-31, during the first drill sweep against a physical
POCO F1 (Android 16 / API 36, PixelOS `BP3A.250905.014` — **not** a Xiaomi ROM,
so no OEM autostart policy is involved). Not a code defect against any existing
contract — the client requests the right thing and the OS declines it, for a
reason now traced to AOSP source — so it is a question about intended delivery
behaviour, which under `PROJECT_RULES.md` §9 is the owner's to answer.

**Observed (FACT).** With the app backgrounded and the device **unlocked and in
use**, SystemUI logged:

```
19:42:24.839 W/VisualInterruptionDecisionProvider:
FSI suppressed: no HUN or keyguard (key=0|id.web.quakealert.debug|4301|null|10309)
```

Notification 4301 was posted correctly and is visible in `dumpsys notification`
with `channel=quakealert_emergency_alerts`, `mImportance=4`, `pri=2`,
`category=alarm`, `vis=PUBLIC`, `flags=ONGOING_EVENT|HIGH_PRIORITY`, and a live
`fullscreenIntent`. Android suppressed the full-screen intent, and **no heads-up
appeared in its place**. `WarningActivity` never started (zero occurrences in
logcat). Net effect: nothing was visible to the user.

**Reproduced in both process states (FACT), which rules out process lifecycle as
a cause.** Same outcome with the process alive and backgrounded (drill
`test-305be0e3-…`), and with the process **dead** after a recent-apps swipe and
revived by FCM (drill `test-819e7d81-…`, `pidof` empty before / 21007 after).
In the second case FCM woke the app successfully and `WarningActivity` still never
started; `topResumedActivity` remained the launcher. The single determining
variable is the keyguard, not whether the app was running.

**Contrast, same build, same drill (FACT).** Device locked:

| Condition | Process revived | Alarm visible |
| --- | --- | --- |
| Locked, process alive | — | yes, `Displayed +467ms` |
| Locked, swiped away | yes | yes, `Displayed +960ms` |
| Unlocked, backgrounded | — | **no** |
| Unlocked, swiped away | yes | **no** |

The locked runs show the alarm screen appearing over the lock screen unaided
~1.8 s after dispatch, waking the device from Doze. So the hardest path works and
the failing path is the *easier* one.

**Why it matters, and why this ordering is backwards.** A user holding an
unlocked phone is disproportionately likely to be standing, walking, or inside a
building — the population an early-warning system exists to move. The current
behaviour warns the person whose phone is face-down on a table and stays silent
for the person holding it.

**What is already ruled out (FACT).** `POST_NOTIFICATIONS granted=true`.
`mZenMode=ZEN_MODE_OFF`. Channel importance is 4 (HIGH), not user-lowered
(`mUserLockedFields=0`). The app is Doze-exempt. FCM delivery works — the same
event's stand-down was received and logged. `AlertGate` did not reject it: the
notification was posted, which happens only after the gate agrees. Process state
is ruled out by the table above.

**ROOT CAUSE — heads-up is disabled device-wide (FACT, 2026-08-31).**

```
adb shell settings get global heads_up_notifications_enabled  →  0
```

`PeekDisabledSuppressor` (AOSP `CommonVisualInterruptionSuppressors.kt`) is a
device-wide `VisualInterruptionCondition`, not a per-notification filter:

```kotlin
class PeekDisabledSuppressor(...) :
    VisualInterruptionCondition(types = setOf(PEEK), reason = "peek disabled by global setting") {
    private var isEnabled = false
    override fun shouldSuppress(): Boolean = !isEnabled
    // isEnabled = globalSettings.getInt(HEADS_UP_NOTIFICATIONS_ENABLED, HEADS_UP_OFF) != HEADS_UP_OFF
}
```

`HEADS_UP_OFF` is `0`, so a stored `0` makes `isEnabled=false` and
`shouldSuppress()=true`. `makeLoggablePeekDecision()` evaluates
`checkConditions(PEEK)` **first**, before any filter, so heads-up is suppressed for
**every notification from every app on this device** — `couldHeadsUp=false`
unconditionally, which is exactly the input that drives
`FullScreenIntentDecisionProvider` to its terminal `NO_FSI_NO_HUN_OR_KEYGUARD`.

Corroborating FACT: across every logcat capture this session, `HeadsUp` and
`VisualInterruptionDecisionProvider` appear **0** times for any package other than
the suppression warning itself. No app on this phone produced a heads-up.

**This is a device setting, not a QuakeAlert defect.** The app's notification is
otherwise fully eligible: `mImportance=4` (passes `PeekNotImportantSuppressor`),
screen on and not dreaming (passes `PeekDeviceNotInUseSuppressor`),
`mZenMode=ZEN_MODE_OFF` (passes `PeekDndSuppressor`), and it carries a
`fullScreenIntent`, which `PeekOldWhenSuppressor` treats as inherently
time-sensitive and exempts from its `when`-age check regardless of timestamp:

```kotlin
entry.sbn.notification.fullScreenIntent != null || … -> false   // never suppressed
```

So the `setWhen()` / clock-skew line of enquiry is **ruled out** (FACT): with an
FSI attached, `when` age cannot suppress heads-up.

**Superseded explanation — retracted.** An earlier version of this entry claimed
the cause was CONFIRMED as the silent channel via
`HunSilentNotificationSuppressor`. That was asserted from the filter's *name* in
`VisualInterruptionDecisionProviderImpl.start()` without reading its body, and it
overstated the evidence. The body is gated:

```kotlin
override fun shouldSuppress(entry: NotificationEntry) =
    entry.sbn.let { Flags.notificationSilentFlag() && it.notification.isSilent }
```

The flag *is* on for this device (FACT:
`device_config` → `android.service.notification.notification_silent_flag=true`),
so this filter is live — but it is a **secondary** suppressor that could only
matter once the device-wide setting is re-enabled, and whether
`notification.isSilent` is true for QuakeAlert's notification is **UNVERIFIED**.
The silent-channel theory is therefore *possible but unproven*, and it is not the
observed cause.

**Remaining uncertainty (UNVERIFIED).**
- Whether `heads_up_notifications_enabled=0` is a PixelOS default, a user setting,
  or an artifact of this device's history. Not determinable from the value alone.
- What fraction of real users have it off. Unknown, and it decides how much of the
  in-use gap is device-specific versus universal.
- Whether QuakeAlert's notification would heads-up with the setting on — i.e.
  whether `HunSilentNotificationSuppressor` then blocks it. **This is the single
  experiment that separates "device-specific" from "app design issue" and it has
  not been run.**

**Smallest appropriate fix — RECOMMENDATION ONLY, not a decision.** Do not change
the channel or the architecture on this evidence. Re-run one drill with
`heads_up_notifications_enabled=1` and observe whether a heads-up appears. Only
that result justifies touching the channel, and only if heads-up is still
suppressed. If heads-up *does* appear, the correct scope shrinks to detecting the
setting and telling the user their device will not show the alert while in use —
which is a UI/diagnostics change, not a delivery-behaviour change.

**What the owner still decides (unchanged).** Even with the mechanism understood,
what an in-use device *should* receive remains a policy question:
 (b) heads-up notification plus siren — visible without seizing the screen;
 (c) in-app takeover when the app is foreground, heads-up when backgrounded.
Option (a) (full-screen alarm regardless of keyguard) was **rejected by the owner
2026-08-31**: it would hijack the screen of a user who may be driving or in a call.

**Precedent worth weighing.** Google's own Android Earthquake Alerts System ships
two tiers: *Be Aware* (weak/light shaking) respects volume, Do Not Disturb and
notification settings; *Take Action* (moderate/extreme) breaks through Do Not
Disturb, turns the screen on and plays a loud sound. The tiering is by shaking
severity, not by device state — which maps onto this project's existing
advisory/confirmed split (D-009) rather than cutting across it.

**Cost of the channel-sound remedy, if it ever becomes justified.** Making the
siren the *channel* sound moves control of the alert sound from the app to the OS:
the user could then silence it from Android's channel settings, and the app's own
mute button no longer owns that audio. Channel behaviour is immutable after
creation — Android's documentation states importance and other notification
behaviours cannot be changed once the channel is registered — so it means a **new
channel ID**, and migrating users to one is itself a delivery-behaviour change. A
single emergency channel remains the right shape either way: D-009 already keeps
advisories off push entirely, so everything reaching this channel is a CONFIRMED
event past the 200 km gate and deserves one urgency level, and a second channel
would only add another switch a user can turn off on a life-safety path.

**Forward risk (FACT about AOSP, INFERENCE about impact).** The same AOSP file
contains a flagged branch:

```kotlin
if (android.service.notification.Flags.notificationSilentFlag()) {
    if (sbn.notification.isSilent) return NO_FSI_SUPPRESSIVE_SILENT_NOTIFICATION
}
```

The flag is on for this device. If `notification.isSilent` is true for
QuakeAlert's alert, this would remove the full-screen intent **unconditionally,
including on the lock screen** — the one path that works today. Whether
`isSilent` is true here is **UNVERIFIED** and is the same unknown as above, which
makes it the highest-value thing to measure next.

**Affects:** delivery behaviour, Android client. **Related:** U-001 (which is the
same shape of gap one tier down: unconfirmed events reach a locked device not at
all), U-013.

### U-013 — Should the alert-raising path be observable in logs at all?
Found while diagnosing U-012 on 2026-08-31, and it is the reason that diagnosis
took three drills instead of one.

**Observed (FACT).** The first drill (app foreground) produced **no log line of
any kind** from the alert path. `WarningViewModel.raiseAlert()` and `raise()`
call no logger, and neither does the gated-out branch at
`android/.../ui/warning/WarningViewModel.kt:434-451`. `WarningNotifier.notify()`
logs only on the *failure* branch
(`full-screen intents not permitted; falling back to heads-up`) and is silent on
success.

**Consequence.** A successful alarm and a silently gated-out one are
**indistinguishable** in logcat. During this session that made it impossible to
tell "the alert was never processed" from "the alert was processed and shown"
without a second and third run under changed conditions. In the field it means a
user report of "my phone did not alarm" carries no evidence, on the one path
where evidence matters most.

By contrast the FCM path (`QuakeMessagingService.kt`) logs every rejection —
unrecognised payload, older than the recent window, already handled, outside
coverage — and those lines are what made U-012 diagnosable at all. The asymmetry
between the two paths is the defect, whichever way it is resolved.

**Why this is a question and not a fix:** what an alerting app writes to the
system log is a privacy surface. Logcat is readable by adb and, for some OEM
builds, by more than that. An alert line naturally wants to carry `event_id`,
coordinates, and distance-to-user — the last of which is device location, and
`UserLocationRepo` already deliberately redacts it (`sync(force=false) ->
Unchanged(position redacted)` was observed this session, so a redaction
convention exists in the codebase and should be followed rather than
contradicted).

**What would settle it:** an owner decision on whether the raise path logs, and
if so at what granularity — `event_id` and outcome only, or including the gate
decision and distance. A defensible default exists (log `event_id` + outcome,
never raw coordinates, reuse the existing redaction convention) but it is still a
decision, because it changes what a shipped build writes about a user.

**Affects:** Android client, diagnosability, privacy. **Related:** U-012.

---

## Superseded decisions

Superseded decisions are preserved, never deleted.

### Phase 2 consensus semantics — SUPERSEDED by D-002, D-003 on 2026-08-27
An 8-second correlation window, an `ACCUMULATING` / `ADVISORY_ISSUED` state
vocabulary, and a per-cluster cooldown. Replaced by the five-state lifecycle and
a longer correlation window. The original description survives in
`docs/SYSTEM_SPEC.md`, which is now historical. Code retained as the rollback
path (**U-008**).

### Grid-cell independence — SUPERSEDED by D-004 on 2026-08-27
Independence counted as the number of occupied grid cells at a configured cell
size. Rejected because the threshold depended on where cell boundaries fell
relative to the network: a reference pair separated by ~9.4 km fell into one
cell or two depending on a few hundred metres of grid alignment. Rows written
under the earlier `algo_ver` retain the earlier meaning (D-006).

### Fixed 3×3 candidate neighbourhood — SUPERSEDED by D-005 on 2026-08-27
A fixed cell neighbourhood, sufficient only within the latitude band it was
derived from, plus a boot-time error for nodes outside that band. Both removed:
the band was an artifact of one archipelago's coordinates, and the error would
now be a false warning — and a false startup warning is one an operator learns
to ignore before the day it is right.

### QoS 1 trigger delivery — SUPERSEDED by D-008 on 2026-08-27
Claimed at-least-once trigger delivery. The transport never provided it.
