# TEMP — Android compatibility & Phase 4 validation checklist

**TEMPORARY EXECUTION CHECKLIST. NOT a specification.** Delete when the work it
tracks is done. It defines nothing: every requirement here already exists in
`/contracts`, `PROJECT_RULES.md`, `docs/DECISIONS.md` (accepted), or is a finding
of the Android compatibility audit dated 2026-08-28. Authority order is
`PROJECT_RULES.md` §5 — this file sits below all of it and cannot outrank
anything.

Baseline: branch `phase-1-observation-ledger`, HEAD `9752c5e`.

## Scope warnings (read before executing anything)

- **`ROADMAP.md` ACTIVE PHASE is `Phase 4 — PLANNED`, and no phase is
  `IN_PROGRESS`.** Phase 4 work must not begin until the owner marks it
  `IN_PROGRESS`. Checkpoints 3 and 4 below are therefore **not executable yet**;
  they are written down so the sequence is known, not so it can be started.
- **Phase F (field validation) is `BLOCKED`** on the owner deploying more nodes.
  Not clearable by code.
- Checkpoint 1 items are Android-client defect fixes against contracts that
  already exist. They are not Phase 4 scope and do not need Phase 4 to open.
- `IMPLEMENTED → VALIDATED` cannot be granted by whoever implemented it
  (`ROADMAP.md`, `PROJECT_RULES.md` §8). Nothing in Checkpoint 2 may be ticked by
  the agent that wrote the code it exercises.
- No item here changes a threshold, the confirmation policy, or delivery
  behaviour. If executing an item appears to require that, it is out of scope —
  stop and report.

## Legend

| Tag | Meaning |
| --- | --- |
| **REQUIRED FIX** | A defect against an existing contract or an accepted decision. Fixable now. |
| **VALIDATION** | Produces evidence. Produces no feature. Cannot be self-certified. |
| **OPEN DESIGN DECISION** | Needs an owner decision recorded in `docs/DECISIONS.md` FIRST. Must not be resolved by implementing one reading. |
| **FUTURE WORK** | Known, deliberately not scheduled. Not a requirement. |


---

## Checkpoint 1 — Android compatibility fixes

Executable now. All three are defects against contracts that already exist.

### 1.1 — F-1 `event_id`-aware stand-down — **REQUIRED FIX** (P1)

`EVENT_RESOLVED` is acted on without comparing `event_id` at three sites:

- `android/.../service/QuakeMessagingService.kt:82-90` — clears notification 4301
- `android/.../device/BackgroundAlertBridge.kt:78-93` — clears notification 4301
- `android/.../ui/warning/WarningViewModel.kt:403` — releases siren, stops torch,
  clears notification, nulls `activeAlertDetails`

The correct comparison already exists at
`android/.../ui/warning/WarningActivity.kt:118-120` and is the pattern to match
(blank `event_id` still resolves, which is the pre-Phase-3 fallback).

- [x] Compare `event_id` at all three sites — **PASS** (2026-08-29). `QuakeMessagingService.kt:83` `WarningNotifier.clear(applicationContext, message.eventId)`; `BackgroundAlertBridge.kt:84` same with `appContext`; `WarningViewModel.kt:403` `standDown(message.eventId, message.eventState)`.
- [x] Unit test: an all-clear for event B leaves event A's alarm state intact — **PASS**. `WarningNotifierStandDownTest`: `stand-down for different event does NOT clear`, `notify A then clear B then notify B then clear A`. 7/7 green.
- [x] Unit test: a blank `event_id` still stands down (pre-Phase-3 behaviour kept) — **PASS**. Three cases: blank standDownId, blank activeId, both blank.

Invariant protected: an all-clear for one event may not stand down another.
Reachability is not the justification — `internal/event/tracker.go` already holds
concurrent events and `internal/event/emit.go` emits one frame per transition per
event id, so this is server-reachable the moment a multi-node fleet exists.

### 1.2 — F-3 REST `EventDto` lifecycle fields — **REQUIRED FIX** (P1)

`server/internal/api/api.go:1079-1084` publishes and
`contracts/openapi/openapi.yaml` documents five fields on `EarthquakeEvent`:
`event_state`, `event_revision`, `origin_ts`, `origin_ts_source`,
`independent_cell_count`. `android/.../data/network/model/EventDto.kt:29-40`
declares none, and `ignoreUnknownKeys = true`
(`android/.../data/network/HttpCalls.kt:31-32`) drops them silently.

