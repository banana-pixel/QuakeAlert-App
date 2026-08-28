# PROJECT_RULES.md — Permanent Rules

QuakeAlert is a **global, independent, crowdsourced earthquake early-warning
system**. Life-safety software. Monorepo: `android/`, `server/`, `firmware/`,
`contracts/`, `deploy/`, `docs/`.

This file holds **permanent principles only**. It contains no thresholds, no
configuration values, and no current implementation details. If a rule here
would need editing because a number changed, the rule is in the wrong file —
current values belong in `docs/CURRENT_STATE.md`, current choices in
`docs/DECISIONS.md`.

Every rule below is classified:
`[PERMANENT PRINCIPLE]` — architectural, expected never to change.
`[PROCESS RULE]` — how work is done; may be refined.

---

## 1. Mission [PERMANENT PRINCIPLE]

Warn as early as practically possible before strong shaking, or immediately
after local shaking when earlier is impossible. Optimize practical life-safety
value, not classification accuracy.

Low-cost MEMS sensing (ESP32 + accelerometer) is a first-class sensing
platform. Its limits are addressed by engineering — network density, honest
confidence semantics, honest latency claims — not by dismissing the hardware.

## 2. Independence from external agencies [PERMANENT PRINCIPLE]

The real-time path never depends on any government agency, seismic institution,
or external catalogue. BMKG, USGS, EMSC and equivalents are **never ground
truth** and are **never required dependencies**.

External catalogues may be used as **optional, post-event reference only**.
The real-time warning path must remain fully operational when every external
system is unreachable.

## 3. Separation of concerns [PERMANENT PRINCIPLE]

Four distinct concerns. Never conflated. A change must state which it touches:

1. **Detection** — does a sensor observe shaking.
2. **Confirmation** — does the server correctly believe an event is real.
3. **Delivery** — does the warning reach a person.
4. **Post-event validation** — was it in fact an earthquake.

## 4. Canonical units and vocabulary [PERMANENT PRINCIPLE]

PGA = **gal** (cm/s²). Timestamps = **ms epoch UTC** (`int64`). Distance = **km**.
RSSI = **dBm**. Duration = **ms**. Conversion to `g` is for display only.

A computed cluster centre is an `estimated_centroid`, never an `epicenter`.
Public contract vocabulary is not renamed silently.

## 5. Authority hierarchy [PERMANENT PRINCIPLE]

```
/contracts                      (OpenAPI, MQTT schemas, FCM payload, DDL)
  > PROJECT_RULES.md            (this file)
  > docs/DECISIONS.md           (accepted decisions)
  > docs/CURRENT_STATE.md       (verified current state)
  > ROADMAP.md                  (phase status and scope)
  > docs/*.md prose             (design documents, may be historical)
  > .hermes/plans/*             (HISTORICAL planning artifacts — never authoritative)
```

When two sources disagree, the higher wins and the lower is a **defect to be
reported**, not silently reconciled. See ADR-0004.

## 6. Evidence requirements [PERMANENT PRINCIPLE]

Safety-critical thresholds change only on reproducible evidence over a
**population** of events. A single event never justifies a threshold change.
A threshold is never changed in order to make a phase's exit criterion pass.

## 7. Global by default [PERMANENT PRINCIPLE]

Geometry, time, and language assumptions are global. No country-specific
constant may enter a global component without an accepted decision recorded in
`docs/DECISIONS.md`.

Known failure classes, permanently prohibited: fixed latitude bands, fixed
cell neighbourhoods whose sufficiency depends on latitude, and longitude
arithmetic that does not wrap at the antimeridian.

## 8. Honesty [PERMANENT PRINCIPLE]

- Never claim a capability that has not been demonstrated.
- **IMPLEMENTED ≠ VALIDATED ≠ RELEASED.** These are tracked separately.
- **"Not yet proven" is not "not implemented."** `docs/CURRENT_STATE.md` carries
  an explicit *NOT demonstrated* list for exactly this reason.
- Naming must match behaviour. A schema claiming a reliability property the
  code does not provide is a defect in the schema.
- Network density cannot be replaced by threshold tuning, and tuning must never
  be presented as if it could.

## 9. AI-agent scope control [PROCESS RULE]

