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

## Running it without a Shield

An Android TV emulator on API 30 matches the Shield's Android 11 exactly, and boots in about 20
seconds with KVM.

```sh
sdkmanager "emulator" "system-images;android-30;android-tv;x86"
avdmanager create avd -n loomarr-tv -k "system-images;android-30;android-tv;x86" -d tv_1080p
emulator -avd loomarr-tv -no-window -no-audio -gpu swiftshader_indirect -port 5560

adb install -r app/build/outputs/apk/debug/app-debug.apk
# 10.0.2.2 is the emulator's alias for the HOST — localhost inside the VM is the VM.
adb shell am start -n tv.loomarr.tv/.MainActivity -e server http://10.0.2.2:18305
adb exec-out screencap -p > screen.png
```

**What the emulator proves:** the screen renders at 10-foot scale, the pairing handshake completes,
the token persists. **What it cannot:** hardware HEVC/AV1 decoding, surround passthrough, or real
tune latency — the emulator decodes in software. Those stay hardware questions.

## Two runtime traps the build could not catch

Both compiled cleanly and failed only when the app actually ran:

- **`viewModel()` needs a factory.** With none, it reflects on a *no-arg* constructor, so a
  ViewModel taking arguments throws `Cannot create an instance of class …` the moment the screen
  renders. `viewModel()` is generic, so the type checker cannot see it.
- **Cleartext HTTP is blocked from API 28.** A self-hosted Loomarr on a LAN is plain `http://` with
  no certificate, and without a network security config OkHttp fails with a generic exception that
  reads as "server unreachable". Note `<domain>` matches hostnames/suffixes, **not** CIDR — listing
  `192.168.0.0` does not cover `192.168.1.47`, so it must be `base-config`.

## ⚠ Local resource limits

An earlier session froze this machine by accumulation — Gradle and Kotlin daemons across many
builds, plus an emulator, scrcpy, a backend and a frontend dev server. Each was reasonable alone.

`gradle.properties` therefore pins `workers.max=4` and `parallel=false`, and gives the Kotlin daemon
its own ceiling — it forks separately from the Gradle daemon, so capping `org.gradle.jvmargs` alone
bounds only half the memory. The build still takes about 7 seconds.

```sh
make android-load    # what is already running before you add to it
make android-stop    # release this module's daemons when finished
```

**Do not pass `--no-daemon`.** It reads as the careful option and is the opposite for repeated
builds: it starts a fresh JVM per invocation, where a reused daemon is one bounded process.

Run one heavy thing at a time — the emulator or a full gate, not both — and kill the emulator when
you are done with it (`adb -s emulator-5560 emu kill`).
