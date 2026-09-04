# Android TV beta releases

Loomarr's accepted React Native TV client owns the permanent Play identity `loomarr.media`.
Prototype development builds use `media.loomarr.tv.prototype` and cannot replace a Play build.

The full rationale and current Google requirements are in
[`FINDINGS-android-tv-beta-distribution-2026-08-22.md`](../engineering/FINDINGS-android-tv-beta-distribution-2026-08-22.md).

## Release identity

The protected workflow accepts one version name and derives its Play code:

```text
major * 100000000 + minor * 1000000 + patch * 10000 + channel
```

`beta.N` uses 1–7999, `rc.N` uses 8001–8999, and a stable release uses 9999. For example,
`0.1.0-beta.1` is code `1000001`. The mapping is monotonic across beta, release candidate, stable,
and the next patch. Unsupported names fail before signing.

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

## First signed AAB

Run **Android TV beta** from `main` with:

- version `0.1.0-beta.1`;
- **Publish to Play** disabled.

The workflow requires the exact `main` commit to have a successful CI run in which the Android job
actually executed. It generates the Android project from `web/apps/tv`, builds with the protected
upload key, verifies the signature and certificate, checks `loomarr.media`, requires all four
Android ABIs, verifies 16 KiB ELF alignment for every packaged 64-bit native library, and retains
the AAB plus its JSON evidence for 30 days. Download that AAB and upload it manually while enrolling
in Play App Signing.

Do not use Internal App Sharing for acceptance; it re-signs artifacts with a disposable identity and
does not prove the beta update path.

## Automated testing-track release

After the manual bootstrap and service-account setup, dispatch the workflow with **Publish to Play**
enabled. The publisher opens one Play edit, uploads the exact digest-verified AAB, replaces the
Internal track release, and commits the edit. Global concurrency prevents two edits from racing.
The workflow exposes no Closed, Open, or Production choice.

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
