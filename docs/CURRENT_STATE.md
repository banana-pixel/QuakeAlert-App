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
| Near-confirmation observability log | yes | no | yes, private VPS | In-memory, resets on restart. |
| Admin tracker endpoints | yes | no | yes, private VPS | Behind the admin API key. |
| WebSocket delivery | yes | partial | yes, private VPS | Advisory frames observed; alert frames not observed in production. |
| Push delivery | yes | no | yes, private VPS | Never triggered in production; one device vendor only in testing. |
| Firmware detection | yes | partial | one node | One board, one location, one firmware build. |
| Android client | yes | partial | sideloaded, not published | Advisory-never-wakes enforced in three independent places. |

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

## NOT demonstrated

Read this list as authoritative. **Absence from it is not proof of validation;
only presence in *Demonstrated* is** (`PROJECT_RULES.md` §8).

- **No CONFIRMED event has ever occurred in production.** With one node, the
  gate is unreachable — this is a network-density fact, not a gate defect (S2).
- **No multi-node correlation in the field.** All multi-node behaviour is proven
  by test only.
- **No measured false-positive rate.** No population of events exists.
- **No measured false-negative rate.**
- **No measured lead time.** No end-to-end warning has preceded shaking for any
  real user. Do not claim EEW lead time.
- **Push delivery not verified across device vendors.** One vendor, in testing.
- **Firmware not validated beyond one board in one location.**
- **Reconciliation across a real restart during a real event** — tested, not
  observed in production.
- **Behaviour at high latitude and across the antimeridian** — correct by proof,
  never exercised by real sensors.

---

## Known open defects

- `docs/SYSTEM_SPEC.md` and `docs/GAP_ANALYSIS.md` describe superseded Phase 2
  architecture. Both now carry historical banners. Not rewritten.
- `contracts/mqtt/alert.schema.json` documents a channel the server does not
  publish (**U-006**).
- `docs/CHAT_DESIGN.md` versus `docs/GAP_ANALYSIS.md` disagree on whether chat
  is in scope (**U-005**).

## Maintenance

Update this file when the baseline commit changes, when a status column changes,
or when an item moves between *Demonstrated* and *NOT demonstrated*. Do not
record decisions here — those belong in `docs/DECISIONS.md`.
