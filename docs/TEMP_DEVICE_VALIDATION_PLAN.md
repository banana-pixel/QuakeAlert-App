# TEMP — Physical-device validation plan (Android)

**READ-ONLY PLAN. Executing it changes no code, no contract, no threshold, and no
notification policy.** It defines nothing: every expected result below is the
behaviour already implemented at `9752c5e` + the working-tree F-1/F-3/F-6 fixes.
Authority order `PROJECT_RULES.md` §5 — this file sits below all of it.

Scope: only what a unit test cannot prove. The 275 JVM tests already cover mapping,
dedup arithmetic, the stand-down predicate and copy selection; none of them can
prove the OS delivered anything.

## Fleet reality (decides what is runnable)

- One physical ESP32. CONFIRMED needs quorum 3 **and**
  `MinIndependentCells = 2`, so **a real earthquake cannot reach CONFIRMED on
  today's fleet.** One node produces UNCONFIRMED / advisory at most.
- Consequence: every test that needs a real CONFIRMED → RESOLVED pair, a second
  event, or a CANCELLED, is in category **B** or **C** below — not runnable from the
  sensor alone. This is a fleet limit, not a defect.

## Categories

| Cat | Meaning |
| --- | --- |
| **A** | Runnable with the one real ESP32 |
| **B** | Requires simulated / multiple nodes (`server/scripts/sim_multi_node.sh`, `sim_dual_event.sh`, local dev stack) |
| **C** | Requires server-side injection (`deploy/scripts/test-alert.sh` drill, or admin unverify) |
| **D** | Android device / OS only — no server event needed |

## Injection tools that already exist (use as-is, do not modify)

- `deploy/scripts/test-alert.sh` → `POST /api/v1/admin/test-alert`. Broadcasts on
  **both** channels (`d.hub.Broadcast` + `dispatchTestFCM`), sends its own all-clear
  after **20 s**, same `event_id`, `is_test = "true"`.
  Three hard limits, all by design:
  - FCM goes to topic `test_alerts` **only** — never `geo_alert_all`. Only a **debug**
    build subscribes (`PushRegistrar.kt:124-128`), and a release build drops any
    `is_test` frame in the mapper. **So every drill-based test below is a debug-build
    test.** A release build cannot be validated this way, and must not be made to.
  - It writes no `earthquake_events` row → nothing to validate in History.
  - It carries no `event_state` → the stand-down always reads as all-clear.
    **CANCELLED wording is not testable by drill.**
- `server/scripts/sim_multi_node.sh` — 3 virtual nodes → real CONFIRMED, local stack.
- `server/scripts/sim_dual_event.sh` — two clusters → two `event_id`s, and its stated
  purpose already includes the F-1 claim ("RESOLVED for B does NOT clear A").
- Admin unverify (`POST /admin/nodes/{id}/verify {"verified":false}`) → evidence
  withdrawal → CANCELLED. Local stack only.

## Evidence to capture on every test

Record for each run, or the result is not evidence:

1. `adb logcat` filtered to `QuakeMessaging|BackgroundAlertBridge|WarningNotifier|Warning`
2. Screen recording or photo of the lock screen / shade at the moment of delivery
3. Device OEM, model, Android release, API level, build type (debug/release)
4. Whether battery optimisation is exempted for the app
5. Wall-clock timestamps of injection and of observed delivery
6. `event_id` seen in logs, compared against the injector's own output

---

## Group 1 — Permissions and OS grants (category D)

Nothing here needs a server event. Run first: every later test is uninterpretable
until these two values are known for the device.

### T-1 — `POST_NOTIFICATIONS` granted (D)

- **Precondition:** Android 13+ device, app installed, permission granted.
- **Action:** `adb shell dumpsys package <pkg> | grep POST_NOTIFICATIONS`.
- **Expected Android result:** `granted=true`.
- **Evidence:** the dumpsys line.
- **Pass:** granted=true. **Fail:** anything else — T-3…T-12 are then measuring a
  revoked permission, not the app.

### T-2 — `canUseFullScreenIntent()` on this device (D) — **F-2, the open question**

