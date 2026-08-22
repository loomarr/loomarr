# FINDINGS — Android TV beta distribution (2026-08-22)

This note records the current Google Play requirements and a recommended release path for Loomarr's
Android TV client. It uses Google and Android first-party documentation only.

## Recommendation

Reserve `loomarr.media` in Play Console and treat it as permanent. Bootstrap the app manually,
enroll it in Play App Signing, upload the first signed Android App Bundle (AAB), opt in to Android
TV, and release first to the **Internal testing** track. After the Shield install and one in-place
Play update are proven, promote the same release process to a named **Closed testing** track.

Do not use Internal App Sharing as the beta channel. It re-signs uploads with a separate internal
sharing key, permits reused version codes, limits each link to 100 downloads, and expires links after
60 days. Those properties make it useful for disposable artifact checks, not for proving the real
signing and upgrade path. See [Share app bundles and APKs internally][internal-sharing].

## Permanent identity and signing

- Play package names are unique, permanent, cannot be deleted, and cannot be reused. Confirm the
  developer account and reserve `loomarr.media` before automating anything. Use `loomarr.media` for
  both the Kotlin namespace and installed application identity so current code carries no legacy
  package name. See
  [Create and set up your app][create-app].
- New apps must use Play App Signing. The release AAB is signed locally with an **upload key**;
  Google holds the distinct **app signing key** and signs the APKs installed on devices. Keep the
  upload keystore and passwords outside Git, back them up, and expose them to CI only as protected
  release credentials. A lost or compromised upload key can be reset without changing the app
  signing key. See [Sign your app][app-signing] and [Upload your app][upload-bundle].
- Let Play generate and protect the app signing key unless Loomarr has a concrete requirement to
  distribute updates through another store under the same certificate. Google says to provide your
  own signing key at enrollment when cross-store use of the same signing identity is required.
- Every successive release needs a larger positive `versionCode`; Play rejects a code already used,
  and Android blocks installation of a lower code over a higher one. The current hard-coded value
  `1` therefore needs a monotonic CI-owned scheme before the second upload. `versionName` remains a
  user-facing label and need not drive ordering. See [Version your app][versioning].

### Migration of the current Shield install

The Shield currently has `tv.loomarr.tv` installed with Android's debug certificate. The Play build
uses the distinct `loomarr.media` package and Play app-signing certificate, so it can be installed
alongside that historical build. The one-time migration is therefore:

1. Record the configured server URL if desired.
2. Opt the Shield's Google account into the Internal test and install `loomarr.media` from Google
   Play without removing the historical app.
3. Pair and validate the Play build independently.
4. Remove `tv.loomarr.tv` only after the Play build passes, then remove its obsolete server device
   authorization if it remains.

Future beta releases then update `loomarr.media` in place through Play. Debug builds use
`loomarr.media.debug`, so debug and Play installs remain separate. The certificate rule is documented
under [Signing considerations][app-signing].

## Artifact and platform requirements

- All TV apps and updates on Google Play must be AABs; APK updates for TV stopped being accepted on
  June 30, 2023. The AAB is an upload format, and Play generates device-specific signed APKs from it.
  See [About Android App Bundles][app-bundle] and
  [Latest releases and bundles][latest-bundles].
- As of August 22, 2026, Android TV submissions must target at least Android 14 / API 34. Loomarr
  currently targets API 36, so it clears the TV-specific requirement that remains in effect after
  the August 31, 2026 policy change. See [Target API requirements][target-api].
- Since August 1, 2026, a TV app must support both 32-bit and 64-bit architectures and 16 KB page
  sizes. Loomarr's source is Kotlin/Java and does not declare NDK code, which is compatible in
  principle, but the release gate should inspect the resolved AAB for transitive `.so` libraries.
  If any exist, verify their ABIs and 16 KB alignment; test the delivered build on a 64-bit device
  and a 16 KB emulator. See [TV app quality][tv-quality], [Support 64-bit architectures][support-64]
  and [Support 16 KB page sizes][support-16k].
- The TV quality floor also requires `minSdkVersion` 31 or lower; Loomarr's `minSdk = 23` qualifies
  and retains compatibility with the API 30 Shield.

## Android TV manifest, quality, and listing

The current manifest already contains the core discovery declarations: required
`android.software.leanback`, touchscreen not required, a landscape activity with
`MAIN` + `LEANBACK_LAUNCHER`, and an application banner. The checked-in launcher banner is 320×180
and includes the Loomarr name. Preserve these checks in release validation.

Before Play review, verify the complete Tier 3 TV Ready checklist, especially:

- the app appears in the TV launcher with a 320×180 full-size banner and at least a 160×160 xhdpi
  icon, and the banner contains the app name;
- every function is reachable with five-way D-pad navigation, without relying on a remote Menu
  button; Back ultimately returns to Android TV home;
- the landscape UI fills the screen, has a non-transparent background, and clips neither text nor
  controls at the edges;
- playback and Ambient Mode behavior, low-RAM behavior, installation, startup, and stability meet
  the TV criteria; and
- if review requires access to a configured Loomarr instance, provide working review access and
  instructions in Play Console's App Access section.

