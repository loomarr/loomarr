# FFmpeg pin n9.0.2-62-gac1c6e003f

Written by `scripts/ffmpeg-pin-refresh.sh` when it published the source release. Release acceptance
is separate: the pin's PR runs `make test-ffmpeg` and the image build.

| Artifact | Identity | SHA256 |
| --- | --- | --- |
| Linux amd64 binary archive | BtbN `autobuild-2026-11-23-12-43`, `n9.0.2-62-gac1c6e003f` | `17cf42701de208afbf8de9ae4cd016122c875d95beedc6c4b37894d35a523063` |
| Linux arm64 binary archive | Same release/build | `ae8581be1b2d9c42879c7645ca9392144829b7f85844355cbd4a95c470732ae3` |
| FFmpeg source archive | `ac1c6e003f9e298536363e716750abc0c8f25b37` (62 commits after `n9.0.2`) | `ded2d2cebf0260eca913e1f61a9bbbfff7f829825bf41b195aad41e84bc31714` |
| BtbN build recipe archive | `7f2a6b1050d302144de8366afa37a75e46f326aa` (built by run `45171645535`) | `e9bb8c9b0e0b5927bca3dd7817a959daaee24eef486eb857b2b8cbae3959568f` |
| Original dependency download cache | Run `45171645535`, artifact `14816429255`, 2257452439 bytes | `4d17781b6339d4b30e5ce352c940776423ffc354db39cce1b0bddb3d0b52db4d` |

- **Build selection.** `autobuild-2026-11-23-12-43` was BtbN's newest autobuild, and `n9.0.2-62-gac1c6e003f` its newest release-series
  (`9.0`) GPL build. Both binary digests match BtbN's `checksums.sha256` and GitHub's asset digests.
- **Provenance.** Run [`45171645535`](https://github.com/BtbN/FFmpeg-Builds/actions/runs/45171645535) built
  `Build ffmpeg (linux64, gpl 9.0)` and `Build ffmpeg (linuxarm64, gpl 9.0)` from recipe
  `7f2a6b1050d302144de8366afa37a75e46f326aa`, and its `Publish release` job created the release. The release tag points at `b9ebe81995b58cd4e3177a6c0ad5dc2392f7f7fd`,
  BtbN's master when the release was published.
- **Source archives.** These are GitHub's archives of the two commits; two downloads of each agreed byte
  for byte before publication. The cache is the run's own `download-cache` artifact, matched against
  GitHub's artifact digest.
- **Publication.** Everything is published in [third-party-sources-2026-11-23](https://github.com/loomarr/loomarr/releases/tag/third-party-sources-2026-11-23), together with both binary archives.
  The Dockerfile downloads the binaries from that release, and
  [source-distribution-2026-11-23.json](source-distribution-2026-11-23.json) binds every asset's URL, size
  and digest to it.