- **Precondition:** Android 14+ (API 34+) physical device. Note the app declares
  `USE_FULL_SCREEN_INTENT` (`AndroidManifest.xml:27`) but declares no calling or
  alarm-clock category, so the pre-grant is **not** expected.
- **Action:** trigger any alert (T-5 is the cheapest) and read logcat.
  `WarningNotifier.kt:139-143` logs `full-screen intents not permitted; falling back
  to heads-up` on the false branch, and logs nothing on the true branch.
- **Expected Android result:** unknown — **that is what this test is for.** Both
  outcomes are valid data.
- **Evidence:** presence or absence of that exact log line, plus
  `adb shell dumpsys notification_manager | grep -i fullscreen` if available, plus
  Settings → Apps → QuakeAlert → "Full screen notifications" toggle state.
- **Pass criterion:** the value is **recorded**, not that it is true. If false, the
  documented fallback (heads-up) must still appear — that is the pass condition.
- **Do not:** declare a category, add a permission prompt, or change the channel to
  make this pass. Record and report.

### T-3 — Permission denied still alarms in foreground (D)

- **Precondition:** revoke notification permission; app in foreground on the warning
  screen; WebSocket connected.
- **Action:** inject a drill (T-5 path).
- **Expected:** `WarningNotifier.notify` returns false and logs
  `POST_NOTIFICATIONS not granted; alert cannot be shown`, **but** the in-app
  full-screen alert and siren still fire — the foreground path is the ViewModel, not
  the notifier.
- **Evidence:** logcat line + screen recording of the in-app alert.
- **Pass:** siren audible and alert screen shown despite no notification.

### T-4 — Battery optimisation state recorded (D)

- **Precondition:** none.
- **Action:** `adb shell dumpsys deviceidle whitelist | grep <pkg>`.
- **Expected:** either state is acceptable.
- **Evidence:** the output, attached to every Doze test (T-8).
- **Pass:** state recorded. A Doze failure with unknown exemption state proves nothing.

---

## Group 2 — Delivery by app state (category C, debug build)

All of Group 2 uses the drill injector, so all of it is **debug build only**. Set the
device location near the drill centroid (default Bandung, `-6.9175 / 107.6191`) or
within `SafetyPolicy.ALERT_RADIUS_KM = 200` of it — outside that the gate turns the
alert into a banner, correctly, and the test measures nothing.

Command for every test in this group:

    export ADMIN_API_KEY=...
    API_BASE=<server> ./deploy/scripts/test-alert.sh --pga 300

`--pga 300` is above the severe threshold, so `SEVERE_OVERRIDE` applies and the
distance gate cannot silently suppress the run. Note the returned `event_id`
(`test-…`) — it is the correlation key for every assertion below.

### T-5 — Foreground delivery (C)

- **Precondition:** app open on the warning screen, socket connected, screen on.
- **Action:** inject the drill.
- **Expected:** full-screen in-app alert with drill wording, siren on the alarm
  stream, `WarningActivity` shows "TEST" framing. Exactly **one** siren despite the
  frame arriving on both WS and FCM — `AlertDedup` suppresses the second
  (`WarningViewModel.kt:459` → `startSiren = !alreadyRaised`).
- **Evidence:** recording + logcat showing the second channel logged as
  `already handled; dropped`.
- **Pass:** one alert, one siren, correct `event_id` in both log lines.
- **Fail:** two sirens, or the second frame raising a fresh alert.

### T-6 — Background (process alive) delivery (C)

- **Precondition:** app started then sent to background (Home), process alive, screen on.
- **Action:** inject the drill.
- **Expected:** `BackgroundAlertBridge` posts notification 4301 (it owns this case;
  `foreground == false`), full-screen intent fires if T-2 said it may, otherwise
  heads-up. Siren via `WarningActivity` when it launches.
- **Evidence:** logcat `BackgroundAlertBridge`, shade screenshot, recording.
- **Pass:** exactly one emergency notification, and it is not dismissible by swipe
  (`setOngoing(true)`).

### T-7 — Locked screen (C)

- **Precondition:** device locked, screen off, app backgrounded.
- **Action:** inject the drill.
- **Expected:** if T-2 = true, `WarningActivity` appears **over** the lock screen
  without user action. If T-2 = false, a heads-up/lock-screen notification with
  `VISIBILITY_PUBLIC` content visible.
