plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
}

/**
 * Firebase is optional at build time.
 *
 * The google-services plugin fails the build outright when it cannot find a
 * `google-services.json`, so it is applied only when one exists. Without it the app
 * still compiles and runs — `FirebaseApp` simply never initialises, every push entry
 * point logs and returns, and alerts arrive over the WebSocket exactly as they do
 * today. See docs/FIREBASE_SETUP.md.
 */
/**
 * A build secret from the environment, or from a Gradle property, or null.
 *
 * Env var first so CI can inject one without writing a file; the Gradle property
 * is the local convenience, and it must live in `~/.gradle/gradle.properties`
 * rather than the project's, which is committed. Blank is treated as absent
 * because an unset CI variable expands to an empty string, and an empty password
 * would otherwise fail deep inside the signing task instead of here.
 */
fun quakeSecret(envName: String, propertyName: String): String? =
    (System.getenv(envName) ?: providers.gradleProperty(propertyName).orNull)
        ?.takeIf { it.isNotBlank() }

val hasFirebaseConfig = file("google-services.json").exists()
if (hasFirebaseConfig) {
    apply(plugin = "com.google.gms.google-services")
} else {
    logger.lifecycle(
        "QuakeAlert: no app/google-services.json — building without FCM (WebSocket alerts only)."
    )
}

android {
    namespace = "id.web.quakealert"
    compileSdk = 37

    defaultConfig {
        applicationId = "id.web.quakealert"
        minSdk = 28
        targetSdk = 36
        versionCode = 1
        versionName = "1.0"

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    /**
     * Release signing, supplied by the environment rather than by a file in the
     * repository.
     *
     * Every value comes from an env var, with a Gradle property as the local
     * fallback (`~/.gradle/gradle.properties`, which is outside the repo). No
     * keystore, alias or password is ever committed; `.gitignore` refuses `*.jks`
     * and `*.keystore` as a second line of defence.
     *
     * When the variables are absent the config is simply not registered, so
     * `assembleRelease` still builds an *unsigned* APK. That is deliberate: CI must
     * be able to prove the release build compiles and passes R8 on every pull
     * request without holding the production signing key.
     */
    val releaseKeystore = quakeSecret("QUAKE_KEYSTORE_PATH", "quake.keystorePath")
        ?.let(::file)
        ?.takeIf { it.exists() }
    val releaseStorePassword = quakeSecret("QUAKE_KEYSTORE_PASSWORD", "quake.keystorePassword")
    val releaseKeyAlias = quakeSecret("QUAKE_KEY_ALIAS", "quake.keyAlias")
    val releaseKeyPassword = quakeSecret("QUAKE_KEY_PASSWORD", "quake.keyPassword")
    val canSignRelease = releaseKeystore != null &&
        releaseStorePassword != null &&
        releaseKeyAlias != null &&
        releaseKeyPassword != null

    signingConfigs {
        if (canSignRelease) {
            create("release") {
                storeFile = releaseKeystore
                storePassword = releaseStorePassword
                keyAlias = releaseKeyAlias
                keyPassword = releaseKeyPassword
                // v1 is unnecessary below API 24 and this app is minSdk 28.
                enableV1Signing = false
                enableV2Signing = true
                enableV3Signing = true
            }
        } else {
            logger.lifecycle(
                "QuakeAlert: no release signing credentials in the environment - " +
                    "assembleRelease will produce an unsigned APK."
            )
        }
    }

    buildTypes {
        debug {
            // Emulator loopback to the Go server on the host (docs/CLIENT_SPEC.md §1).
            // Cleartext is permitted for this host only, via res/xml/network_security_config.xml.
            //
            // On a REAL PHONE 10.0.2.2 does not exist; override per install:
            //   adb reverse tcp:8080 tcp:8080
            //   ./gradlew installDebug -PquakeDebugBaseUrl="http://localhost:8080/"
            // (network_security_config.xml already permits cleartext localhost.)
            val quakeDebugBaseUrl = (project.findProperty("quakeDebugBaseUrl") as String?)
                ?: "http://10.0.2.2:8080/"
            buildConfigField("String", "QUAKE_BASE_URL", "\"$quakeDebugBaseUrl\"")
            // A distinct application id so a tester's drill build installs *beside*
            // the production app instead of replacing it (docs/CLIENT_SPEC.md §5.8).
            // Two reasons this matters more than convenience: the drill build is the
            // only one that subscribes to the test_alerts FCM topic
            // (data/push/PushRegistrar.kt), so replacing production with it would
            // leave a real user's phone receiving drills; and a tester must be able
            // to keep the real warning app installed while testing, because the
            // drill build is not the one that will wake them at 3am.
            //
            // Firebase resolves credentials by application id, so the suffixed
            // package needs its own entry in app/google-services.json — until it has
            // one, the drill build simply has no Firebase and falls back to the
            // WebSocket, exactly as a checkout without credentials does today.
            applicationIdSuffix = ".debug"
            versionNameSuffix = "-debug"
        }
        release {
            // ADR-0003: production transport is HTTPS/WSS only.
            buildConfigField("String", "QUAKE_BASE_URL", "\"https://api.quakealert.web.id/\"")
            // R8: shrink, optimise and obfuscate. The rules R8 cannot infer -
            // MapLibre's JNI callbacks, the serializers looked up by class, the
            // ServiceLoader-found main dispatcher - are in app/proguard-rules.pro,
            // which explains each keep rather than leaning on a blanket one.
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
            // Null when the environment holds no credentials, which leaves the APK
            // unsigned instead of failing the build. See signingConfigs above.
            signingConfig = signingConfigs.findByName("release")
        }
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_11
        targetCompatibility = JavaVersion.VERSION_11
    }
    buildFeatures {
        compose = true
        buildConfig = true
    }
}

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation(libs.androidx.lifecycle.process)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)

    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.ui.graphics)
    implementation(libs.androidx.compose.ui.tooling.preview)
    implementation(libs.androidx.compose.material3)
    implementation(libs.androidx.compose.ui.text.google.fonts)

    // REST + WebSocket transport, JSON contracts and token persistence.
    implementation(libs.okhttp)
    implementation(libs.kotlinx.serialization.json)
    implementation(libs.androidx.datastore.preferences)

    // Device position: Fused provider when Play Services is available, AOSP
    // LocationManager otherwise (id.web.quakealert.device.LocationSources).
    implementation(libs.play.services.location)
    implementation(platform(libs.firebase.bom))
    implementation(libs.firebase.messaging)
    implementation(libs.kotlinx.coroutines.play.services)

    // Basemap for the Warning, Sensors and Event Detail map cards.
    implementation(libs.maplibre.android.sdk)

    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
    androidTestImplementation(libs.androidx.junit)
    androidTestImplementation(libs.androidx.espresso.core)
    androidTestImplementation(platform(libs.androidx.compose.bom))
    androidTestImplementation(libs.androidx.compose.ui.test.junit4)
    debugImplementation(libs.androidx.compose.ui.tooling)
    debugImplementation(libs.androidx.compose.ui.test.manifest)
}