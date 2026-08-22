# ==========================================================================
# QuakeAlert release (R8) rules.
#
# R8 runs in full mode and most libraries here ship their own consumer rules,
# so this file holds only what those rules cannot know about: our own
# reflectively-reached code, and the two dependencies that reach across a JNI
# or ServiceLoader boundary R8 cannot see through.
#
# Rule of thumb applied throughout: keep the narrowest thing that works. A
# blanket `-keep class **` would make every build green and every APK big, and
# would hide the next real reflection bug until a user hit it.
# ==========================================================================

# --- Crash-report readability -------------------------------------------------
# A release stack trace with no line numbers is unusable for a life-safety app:
# the alert path is asynchronous, so the frame that failed is the only clue
# about which of several coroutines died. The mapping file
# (app/build/outputs/mapping/release/mapping.txt) is what de-obfuscates these,
# so it must be archived with every release build.
-keepattributes SourceFile,LineNumberTable
-renamesourcefileattribute SourceFile

# Kept for kotlinx.serialization and for OkHttp's generic-signature reflection.
-keepattributes Signature,InnerClasses,EnclosingMethod
-keepattributes RuntimeVisibleAnnotations,RuntimeVisibleParameterAnnotations

# --- kotlinx.serialization ----------------------------------------------------
# The runtime looks a serializer up by class, so the generated $$serializer and
# the Companion that returns it must survive. Scoped to our DTO package rather
# than applied globally: those are the only @Serializable types we own, and
# naming the package means a DTO added outside it fails loudly in a release
# smoke test instead of silently at the first parse.
-keepclassmembers class id.web.quakealert.data.network.model.** {
    *** Companion;
}
-keepclasseswithmembers class id.web.quakealert.data.network.model.** {
    kotlinx.serialization.KSerializer serializer(...);
}
-if class id.web.quakealert.data.network.model.**
-keep,allowobfuscation,allowoptimization class <1>$$serializer { *; }

# Enum members are matched by name when a payload names a variant.
-keepclassmembers enum id.web.quakealert.data.network.model.** {
    <fields>;
    public static **[] values();
    public static ** valueOf(java.lang.String);
}

# --- OkHttp / Okio ------------------------------------------------------------
# Both ship consumer rules. These are the optional platform integrations OkHttp
# probes for at runtime and that we do not depend on, so R8 must be told the
# missing references are expected rather than a broken build.
-dontwarn okhttp3.internal.platform.**
-dontwarn org.conscrypt.**
-dontwarn org.bouncycastle.**
-dontwarn org.openjsse.**

# --- Coroutines --------------------------------------------------------------
# The main-dispatcher factory is found through ServiceLoader, which R8 cannot
# trace. Losing it does not fail the build; it fails the first Main-dispatched
# coroutine, which on this app is every screen.
-keep class kotlinx.coroutines.android.AndroidDispatcherFactory { *; }
-keepclassmembers class kotlinx.coroutines.** {
    volatile <fields>;
}
-dontwarn kotlinx.coroutines.**

# --- MapLibre ----------------------------------------------------------------
# The SDK is a JNI binding: native code calls back into these classes by name,
# and no static analysis can see those edges. This is the one place a broad
# keep is the correct answer rather than a lazy one.
-keep class org.maplibre.android.** { *; }
-keep interface org.maplibre.android.** { *; }
-keep class com.mapbox.** { *; }
-dontwarn org.maplibre.android.**
-dontwarn com.mapbox.**

# --- DataStore / Firebase / Play Services -------------------------------------
# All three ship consumer rules that cover their own reflection. Only the
# warnings need suppressing, from optional transitive code we never call.
-dontwarn androidx.datastore.**
-dontwarn com.google.android.gms.**
-dontwarn com.google.firebase.**

# --- Our own reflectively-reached entry points --------------------------------
# The FCM service is instantiated by the framework from its manifest string, so
# neither the class nor the callbacks it overrides may be renamed. The default
# Android rules already keep manifest components; this is here because the
# override signatures are what Firebase dispatches on.
-keep class id.web.quakealert.service.QuakeMessagingService { *; }

# Enum *names* cross a persistence boundary and are read back by valueOf:
# UnitSystem is stored in DataStore (AppSettingsRepository.setUnitSystem) and
# MainDestination is restored from saved instance state (MainScreen). An
# obfuscated name is written one build and unreadable the next, which shows up
# as a silently reset preference rather than a crash.
-keepclassmembers enum id.web.quakealert.** {
    <fields>;
    public static **[] values();
    public static ** valueOf(java.lang.String);
}
