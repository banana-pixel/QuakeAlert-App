/**
 * QuakeAlert ESP32 - V6.9.6
 * Mission-critical earthquake detection firmware with:
 * - true dual-core task isolation
 * - semaphore-driven MPU6050 interrupt handling
 * - no scheduled or heap-triggered restarts
 * - background network maintenance task
 */

#include <Wire.h>
#include <WiFi.h>
#include <WiFiClientSecure.h>
#include <esp_task_wdt.h>
#include <Preferences.h>
#include <esp_random.h>
#include "secrets.h"
#include "config.h"
#include "state.h"
#include "utils.h"
#include "network.h"
#include "sensor.h"
#include "mqtt.h"
#include "mqtt_tls.h"

// Ensure Arduino LED define exists
#ifndef LED_BUILTIN
#define LED_BUILTIN LED_BUILTIN_PIN
#endif

// ========================================
// CREDENTIALS
// ========================================
// Nilai bawaan build (secrets.h) berlaku sampai portal /config menulis
// override per-node ke NVS — dibaca ulang tiap boot oleh loadBrokerConfig().
// Buffer, bukan pointer: nilai NVS disalin saat boot sehingga tidak bergantung
// pada masa hidup objek Preferences.
char mqtt_server[MQTT_BROKER_BUFFER_SIZE] = SECRET_MQTT_SERVER;
int  mqtt_port     = SECRET_MQTT_PORT;
bool mqtt_use_tls  = true;
const char* mqtt_user     = SECRET_MQTT_USER;
const char* mqtt_password = SECRET_MQTT_PASS;

// ========================================
// NTP
// ========================================
const char* ntpServer = "id.pool.ntp.org";
const long gmtOffset_sec = 0;
const int daylightOffset_sec = 0;

// ========================================
// GLOBAL OBJECT DEFINITIONS
// ========================================
MPU6050 mpu;
TaskHandle_t SensorTask = nullptr;
TaskHandle_t NetworkMaintenanceTask = nullptr;
WiFiClientSecure espClient;
WiFiClient plainClient;               // jalur plaintext dev saat mqtt_use_tls = false
PubSubClient mqttClient(espClient);
SemaphoreHandle_t i2cMutex = nullptr;
SemaphoreHandle_t mpuInterruptSemaphore = nullptr;
SemaphoreHandle_t stateMutex = nullptr;

portMUX_TYPE reportMux = portMUX_INITIALIZER_UNLOCKED;
portMUX_TYPE eventTriggerMux = portMUX_INITIALIZER_UNLOCKED;

volatile EventReport pendingPrelim = {false, 0.0f, 0.0f, 0UL, false, 0, 0UL, 0, 0UL, 0UL};
volatile EventReport pendingReport = {false, 0.0f, 0.0f, 0UL, false, 0, 0UL, 0, 0UL, 0UL};
volatile bool eventTriggered = false;
volatile bool rebootRequestReceived = false;

bool DMPReady = false;
uint8_t devStatus = 0;
uint16_t packetSize = 0;
uint8_t FIFOBuffer[64] = {0};
Quaternion q;
VectorInt16 aa;
VectorInt16 aaReal;
VectorFloat gravity;

char StationID[STATION_ID_BUFFER_SIZE] = "SEIS-01";
char lokasiAlat[LOCATION_TEXT_BUFFER_SIZE] = "Mencari lokasi...";
bool potentialEvent = false;
bool eventInProgress = false;
bool alertSent = false;
float pga = 0.0f;
bool ledState = false;
bool isNtpSynced = false;
bool startupMessageSent = false;
bool locationResolved = false;

unsigned long potentialEventTime = 0;
unsigned long eventStartTime = 0;
unsigned long lastReportTime = 0;
unsigned long lastBlinkTime = 0;
unsigned long lastNtpSync = 0;
unsigned long lastNtpAttempt = 0;
unsigned long lastWifiCheck = 0;
unsigned long lastMqttAttempt = 0;
unsigned long lastHeartbeat = 0;

