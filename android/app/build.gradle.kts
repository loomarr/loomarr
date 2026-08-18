plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
    id("org.jlleitschuh.gradle.ktlint")
}

// ⚠ ktlint's formatter and its checker disagree, so the FORMATTER wins by decision.
//
// `ktlintFormat` writes `suspend fun start(...): Pairing = withContext(Dispatchers.IO) {` — the
// function-signature rule prefers a body on the signature line when it fits — and `ktlintCheck`
// then rejects that exact output under multiline-expression-wrapping. Left alone,
// `make android-fmt` produces code `make android` refuses: a gate nobody can satisfy, which is
// worse than no gate at all.
//
// Set here rather than in `.editorconfig` because that route did not take effect — the property
// was read and the rule kept firing. This is the mechanism that actually applies.
ktlint {
    // ⚠ Pin the ENGINE, not just the plugin. Plugin 12.1.2 bundles a ktlint engine older than
    // Kotlin 2.1, and its embedded parser then fails on syntax the Kotlin 2.1 compiler accepts —
    // reported as "KtLint failed to parse file" for a file that compiles cleanly, which reads as a
    // code error rather than the version mismatch it is.
    version.set("1.5.0")
    additionalEditorconfig.set(
        mapOf(
            "ktlint_standard_multiline-expression-wrapping" to "disabled",
            // @Composable functions are PascalCase by Compose convention (`PairingScreen`), which
            // ktlint's function-naming rule — written for ordinary functions — flags.
            "ktlint_standard_function-naming" to "disabled",
        ),
    )
}

android {
    namespace = "tv.loomarr.tv"
    compileSdk = 36

    defaultConfig {
        applicationId = "tv.loomarr.tv"
        // Matching Jellyfin's Android TV client, which ships these levels and supports the Shield.
        //
        // ⚠ minSdk and targetSdk answer DIFFERENT questions, and conflating them is the usual
        // mistake. minSdk is the oldest device allowed to install; targetSdk is the platform
        // behaviour set the app is written against. Targeting far above the floor is normal — the
        // Shield (API 30) runs a targetSdk 36 app fine, because targetSdk changes how the platform
        // treats the app, not which platforms accept it.
        //
        // 23 rather than 30 costs almost nothing here: Compose, Media3, DataStore and coroutines
        // all support it, and it widens the device range at no design cost.
        minSdk = 23
        targetSdk = 36
        versionCode = 1
        versionName = "0.1.0"
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }

    buildFeatures {
        compose = true
    }

    lint {
        // Fail the build on real defects — the API-level errors this caught before minSdk dropped to
        // 23 are exactly the class worth gating on.
        abortOnError = true
        warningsAsErrors = true

        disable +=
            setOf(
                // Dependency versions are PINNED deliberately: the newest Compose BOM and lifecycle
                // releases require compileSdk 37, which the SDK manager does not offer yet. Lint is
                // right that newer versions exist and wrong that we can build against them, so this
                // warning is noise until API 37 ships.
                "GradleDependency",
                "AndroidGradlePluginVersion",
                // The cleartext allowance in network_security_config.xml is a considered decision,
                // not an oversight: a self-hosted Loomarr on a LAN is plain http:// with no
                // certificate, and the file documents the trade. Lint cannot see that reasoning.
                "InsecureBaseConfiguration",
                // "Should not restrict activity to fixed orientation" is advice for phones and
                // foldables. A television has one orientation, and the app declares leanback +
                // touchscreen-not-required, so it never reaches a device this would help.
                "DiscouragedApi",
                // Adaptive-icon polish for launchers this app does not appear in — the TV home row
                // uses the banner, not the launcher icon.
                "MonochromeLauncherIcon",
                // Backup rules for an app whose only stored state is a device token that is
                // revocable server-side and re-obtainable by pairing again.
                "DataExtractionRules",
                // The placeholder banner/launcher art is vector; real artwork replaces it.
                "VectorRaster",
            )
    }

    packaging {
        resources {
            excludes += "/META-INF/{AL2.0,LGPL2.1}"
        }
    }
}

dependencies {
    // ⚠ Pinned BELOW the newest BOM on purpose. 2026.08.00 pulls Compose 1.12, which requires
    // compileSdk 37 — a platform the SDK manager does not offer yet, so the newest BOM simply
    // cannot build. Android Lint reports the newer version as available and is right that it
    // exists; it does not know the platform to compile it against does not.
    implementation(platform("androidx.compose:compose-bom:2025.06.01"))
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.ui:ui-tooling-preview")
    implementation("androidx.activity:activity-compose:1.10.1")
    implementation("androidx.lifecycle:lifecycle-runtime-ktx:2.9.4")
    implementation("androidx.lifecycle:lifecycle-viewmodel-compose:2.9.4")

    // ⚠ NO `androidx.tv` (tv-material / tv-foundation). Jellyfin's Android TV client — the mature
    // comparable, shipping on Shield hardware for years — imports none of it, using plain
    // compose.foundation plus its own design tokens. Two reasons that is the better bet here:
    // tv-material 1.0.0 is young, and `tv-foundation` already removed TvLazyRow/TvLazyColumn in
    // 1.0.0-alpha12 (Jan 2025) after tutorials were written against them. Standard LazyRow/
    // LazyColumn have carried TV focus handling since Foundation 1.7.0, so the TV-specific layer
    // buys styling we already get from LoomarrTokens.
    implementation("androidx.compose.foundation:foundation")
    implementation("androidx.compose.material3:material3")

    // Media3 for playback. Jellyfin is Media3-only as of 2026 (no embedded libVLC), which is the
    // simplest stack that plays HLS on Shield-class hardware.
    implementation("androidx.media3:media3-exoplayer:1.11.0")
    implementation("androidx.media3:media3-exoplayer-hls:1.11.0")
    implementation("androidx.media3:media3-ui:1.11.0")

    // QR generation for the pairing screen. `core` ONLY — the `android-core`/`zxing-android-embedded`
    // wrappers exist for SCANNING, which needs a camera this app does not have and does not want.
    // Encoding is pure Kotlin/Java, and the matrix is drawn with Compose so the code uses our own
    // tokens rather than a bundled black-on-white bitmap.
    implementation("com.google.zxing:core:3.5.3")

    implementation("androidx.datastore:datastore-preferences:1.1.7")
    implementation("com.squareup.okhttp3:okhttp:4.12.0")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.9.0")

    testImplementation("junit:junit:4.13.2")
    testImplementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.9.0")
    testImplementation("com.squareup.okhttp3:mockwebserver:4.12.0")
    // ⚠ org.json lives in the Android framework, and the android.jar on a local unit test's
    // classpath is STUBBED — every method throws "not mocked". The alternative fix,
    // `unitTests.isReturnDefaultValues = true`, makes those stubs return null/0 instead, which
    // would let a parsing bug pass as a null field rather than fail. A real implementation means
    // these tests exercise the parsing they claim to.
    testImplementation("org.json:json:20240303")
}