- [x] **PASS** — Declare the five fields on `EventDto`, all optional — absent means "server
      did not record it", never a named state and never revision zero
- [x] Unit test: absence stays absent (a pre-Phase-3 row must not read as CONFIRMED) — **PASS**. `MappersTest` 32/32 green; `eventState` null on legacy row, unknown state reads null.
- [x] Unit test: `event_state = CANCELLED` survives into the domain model — **PASS**. `MappersTest:225-232` asserts `EventState.CANCELLED`; `:441-443` asserts `CONFIRMED` on the REST path.

Consequence being fixed, stated exactly: `server/internal/event/store.go:56-61`
projects every terminal state onto `status = 'RESOLVED'`, so on cold start a
withdrawn report is currently indistinguishable from an ended earthquake.

**Note — F-4 is downstream of this, not a separate fix.** Whether History then
*displays* the withdrawn/ended distinction, and in what words, is presentation
scope. Declaring the field does not decide it.

### 1.3 — F-6 FCM `is_test` contract drift — **REQUIRED FIX** (P2), contract first

`contracts/fcm/alert_payload.json` declares `data` with
`additionalProperties: false` and does not list `is_test`.
`server/internal/dispatch/fcm.go` `BuildAlertData` emits `is_test: "true"` for a
drill. The server's own drill payload fails its own schema. Android already reads
it (`android/.../mapper/FcmAlertMapper.kt:53`, exact string `"true"` only).

Per D-001 / ADR-0004 the contract changes first, then nothing else needs to.

- [x] Add `is_test` to the FCM payload contract as optional, present only when true — **PASS**. `contracts/fcm/alert_payload.json:118` under `/properties/message/properties/data`, `enum: ["true"]`, not in `required`, sibling `additionalProperties: false` now satisfied.
- [x] Confirm no server or Android code change is required by the addition — **PASS**. `FcmAlertMapperTest` 15/15 green against the unchanged mapper.

### 1.4 — F-7 delayed / stale alert re-raise — **OPEN DESIGN DECISION** (P2)

`android/.../domain/AlertDedup.kt` keys on `"${type}:$eventId"` deliberately, so
an all-clear is never mistaken for a duplicate of the alarm it clears. The
consequence is that the two keys never consult each other: a device offline for
the alert and online for the resolve holds no `EARTHQUAKE_ALERT:<id>` entry, so a
late FCM copy of the original alert can raise the siren for an event the server
already withdrew. The only guard is the 15-minute `isRecent` window
(`android/.../domain/WsAlertMessage.kt:134`) against a server `resolveAfter` of
90 s.

This is **not** ticked off by implementing one reading. Suppressing a late alert
because a stand-down was seen is a change to alert behaviour, and
`.clinerules/00-project-overview.md` requires stopping rather than choosing.

- [ ] Owner decision recorded in `docs/DECISIONS.md` before any code — **RECORDED as U-010** (2026-08-31). The question is now `docs/DECISIONS.md` **U-010**, with both candidate answers, the terminal-state invariant that constrains them, and the CAP v1.2 precedent. Still UNRESOLVED — no owner decision yet, so still nothing to implement against.
- [ ] **BLOCKED** — Decision names which failure it prefers: a siren for a withdrawn event, or
      a suppressed siren for a still-live one. **Note (U-010):** `TestTerminalStatesHaveNoExit`
      (`server/internal/event/state_test.go:55`) shows the second failure is not reachable for a
      *matched* `event_id` — a stood-down id never becomes live again. The real trade-off is
      client-side memory (loses its guard on process death) versus server-declared validity on
      the wire (contract change across three components).

Related but distinct: **F-5 multi-event behaviour** — `activeAlertDetails` is one
nullable field (`WarningViewModel.kt:131`) and `NOTIFICATION_ID = 4301` is one
constant, so a second concurrent event overwrites the first rather than
coexisting. Also an **OPEN DESIGN DECISION**: whether two simultaneous alarms
should coexist at all is a delivery-behaviour question. 1.1 must be fixed
regardless — it is a defect under either decision.

---

## Checkpoint 2 — Physical-device validation

