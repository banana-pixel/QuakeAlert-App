# ROADMAP.md

> ## ACTIVE PHASE: **Phase 4 — Self-Measurement & Forensics** — status `PLANNED`
> Scope not yet approved. No phase is `IN_PROGRESS`. Implementing agents must
> not begin Phase 4 work until the owner marks it `IN_PROGRESS` here.

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
| 4 — Self-measurement & forensics | VALIDATION | `PLANNED` | 3.x | see below |
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

### Phase 4 — Self-measurement & forensics — `PLANNED`
**Concept only. Not approved. Requirements deliberately not invented here.**

Phase 4 measures **this system against itself**. It is a `VALIDATION` phase and
may legitimately produce no new features.

Intended concept:
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
- [ ] **P4-M2′ — Near-confirmation log is queryable and survives a restart.**
      With one node the correct answer is **empty**, and it must still be empty —
      and answerable — after the process restarts, rather than merely absent
      because the in-memory map was rebuilt.
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
- [ ] **P4-M4′ — Deterministic replay.** Replaying recorded observations grouped
      by `algo_ver` through a fresh tracker reproduces the same `event_id`,
      `revision`, and `independent_cell_count` decisions (V7).
- [ ] **P4-M5′ — Simulated multi-node runs in CI.** `sim_multi_node.sh` and
      `sim_dual_event.sh` pass in CI and archive their tracker counters and
      evidence snapshots. **This is software validation, never field
      validation** (S9) — it may not be cited as multi-node correlation.
- [ ] **P4-M6′ — Forensic timeline for one event.** A read-only path returns, for
      one `event_id`, the event row, its ordered `event_state_log` history, the
      `evidence_summary` per revision, and the contributing observations.

Explicitly **out of scope** for Phase 4:
- Any external catalogue as ground truth (`PROJECT_RULES.md` §2).
- Automatic false-positive classification.
- Automatic threshold calibration.
- Any change to detection thresholds, confirmation semantics, or delivery
  behaviour. Those require accepted decisions, not a phase.

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
