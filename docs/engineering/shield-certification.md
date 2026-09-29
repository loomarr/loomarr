# Shield certification

**For:** whoever certifies the TV app on a real device (#1037) against the #1512 surf and no-blip gates.
**You'll get:** how to build an instrumented APK, the exact commands to run, and how to read the numbers.

The TV app writes one `LoomarrCert` line to logcat per playback event. Two scripts drive the device over
adb and turn those lines into the gate numbers:

| Gate | Threshold | Script | Measured from |
| --- | --- | --- | --- |
| G3 warm surf | p95 ≤ 600 ms | `scripts/shield-cert/surf.sh` | remote key → first decoded frame, tunes that reused a warmed neighbour |
| G3 cold surf | p95 ≤ 1.5 s | `surf.sh` with `JUMP` | number entry commit → first decoded frame, tunes that minted |
| G3 held frame / OSD | p95 ≤ 100 ms | `surf.sh` | remote key → switch readout committed (`held OSD`); the still is reported beside it |
| G4 stalls | < 1 per viewer-hour over 24 h | `scripts/shield-cert/soak.sh` | buffering after an attempt's first frame while not paused |
| G4 codec re-inits | 0 at programme ↔ commercial boundaries | `soak.sh` | decoder instantiations outside a tune window |

## The marks

`web/packages/player/src/playback-marks/` formats them; the native transport
(`web/packages/player/src/native/native.tsx`), the player controller's `onTune` report and the switch
overlay's `onShown` feed it; `web/apps/tv/src/app.tsx` wires them. A line is `key=value` tokens after a
fixed prefix, on one monotonic clock (`performance.now()`, milliseconds, per app process):

```text
LoomarrCert v=1 ev=tune t=748024.4 att=39 ch=<channel id> key=748024.3 num=102 path=warm why=step
```

| `ev` | Written when | Fields |
| --- | --- | --- |
| `tune` | an attempt starts (the controller published its tuning snapshot) | `att`, `ch`, `num`, `why` (step, number, channel, previous, retry, catalog), `path` (warm = a warmed neighbour's source, cold = minted), `key` (the remote key's time, when a key caused it) |
| `held` | the switch overlay committed its "CH n / TUNING IN" readout (`what=osd`) or its still finished loading (`what=still`) | `att`, `what` |
| `first-frame` | the native view rendered the attempt's first frame | `att` |
| `stall-start` / `stall-end` | ExoPlayer went back to buffering after the first frame while the viewer had not paused, and came back | `att`; end adds `dur` and `why` (resumed, retuned, released, error) |
| `error` | the player reported an error | `att`, `cause` (a closed token such as `http_404` or `network`; never the native message, which can carry the signed URL) |
| `format` | ExoPlayer's video input format changed (`mime=none` when a replace clears the track) | `att`, `mime`, `w`, `h`, `fps`, `br` |

Lines carry ids, numbers and closed tokens only: never a URL, a title or an error message.

**When they are on.** Always in a development build (`__DEV__`). A release build writes them only when
its JavaScript bundle was built with `EXPO_PUBLIC_LOOMARR_CERT_MARKS=1` (a build-time flag; Metro inlines
it, so there is no runtime switch to leave on by accident). Off, the marks are a shared no-op object and
the transport does not even subscribe to track changes.

**They survive release builds.** React Native sends `console.info` to logcat under the tag
`ReactNativeJS` at priority I in a release build too. Checked on 2026-09-28 with the CI-built,
minified release bundle (`make android`, converted as below) on the Android 11 TV emulator.

## Build the APK

Build the release bundle the same way CI does, with the flag set, then turn the unsigned bundle into one
installable APK signed with the Android debug key:

```sh
EXPO_PUBLIC_LOOMARR_CERT_MARKS=1 ANDROID_HOME=~/Android JAVA_HOME=/usr/lib/jvm/java-21-openjdk \
  flock /tmp/loomarr-heavy-gate.lock make android
# -> .artifacts/android-ci/loomarr-tv-<version>-<code>-unsigned.aab

java -jar bundletool.jar build-apks --mode=universal \
  --bundle=.artifacts/android-ci/loomarr-tv-<version>-<code>-unsigned.aab --output=cert.apks \
  --ks=$HOME/.android/debug.keystore --ks-pass=pass:android \
  --ks-key-alias=androiddebugkey --key-pass=pass:android
unzip -o cert.apks universal.apk
```

The APK is the permanent identity (`loomarr.media`) with all four ABIs. A copy of `loomarr.media` signed
with another key refuses the update (`INSTALL_FAILED_UPDATE_INCOMPATIBLE`), so uninstall it first; that
drops its pairing. `adb install -r` can report success and keep the old APK, so compare checksums:

```sh
adb -s "$ADB_SERIAL" uninstall loomarr.media
adb -s "$ADB_SERIAL" install universal.apk
adb -s "$ADB_SERIAL" shell md5sum "$(adb -s "$ADB_SERIAL" shell pm path loomarr.media | tr -d '\r' | sed 's/package://')"
md5sum universal.apk
```

Pair the app with the server under test and leave it on Watching. Enter the address by hand: LAN discovery
also lists every other Loomarr server on the network.

## Run

Both scripts are Bash 3.2 safe (macOS and Linux), need only `adb` and `awk`, and name the device on every
adb call: `ADB_SERIAL` is required, because a desk often has an emulator and a real device attached at once.

```sh
# G3 warm: CHANNEL_UP every 5 s for 10 minutes (KEYS=dpad steps with D-pad up instead)
ADB_SERIAL=<serial> MINUTES=10 INTERVAL=5 scripts/shield-cert/surf.sh

# G3 cold: jump by number between channels nobody warmed (the entry commits 1.2 s after the digits)
ADB_SERIAL=<serial> MINUTES=10 INTERVAL=6 JUMP="101 104 102 105 103 106" scripts/shield-cert/surf.sh

# G4: 24 h, next channel every 30 minutes; Ctrl-C stops early and still reports
ADB_SERIAL=<serial> HOURS=24 ROTATE_MINUTES=30 scripts/shield-cert/soak.sh
```

Each run writes `marks.log` (the kept lines), a JSON report (`surf.json` or `soak.json`) and
`summary.txt` under `.artifacts/shield-cert/` (`OUT` overrides). The capture keeps only `LoomarrCert`
lines and codec lines, so nothing else from the device is written down. The soak rewrites its report
every `REPORT_MINUTES` (default 10), re-attaches after an adb disconnect (`adb connect` for a network
serial) and resumes the capture after the last line it kept, and relaunches the app whenever it is not in
the foreground. `MINUTES` shortens a soak for a trial; the verdict still judges against `HOURS`.

## Reading the numbers

- **Latency** runs from the key (the app handling the remote's key-up, where it acts) to the first frame.
  A number jump runs from the entry's commit instead, so the deliberate 1.2 s entry window is not counted.
- **Unfinished surfs** (no first frame before the next key) count as infinitely slow: they can raise a
  percentile, never hide. Give `INTERVAL` room above the cold threshold.
- **Cold by format.** A cold surf whose first picture is 2160 lines or taller (the 4K premium) is judged
  against its own 2.5 s ceiling (`cold4k`, `cold4kP95Max2500`); every other cold surf, and one that
  never reported a picture, against the 1.5 s baseline. Warm has one 600 ms ceiling for every format.
- **Refused surfs** logged an error (the server's `http_503`, say) and never framed, not even through the
  player's own retries. They have no latency to time, so they sit outside the percentiles, are counted by
  cause (`refusedBy`), and fail their own gate (`refusedMax0`). A surf that a retry recovered is timed
  from its key to the retry's first frame.
- **Held OSD** is when React committed the switch overlay, not when the panel lit: allow a frame or two.
  **Held still** is when the channel's still image finished loading behind the readout.
- **Codec re-inits.** ExoPlayer logs `DMCodecAdapterFactory: Creating an asynchronous MediaCodec adapter
  for track type video|audio` each time it creates a decoder, whatever the vendor stack (OMX or Codec2),
  in release builds too. The soak counts those lines from the app's own process: inside a tune window
  (500 ms before the tune to 2 s after its first frame) they are the tune's; anywhere else they are
  mid-play re-inits, which G4 requires to be zero. `formatChangesMidAttempt` counts video format changes
  inside one attempt, the usual cause. If a device logs decoder creation differently, set `CODEC_INIT_RE`
  to an extended regex for one line per decoder. A soak that tuned but matched no codec line reports
  `UNVERIFIED`, never zero.
- **Viewer time** is first frame to the next tune; after an app restart the old process's time stops
  where the new one's first line appears.
- The verdict is `PASS` only for a complete run; a clean short run is `INCOMPLETE`.

`scripts/shield-cert/test.sh` checks the analyser against fixed captures (`release-verify` runs it).

## Emulator reference run

The Android 11 x86_64 TV emulator (`loomarr-tv-x64`, software GPU) against a lane backend with the demo
library, release APK built as above, 2026-09-28, with the previous card-style switch overlay (#1770
has since replaced it with the B4 readout). The emulator decodes differently from the Shield, so these
numbers prove the tooling, not the gates.

| Run | Result |
| --- | --- |
| Step surf, `CHANNEL_UP` every 5 s, 3 min | 36 of 36 surfs warm; warm p50 524 ms, p95 653 ms; held card p50 7 ms, p95 13 ms; still p50 220 ms, p95 412 ms; 0 stalls |
| Jump surf, 6 channels, every 6 s, 2 min | 20 surfs, 17 cold; cold p50 532 ms, p95 2482 ms; held card p95 29 ms; still p95 515 ms |
| Soak, 8 min, rotating every 2 min, one forced adb disconnect and one app kill | 0.132 viewer-hours, 5 tunes, 2 app processes, 1 reconnect, 1 relaunch; 0 stalls; 4 decoder inits, all at tunes (ExoPlayer reused decoders across some tunes), 0 mid-play; verdict `INCOMPLETE` (short run) |

## Known gap

On the emulator, pressing OK right after typing a channel number tuned the channel and also opened the
guide (cause not yet traced). The guide covers the switch overlay, so no `held` mark appears and later
digits land in the guide. `surf.sh` lets
the entry commit on its own to stay clear of it.
