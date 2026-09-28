# Android TV beta releases

Loomarr's accepted React Native TV client owns the permanent Play identity `loomarr.media`.
Prototype development builds use `media.loomarr.tv.prototype` and cannot replace a Play build.

The full rationale and current Google requirements are in
[`FINDINGS-android-tv-beta-distribution-2026-08-22.md`](FINDINGS-android-tv-beta-distribution-2026-08-22.md).

## Release identity

[`web/apps/tv/android-release.json`](../web/apps/tv/android-release.json) owns the next version
name. CI and the protected workflow independently derive and verify its Play code:

```text
major * 100000000 + minor * 1000000 + patch * 10000 + channel
```

`beta.N` uses 1–7999, `rc.N` uses 8001–8999, and a stable release uses 9999. For example,
`0.1.0-beta.1` is code `1000001`. The mapping is monotonic across beta, release candidate, stable,
and the next patch. Unsupported names fail before signing.

## Artifact and build contract

Moved verbatim from `design.md` §9.1 (#1572). The TV client's design is in
[`docs/design/playback-clients.md`](../docs/design/playback-clients.md).

`loomarr.media` is the permanent production application id for the accepted React Native Shield
replacement. Ordinary development and Storybook builds retain the isolated prototype identity;
only an explicit Shield release configuration may select the production id, application name,
launcher icon, and TV banner. Both the sideload and Play configurations use that release identity
and fail closed unless they receive a supported SemVer name and its valid derived Android version
code.

Shield client releases use SemVer names and a deterministic, increasing `versionCode`. The code
allocates two decimal digits each to minor and patch and four release slots within a patch:
`major * 100000000 + minor * 1000000 + patch * 10000 + channel`, where `beta.N` occupies 1–7999,
`rc.N` occupies 8001–8999, and the stable release is 9999. Major is bounded to 20 so every result
stays below Android's version-code ceiling. The build derives the code from the version name; an
operator does not type two independent identities that can drift.

The sideload artifact is a signed APK containing the production React Native entry and only the
`arm64-v8a` native libraries required by the Shield. The Play producer compiles one unsigned Android
App Bundle from the same React Native TV source, the exact merge-result commit, and a
source-controlled release identity. It contains `armeabi-v7a`, `arm64-v8a`, `x86`, and
`x86_64`; every packaged 64-bit ELF LOAD segment is aligned for 16 KiB pages. Android's 16 KiB
devices are 64-bit, so the required `arm64-v8a` and `x86_64` libraries carry that alignment while
the separately required 32-bit TV ABIs retain their platform alignment. CI verifies package, name,
code, launcher activity, TV launcher metadata, icon/banner resources, embedded startup identity,
JavaScript bundle, ABI set, and the unsigned artifact digest, then retains that bundle with evidence
bound to the exact workflow run and commit. Before release dispatch, the maintainer's compile-free
emulator harness verifies the same digest, installs device-specific splits, and supplies the visible
clean-install, discovery, manual fallback, startup-animation, pairing, playback, and playbar evidence
that archive inspection cannot. The protected Internal-release job downloads that immutable artifact
by id, rejects missing/expired/ambiguous provenance, digest drift, any pre-existing signature, and
unexpected `META-INF` material, signs it with the durable upload key using the pinned JDK, proves
every non-signature ZIP entry is unchanged, and re-runs the certificate-bound verifier before
optional publication. It performs no Gradle, CMake, Expo prebuild, Node installation, or Apple build.
There is no rebuild fallback and no name-only/latest-artifact selection. The sideload path still
requires all four keystore inputs and records the same applicable artifact evidence. Local release
tests create ephemeral signing material. The sideload test
also cleanly uninstalls any prior `loomarr.media` package from a Loomarr-owned Android TV emulator,
installs the APK, and cold-launches the Leanback activity.

Android build performance (#1050) is measured without changing the artifact contract. The
`android-profile` Make target runs the normal four-ABI Android gate, retaining runner identity,
wall time, actual Gradle settings and local `--profile` reports in a separate diagnostic artifact.
It never uses an externally uploaded build scan or adds diagnostic files to the unsigned promotion
artifact. Local builds default to one native worker and one Gradle worker. CI runs at most two
Gradle projects in parallel while retaining one native compiler/link slot inside each task; the
wrapper rejects any Gradle worker count other than one or two. The bounded hosted experiment cut
fresh-source builds from the 30m56s one-worker cold control to 19m13s and 16m15s with zero OOM event
deltas, all four ABIs, and the same verified artifact. A measured three-worker candidate regressed to
19m07s and is rejected. Release continues to promote the already verified producer artifact.

The TV application does not import Reanimated or Worklets and therefore must not declare either as
a direct dependency. Expo autolinking treats a direct declaration as native application authority:
the otherwise-unused modules added 9m39s and 3m29s respectively to a measured warm four-ABI build,
and 832 precompiled-header compiler calls bypassed ccache. The workspace compatibility overrides
still pin their exact Expo-supported versions for transitive development tooling. The Android gate
verifies the generated TV graph and complete artifact rather than shipping unused native modules as
a compatibility precaution.

The mobile application likewise does not import Reanimated or Worklets. Expo Router retains them as
transitive optional peers, and SDK 54 or newer searches transitive React Native dependencies, so
removing only the direct manifest entries does not remove them from the generated native graph. The
mobile manifest must both omit the unused direct dependencies and exclude the two package names from
**Android** autolinking. The exclusion is application-scoped and platform-scoped rather than a
workspace package removal: exact workspace overrides keep the supported versions available to
Storybook and other transitive tooling. Apple excludes only Reanimated, while Gesture Handler can
still satisfy its conditional `RNWorklets` dependency. The standalone embedded mobile APK remains
the Android native acceptance artifact.

The producer may additionally use the §14-pinned ccache executable through the generated Expo/CMake
plugin. CI requires an absolute verified launcher, content-based compiler identity, a checkout-relative
base directory, no permissive sloppiness, and a bounded dedicated cache directory. Local release tests
acquire the same exact macOS or Linux pin into the worktree artifacts by default, reuse compiler results
on later builds, and retain an explicit or acquisition-failure cold path. Only compiler results are
restored across source identities. Generated Android projects, `.cxx` trees, bundles, keys, and
promotion evidence are never cached. The profiler retains exact version/configuration, zeroed pre/post
JSON statistics, and every primary generated Ninja rules file used to prove launcher propagation
across app and library projects. Nested compiler-capability probes are not application/library rules
and do not participate in that proof. Pull-request and merge-queue refs restore compiler objects from
the default-branch cache but cannot publish into that shared scope. After a successful Android
merge-queue build lands, that producer transfers only its bounded ccache directory plus a
commit/run/workflow/key/tree-digest manifest. A trusted `push` workflow on the exact admitted main
commit validates the successful merge-group run, immutable transfer, and manifest before publishing
one rolling default-branch cache generation. It performs no Gradle, CMake, Expo, Node, or product
build, deletes the one-day transfer, and retires superseded Android main-cache generations.

On Linux, the observer records its inherited cgroup v2 memory scope, limits, lifetime peak and
OOM/limit event counters before and after the build, plus sampled current usage and host available
memory. The lifetime peak is an upper bound for that scope, not a reset or isolated phase peak;
sampled peaks can miss short spikes. Unavailable metrics remain explicit and cannot qualify a
memory-safety claim. A single process's RSS is not aggregate compiler/Gradle memory. Observation does
not change cgroup limits, build concurrency, JVM heap, caches, ABI scope or artifact checks.

The accepted replacement is installed on the maintainer's Shield by removing the Kotlin application,
sideloading the React Native APK, and pairing again. That physical journey has been accepted. The
same permanent package now also has an Internal-testing-only Google Play path: Google manages the
app-signing key, Loomarr protects a durable upload key in the reviewed GitHub environment, the first
bundle may be uploaded manually for Console bootstrap, and later uploads use a package-scoped service
account with no Production permission. The workflow has no open, closed, staged, or Production track
choice. Because the accepted sideload used an intentionally ephemeral key, a Play install may require
one more uninstall and fresh pairing; cross-channel signature continuity is not promised.

Kotlin/Compose source, Gradle build files, generated Kotlin tokens, JVM screenshot references, and
their dedicated CI lane are deleted only after the React Native sideload acceptance and React Native
Play bundle verification exist in the same ancestry. Distribution-neutral store descriptions and
artwork remain generated from the shared brand contract outside the retired Kotlin tree. Preserving
installed credentials, public Play distribution, staged rollout, cross-channel in-place updates, and
rollback machinery remain outside this program.

## One-time Play Console bootstrap

An account owner must do these steps; the Publishing API cannot create an application or accept
legal declarations.

1. Create **Loomarr** in Play Console and confirm the unclaimed package `loomarr.media` by uploading
   the first AAB. Treat the package as permanent.
2. Enrol in Play App Signing and let Play generate the app-signing key. Record both the Play
   app-signing SHA-256 fingerprint and Loomarr upload-key SHA-256 fingerprint in the release record.
3. Add Android TV as a form factor and complete App access, Data safety, content rating, target
   audience, ads, privacy policy, and the other required Console declarations truthfully.
4. Supply the 512 × 512 Play icon, 1024 × 500 feature graphic, separate 1280 × 720 TV banner, at
   least one real TV screenshot, and a description that names Android TV. Four real 1920 × 1080
   screenshots are the listing target.
5. Create the Internal tester list, add the Shield's Google account, feedback address, and review
   access instructions for a working Loomarr server.

The upload keystore is an upload credential, not the Play app-signing key. Back it up outside the
repository. Losing it requires an upload-key reset; losing the Play account is a different and more
serious incident.

## Protected GitHub environment

Create the `android-beta` environment with required reviewers and no unprotected deployment
branches. Add these environment secrets:

| Name | Value |
| --- | --- |
| `ANDROID_UPLOAD_KEYSTORE_BASE64` | Base64 of the upload PKCS12/JKS bytes |
| `ANDROID_UPLOAD_KEYSTORE_PASSWORD` | Upload keystore password |
| `ANDROID_UPLOAD_KEY_ALIAS` | Upload key alias |
| `ANDROID_UPLOAD_KEY_PASSWORD` | Upload private-key password |
| `GOOGLE_PLAY_SERVICE_ACCOUNT_JSON_BASE64` | Added only after manual bootstrap; base64 service-account JSON |

Add environment variable `ANDROID_UPLOAD_CERT_SHA256` with the upload certificate's SHA-256
fingerprint. The workflow compares it with both the keystore and signed AAB. A wrong key fails before
publication.

The service account is invited to Play Console only after the first manual AAB exists. Restrict it
to `loomarr.media` and testing-track release rights; do not grant account administration or
Production access. Enable the Google Play Developer API in its Cloud project.

## Artifact production and first signed AAB

Changing the release identity or any Android input makes the merge queue compile one unsigned AAB.
CI verifies and retains it with its exact commit, workflow run, immutable artifact id, and SHA-256
digest. Before the first automated publication, run **Android TV beta** from that exact `main` with:

- **Publish to Play** disabled.

The protected workflow accepts only the unique, unexpired Android artifact produced for its exact
current-main commit. It downloads by immutable artifact id, verifies source/run/digest and unsigned
state, signs with the protected upload key, proves all non-signature entries stayed byte-identical,
then repeats package, version, ABI, 16 KiB, launcher, banner, JavaScript, and certificate checks. It
retains the signed AAB and evidence for 30 days. It does not install Node, run Expo, Gradle, CMake, or
any Apple build. Download that first signed AAB and upload it manually only while enrolling in Play
App Signing; subsequent Internal releases use the publisher.

Do not use Internal App Sharing for acceptance; it re-signs artifacts with a disposable identity and
does not prove the beta update path.

## Automated testing-track release

After the manual bootstrap and service-account setup, dispatch the workflow with **Publish to Play**
enabled. The publisher opens one Play edit, uploads the exact signed and digest-verified AAB, replaces the
Internal track release, and commits the edit. Global concurrency prevents two edits from racing.
The workflow exposes no Closed, Open, or Production choice.

Before dispatch, the exact CI artifact must pass the clean-install journey on a Loomarr-owned Android
TV emulator on the maintainer's machine. The journey starts with empty app data and no embedded
server URL, observes the launcher artwork and process-dead launch animation, uses automatic LAN
discovery (plus the manual fallback in a separate case), completes pairing, restarts into Watching,
proves the playbar hides after five seconds of remote inactivity, and observes a server-authored
programme → named filler clip → programme transition in Watching chrome. Agents never use the
physical Shield for this release gate.

Download the exact merge-queue artifact, then run the compile-free acceptance harness with one
explicit emulator serial:

```bash
LOOMARR_TV_EMULATOR_SERIAL=emulator-5554 \
  ./scripts/test-android-release-emulator.sh \
  .artifacts/android-ci/loomarr-tv-<version>-<code>-unsigned.aab \
  .artifacts/android-ci/loomarr-tv-<version>-<code>-unsigned.json
```

The harness refuses physical-device serials, verifies the producer digest and bundle identity,
creates device-specific APK splits with a disposable local key, and installs only those splits. It
then drives automatic DNS-SD pairing and a separate manual-address pairing against an isolated
fixture, checks the five-second playbar deadline and programme/filler/programme identity, and retains
screenshots, a paired cold-launch recording, and digest-bound acceptance evidence under
`.artifacts/emulator-proof/`. It downloads
the pinned official bundletool only when `LOOMARR_BUNDLETOOL_JAR` is not supplied; no application
source is compiled.

If publication fails, inspect the Play edit error and dispatch a new version only after determining
whether Play consumed the code. Never lower or reuse a code. Halt a bad rollout in Play Console;
Android cannot downgrade an installed app, so rollback is a new fixed version with a higher code.

## Shield acceptance

The physical Shield acceptance journey already passed with the React Native `loomarr.media`
sideload. That build used an ephemeral signing key, so Android may require it to be uninstalled
before the separately signed Play build can be installed. Losing that local pairing is accepted:

1. Opt the Shield account into the Internal test. If Android rejects the Play install because the
   signatures differ, uninstall the accepted sideload, then install Loomarr from its Play link.
2. Confirm Settings reports package `loomarr.media`, the expected version code/name, and the Play
   signing certificate. Pair it afresh to the production Loomarr server when needed.
3. Exercise playback, Guide, Surf, D-pad focus, Back-to-home, process restart, device restart, and
   both 1080p and 4K output. Record the installed version and evidence.
4. Revoke obsolete paired-device entries if the server still lists them.

An in-place second-version update proof and wider Play tracks are later release work, not acceptance
criteria for this Internal beta.