Produces evidence only. Nothing here is cleared by a unit test, and nothing here
may be self-certified — `PROJECT_RULES.md` §8: unit tests grant IMPLEMENTED, field
observation grants VALIDATED, and not by the agent that implemented it.

### 2.1 — F-2 full-screen alarm on Android 14+ — **VALIDATION** (P1)

`android/.../service/WarningNotifier.kt:170` gates the full-screen intent on
`canUseFullScreenIntent()`. That permission is pre-granted only to apps declaring
a calling or alarm-clock category; QuakeAlert declares neither, and the current
degradation path is a `Log.w` and a heads-up notification
(`WarningNotifier.kt` full-screen branch). Whether a real locked device shows the
alarm is therefore unknown, not assumed.

- [x] Locked screen, app killed, Android 14+ physical device: record whether the
      full-screen alarm appears, or only heads-up — **PASS** (2026-08-31). Locked
      (`isKeyguardShowing=true`, `mWakefulness=Dozing`), app backgrounded, drill
      `test-46185c29-7eeb-42af-b252-c9265ae13c56` via
      `POST /api/v1/admin/test-alert` against production. Logcat:
      `19:59:16.808 START … WarningActivity BAL_ALLOW_NON_APP_VISIBLE_WINDOW`,
      `19:59:17.253 Displayed … WarningActivity for user 0: +467ms`,
      `19:59:33.632 Transition CLOSE` on the drill's own all-clear. Full-screen
      alarm appeared **over** the lock screen with no user action ~1.8 s after
      dispatch; device woke from Doze. Owner confirmed visually.
- [x] Record the value `canUseFullScreenIntent()` actually returns on that device —
      **PASS, and the earlier prediction was WRONG** (2026-08-31). The appop is still
      `USE_FULL_SCREEN_INTENT: default` with a fresh `rejectTime`, and the predicted
      `full-screen intents not permitted; falling back to heads-up` line **never
      appeared** — while the full-screen alarm demonstrably launched (above). So the
      op state does not decide this: SystemUI grants the FSI for a
      `category=alarm` notification on a showing keyguard. The real gate is the
      keyguard, not the permission — see the unlocked-device case, recorded as
      **U-012** in `docs/DECISIONS.md`:
      `W/VisualInterruptionDecisionProvider: FSI suppressed: no HUN or keyguard`.
- [ ] Repeat on Android 13 or lower for the contrast — **BLOCKED**. Only one device available (API 36).
- [x] Record OEM and OS build — **PASS** (2026-08-29). Xiaomi POCO F1 (`beryllium`,
      `custom_beryllium`), Android 16, API 36, serial `8553bc38`. Package
      `id.web.quakealert.debug` uid 10309, `versionName=1.0-debug`, `minSdk=28`,
      `targetSdk=36`, DEBUGGABLE. Doze-exempt (`deviceidle whitelist`), standby bucket 5.
      `POST_NOTIFICATIONS granted=true` (USER_SET); channel `quakealert_emergency_alerts`
      live at `mImportance=4`, `mSound=null`, `mUserLockedFields=0`.

If the alarm does not appear, the remedy (declaring a category, or an in-app
permission prompt) is **not** part of this checkpoint. Record and report.

### 2.2 — Notification permission and background paths — **VALIDATION**

**UNBLOCKED and largely PASSED 2026-08-31.** The 2026-08-30 blocker below is
resolved: the owner authorised reading `ADMIN_API_KEY` from `deploy/.env.prod` on
the VPS, and three drills were injected against production
(`POST /api/v1/admin/test-alert`, `--pga 300`, Bandung centroid). The key was held
in an environment variable and a mode-600 temp file, never printed. Server side
confirmed each dispatch: `peringatan LATIHAN didispatch topic=test_alerts`, then
`all-clear LATIHAN didispatch` 20 s later.

Device: POCO F1, Android 16, API 36, `id.web.quakealert.debug` `1.0-debug`,
connected over wireless adb. `POST_NOTIFICATIONS granted=true`,
`ACCESS_FINE_LOCATION granted=true`, Doze-exempt, `mZenMode=ZEN_MODE_OFF`.

