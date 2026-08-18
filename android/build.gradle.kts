// Root build file. Plugins are declared here with `apply false` and applied per-module, which is
// the current AGP convention and keeps version resolution in one place.
plugins {
    id("com.android.application") version "8.7.3" apply false
    id("org.jetbrains.kotlin.android") version "2.1.0" apply false
    // Required from Kotlin 2.0: the Compose compiler ships as a Kotlin plugin rather than an AGP
    // extension version. Its version tracks the Kotlin version, so both move together.
    id("org.jetbrains.kotlin.plugin.compose") version "2.1.0" apply false
}
