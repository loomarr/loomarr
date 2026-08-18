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
}