uint32_t mpuOverflowCount = 0;
uint32_t totalEventsDetected = 0;
int mpuErrorCounter = 0;
uint32_t minHeapSeen = 0xFFFFFFFF;
float maxHeapFragmentationSeen = 0.0f;
unsigned long lastHeapCheck = 0;
int bootCount = 0;
uint16_t inBootSeq = 0;
Preferences preferences;

char lastPgaStr[16] = "N/A";
char lastIntensity[INTENSITY_TEXT_BUFFER_SIZE] = "N/A";
char lastEventTime[TIME_TEXT_BUFFER_SIZE] = "N/A";

float stationLat = 0.0f;
float stationLon = 0.0f;

uint32_t wifiFailCount = 0;
unsigned long lastLocRetry = 0;

// ========================================
// INTERNAL HELPERS
// ========================================
static bool initializeCorePrimitives() {
    i2cMutex = xSemaphoreCreateMutex();
    stateMutex = xSemaphoreCreateMutex();
    mpuInterruptSemaphore = xSemaphoreCreateBinary();

    if (i2cMutex == nullptr || stateMutex == nullptr || mpuInterruptSemaphore == nullptr) {
        Serial.println("FATAL: Failed to create RTOS synchronization primitives");
        return false;
    }

    return true;
}

static void assignStationId() {
    // Use hardware RNG + NVS to generate a persistent, anonymous node identity.
    // The MAC address is never exposed — a random NODE-XXXXXXXX string is generated
    // exactly once and stored permanently in NVS under the "quake-app" namespace.
    Preferences prefs;
    prefs.begin("quake-app", false);  // open R/W

    if (!prefs.isKey("station_id")) {
        // First boot: generate an 8-char hex ID from the ESP32 hardware RNG.
        // esp_random() returns a true hardware random 32-bit value.
        const uint32_t rndVal = esp_random();
        char newId[STATION_ID_BUFFER_SIZE];
        snprintf(newId, sizeof(newId), "NODE-%08X", rndVal);
        prefs.putString("station_id", newId);
        Serial.printf("Generated anonymous Station ID: %s\n", newId);
    }

    // Load the persisted ID directly into the global C-string buffer (no heap String).
    prefs.getString("station_id", StationID, sizeof(StationID));
    prefs.end();

    Serial.printf("Station ID: %s\n", StationID);
}

static void loadBrokerConfig() {
    // Override broker per-node dari portal /config (wizard "Add a Sensor").
    // Field yang absen atau di luar rentang membiarkan nilai bawaan secrets.h:
    // NVS hanya pernah berisi nilai yang sudah lolos validasi di network.cpp,
    // tapi pemeriksaan ulang saat baca membuat loader ini tidak bisa dipakai
    // untuk menyalakan sesuatu yang aneh walau NVS ditulis langsung.
    Preferences prefs;
    prefs.begin("quake-app", true);
    String broker = prefs.getString(NVS_KEY_MQTT_BROKER, "");
    const int port = prefs.getInt(NVS_KEY_MQTT_PORT, 0);
    mqtt_use_tls = prefs.getBool(NVS_KEY_MQTT_TLS, true);
    prefs.end();

    if (broker.length() > 0 && broker.length() < sizeof(mqtt_server)) {
        broker.toCharArray(mqtt_server, sizeof(mqtt_server));
    }
    if (port > 0 && port <= 65535) {
        mqtt_port = port;
    }
    Serial.printf("MQTT target: %s:%d (%s)\n", mqtt_server, mqtt_port,
                  mqtt_use_tls ? "TLS" : "PLAINTEXT");
}

static void initPersistentState() {
    setLocationStatusSearching();
    setLastEventTime("N/A");
    setLastIntensity("N/A");
    setLastPga("N/A");

    preferences.begin("quake-app", false);
    bootCount = preferences.getInt("boots", 0) + 1;
    preferences.putInt("boots", bootCount);
    preferences.end();
}

static void initTaskWatchdog() {
    esp_task_wdt_deinit();

#ifdef USE_LEGACY_WDT
    esp_task_wdt_init(WDT_TIMEOUT, true);
#else
    esp_task_wdt_config_t twdt_config = {
        .timeout_ms = WDT_TIMEOUT * 1000,
        .idle_core_mask = (1 << 0) | (1 << 1),
        .trigger_panic = true
    };
    esp_task_wdt_init(&twdt_config);
#endif

    esp_task_wdt_add(nullptr);
}