Note for anyone repeating this: the phone's clock ran **~1.5 s behind** the
laptop's (measured three times), so cross-device latency figures below are
approximate to that margin. `deploy/scripts/test-alert.sh` also defaults
`API_BASE` to `https://api.quakealert.id`, which is the **wrong** domain — pass
`API_BASE=https://api.quakealert.web.id` explicitly. That default is a known
open defect, unfixed here.

**Historical blocker (2026-08-30), retained:** the installed APK's only server is
production; `QUAKE_BASE_URL` is a compile-time `buildConfigField`
(`android/app/build.gradle.kts:106-108`), so retargeting it at a local sim stack
needs a rebuild. Both production injection paths sit behind `AdminKeyMiddleware`
(`server/internal/api/router.go:74-88`) — verified again 2026-08-31: the guarded
paths return **401** unauthenticated and **200** with the key.

- [x] Foreground: CONFIRMED alert arrives over WebSocket while the app is visible;
      siren + WarningActivity — **PARTIAL PASS, and it exposed U-013**
      (2026-08-31). Drill `test-9b22ca79-…` injected with the app foreground.
      The alert was delivered (`websocket connected` beforehand; server dispatch
      confirmed) and the stand-down arrived on schedule
      (`19:32:38.477 QuakeMessaging: push stand-down test-9b22ca79-…: null`,
      exactly 20 s after inject). But the raise path emitted **no log line at
      all**, and the owner reported seeing nothing on screen. Whether the alert
      was shown and unobserved, or gated out silently, **cannot be determined
      from this run** — that indistinguishability is now **U-013** in
      `docs/DECISIONS.md`. Re-run this row once U-013 is resolved.
- [ ] Permission denied: confirm the WebSocket foreground path still alarms —
      **NOT RUN** (2026-08-31). Requires revoking `POST_NOTIFICATIONS`; deferred
      rather than blocked, since an injection path now exists.
- [x] Doze / background: confirm an FCM CONFIRMED alert wakes the device —
      **PASS** (2026-08-31). Drill `test-46185c29-…` with the device locked and
      `mWakefulness=Dozing`: device woke, `WarningActivity` displayed in +467ms.
      See 2.1 for the full log extract.
- [x] Background (process alive), device **unlocked** — **FAIL, recorded as
      U-012** (2026-08-31). Drill `test-305be0e3-…`, app backgrounded, screen on
      and unlocked. Notification 4301 posted correctly
      (`importance=4 pri=2 category=alarm vis=PUBLIC`,
      `flags=ONGOING_EVENT|HIGH_PRIORITY`, live `fullscreenIntent`) but SystemUI
      logged `FSI suppressed: no HUN or keyguard` and **no heads-up replaced it**;
      `WarningActivity` never started. Nothing was visible to the user. Root cause
      traced 2026-08-31 to a **device setting**:
      `settings get global heads_up_notifications_enabled` → `0`, which makes AOSP's
      `PeekDisabledSuppressor` suppress heads-up for every app on the phone. Not an
      app defect on this evidence — see U-012, including the retraction of the
      earlier silent-channel explanation and the one experiment still outstanding
      (re-run with the setting enabled).
- [x] Process death (recent-apps **swipe**), locked — **PASS** (2026-08-31). Drill
      `test-b04e344a-…`: `pidof` empty before, 20670 after, so FCM revived the
      process; `Displayed WarningActivity +960ms` (roughly double the +467ms of the
      warm case, consistent with cold process start). `AlertDedup` is
      process-lifetime by design and correctly did not suppress the frame.
      **Method note:** the screen was locked via `input keyevent 26` before
      dispatch, which mixed two variables in one run — process state and keyguard.
      Recorded so the next run does not repeat it.
- [x] Process death (swipe), device **unlocked** — **FAIL, same U-012 cause**
      (2026-08-31). Drill `test-819e7d81-…`, no keyevent used: `pidof` empty before,
      21007 after — the process *was* revived by FCM — yet
      `FSI suppressed: no HUN or keyguard` again and `topResumedActivity` stayed on
      the launcher. Taken together with the row above, this isolates the keyguard as
      the single determining variable and rules process lifecycle out of U-012.
