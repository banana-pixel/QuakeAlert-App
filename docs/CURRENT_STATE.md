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
evidence in *Demonstrated*; committed, **not deployed**. P4-M1′, P4-M4′, P4-M5′ and
P4-M6′ are not met.

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

## Maintenance

Update this file when the baseline commit changes, when a status column changes,
or when an item moves between *Demonstrated* and *NOT demonstrated*. Do not
record decisions here — those belong in `docs/DECISIONS.md`.
