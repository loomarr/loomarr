# FFmpeg pin n9.0.2-42-gd7643a0fe3

Written by `scripts/ffmpeg-pin-refresh.sh` when it published the source release. Release acceptance
is separate: the pin's PR runs `make test-ffmpeg` and the image build.

| Artifact | Identity | SHA256 |
| --- | --- | --- |
| Linux amd64 binary archive | BtbN `autobuild-2026-11-01-12-49`, `n9.0.2-42-gd7643a0fe3` | `11570b7bb1436ff432c91202ae0e7ea4be65359fd06650e18935c1ee9d005bf6` |
| Linux arm64 binary archive | Same release/build | `b45eb0c42d311d12f3bddcd7b3a29a3e7fcc2455076d13c79552d45b48d57c08` |
| FFmpeg source archive | `d7643a0fe3f4f65516c302eba045ba884faed015` (42 commits after `n9.0.2`) | `7d9d8480ae21f133237f906d21afe95f36712e5b8b49740b522e7cea65ea1d17` |
| BtbN build recipe archive | `5ece62897c650b863c1ddda7b6964845344aef2d` (built by run `41305008201`) | `3399050a6fb3d93fffb6e6a565889ff02c483fe7b8aea09e95a777043309b1af` |
| Original dependency download cache | Run `41305008201`, artifact `13082723317`, 2240577333 bytes | `db20b35b3bd7971eb845b70159e9e01627a385cf38b5477402c8a95a2c53a213` |

- **Build selection.** `autobuild-2026-11-01-12-49` was BtbN's newest autobuild, and `n9.0.2-42-gd7643a0fe3` its newest release-series
  (`9.0`) GPL build. Both binary digests match BtbN's `checksums.sha256` and GitHub's asset digests.
- **Provenance.** Run [`41305008201`](https://github.com/BtbN/FFmpeg-Builds/actions/runs/41305008201) built
  `Build ffmpeg (linux64, gpl 9.0)` and `Build ffmpeg (linuxarm64, gpl 9.0)` from recipe
  `5ece62897c650b863c1ddda7b6964845344aef2d`, and its `Publish release` job created the release. The release tag points at `5ece62897c650b863c1ddda7b6964845344aef2d`,
  BtbN's master when the release was published.
- **Source archives.** These are GitHub's archives of the two commits; two downloads of each agreed byte
  for byte before publication. The cache is the run's own `download-cache` artifact, matched against
  GitHub's artifact digest.
- **Publication.** Everything is published in [third-party-sources-2026-11-02](https://github.com/loomarr/loomarr/releases/tag/third-party-sources-2026-11-02), together with both binary archives.
  The Dockerfile downloads the binaries from that release, and
  [source-distribution-2026-11-02.json](source-distribution-2026-11-02.json) binds every asset's URL, size
  and digest to it.
