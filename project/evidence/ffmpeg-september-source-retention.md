# September FFmpeg source retention

Engineering traceability for the September pin update (v0.2.0-beta.9). It follows the
[August record](ffmpeg-august-source-retention.md) and does not establish release acceptance.

The Dockerfile moves from August's n9.0 build to the n9.0 GPL build in BtbN's
`autobuild-2026-09-30-13-08`, September 2026's final autobuild. BtbN's `util/prunetags.sh` (unchanged
since 2023, bound here at the release commit `6c9aec5f…`) keeps the newest 14 autobuilds plus the
newest autobuild of each of the last 24 months, so this release is retained and the newer October
dailies are not: any of them is deleted once a later October build and 14 newer builds exist.

| Artifact | Immutable identity | SHA256 |
| --- | --- | --- |
| Linux amd64 binary archive | `autobuild-2026-09-30-13-08`, `n9.0.2-17-g2a571b6068` | `68ee646831adaae2495618346f3bba94ff207ff83bbd34d643e7004730d66269` |
| Linux arm64 binary archive | Same release/build | `91afcd7695d4bbbb9e0057aa10baaf9875ec4e1a97303f9059ec971336a2ab46` |
| FFmpeg source archive | `2a571b606854520cf89804d8030c8b328e621689` | `f02fc0809a13f37e3d4e1f74254d07e035a5ef01a6d91e727b915b1764d97540` |
| BtbN build recipe archive | `6c9aec5fc9a72ec3abedd1fa84db141fa18cf52b` | `81da4f7c878dc0f60e6b8d0049a112e32178e20f025d468fe4e99e77b2dfe0ec` |
| Original dependency download cache | Run `36713993443` — **expired, not retained** | — |

Both binary digests were computed locally from the downloaded archives (2026-10-26) and match the
release's `checksums.sha256` and GitHub's recorded asset digests. The source archives are GitHub's
archives of those commits, the same method as the August rows; re-downloading the August recipe
archive on 2026-10-26 reproduced `08279484…` byte for byte. The source commit is on FFmpeg's
`release/9.0` branch exactly 17 commits after `n9.0.2`, which is the build ID `n9.0.2-17-g2a571b6068`.
The recipe is 56 commits after August's `8267213e…` and changes dependency stages, so the August
dependency cache does not cover this build.

## Open: corresponding-source publication

BtbN uploads each run's `download-cache` artifact with a 14-day lifetime. Run `36713993443` (the
scheduled run at `6c9aec5f…` that published this release, including the `linux64` and `linuxarm64`
`gpl 9.0` jobs) lists no artifacts on 2026-10-26, so the original dependency download cache that the
August publication carries cannot be retrieved for this build. July was replaced for the same reason.

Until one of the following is done and recorded here with a `source-distribution-*.json`, this pin
has no published corresponding source and must not ship in a release image:

1. Publish a source release with the FFmpeg and recipe archives above plus a dependency cache
   regenerated from the recipe at `6c9aec5f…` (`download.sh`), verified stage-by-stage against the
   recipe's download hashes. This does not claim the original upstream bytes.
2. Or re-pin to October's final autobuild (expected 2026-10-31) and retain its run's `download-cache`
   artifact before it expires (14 days), which keeps the August pattern exactly.
