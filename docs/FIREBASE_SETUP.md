# Firebase Cloud Messaging setup

Push is the only alert path that survives the app being backgrounded or killed. The
WebSocket stream (`GET /ws`) covers the foreground case, and the app is built so that
**everything below is optional**: with no `android/app/google-services.json` the Gradle
plugin is not applied, no `FirebaseApp` is initialised, `PushRegistrar` logs and returns,
and alerts arrive over the WebSocket only. Nothing crashes and nothing needs commenting
out. Configure FCM when you want alerts to wake a device that is not running the app.

Two halves have to agree: the **app** needs `google-services.json`, and the **server**
needs a service account so it can call the FCM v1 send API.

## 1. Firebase project and Android app

1. Create (or open) a project at <https://console.firebase.google.com>.
2. Add an Android app with package name **`id.web.quakealert`** — it must match
   `namespace`/`applicationId` in `android/app/build.gradle.kts` exactly, or FCM will
   reject the registration.
3. A debug SHA-1 is *not* required: QuakeAlert uses no Firebase feature that verifies the
   signing certificate (no Auth, no Dynamic Links).
4. Download `google-services.json` and drop it at `android/app/google-services.json`.
   `android/app/google-services.json.example` shows the shape; the real file is
   gitignored (`android/**/google-services.json`) because it pins a build to one project.
5. Rebuild. The build log line
   `QuakeAlert: no app/google-services.json — building without FCM (WebSocket alerts only).`
   disappearing is how you know the plugin picked the file up.

## 2. Server credentials

The dispatcher reads two variables (`server/internal/config/config.go`):

| Variable | Value |
| --- | --- |
| `FCM_PROJECT_ID` | the `project_id` from `google-services.json` |
| `FCM_CREDENTIALS_FILE` | path to a service-account JSON key with the *Firebase Messaging API* role |

Generate the key under **Project settings → Service accounts → Generate new private key**.
It is a real secret: mount it into the container, keep it out of the repo, and never paste
its contents into an issue or a commit.

With either variable empty the server skips FCM and dispatches over the WebSocket only —
the mirror image of the client-side guard, so a half-configured deployment degrades
instead of erroring.

## 3. Topic subscription

The server broadcasts to the topic **`geo_alert_all`** (`dispatch.GeoTopic`) for users
whose position it does not know, and sends to individual tokens for users inside the event
radius. The client subscribes to that topic in `PushRegistrar.register()` and uploads its
token via `PUT /api/v1/users/fcm-token`, on app start and again from
`QuakeMessagingService.onNewToken`. Both are needed: the topic is the fallback, the token
is what makes radius targeting possible.

## 4. Verifying

1. Install a build that has `google-services.json`, open the app once (this uploads the
   token and subscribes to the topic), then **force-stop** it.
2. Trigger an event — `server/run_e2e_test.sh`, or three simulated node triggers inside
   the 8 s consensus window.
3. `WarningActivity` should appear over the lock screen with the siren playing. If the
   notification arrives but no full-screen alert does, the device denied
   `USE_FULL_SCREEN_INTENT`: from API 34 the grant is automatic only for calling and alarm
   apps, `WarningNotifier` detects this via `canUseFullScreenIntent()` and falls back to a
   heads-up notification. Grant it under **Settings → Apps → QuakeAlert → Full screen
   intents**.
4. Nothing at all? Check, in order: the app's notification permission
   (`POST_NOTIFICATIONS`), that the server logged an FCM send rather than a credentials
   error, and that the event centroid is inside the coverage radius in Settings — the
   Haversine gate in `AlertGate` deliberately suppresses distant events, and it runs
   before the notification is posted.

## Payload contract

Data-only messages, every value a string (`contracts/fcm/alert_payload.json`).
`FcmAlertMapper` converts them into the same `WsAlertMessage` the socket produces, so push
and socket share one state machine, one distance gate and one dedup key. Notification-style
payloads (`notification:` block) are deliberately unsupported: the system tray would render
them itself, bypassing the gate and the siren.
