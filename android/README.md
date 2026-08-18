# Loomarr for Android TV

The native client. **Android TV generally**, not one device: the Nvidia Shield is the reference
hardware and sets the `minSdk` floor, but nothing here is Shield-specific.

## What this slice does

Device pairing (§11, Shield P1) and nothing else. The app starts a pairing, shows the code and the
address on screen, polls until a human approves it in the web UI, and receives a durable token.
Playback is the next slice.

## Why it works on more than a Shield

| Concern | How |
| --- | --- |
| Device support | `minSdk 30` is a **floor** set by the Shield's frozen Android 11. Newer boxes clear it. |
| Codec capability | Probed at runtime from `MediaCodecList`, never hardcoded — a Shield reports HEVC 10-bit, a cheap stick reports h264, and each gets what it proved. |
| Identity | The server resolves `DeviceProfile → EncodePlan`; the client only reports what it can decode. |

Google TV and Chromecast are Android TV underneath and are covered. Fire TV is Android-based and
will likely sideload, but is untested and not claimed.

## Toolchain

Requires **JDK 21** and the Android SDK. JDK 26 does **not** work — AGP 8.x does not support it, and
the failure surfaces as confusing Kotlin/Gradle daemon errors rather than a version message.

```sh
sudo pacman -S jdk21-openjdk          # or your platform's JDK 21
export JAVA_HOME=/usr/lib/jvm/java-21-openjdk
export ANDROID_HOME=$HOME/Android     # cmdline-tools + platform 35 + build-tools 35
```

## Build and test

```sh
cd android
./gradlew :app:assembleDebug          # → app/build/outputs/apk/debug/app-debug.apk
./gradlew :app:testDebugUnitTest      # pairing client, against a mock server
```

## Notes worth keeping

- **`tv-foundation` is deliberately absent.** `TvLazyRow`/`TvLazyColumn` were **removed** in
  1.0.0-alpha12 (Jan 2025); standard `LazyRow`/`LazyColumn` carry TV focus handling since Foundation
  1.7.0. Most Compose-TV guide-grid tutorials online are dead code.
- **`org.json` needs a real implementation in unit tests.** The `android.jar` on a local test's
  classpath is stubbed and every method throws "not mocked". The tempting fix,
  `unitTests.isReturnDefaultValues = true`, makes stubs return null/0 instead — which would let a
  parsing bug pass as a null field. `testImplementation("org.json:json")` keeps the tests honest.
- **428 is not an error.** A pending pairing answers 428 and a dead one 404, and the device does
  opposite things with them. Collapsing them makes the TV discard a good code every few seconds.
- **`LEANBACK_LAUNCHER`, not `LAUNCHER`.** The TV home screen only shows the former; an app with
  only the phone category installs and is then unreachable.