- [x] Process death (`am force-stop`), locked — **NOT A DEFECT; expected Android
      behaviour** (2026-08-31). Drill `test-244fa4dd-…` after
      `am force-stop id.web.quakealert.debug`: GCM logged
      `broadcast intent callback: result=CANCELLED forIntent { act=…c2dm.intent.RECEIVE
      pkg=id.web.quakealert.debug }` for both the alert and its all-clear, the process
      was never started, and nothing was posted. A force-stopped app is in the stopped
      state and Android deliberately withholds broadcasts until the user launches it
      again; Firebase documents that FCM cannot deliver to a force-stopped app. No
      application can opt out of this.
      **Correction:** an intermediate reading of this run as an app defect, and the
      suspicion of a MIUI autostart policy, were both wrong — the device runs PixelOS
      `BP3A.250905.014` (`ro.miui.ui.version.name` empty), so no OEM policy is
      involved. `docs/TEMP_DEVICE_VALIDATION_PLAN.md` T-9 already said to run swipe
      and force-stop separately; only the swipe result speaks to real-user behaviour.
      Also note a stale notification 4301 (`when=…144605`, i.e. 19:42:24) was still
      posted from an earlier drill whose all-clear arrived after the app was killed —
      an `ONGOING_EVENT` notification outlives its own process, which is worth knowing
      but is not this row's finding.
- [ ] Cold start during a live event: confirm `fetchWarning()`
      (`WarningViewModel.kt:251`) restores the banner from REST — **BLOCKED**
      (2026-08-31). Still needs a *real* event on production: a drill writes no
      `earthquake_events` row. Production `event_created_total` is now **20** with
      `event_transitions_to_confirmed_total=0`, so no confirmed event exists to
      restore.

**Also PASSED this session, and worth recording separately:**

- [x] **Cross-channel de-duplication on a real device (T-12).** With the app
      backgrounded, logcat shows exactly one line for the alert:
      `19:42:25.866 QuakeMessaging: push alert test-305be0e3-… already handled; dropped`.
      The WebSocket frame won and the FCM copy was suppressed by `AlertDedup` —
      one earthquake, one alarm, across two independent transports, proven on
      hardware rather than by unit test.
- [x] **All-clear takes the notification down (T-14).** After the drill's 20 s
      all-clear, notification 4301 count in the **live** list is `0`.
      *Correction to an intermediate claim made during this session:* an earlier
      count of "2 still posted" was wrong — it was reading `mArchive`
      (`dumpsys notification` keeps the last 100 notifications there), not the
      live list. `WarningNotifier.clear()` works as documented.
      Both stand-down paths logged: `BackgroundAlertBridge: background stand-down
      test-305be0e3-…: All Clear` then `QuakeMessaging: push stand-down …`.

**Device connectivity established** (2026-08-29): app launched, `QuakeWebSocket:
websocket connected`, `PushRegistrar: debug build subscribed to test_alerts`. The
installed APK's only server is production `https://api.quakealert.web.id/` — no
`10.0.2.2` or `localhost` string exists anywhere in its 19 dex files, and
`QUAKE_BASE_URL` is a compile-time `buildConfigField`
(`android/app/build.gradle.kts:106-108`), so the target cannot be changed without a
rebuild and reinstall.

### 2.3 — Stand-down on a real device — **VALIDATION**

- [ ] CONFIRMED then RESOLVED: alarm stops, notification 4301 goes away
- [ ] After 1.2 lands, a CANCELLED event shows "Report Withdrawn", not "All Clear"
      (`android/.../domain/AlertLifecycleCopy.kt:36-49`)

Both depend on the server actually reaching CONFIRMED, which needs checkpoint 3.
With one node, quorum 3 is unreachable — so 2.3 is blocked on 3, and only the
drill/test path can exercise it before then.

---

## Checkpoint 3 — Multi-node simulation

