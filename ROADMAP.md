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

---

## Superseded and abandoned phase definitions

- **Phase 4 as "BMKG catalogue comparison / automatic false-alarm-rate
  calibration"** — `ABANDONED`. No successor carries this content. It conflicts
  with `PROJECT_RULES.md` §2. It still appears in
  `.hermes/plans/2026-08-27_quakealert-final-architecture-plan.md`, which is a
  historical artifact and not authoritative.
- **Phase 5 calibration** — folded into the abandoned definition above. Not a
  current phase.
