# v0.2.0-beta.8

Draft release notes. Publication and household qualification are pending under the
[beta.8 acceptance contract](beta8-household-acceptance.md). This file is not evidence of a release.

Beta.8 introduces on-demand channel playout. Channels encode while watched, with spare capacity
used to warm neighbouring channels. One channel packager keeps the programme/commercial timeline
continuous, and resource-aware admission protects the server as viewers arrive and leave.

The release qualification targets the household Intel Arc A380 deployment with one or two viewers
using Web and Shield. Mac, NVIDIA, software-only hosts, other devices and higher concurrency remain
experimental in this beta. Their existing safety and admission checks remain enabled.

Changes include rendition-aware neighbour warming, Web player lifecycle repairs, a retention index,
VideoToolbox correctness repairs, and startup measurements based on complete published segments.
These repairs do not establish performance certification for every hardware family.

Known limitations:

- Mac and NVIDIA strict startup/throughput benchmarks still miss their targets; NVIDIA also missed
  the full-capacity CPU target. No cross-hardware performance parity is claimed.
- Warm channel switching has not met the 600 ms target on the diagnostic Shield/server pairing.
  Record the qualified candidate's measurements before publication.
- The 24-hour soak and many-channel/full-capacity certification remain follow-up work. The bounded
  household check does not establish long-term stability.
- Premium renditions and external tuner integrations are qualified only where the release evidence
  explicitly records them. The 1080p SDR baseline does not certify every output format.

The image includes FFmpeg n9.0.1. Corresponding source and build materials are available in
[the September source release](https://github.com/loomarr/loomarr/releases/tag/third-party-sources-2026-09-27);
yt-dlp and retained dependency sources are in
[the August source release](https://github.com/loomarr/loomarr/releases/tag/third-party-sources-2026-08-31).
Packaged notices and the final source/image/signature/SBOM/provenance evidence remain required by
[#661](https://github.com/loomarr/loomarr/issues/661).