static void startWorkerTasks() {
    BaseType_t sensorResult = xTaskCreatePinnedToCore(
        sensorTask,
        "SensorTask",
        SENSOR_TASK_STACK_SIZE,
        nullptr,
        SENSOR_TASK_PRIORITY,
        &SensorTask,
        SENSOR_TASK_CORE
    );

    BaseType_t networkResult = xTaskCreatePinnedToCore(
        networkMaintenanceTask,
        "NetworkMaintenanceTask",
        NETWORK_MAINTENANCE_TASK_STACK_SIZE,
        nullptr,
        NETWORK_MAINTENANCE_TASK_PRIORITY,
        &NetworkMaintenanceTask,
        NETWORK_MAINTENANCE_TASK_CORE
    );

    if (sensorResult != pdPASS) {
        Serial.println("ERROR: Failed to start SensorTask");
    } else {
        Serial.println("SensorTask started on Core 0");
    }

    if (networkResult != pdPASS) {
        Serial.println("ERROR: Failed to start NetworkMaintenanceTask");
    } else {
        Serial.println("NetworkMaintenanceTask started on Core 1");
    }
}

// ========================================
// ALERT HANDLER (glue: sensor -> mqtt)
// ========================================
// servePendingSlot memublikasikan satu slot laporan bila ada yang menunggu, dengan
// retry, backoff, dan kedaluwarsa yang sama untuk PRELIM dan FINAL.
//
// Satu fungsi untuk kedua fase, bukan dua salinan: perbedaan keduanya hanyalah
// phase, ada/tidaknya detrigger_ts, dan siapa yang memperbarui tampilan "event
// terakhir". Dua salinan dari logika retry ini akan menyimpang, dan yang menyimpang
// adalah jalur yang lebih jarang dijalankan — justru PRELIM, satu-satunya
// publikasi yang punya nilai peringatan.
//
// Fungsi global, bukan static / anonymous namespace: preprocessor .ino Arduino
// membangkitkan prototipe global untuk setiap definisi fungsi di berkas ini, dan
// prototipe itu akan bertabrakan dengan versi yang di-internal-linkage-kan.
void servePendingSlot(volatile EventReport& slot, bool isFinal) {
    bool shouldSend = false;
    float reportPga = 0.0f;
    float reportDuration = 0.0f;
    int64_t obsSeq = 0;
    unsigned long onsetMillis = 0UL;
    unsigned long detriggerMillis = 0UL;
    uint8_t attemptsSoFar = 0;

    portENTER_CRITICAL(&reportMux);
    if (slot.ready && !slot.processed) {
        shouldSend = true;
        reportPga = slot.maxPga;
        reportDuration = slot.duration;
        obsSeq = slot.obsSeq;
        onsetMillis = slot.onsetMillis;
        detriggerMillis = slot.detriggerMillis;
        attemptsSoFar = slot.publishAttempts;
        slot.processed = true;
    }
    portEXIT_CRITICAL(&reportMux);

    if (shouldSend) {
        if (isFinal) {
            // Tampilan lokal hanya mengikuti FINAL: PRELIM sengaja belum final, dan
            // menampilkannya akan membuat layar melaporkan puncak yang lebih rendah
            // daripada yang sebenarnya terjadi.
            char waktu[TIME_TEXT_BUFFER_SIZE];
            getWaktuString(waktu, sizeof(waktu));

            char pgaText[16];
            snprintf(pgaText, sizeof(pgaText), "%.2f gal", reportPga);

            setLastEventTime(waktu);
            setLastIntensity(toIntensity(reportPga));
            setLastPga(pgaText);
        }

        // Epoch dikonversi dari millis() DI SINI, pada setiap percobaan: node yang
        // NTP-nya baru sinkron setelah onset tetap melaporkan onset yang benar.
        TriggerPublish obs = {};
        obs.phase = isFinal ? PHASE_FINAL : PHASE_PRELIM;
        obs.obsSeq = obsSeq;
        // attempt_no 1-based: percobaan PERTAMA adalah 1, dan hanya pada nilai itu
        // server memberlakukan batas ATAS koherensi onset (§14.4).
        obs.attemptNo = (attemptsSoFar < 255) ? static_cast<uint8_t>(attemptsSoFar + 1) : 255;
        obs.pgaGal = reportPga;
        obs.durMs = static_cast<uint32_t>(reportDuration * 1000.0f);
        obs.onsetTsMs = epochAtMillis(onsetMillis);
        obs.detriggerTsMs = isFinal ? epochAtMillis(detriggerMillis) : 0;

        if (publishTrigger(obs)) {
            if (isFinal) {
                totalEventsDetected++;
            }
            portENTER_CRITICAL(&reportMux);
            slot.ready = false;
            slot.processed = false;
            slot.publishAttempts = 0;
            slot.lastAttemptMs = 0;
            portEXIT_CRITICAL(&reportMux);
        } else {
            // Publish gagal (MQTT putus / NTP belum sinkron). JANGAN buang
            // laporan gempa: tandai sudah dicoba, coba lagi di loop berikutnya
            // dengan backoff. Laporan dinyatakan hilang hanya jika batas
            // percobaan ATAU batas usia terlampaui — dan selalu dengan log.
            portENTER_CRITICAL(&reportMux);
            slot.publishAttempts++;
            slot.lastAttemptMs = millis();
            const uint8_t attempts = slot.publishAttempts;
            portEXIT_CRITICAL(&reportMux);

            Serial.printf("Trigger publish failed (%s attempt %u/%u) — will retry\n",
                          obs.phase, attempts, TRIGGER_MAX_ATTEMPTS);
            if (attempts >= TRIGGER_MAX_ATTEMPTS) {
                Serial.printf("ERROR: %s publish gave up after max attempts — OBSERVATION LOST\n",
                              obs.phase);
                portENTER_CRITICAL(&reportMux);
                slot.ready = false;
                slot.processed = false;
                slot.publishAttempts = 0;
                portEXIT_CRITICAL(&reportMux);
            }
        }
    }

    // Retry berkala untuk laporan yang belum berhasil dipublish: hormati
    // backoff antar percobaan, dan beri batas usia jauh lebih longgar dari
    // semula agar event tidak dibuang saat MQTT sedang reconnect.
    portENTER_CRITICAL(&reportMux);
    if (slot.ready && slot.processed) {
        const unsigned long age = millis() - slot.timestamp;
        const unsigned long sinceAttempt = millis() - slot.lastAttemptMs;
        if (age > TRIGGER_MAX_AGE_MS) {
            slot.ready = false;
            slot.processed = false;
            slot.publishAttempts = 0;
            portEXIT_CRITICAL(&reportMux);
            Serial.println("ERROR: stale unpublished observation expired — OBSERVATION LOST");
            return;
        }
        if (sinceAttempt >= TRIGGER_RETRY_BACKOFF_MS) {
            slot.processed = false;  // percobaan berikutnya akan mengambilnya lagi
        }
    }
    portEXIT_CRITICAL(&reportMux);
}

