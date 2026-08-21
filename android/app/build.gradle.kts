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

    buildTypes {
        debug {
            // Emulator loopback to the Go server on the host (docs/CLIENT_SPEC.md §1).
            // Cleartext is permitted for this host only, via res/xml/network_security_config.xml.
            buildConfigField("String", "QUAKE_BASE_URL", "\"http://10.0.2.2:8080/\"")
        }
        release {
            // ADR-0003: production transport is HTTPS/WSS only.
            buildConfigField("String", "QUAKE_BASE_URL", "\"https://api.quakealert.id/\"")
            isMinifyEnabled = false
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
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

    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
    androidTestImplementation(libs.androidx.junit)
    androidTestImplementation(libs.androidx.espresso.core)
    androidTestImplementation(platform(libs.androidx.compose.bom))
    androidTestImplementation(libs.androidx.compose.ui.test.junit4)
    debugImplementation(libs.androidx.compose.ui.tooling)
    debugImplementation(libs.androidx.compose.ui.test.manifest)
}