- **Evidence:** video of the locked device from injection to display.
- **Pass:** one of the two above, matching what T-2 recorded. A silent device is a fail.

### T-8 — Doze / idle (C)

- **Precondition:** `adb shell dumpsys deviceidle force-idle`, screen off, app
  backgrounded. Record T-4 state.
- **Action:** inject the drill (FCM priority is HIGH on the drill path, which is the
  point of testing it here).
- **Expected:** device wakes and the alert appears; delay recorded.
- **Evidence:** `dumpsys deviceidle` state before, timestamps, video.
- **Pass:** alert delivered. **Record the latency** — a 40 s delivery is a finding
  even though it is not a failure of the code.

### T-9 — Process killed (C)

- **Precondition:** `adb shell am force-stop <pkg>` (not swipe — swipe behaviour is
  OEM-specific; run both and record separately).
- **Action:** inject the drill.
- **Expected:** FCM restarts the process, `QuakeMessagingService.onMessageReceived`
  runs, notification 4301 posted. `AlertDedup` is process-lifetime, so a fresh
  process cannot suppress this frame.
- **Evidence:** logcat from process start, shade screenshot.
- **Pass:** alert delivered after force-stop. This is the single most
  OEM-dependent test — record OEM prominently.

### T-10 — App restart during a live alert (C)

- **Precondition:** drill injected, alert showing, **less than 20 s** elapsed (the
  drill's own all-clear lands at 20 s).
- **Action:** kill and reopen the app.
- **Expected:** `fetchWarning()` restores state from REST — **but a drill writes no
  `earthquake_events` row**, so the expected result here is the idle screen, not a
  restored alert. That is correct behaviour for a drill and proves nothing about a
  real event.
- **Evidence:** recording.
- **Pass:** app opens to idle without crashing, and no phantom alert.
- **Note:** the real restore-during-live-event case is **T-16 (category B)**.

---

## Group 3 — Channel ordering and duplication (category C)

`AlertDedup` is keyed `"${type}:$eventId"` and compares `event_revision`
(`AlertDedup.kt:39-52`). A drill carries **no** `event_revision`, so every drill
frame is revision 0 and the "first frame wins, repeats suppressed" branch is what
Group 3 exercises. Revision-ordering across channels needs category B.

### T-11 — FCM arrives before WS (C)

- **Precondition:** app backgrounded with socket **disconnected** (airplane mode off
  but socket dropped, e.g. app backgrounded long enough, or toggle Wi-Fi and let FCM
  arrive over mobile data).
- **Action:** inject the drill; let FCM land first, then bring the app forward so the
  socket connects and replays its last frame.
- **Expected:** FCM path raises the notification and marks dedup. The socket's
  replayed frame is then suppressed — `markIfNew` false → `startSiren = false` — and
  the user's mute choice is preserved (`raise()` carries `isMuted` for the same
  `event_id`).
- **Evidence:** two logcat lines with the same `event_id`, second one `dropped`.
- **Pass:** one siren total, no second alarm on reconnect.

### T-12 — WS arrives before FCM (C)

- **Precondition:** app foreground, socket connected.
- **Action:** inject the drill.
- **Expected:** ViewModel path alarms; the FCM copy arriving seconds later is
  suppressed by dedup and posts no second notification.
- **Evidence:** logcat ordering.
- **Pass:** one siren, one notification.

### T-13 — Duplicate delivery, both channels, twice (C)

- **Precondition:** any state.
- **Action:** inject the **same drill twice** is not possible (each drill gets a fresh
  `event_id`), so instead inject once and observe the natural WS+FCM pair, then
  reconnect the socket to force a replay of the same frame.
- **Expected:** at most one alarm per distinct `event_id`, regardless of how many
  copies arrive.
- **Evidence:** count of `already handled; dropped` lines.
- **Pass:** siren count == distinct `event_id` count.

### T-14 — RESOLVED clears siren and notification (C)

- **Precondition:** T-5 or T-6 in progress, alert active.
- **Action:** wait 20 s for the drill's own all-clear (no extra command needed).
- **Expected:** siren released, torch off if on, notification 4301 cancelled,
  `WarningActivity` finishes, idle banner shows **all-clear** wording
  (`standDownCopyFor(null)` — a drill carries no `event_state`, so the fallback path
  is what runs, and that fallback is correct).
