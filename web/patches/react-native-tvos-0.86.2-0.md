# React Native TV 0.86.2-0 Hermes acquisition correction

Tracking: [#1170](https://github.com/loomarr/loomarr/issues/1170).

pnpm binds this source patch into the exact existing package pin. No React Native, Hermes,
compiler, Expo, or native framework version changes. The normal source type always selects the
package's versioned release archives; a failed HTTP HEAD no longer selects snapshot XML or a moving
source revision. Explicit upstream development overrides remain available outside the Apple
verifier, which rejects them.

Metadata and archive acquisition use checked HTTP status with bounded connection, transfer, and
retry durations. SHA1 metadata must contain exactly 40 hexadecimal characters. Every Pods/shared
cache hit and newly downloaded archive must match that digest, including when caches are disabled.
Bad cache bytes are reacquired; failed or mismatched downloads never become published cache files.
Unavailable checksum metadata fails closed. This preserves the upstream Maven checksum authority;
it does not claim that SHA1 provides signed provenance or introduce a new checksum source.

CocoaPods receives the already verified debug archive as a local HTTP source with its digest.
The source type remains the release type, preserving the pinned Hermes compiler and upstream
Debug/Release archive replacement phase. Both configuration archives are verified before pod
installation. This avoids a second unchecked remote CocoaPods download.

The network-free consumer regression runs before either native simulator build:

```sh
ruby web/scripts/hermes-download-test.rb
```

It exercises the installed patched Ruby helper, not a reimplementation. It checks HTTP errors,
malformed metadata, both cache locations, corrupt archive bytes, cache bypass, and the CocoaPods
source. A compatible upstream replacement must pass these contracts and both native build/install/
launch gates before removing this patch. The unrelated initial loader failures remain unqualified
until a current native run supplies passing evidence or an actionable crash reason.
