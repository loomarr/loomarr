# FFmpeg pin n9.0.2-48-gd4070a14ce

Written by `scripts/ffmpeg-pin-refresh.sh` when it published the source release. Release acceptance
is separate: the pin's PR runs `make test-ffmpeg` and the image build.

| Artifact | Identity | SHA256 |
| --- | --- | --- |
| Linux amd64 binary archive | BtbN `autobuild-2026-11-08-12-35`, `n9.0.2-48-gd4070a14ce` | `bf5e935a51afb65ddd14bf67104361508bcc7c77026a66d7e193a05a56ab8d77` |
| Linux arm64 binary archive | Same release/build | `78972a433ec501179670b7a35720819deb31569ac25e9040e9bb6f8a7cfc8306` |
| FFmpeg source archive | `d4070a14cecc7e29eb278f9b6036c2284255f70d` (48 commits after `n9.0.2`) | `d5cd1dbfc7959c09b8af96640b4596ba333b6895f533ddb688aa0f13e00341f8` |
| BtbN build recipe archive | `c9fe5114060e507409f7ee07edfd82efaf25d767` (built by run `42478811497`) | `fb708487f587ffbf6c80c8d34fe0f96f7da8a1f88b8b6b5d1b27d91715fa2595` |
| Original dependency download cache | Run `42478811497`, artifact `13617050102`, 2243238781 bytes | `a0a7fae76a923b2a47a490d754acd1795d621076ecc84bef7d36d30ff670cafa` |

- **Build selection.** `autobuild-2026-11-08-12-35` was BtbN's newest autobuild, and `n9.0.2-48-gd4070a14ce` its newest release-series
  (`9.0`) GPL build. Both binary digests match BtbN's `checksums.sha256` and GitHub's asset digests.
- **Provenance.** Run [`42478811497`](https://github.com/BtbN/FFmpeg-Builds/actions/runs/42478811497) built
  `Build ffmpeg (linux64, gpl 9.0)` and `Build ffmpeg (linuxarm64, gpl 9.0)` from recipe
  `c9fe5114060e507409f7ee07edfd82efaf25d767`, and its `Publish release` job created the release. The release tag points at `c9fe5114060e507409f7ee07edfd82efaf25d767`,
  BtbN's master when the release was published.
- **Source archives.** These are GitHub's archives of the two commits; two downloads of each agreed byte
  for byte before publication. The cache is the run's own `download-cache` artifact, matched against
  GitHub's artifact digest.
- **Publication.** Everything is published in [third-party-sources-2026-11-09](https://github.com/loomarr/loomarr/releases/tag/third-party-sources-2026-11-09), together with both binary archives.
  The Dockerfile downloads the binaries from that release, and
  [source-distribution-2026-11-09.json](source-distribution-2026-11-09.json) binds every asset's URL, size
  and digest to it.