- **Evidence:** video across the 20 s boundary; logcat `push stand-down <id>: null`.
- **Pass:** all four effects (siren, torch, notification, screen) stand down together.
- **Fail:** notification survives, or siren continues.

### T-15 — CANCELLED wording — **NOT RUNNABLE (category B/C, blocked)**

- A drill never sets `event_state`, so the `CANCELLED` branch of `standDownCopyFor`
  cannot be reached by any drill. Reaching it needs a real event whose evidence is
  invalidated → category B (`sim_multi_node.sh` + admin unverify).
- Listed so its absence is deliberate. Do not approximate it with a drill.

---

## Group 4 — Real lifecycle: needs simulated nodes (category B)

Local dev stack + the two existing sim scripts. The **Android device must point at
that stack**, which means a debug build with a reachable `API_BASE`, and FCM will
only work if the stack has `FCM_PROJECT_ID` / `FCM_CREDENTIALS_FILE` configured.
Without FCM credentials the stack still exercises **WebSocket** delivery in full —
run Group 4 over WS and say so in the evidence.

### T-16 — Real CONFIRMED, foreground and background (B)

- **Precondition:** `ADMIN_API_KEY=… ./server/scripts/sim_multi_node.sh` running
  against the local stack; device pointed at it, logged in, location within 200 km of
  the sim centroid.
- **Action:** let the sim drive 3 nodes to CONFIRMED.
- **Expected:** advisory (UNCONFIRMED) updates the banner only — **no siren**
  (D-009); then CONFIRMED raises the alert. `event_state = CONFIRMED`,
  `event_revision` ≥ 1, non-blank UUID `event_id` in logs.
- **Evidence:** logcat with `event_id`/`event_state`, recording, sim script output.
- **Pass:** exactly one siren, and it is on the CONFIRMED frame, not the advisory.
- **Fail:** advisory sirens — that would be a D-009 violation.

### T-17 — Restore during a live real event (B)

- **Precondition:** T-16 CONFIRMED, event still open (before `ResolveAfterMs` 90 s).
- **Action:** force-stop and reopen the app.
- **Expected:** `fetchWarning()` restores the event from REST. With F-3 landed, the
  five lifecycle fields are now parsed — `event_state`, `event_revision`,
  `origin_ts`, `origin_ts_source`, `independent_cell_count`.
- **Evidence:** REST response body captured (`adb logcat` or a parallel `curl` of
  `/api/v1/events`), plus the app's rendered state.
- **Pass:** the five fields are present in the response **and** the app renders the
  event without crashing. **Note:** nothing in `ui/` reads these fields yet, so the
  pass bar is parse-and-survive, not display. Displaying them is F-4, still open.

### T-18 — Two simultaneous events, F-1 isolation (B) — **the F-1 device proof**

- **Precondition:** `ADMIN_API_KEY=… ./server/scripts/sim_dual_event.sh`; device
  attached and within range of both clusters, app in foreground.
- **Action:** let both clusters reach CONFIRMED; observe the RESOLVED for B while A is
  still live.
- **Expected:** RESOLVED(B) does **not** stop A's siren, does **not** cancel
  notification 4301 while it belongs to A, and does **not** clear
  `activeAlertDetails`. Logcat should show
  `stand-down for <B> ignored; active alert is for <A>` (`WarningViewModel.kt:507`)
  and/or `stand-down for <B> ignored; active notification is for <A>`
  (`WarningNotifier.kt:172-176`).
- **Evidence:** both `event_id`s from the sim output, both logcat lines, continuous
  video across RESOLVED(B).
- **Pass:** A's alarm survives RESOLVED(B) and stands down only on RESOLVED(A).
- **Fail:** A's siren stops on B's all-clear — F-1 not effective on device.
- **Known caveat to record, not fix:** `WarningNotifier.activeEventId` is in-memory.
  After a process death the OS still shows notification 4301 while `activeEventId` is
  blank, and the guard then falls open and clears it. Run T-18 **again** with a
  force-stop between the two CONFIRMEDs and record that outcome separately — it is
  expected to clear, and it is a known limit of the current fix, not a new bug.