The authoritative checklist is [TV app quality][tv-quality]. Distribution additionally requires
adding Android TV as a form factor, uploading the AAB, opting in to TV review, and supplying:

- at least one unaltered, accurate Android TV screenshot (four 1920×1080 screenshots are the
  stronger listing target);
- a separate Play listing TV banner, JPEG or 24-bit PNG without alpha at **1280×720** — this is not
  the same artifact as the in-app 320×180 launcher banner;
- the normal 512×512 Play icon and 1024×500 feature graphic; and
- store description text that mentions Android TV.

See [Distribute to Android TV][distribute-tv] and
[Add preview assets][preview-assets]. Opting in submits the app for review against the TV quality
criteria. A dedicated TV release track is irreversible once selected, so it is unnecessary for this
TV-only package; use the app's ordinary testing tracks unless another form factor is later added.

## Tester workflow

Google recommends starting with Internal testing and then expanding to Closed testing:

1. Create an Internal tester email list (maximum 100 Google or Workspace accounts), add the Shield's
   Play account, provide a feedback URL or email, and publish the first AAB.
2. Share the track opt-in link. An internal/closed app that has not reached production is not
   searchable, and every tester must opt in before using its Play Store install link.
3. On the Shield, accept the test, install from Play, pair with Loomarr, and exercise playback,
   guide, surf rail, focus, Back, 1080p/4K rendering, and process/device restart.
4. Publish a second AAB with a higher `versionCode`, then prove Play updates the installed app in
   place and preserves pairing/configuration.
5. When the internal acceptance checks are green, create a named Closed track for the wider beta.
   Closed tracks support controlled email lists or Google Groups and private Play feedback.

Internal releases normally reach testers within minutes. A tester opted into Internal testing is
not eligible for Closed/Open delivery until opting out of Internal and opting into the other test.
Track selection otherwise resolves to the highest compatible `versionCode` among tracks for which
the user is eligible. See [Set up an internal or closed test][testing-tracks].

If the Play developer account is a personal account created after November 13, 2023, production
access later requires a Closed test with at least 12 testers continuously opted in for 14 days. That
does not block Internal testing. See [Personal-account testing requirements][personal-testing].

## Automation boundary

Bootstrap remains a manual Play Console operation. The Publishing API can modify only an existing
app with at least one artifact already uploaded, and it cannot accept publishing legal consents.
After the first AAB is uploaded and the app setup is complete, automate subsequent beta releases:

1. create/choose a Google Cloud project and enable the Google Play Developer API;
2. create a service account, invite it in Play Console, scope it to this app, and grant only the
   release-to-testing-tracks rights needed by CI;
3. in one Publishing API edit, upload the signed AAB, update the Internal (later Closed) track, and
   commit the edit; and
4. serialize publication jobs because a Console change or another committed edit invalidates an
   in-progress edit.

Google recommends service accounts for server-to-server access. Protect the credentials as a
release secret; do not place a service-account JSON file or upload keystore in the repository. See
[API getting started][publisher-api], [Publishing edits][publisher-edits], and
[Upload an AAB with the API][api-upload].

## Release acceptance evidence

The first beta should remain blocked until the repository or release record captures:

- the permanent Play package and developer account owner;
- Play App Signing enrollment plus app-signing and upload certificate fingerprints;
- a reproducible signed `bundleRelease` AAB with a monotonic `versionCode`;
- AAB inspection showing supported device/ABI delivery and 16 KB compatibility;
- passing Android gates plus an actual emulator and Shield pass;
- approved TV form-factor review and complete listing assets;
- successful Internal-track opt-in and Play installation on the Shield; and
- a second Play-delivered build that updates in place and preserves paired state.

[api-upload]: https://developers.google.com/android-publisher/api-ref/rest/v3/edits.bundles/upload
[app-bundle]: https://developer.android.com/guide/app-bundle
[app-signing]: https://developer.android.com/studio/publish/app-signing
[create-app]: https://support.google.com/googleplay/android-developer/answer/9859152?hl=en
[distribute-tv]: https://developer.android.com/training/tv/publishing/distribute
[internal-sharing]: https://support.google.com/googleplay/android-developer/answer/9844679?hl=en
[latest-bundles]: https://support.google.com/googleplay/android-developer/answer/9844279?hl=en-EN
[personal-testing]: https://support.google.com/googleplay/android-developer/answer/14151465?hl=en-IN
[preview-assets]: https://support.google.com/googleplay/android-developer/answer/9866151?hl=en
[publisher-api]: https://developers.google.com/android-publisher/getting_started
[publisher-edits]: https://developers.google.com/android-publisher/edits
[support-16k]: https://developer.android.com/guide/practices/page-sizes
[support-64]: https://developer.android.com/google/play/requirements/64-bit
[target-api]: https://support.google.com/googleplay/android-developer/answer/11926878?hl=en-GB_ALL
[testing-tracks]: https://support.google.com/googleplay/android-developer/answer/9845334?hl=en
[tv-quality]: https://developer.android.com/docs/quality-guidelines/tv-app-quality
[upload-bundle]: https://developer.android.com/studio/publish/upload-bundle
[versioning]: https://developer.android.com/studio/publish/versioning
