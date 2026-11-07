# FFmpeg pin n9.0.2-36-g6882969ce9

Written by `scripts/ffmpeg-pin-refresh.sh` when it published the source release. Release acceptance
is separate: the pin's PR runs `make test-ffmpeg` and the image build.

| Artifact | Identity | SHA256 |
| --- | --- | --- |
| Linux amd64 binary archive | BtbN `autobuild-2026-10-26-14-25`, `n9.0.2-36-g6882969ce9` | `79095572fe46ccc5eff689bc4a272ef331024a8264226e97856e3f155371af2b` |
| Linux arm64 binary archive | Same release/build | `f03f848bf52ec6e011c9349ffbf2f5afeeb6c12ddb0f69b203a7e99ef8f14a1f` |
| FFmpeg source archive | `6882969ce9fd2aa6aa1d0008b2bfe1c838aac11d` (36 commits after `n9.0.2`) | `9dc825750d71d51a9c8d45516ad3ffdf05e74f029a66f8c3efaf431ed7331701` |
| BtbN build recipe archive | `937868360a21d711ec94a33f5591d345ad9b7754` (built by run `40332330812`) | `9ee4b55c6bf1ea2f9ee50a63bac8887532117ce9f7e7b0e2842a533b2d5f046a` |
| Original dependency download cache | Run `40332330812`, artifact `12654339692`, 2210375776 bytes | `902885b1f3b66f6d14f61219fee17c163adf1719dcef72b0afefefacaed74bba` |

- **Build selection.** `autobuild-2026-10-26-14-25` was BtbN's newest autobuild, and `n9.0.2-36-g6882969ce9` its newest release-series
  (`9.0`) GPL build. Both binary digests match BtbN's `checksums.sha256` and GitHub's asset digests.
- **Provenance.** Run [`40332330812`](https://github.com/BtbN/FFmpeg-Builds/actions/runs/40332330812) built
  `Build ffmpeg (linux64, gpl 9.0)` and `Build ffmpeg (linuxarm64, gpl 9.0)` from recipe
  `937868360a21d711ec94a33f5591d345ad9b7754`, and its `Publish release` job created the release. The release tag points at `b4a2b1d7cc241786eb8e2b588a7ade40cad48bbd`,
  BtbN's master when the release was published.
- **Source archives.** These are GitHub's archives of the two commits; two downloads of each agreed byte
  for byte before publication. The cache is the run's own `download-cache` artifact, matched against
  GitHub's artifact digest.
- **Publication.** Everything is published in [third-party-sources-2026-10-27](https://github.com/loomarr/loomarr/releases/tag/third-party-sources-2026-10-27), together with both binary archives.
  The Dockerfile downloads the binaries from that release, and
  [source-distribution-2026-10-27.json](source-distribution-2026-10-27.json) binds every asset's URL, size
  and digest to it.