An agent may implement only work **explicitly within the active phase**
(`ROADMAP.md`, first line) or explicitly approved as its prerequisite.

- Useful work discovered outside that scope is reported as a **PROPOSAL** —
  what, why, cost, risk — and is not implemented.
- Before modifying code, state which invariant the change protects.
- After implementing, verify no unrelated scope entered, and report it
  explicitly: `UNRELATED CHANGES: NONE` or the list.
- Never resolve an item from `docs/DECISIONS.md` § Unresolved by implementing
  one reading of it.
- Never mark work VALIDATED that you implemented.
- Label claims: REQUIREMENT / FACT (with `file:line`) / ASSUMPTION / PROPOSAL /
  UNRESOLVED. An unlabelled claim about system behaviour is a defect.

Stop and ask for a decision when an unresolved question affects safety, alert
behaviour, data semantics, or a public contract. Do not pick a reading and
proceed.

This rule bounds architecture and safety. It does not micromanage coding style.
Small, reversible, local changes inside the active phase proceed without
ceremony.

## 10. Safety rules (EWS) [PERMANENT PRINCIPLE unless noted]

**S1 — Nothing on the decision path blocks on I/O.** Persistence, logging, and
metrics follow emission and are permitted to fail. A database or metrics outage
must never suppress, delay, or downgrade a warning. Exception requires an
explicitly approved safety invariant recorded in `docs/DECISIONS.md`.

**S2 — Latency honesty.** Lead time available to a user is bounded by travel
time from the nearest triggering node to that user. A sparse network yields
near-zero lead time regardless of any threshold. Never present threshold tuning
as a substitute for network density.

**S3 — Monotonic public claims.** A published confidence claim never silently
decreases. Retraction is explicit and separate, never a downgrade. People act
physically on alerts.

**S4 — False positives.** A discredited alarm channel fails every future event;
a late warning fails one. Alarm-grade delivery therefore requires alarm-grade
evidence, and confidence gates are never loosened merely to raise the
confirmation rate.

**S5 — False negatives.** Equally dangerous and harder to observe: a missed
warning leaves no artifact. Pre-confirmation and near-confirmation records
therefore exist as the denominator and must not be pruned for tidiness.

**S6 — Review never gates.** False-positive assessment is **manual and
post-event**. No manual step, human review, or external lookup may sit on the
real-time path. Real-time and post-event tooling are separate code paths, and
the real-time path must remain operational when post-event tooling is absent.

**S7 — External data.** No external service enters the real-time path without
an accepted decision. Post-event reference use must be *structurally* incapable
of affecting real-time behaviour, verified by test.

**S8 — Rollback** [PROCESS RULE]. Every deployed change must be revertible
without data loss. A change that cannot be rolled back must say so before it
lands.

**S9 — Field validation** [PROCESS RULE]. No safety claim reaches VALIDATED
without evidence from running the system. Unit tests prove IMPLEMENTED only.

## 11. Versioning and reproducibility [PERMANENT PRINCIPLE unless noted]

**V1 — A decision is interpretable only together with the parameters that
produced it.** Every persisted decision carries an algorithm version
(`algo_ver`).

**V2 — The version base is a compile-time constant** [CURRENT DECISION], not an
environment variable: it must follow the binary that actually made the decision,
and an operator able to change it at runtime could mislabel past decisions.

**V3 — Bump the version when the *meaning* of any persisted field changes**,
even if its name and type do not.

**V4 — Never rewrite historical rows to match new semantics.** That falsifies
decisions actually taken. A version bump replaces the migration.

**V5 — Analysis must account for `algo_ver`** [PROCESS RULE]. Any analysis
aggregating decision rows groups by `algo_ver`, or states why comparison across
versions is valid.

**V6 — Parameters that change a decision's meaning appear in the version
label.** Parameters affecting only performance or cost do not.

**V7 — Spatial-semantics and confirmation-semantics changes both require a
version bump.** Replay must be reproducible: the same observations under the
same `algo_ver` yield the same decisions.

## 12. Amending this file [PROCESS RULE]

Changes require owner approval and a corresponding entry in
`docs/DECISIONS.md`. Do not add current thresholds, current parameter values,
or transient implementation details here.