void handleAlerts() {
    // eventTriggered menandai onset terkonfirmasi dan hanya dipakai untuk state
    // tampilan/LED. Observasi PRELIM-nya sendiri tidak dibawa oleh flag ini
    // melainkan oleh pendingPrelim, yang diisi SensorTask bersamaan dengan flag —
    // sebuah flag tidak dapat membawa pga, onset, maupun obs_seq.
    portENTER_CRITICAL(&eventTriggerMux);
    if (eventTriggered) {
        eventTriggered = false;
        alertSent = true;
    }
    portEXIT_CRITICAL(&eventTriggerMux);

    // PRELIM lebih dulu: ia yang punya nilai peringatan, dan bila MQTT hanya
    // sempat mengirim satu paket, paket itu harus yang paling dini.
    servePendingSlot(pendingPrelim, false);
    servePendingSlot(pendingReport, true);
}


// ========================================
// SETUP & LOOP
// ========================================
void setup() {
    Serial.begin(115200);

    // -----------------------------------------------------------------
    // Hardware Factory Reset (GPIO 0 — BOOT button)
    //
    // Hold the BOOT button during power-on to wipe all stored data so
    // the device can be redeployed to a new location:
    //   • preferences.clear()  — erases lat, lon, station_id, boot count
    //   • wm.resetSettings()   — erases saved WiFi SSID/password
    // The device then restarts and presents a clean "Quake-Setup" portal.
    // -----------------------------------------------------------------
    pinMode(0, INPUT_PULLUP);
    if (digitalRead(0) == LOW) {
        Serial.println("BOOT button held — performing factory reset...");

        Preferences prefs;
        prefs.begin("quake-app", false);
        prefs.clear();
        prefs.end();

        // Custom WiFi credentials have been erased by prefs.clear()

        Serial.println("Factory reset complete. Restarting...");
        delay(1000);
        ESP.restart();
    }

    pinMode(LED_BUILTIN, OUTPUT);
    digitalWrite(LED_BUILTIN, HIGH);

    if (!initializeCorePrimitives()) {
        Serial.println("System entering safe idle mode due to initialization failure");
        while (true) {
            digitalWrite(LED_BUILTIN, !digitalRead(LED_BUILTIN));
            delay(500);
        }
    }

    assignStationId();
    loadBrokerConfig();
    initPersistentState();
    initTaskWatchdog();

    // Jalur transport dipilih sebelum setServer: PubSubClient menyimpan referensi
    // client, dan configureMqttTls() memasang trust anchor pada espClient —
    // dipanggil hanya saat NVS (fallback secrets.h) menuntut TLS. Jalur plaintext
    // sengaja berisik di serial agar build dev tidak diam-diam sampai ke lapangan.
    if (mqtt_use_tls) {
        configureMqttTls();
        mqttClient.setClient(espClient);
    } else {
        Serial.println("MQTT: PLAINTEXT - HANYA untuk pengembangan lokal!");
        mqttClient.setClient(plainClient);
    }
    mqttClient.setServer(mqtt_server, mqtt_port);
    mqttClient.setBufferSize(2048);
    mqttClient.setKeepAlive(CUSTOM_MQTT_KEEPALIVE);
    mqttClient.setCallback(mqttCallback);
    
    // Trust anchor dipasang sebelum koneksi pertama: setCACert pada sesi yang
    // sudah berjalan tidak berlaku untuk sesi itu. Detail pilihan akar ada di
    // mqtt_ca.h; koneksi ditahan sampai NTP memberi jam yang masuk akal
    // (mqttTlsClockReady), karena verifikasi masa berlaku terhadap jam 1970
    // selalu gagal.
    // (configureMqttTls() dipindah ke cabang TLS di atas.)

    initWifi();
    configTime(gmtOffset_sec, daylightOffset_sec, ntpServer);
    initMPU();

    digitalWrite(LED_BUILTIN, LOW);

    startWorkerTasks();

    delay(250);
    Serial.printf("System Ready (%s) [Boots: %d]\n", FIRMWARE_VERSION, bootCount);
}

void loop() {
    esp_task_wdt_reset();

    handleProvisioningLoop();

    monitorHeap();
    checkMqttConnection();

    if (mqttClient.connected()) {
        mqttClient.loop();

        if (millis() - lastHeartbeat >= HEARTBEAT_INTERVAL_MS) {
            lastHeartbeat = millis();
            sendHeartbeat();
        }
    }

    if (rebootRequestReceived) {
        Serial.println("Remote reboot request received! Rebooting system in 1s...");
        rebootRequestReceived = false;
        delay(1000);
        ESP.restart();
    }

    handleAlerts();

    // Catatan: pembersih "stale pending report" 15 detik yang lama dihapus.
    // Ia menghapus laporan ready&&!processed berusia >15 dtk, yang justru
    // adalah kondisi retry setelah publish gagal — membuang event gempa saat
    // MQTT reconnect. Batas usianya kini TRIGGER_MAX_AGE_MS di handleAlerts().

    delay(10);
}