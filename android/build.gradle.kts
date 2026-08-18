// Root build file. Plugins are declared here with `apply false` and applied per-module, which is
// the current AGP convention and keeps version resolution in one place.
plugins {
    // ⚠ AGP must be new enough for compileSdk 36. 8.7.x predates it, and the symptoms are
    // misleading: AAR-metadata errors naming unrelated androidx libraries, then "Unexpected
    // failure during lint analysis (this is a bug in lint…)" — neither says "your AGP is too old".
    id("com.android.application") version "8.9.2" apply false
    id("org.jetbrains.kotlin.android") version "2.1.0" apply false
    // Required from Kotlin 2.0: the Compose compiler ships as a Kotlin plugin rather than an AGP
    // extension version. Its version tracks the Kotlin version, so both move together.
    id("org.jetbrains.kotlin.plugin.compose") version "2.1.0" apply false
    // ktlint is the Biome analogue: one tool for format + lint, so style is decided by the tool
    // rather than argued about in review. `make android-fmt` applies it.
    id("org.jlleitschuh.gradle.ktlint") version "12.1.2" apply false
    // Screenshot tests on the JVM. Roborazzi renders real Compose through Robolectric rather than
    // an emulator, so a UI regression is caught by `./gradlew verifyRoborazziDebug` in CI with no
    // device attached — the same reason web's visual suite is worth having.
    //
    // ⚠ Held at 1.60.0 to match the runtime artifacts; see the pin note in app/build.gradle.kts.
    // From 1.61.0 the library is built against a Kotlin newer than this project's 2.1.0, whose
    // metadata a 2.1 compiler cannot read.
    id("io.github.takahirom.roborazzi") version "1.60.0" apply false
}