> **STOPPED HERE 2026-08-29 — no injection path against the target server.**
>
> The device's app talks only to production. Driving a CONFIRMED event on production
> needs `POST /api/v1/admin/*`, which returns 401 without `ADMIN_API_KEY`; that key is
> not in the environment and no `.env.prod` exists in the checkout. Admin routes are
> registered on production (401 on the guarded paths, 405 on `POST`-only `test-alert`
> — an unregistered route would 404), so the endpoints exist and only the key is
> missing.
>
> `server/scripts/sim_multi_node.sh` cannot be pointed at production by design: it
> brings up its own compose project (`-p sim31`), runs a server binary from source,
> and reads Postgres directly (`sim_multi_node.sh:25-28,94,109,120,139-152`). Its
> header states "Never connects to the production VPS."
>
> The local staging stack cannot serve the app either: `quakealert-server` publishes
> no host port, `quakealert-caddy` answers only `localhost` / `api.staging.localhost`
> (`deploy/caddy/Caddyfile.staging`), its `ADMIN_API_KEY` is empty (so its admin
> routes are not even registered, per `server/internal/api/router.go:73`), and its
> `FCM_PROJECT_ID` is empty — startup logged "FCM tidak dikonfigurasi — delivery
> background nonaktif (hanya WebSocket)".
>
> **Also observed, and reportable independently of the above:** production
> `GET /api/v1/events` returns 6 events, none carrying any of the five Phase 3
> fields. See the DEFECT note below.

Prerequisite for almost everything above. `docs/CURRENT_STATE.md` records the
fleet as one physical node, and `MinIndependentCells = 2` with quorum 3 means
CONFIRMED is unreachable on hardware today. Phase F is `BLOCKED` in
`ROADMAP.md` on the owner deploying more nodes — simulation is what unblocks the
Android-side checks without waiting on hardware.

### 3.1 — Reach CONFIRMED with simulated nodes — **VALIDATION**

- [x] Drive three simulated nodes in two independence cells into
      `event.Tracker` — **PASS** (2026-08-29). `server/scripts/sim_multi_node.sh`
      against the local dev stack: 9 PASS, 0 FAIL. 1 event created
      (`event_created_total` delta=1), `event_transitions_to_unconfirmed_total=1`,
      CONFIRMED delta=1, `independent_count_at_peak=3` (>= 2 required), no extra
      events, no tombstone evictions, `event_id cbb04fee-4e23-47be-a7a0-a875a9c7764b`.
- [x] Confirm `EVENT_TRACKER_ENABLED` is on — **PASS**. Sim harness starts the
      server with `EVENT_TRACKER_ENABLED=true`. Production also verified `=true`
      by `docker inspect` (see the DEFECT section below).
- [ ] Capture the WebSocket frames: expect advisory (UNCONFIRMED) then alert
      (CONFIRMED), one `event_id`, `event_revision` increasing — **BLOCKED**
      (2026-08-29). `sim_multi_node.sh` asserts via `/admin/tracker` counters and
      REST `/api/v1/events` only; it opens no WebSocket. The UNCONFIRMED→CONFIRMED
      *transition* is proven by counters, but no frame was captured, so
      `event_revision` monotonicity on the wire is unverified. Needs a WS-capturing
      harness that does not exist yet.
- [ ] Confirm the Android app receives exactly one alarm for the pair — an
      advisory must not alarm (D-009: unconfirmed events are WebSocket-only) —
      **BLOCKED**. Requires the physical device attached to the sim stack; the
      device is on the production feed. Same blocker as 2.1/2.2.

### 3.2 — Two simultaneous events — **VALIDATION**, feeds 1.4 / F-5

- [x] Drive two disjoint clusters into CONFIRMED at once; confirm the server
      issues two distinct `event_id`s — **PASS** (2026-08-29).
      `server/scripts/sim_dual_event.sh`: 11 PASS, 0 FAIL. 6 sim nodes, cluster-A
      Bandung / cluster-B Surabaya. 2 events created, both CONFIRMED
      (delta=2), distinct ids `A=92c5b03e-5e86-4c0b-ac24-b20758842bff`
      `B=6d1fe26c-9f29-43e2-befd-e013bda12122`, `event_open_gauge=2` (no merging),
      independence 3 and 3, no tombstone evictions. Both later RESOLVED
      (`transitions_to_resolved=2`), `event_open_gauge=0` after sweep.
- [x] Resolve event A only; observe what the Android app does to event B —
      **PASS at code/integration level, PHYSICAL DEVICE UNVERIFIED**. The harness
      runs `WarningNotifierStandDownTest` (7/7 green) as its final assertion, which
      covers "stand-down for a different event does NOT clear" and the
      notify-A/clear-B/notify-B/clear-A ordering. No physical device was in the
      loop, so the OEM notification stack is untested on this path.