### T-19 — CANCELLED end to end (B + C)

- **Precondition:** T-16 CONFIRMED and still open; `ADMIN_API_KEY` available.
- **Action:** `POST /admin/nodes/{id}/verify {"verified":false}` for each contributor
  until evidence is exhausted (the third one triggers CANCELLED — see
  `server/internal/api/admin_nodes_lifecycle_test.go`).
- **Expected:** one withdrawal frame, `event_state = CANCELLED`, same `event_id`.
  Android stands the alert down and the idle banner uses **withdrawn** wording, not
  all-clear (`standDownCopyFor(EventState.CANCELLED)`).
- **Evidence:** the three HTTP responses, logcat `event_state`, screenshot of the
  banner text.
- **Pass:** banner says withdrawn, not "all clear".
- **Fail:** all-clear wording — means `event_state` did not reach the client.

### T-20 — History after restart: RESOLVED vs CANCELLED (B)

- **Precondition:** one RESOLVED event and one CANCELLED event in the local DB
  (T-16 + T-19).
- **Action:** restart the app, open History.
- **Expected:** both appear. The parent row projects both to `status = 'RESOLVED'`
  (`server/internal/store/store.go:264,284`), so History is **expected not to
  distinguish them yet**. `event_state` in the REST payload should distinguish them.
- **Evidence:** REST body for both rows, History screenshot.
- **Pass:** `event_state` differs in the payload (CANCELLED vs RESOLVED). The UI not
  differing is the **known F-4 gap**, recorded here, not fixed.

### T-21 — WebSocket disconnect / reconnect, stale replay (B)

