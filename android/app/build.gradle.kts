plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
    id("org.jlleitschuh.gradle.ktlint")
    id("io.github.takahirom.roborazzi")
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

// ⚠ Baselines live in the REPO, not under build/.
//
// Roborazzi's default output is build/outputs/roborazzi, which `gradlew clean` deletes and git never
// sees. CI would then run verifyRoborazziDebug with no baseline to compare against and pass — a gate
// that cannot fail, which is worse than no gate at all.
//
// Committed PNGs also make a UI change REVIEWABLE: the diff shows the actual pixels, which is the
// whole reason to keep screenshots rather than assertions about colour values.
roborazzi {
    outputDir.set(file("src/test/screenshots"))
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
        // ⚠ Required for java.time below API 26. minSdk is 23, and Instant/Duration would compile
        // fine and then throw NoClassDefFoundError on an older device — a crash the build cannot
        // see. Desugaring back-ports them instead of hand-rolling an RFC3339 parser, which is the
        // kind of code that quietly mishandles offsets and leap seconds.
        isCoreLibraryDesugaringEnabled = true
    }

    kotlinOptions {
        jvmTarget = "17"
    }

    buildFeatures {
        compose = true
    }

    testOptions {
        unitTests {
            // ⚠ Required by Robolectric, and NOT the same switch as `isReturnDefaultValues`. This
            // one puts the module's real resources, assets and manifest on the unit-test classpath
            // so Robolectric can inflate them; the other makes stubbed framework calls return
            // null/0, which is the thing the org.json note below argues against. Compose cannot
            // render without resources, so a screenshot test needs this.
            isIncludeAndroidResources = true
        }
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

    coreLibraryDesugaring("com.android.tools:desugar_jdk_libs:2.1.5")

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

    // ── screenshot tests ────────────────────────────────────────────────────────────────────────
    //
    // Roborazzi renders real Compose on the JVM through Robolectric, so the design system is gated
    // without an emulator: CI runs `verifyRoborazziDebug` on an ordinary runner. That is the same
    // bargain web's visual suite makes, and it is what makes a styling regression a failing build
    // rather than something noticed later on a television.
    // ⚠ 1.60.0 is the NEWEST release this project can use, and the pin is load-bearing.
    //
    // From 1.61.0 Roborazzi is compiled against a newer Kotlin than this project's 2.1.0, and
    // Kotlin metadata is backward- but not forward-compatible: the 2.1 compiler cannot read it. The
    // build then fails with "Module was compiled with an incompatible version of Kotlin … expected
    // version is 2.1.0" followed by unresolved references to `captureRoboImage` and `onRoot`, which
    // read as a missing dependency rather than as the version mismatch they are.
    //
    // ⚠ Determined by COMPILING each candidate, not by reading POMs. The POM for 1.71.0 declares
    // `kotlin-stdlib 2.0.21`, which looks compatible and is not the compiler that built it — the
    // stdlib it depends on and the compiler that produced its metadata are different things.
    //
    // The alternative — moving this project to Kotlin 2.3 — drags the Compose compiler plugin with
    // it (their versions track together) and the Compose BOM is already pinned by compileSdk 36.
    // A screenshot tool should fit the toolchain, not redefine it. Same failure mode as the ktlint
    // engine pin in the root build file.
    testImplementation("io.github.takahirom.roborazzi:roborazzi:1.60.0")
    testImplementation("io.github.takahirom.roborazzi:roborazzi-compose:1.60.0")
    testImplementation("org.robolectric:robolectric:4.16.1")
    testImplementation("androidx.compose.ui:ui-test-junit4")
    debugImplementation("androidx.compose.ui:ui-test-manifest")
}