- [x] Record the observation as evidence for the F-5 open design decision. This
      checkpoint does not decide it — **PASS**. Evidence recorded: the server keeps
      two independent live events with no shared state (`event_open_gauge=2`), and
      the Android side already discriminates by `event_id`. F-5 remains UNRESOLVED
      in `docs/DECISIONS.md`; nothing here decides it.

Note: 1.1 must be fixed before 3.2 is meaningful. Run against unfixed code and
the resolve for A silently stands down B, which is the defect, not the finding.

### 3.3 — Cancellation path — **VALIDATION**

- [ ] Unverify contributors until evidence is exhausted; confirm CANCELLED with
      `reason = EVIDENCE_INVALIDATED` — **BLOCKED** (2026-08-29). No harness exists.
      `server/scripts/` contains sim scripts for 3.1 and 3.2 only
      (`sim_multi_node.sh`, `sim_dual_event.sh`); neither drives the unverify path.
      Authoring a cancellation harness is new tooling and outside the "do not
      improve, validate only" instruction for this run.
- [ ] Confirm the Android app distinguishes it from RESOLVED (needs 1.2)
- [ ] Confirm History after restart still distinguishes them — this is where the
      `status = 'RESOLVED'` projection in `server/internal/event/store.go:56-61`
      becomes visible or does not

### 3.4 — Reconnect and stale delivery — **VALIDATION**, feeds 1.4 / F-7

- [ ] Disconnect across the alert, reconnect after the resolve; record whether a
      replayed or late alert re-raises the siren — **BLOCKED** (2026-08-29). Needs
      both a WebSocket client harness (does not exist, see 3.1) and the physical
      device. Feeds F-7, which is itself UNRESOLVED (checkpoint 1.4).
- [ ] Record the observed timing against `isRecent` 15 min
      (`android/.../domain/WsAlertMessage.kt:134`) versus server `resolveAfter`
      90 s. The gap is the F-7 exposure, measured rather than argued

---

## DEFECT observed 2026-08-29 — production feed carries no Phase 3 fields (RESOLVED: not a defect)

FACT. `curl https://api.quakealert.web.id/api/v1/events` returns 6 events. The union
of keys across all of them is exactly the pre-Phase-3 set:

```
created_at, depth_km, event_id, intensity_label, latitude, location_name,
longitude, mmi, pga, resolved_at, status, triggered_nodes_count
```

None of `event_state`, `event_revision`, `origin_ts`, `origin_ts_source`,
`independent_cell_count` appears on any row.

FACT. `server/internal/api/api.go:1079-1084` declares all five with `,omitempty`, so a
zero value is indistinguishable from a field the server never wrote — the absence is
consistent with *either* a server that predates Phase 3 *or* rows written before the
Tracker was enabled.

FACT. Phase 3 landed in `9752c5e` on 2026-08-27 20:38 +0700. The newest production row
is `created_at 2026-08-25T13:13:34Z`, `location_name "VPS Test Bench"` — written before
that commit. `EVENT_TRACKER_ENABLED` defaults to **false**
(`server/internal/config/config.go:195`), and the deployed build's flag is not
observable from outside.

RESOLVED 2026-08-29 by direct VPS inspection (owner granted SSH). The earlier
ASSUMPTION is now settled: **production runs a Phase-3 binary with the Tracker ON.**

FACT. `docker inspect quakealert-server` env contains `EVENT_TRACKER_ENABLED=true`.

FACT. Image `quakealert-server:prod` created `2026-08-28T11:51:51Z`, container started
`2026-08-28 11:58:14 +0000` — i.e. built *after* Phase 3 landed in `9752c5e`
(2026-08-27 20:38 +0700).

FACT. Boot log line: `"event tracker aktif (Fase 3)"` with
`correlation_window 8000000000` (8 s), `attach_radius_km 50`, `independence_cell_km 5`,
`min_independent_cells 2`, `resolve_after 90000000000` (90 s), `sweep_interval 5 s`.
Also `"observation ledger aktif"` and `"FCM sender aktif" project_id quakealert26`.

FACT. Boot log `"event: pemeriksaan independensi fleet lulus"` reports
`active_verified_nodes 6`, `independence_cells 2` — the prod fleet already satisfies the
2-cell minimum, so CONFIRMED is reachable there in principle.

FACT. `event_created_total 0` on every periodic counter line since the 2026-08-28 restart,
and `ledger_rows_written_total 0`. No event has been created under the Phase-3 build.