- **Precondition:** T-16 CONFIRMED, alert active.
- **Action:** drop connectivity across the RESOLVED (airplane mode on before the
  server's 90 s resolve, off after), then let the socket reconnect.
- **Expected, and this is the F-7 observation:** the socket replays its last frame to a
  new subscriber. If the replayed frame is the **alert** rather than the resolve, the
  device may re-raise a siren for an event the server already ended — `isRecent`'s
  15-minute window (`WsAlertMessage.kt:134`) is the only guard, against a 90 s server
  resolve.
- **Evidence:** exact timestamps of disconnect, resolve, reconnect; logcat of the
  first frame after reconnect; whether a siren fired.
- **Pass criterion:** the behaviour is **recorded**. F-7 is an open design decision;
  this test produces its evidence and does not settle it.
- **Do not:** add suppression logic based on this result. That needs a decision in
  `docs/DECISIONS.md` first.

---

## Group 5 — What the single real ESP32 can prove (category A)

The real node cannot produce CONFIRMED, so category A is narrow — and honest about it.

### T-22 — Real trigger → advisory reaches the device (A)

- **Precondition:** ESP32 online and verified, device app open, socket connected.
- **Action:** physically tap / shake the node hard enough to exceed its trigger
  threshold. Do not change the threshold to make this easier.
- **Expected:** server ingests the observation, Tracker opens an event as
  UNCONFIRMED, WS advisory reaches the app, **banner updates, no siren, no
  notification** (`WarningViewModel.kt:388-398`, `BackgroundAlertBridge` advisory
  branch is deliberately empty).
- **Evidence:** MQTT/server log of the trigger, logcat of the advisory frame,
  screenshot of the banner.
- **Pass:** banner appears and **nothing alarms**. An alarm here is a D-009 violation
  and a P0 finding.

### T-23 — Advisory does not wake a backgrounded device (A)

- **Precondition:** as T-22 but app backgrounded, screen off.
- **Action:** trigger the node.
- **Expected:** nothing. Advisories are WS-only since Phase 3 and the bridge's
  advisory branch posts nothing.
- **Evidence:** logcat showing the frame arrived and no notification followed.
- **Pass:** frame received, device stays silent.

### T-24 — Node offline / unsynced clock visibility (A)

- **Precondition:** ESP32 running.
- **Action:** power-cycle it and observe the sensors screen while it reconnects.
- **Expected:** the app shows its status without claiming a false healthy state.
  `clock_source` is reported by firmware (commit `0ea126d`) so an unsynced node stays
  visible.
- **Evidence:** sensors screen screenshots across the reconnect.
- **Pass:** status reflects reality at each stage.

---

## Coverage map against the 17 requested focus areas

| # | Focus | Tests | Category |
| --- | --- | --- | --- |
| 1 | Android 14+ full-screen intent grant | T-2, T-7 | D, C |
| 2 | `POST_NOTIFICATIONS` | T-1, T-3 | D, C |
| 3 | Confirmed FCM delivery | T-6, T-9, T-11 | C |
| 4 | Background app | T-6 | C |
| 5 | Foreground app | T-5 | C |
| 6 | Locked screen | T-7 | C |
| 7 | Doze / idle | T-8 (+T-4) | C, D |
| 8 | Process killed | T-9 | C |
| 9 | App restart | T-10 (drill), T-17 (real) | C, B |
| 10 | WS disconnect / reconnect | T-21 | B |
| 11 | FCM before WS | T-11 | C |
| 12 | WS before FCM | T-12 | C |
| 13 | Duplicate FCM/WS | T-5, T-13 | C |
| 14 | RESOLVED | T-14 (drill), T-16+T-21 (real) | C, B |
| 15 | CANCELLED | T-19 — **not reachable by drill** (T-15) | B |
| 16 | `event_id` isolation | T-18 | B |
| 17 | Notification / siren clear | T-14, T-18 | C, B |

Areas 15, 16 and the real-lifecycle half of 9, 10, 14 are **category B only**. With
one ESP32 and no simulated fleet running, those six are not executable today.

## Ordering

1. T-1, T-4 (facts about the device, one command each)
2. T-2 — everything about lock-screen behaviour depends on its answer
3. T-5 → T-6 → T-7 → T-8 → T-9 → T-10 (drill sweep, cheapest to most OEM-dependent)
4. T-11 → T-12 → T-13 → T-14 (ordering and stand-down, still drill)
5. T-22 → T-23 → T-24 (real node, proves the advisory path stays silent)
6. Group 4 last, and only once the local stack is up: T-16 → T-17 → T-18 → T-19 →
   T-20 → T-21

## Out of scope for this plan

Stated so no run drifts into it: no code change, no contract change, no threshold
change, no notification-policy change, no UI redesign, no new product behaviour, and
no resolution of F-4, F-5 or F-7. Findings are recorded and reported.

Also: nothing here may be marked VALIDATED by whoever wrote the code it exercises
(`PROJECT_RULES.md` §8).

---

**DEVICE VALIDATION READY: YES** — for categories A, C and D (T-1 … T-15, T-22 … T-24).
**NO** for category B (T-16 … T-21), which is where F-1's device proof lives.

**FIRST TEST: T-1** (`POST_NOTIFICATIONS` grant) — one adb command, and it decides
whether anything else measured on this device is meaningful. Then immediately **T-2**,
the F-2 open question, since Group 2's expected results branch on its answer.

**BLOCKERS:**

1. **Category B needs the local dev stack running with 3 simulated nodes.** Blocks
   T-16 … T-21, i.e. the real CONFIRMED, the CANCELLED wording, the F-1 two-event
   proof, History RESOLVED-vs-CANCELLED, and the F-7 stale-replay observation. Not
   clearable by any Android-side work.
2. **All drill tests (Group 2, Group 3) are debug-build only.** Topic separation plus
   the client `is_test` fence mean a release build cannot receive a drill — by design.
   Release-build delivery can only be validated by a real or simulated event.
3. **FCM on the local stack needs `FCM_PROJECT_ID` and `FCM_CREDENTIALS_FILE`.**
   Without them Group 4 validates WebSocket delivery only, and T-16/T-18 prove nothing
   about push.
4. **One physical ESP32 → CONFIRMED unreachable on real hardware** (quorum 3,
   `MinIndependentCells = 2`). Phase F stays `BLOCKED` in `ROADMAP.md` until the owner
   deploys more nodes; category A cannot substitute.
5. **`WarningNotifier.activeEventId` does not survive process death.** T-18's
   force-stop variant is expected to fail the isolation guard. Record it; the fix is
   not in this plan's scope.
