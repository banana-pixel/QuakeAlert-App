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
