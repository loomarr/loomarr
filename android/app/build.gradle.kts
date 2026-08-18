plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
}

android {
    namespace = "tv.loomarr.tv"
    compileSdk = 35

    defaultConfig {
        applicationId = "tv.loomarr.tv"
        // ⚠ minSdk 30 is set by the OLDEST device we support, not by a preferred baseline. The
        // Nvidia Shield is frozen on Android 11 (API 30) — its last OS update was December 2021 —
        // so 30 is the floor that keeps it in. A floor includes everything above it, which is why
        // this is an Android TV client generally rather than a Shield one: Google TV, newer
        // Android TV boxes, and (untested, but Android underneath) Fire TV all clear it.
        //
        // Dropping Shield support would allow API 34+ and newer APIs. That is the actual cost of
        // this line, and it is worth paying while the Shield is a target device.
        minSdk = 30
        targetSdk = 35
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

    packaging {
        resources {
            excludes += "/META-INF/{AL2.0,LGPL2.1}"
        }
    }
}

dependencies {
    implementation(platform("androidx.compose:compose-bom:2024.12.01"))
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.ui:ui-tooling-preview")
    implementation("androidx.activity:activity-compose:1.9.3")
    implementation("androidx.lifecycle:lifecycle-runtime-ktx:2.8.7")
    implementation("androidx.lifecycle:lifecycle-viewmodel-compose:2.8.7")

    // Compose for TV. The `tv-material` artifact carries the 10-foot surfaces; `tv-foundation` is
    // deliberately NOT used — its TvLazyRow/TvLazyColumn were REMOVED in 1.0.0-alpha12 (Jan 2025)
    // and standard LazyRow/LazyColumn carry TV focus handling since Foundation 1.7.0.
    implementation("androidx.tv:tv-material:1.0.0")

    implementation("androidx.datastore:datastore-preferences:1.1.1")
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