FACT (re-verified 2026-08-29). `GET https://api.quakealert.web.id/api/v1/events` still
returns the same 6 rows, newest `created_at 2026-08-25T13:13:34Z`, and the key union is
still the pre-Phase-3 set. Consistent with `,omitempty` over rows written by the old
binary — **not** a server defect.

Reclassification: this is NOT a code defect. It is missing field evidence. The Phase-3
fields cannot appear until an event is created on production under the current build.

Consequence for this checklist: checkpoint 1.2's fix is correct and its tests pass, but
the field evidence that would exercise it does not exist on the target server. Any
"Android reads Phase 3 lifecycle fields end-to-end" claim is unproven, and 2.3's
CANCELLED-vs-RESOLVED distinction has nothing on the wire to distinguish yet.

Not fixed here: `PROJECT_RULES.md` scope control and the standing instruction to stop
and report on a defect rather than repair it.

---

## Checkpoint 4 — Early-warning validation

**Not executable yet.** `ROADMAP.md` ACTIVE PHASE is
`Phase 4 Self-Measurement & Forensics`, status `PLANNED`; Phase F is `BLOCKED`.
Everything below is **FUTURE WORK** and is listed so it is not mistaken for work
this session may start.

### 4.1 — End-to-end warning latency — **FUTURE WORK**

- [ ] Measure sensor onset → server CONFIRMED → FCM/WS receipt → siren audible
- [ ] Report each hop separately; a single end-to-end number hides which hop is slow
- [ ] Requires the Phase 4 self-measurement instrumentation that does not exist yet

### 4.2 — Lead time against a real earthquake — **FUTURE WORK**

- [ ] Requires a real event over a real multi-node fleet. Blocked on the same
      deployment as Phase F
- [ ] `origin_ts` and `origin_ts_source` are the fields that make this
      computable at all — another reason 1.2 is a prerequisite, not a nicety

### 4.3 — False-positive / false-negative record — **FUTURE WORK**

- [ ] Log every CONFIRMED against an external catalogue after the fact
- [ ] D-007 holds: thresholds are compile-time constants. This checkpoint may not
      propose changing them to make a result look better
      (`.clinerules/00-project-overview.md`: never change a safety threshold so a
      phase's exit criteria pass)

---

## Open design decisions — not to be resolved by implementing one reading

Listed to keep them visible, per `.clinerules/00-project-overview.md`
("Berhenti dan tanya" — stop when the open question touches safety, alert
behaviour, data semantics, or a public contract).

- **F-5** — should two concurrent events alarm simultaneously, or does the newer
  one replace the older? Currently one `activeAlertDetails` and one notification
  id decide it by accident, not by decision. **Recorded as U-011** in
  `docs/DECISIONS.md` (2026-08-31). Correction to an earlier claim in this file:
  this is **not** blocked on physical sensors — `sim_dual_event.sh` reaches two
  concurrent CONFIRMED events on the local stack. It is blocked on the decision.
- **F-7** — should a late alert be suppressed because a stand-down was already
  seen? Either reading loses something: a siren for a withdrawn event, or a
  suppressed siren for a live one. **Recorded as U-010** in `docs/DECISIONS.md`
  (2026-08-31), where the terminal-state invariant narrows the trade-off — see
  §1.4 above.
- **U-001** — background delivery for unconfirmed events stays unresolved; D-009
  is the accepted decision in force until it changes.

---

## Execution order

**Run 1.1 (F-1 `event_id`-aware stand-down) first.**

Reasons:

- It is the only P1 on a safety path that is fixable now against contracts that
  already exist — no contract change, no owner decision, no hardware.
- The correct comparison already exists in the codebase
  (`WarningActivity.kt:118-120`), so the fix is to make three sites match one that
  is already right.
- It gates checkpoint 3.2: multi-event observations taken against unfixed code
  measure the defect instead of the behaviour.
- It is independent of every open design decision. Under either F-5 reading, an
  all-clear for one event must not stand down another.

Then 1.2 (unblocks History and 4.2), then 1.3 (contract-only), then checkpoint 3
to make CONFIRMED reachable, then checkpoint 2 for device evidence. Checkpoint 4
stays closed until `ROADMAP.md` moves.
