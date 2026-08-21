// Top-level build file where you can add configuration options common to all sub-projects/modules.
plugins {
    alias(libs.plugins.android.application) apply false
    alias(libs.plugins.kotlin.compose) apply false
    // Declared, never applied at the root: app/build.gradle.kts applies it only when
    // a google-services.json is actually present, so a checkout without Firebase
    // credentials still builds.
    alias(libs.plugins.google.services) apply false
}