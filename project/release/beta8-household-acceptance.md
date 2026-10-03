# Beta.8 household acceptance

The maintainer approved this narrower release contract on 2026-10-02 in
[#1512](https://github.com/loomarr/loomarr/issues/1512). It supersedes that issue's original
all-hardware G1–G11 release prerequisite and phase checklists for the final beta.8 cut.
The original requirements, failed measurements, benchmark thresholds, and reports remain intact
as broader qualification work. `PROGRESS.md` owns current status; this document owns acceptance.

## Promise and scope

Beta.8 delivers on-demand household playout on the Intel Arc A380 deployment: one or two viewers,
the Web client and the real Shield, uniform programme/commercial playback, and resource-aware
admission. Qualify the exact installed server image and client builds, including the actual source
classes and output renditions used by that household. A run on a demo lane or a different server
candidate does not qualify this deployment.

Mac VideoToolbox, NVIDIA NVENC, software-only hosts, other devices, and larger viewer counts remain
experimental for this beta. This is a support/evidence boundary, not a new runtime switch. Existing
admission, correctness, authorization, and migration protections apply on every host. Do not claim
that experimental paths passed performance certification or disable protections to admit them.

## Required before the final tag

| Gate | Acceptance and evidence |
| --- | --- |
| Safe installation | Independently reviewed scoped deployment and recovery path; immutable candidate image; full rendered configuration validation; verified pre-start data backup including SQLite/WAL; forward-only schema checks. Retain the candidate and matching backup after a migration-capable startup failure. No automatic image-only rollback. Track media-server [#774](https://github.com/mantonx/fictional-media-server/issues/774) / [#775](https://github.com/mantonx/fictional-media-server/pull/775). |
| On-demand lifecycle | No ahead-of-time encoding. Capture idle process state, tune activity, and encoder drain after the last viewer and the configured grace period. Zero playout encoders remain after drain, including speculative neighbours. |
| Household start and control | On each of Web and Shield, record at least 10 cold tunes and 20 adjacent switches, including the first tune and every failure. Retain cold SDR p50 ≤1.0 s / p95 ≤1.5 s, cold 4K HDR p95 ≤2.5 s where used, and held frame/OSD p95 ≤100 ms. Record warm p50/p95 and failures; the 600 ms warm target is follow-up performance work. Every tune must complete without refusal, player error, or manual recovery at the declared one/two-viewer load. |
| Bounded playback | Observe one continuous hour on Web and one on Shield, with at least 30 minutes overlapping on the same Arc host. Report wall time and viewer-hours separately; this is two viewer-hours, not a 24-hour certification. Exercise at least 10 programme/commercial transitions per client. Require zero stalls, timeline gaps, unexpected decoder reinitializations, player errors, or lost playback across those observed transitions. Keep raw logs and all failures. |
| Real media and quality | Include household SDR and HDR inputs, HDR-to-SDR tone mapping, audio continuity and loudness, and each premium rendition used during the check. Preserve uniform output, direct-file input and Loomarr-owned media facts. A premium format not exercised must be labelled unqualified; do not infer all-format coverage from the baseline. Observe a programme/commercial boundary through the household's external tuner integration if configured. |
| Resource protection and recovery | During the overlapping viewers and surfing, capture the complete Loomarr process tree, CPU/memory, encoder count, admission decisions, database errors and Guide response timings. Guide p95 remains ≤300 ms over at least 20 requests. Filler must retain its caps and yield to playback; no OOM, server/store failure, or starvation is accepted. Verify excess demand is safely limited/refused using existing isolated admission evidence; do not overload the live household to find its maximum. Perform one controlled restart and verify readiness and resumed playback with pairing/data intact. |
| Artifact integrity | Green required CI and exact-current-main release-candidate scope, both native image builds, image-derived `make test-ffmpeg`, immutable source/tag/image identity, verified signatures, SBOM/provenance, packaged notices and public corresponding source. [#661](https://github.com/loomarr/loomarr/issues/661) remains a release blocker. No retagging, omitted tests, or publication-guard bypass. |

One missed required row holds that candidate. Diagnose and repair the failed row, then repeat the
affected evidence; do not restart unrelated qualification or broaden the scope without a concrete
reason. Existing protected CI remains required: this contract changes release qualification scope,
not test results, thresholds, CI selection, or application behaviour.

## Follow-up qualification

These remain open engineering goals, with their original thresholds and failed reports:

- Mac/NVIDIA/software performance and the remaining hardware matrix:
  [#1512](https://github.com/loomarr/loomarr/issues/1512),
  [#1566](https://github.com/loomarr/loomarr/issues/1566), and
  [#1794](https://github.com/loomarr/loomarr/issues/1794). The universal ≤400 ms server startup,
  ≥15× throughput and ≤1 core at full GPU capacity are not final beta.8 blockers. The household
  safety and response gates above still apply.
- Warm switching ≤600 ms across clients, many-channel/full-capacity churn, and the original
  24-hour soak: [#1037](https://github.com/loomarr/loomarr/issues/1037). A short successful run
  does not close this issue or prove long-term stability.
- Wider TV overlay/device qualification: [#1781](https://github.com/loomarr/loomarr/issues/1781).
  The household Shield's ≤100 ms overlay check remains required above; its short diagnostic pass
  does not establish a fix or a pass on every device.
- General filler certification remains beta.11; it is outside this on-demand playout release.

## Evidence and release order

1. Merge this contract and prepare release notes describing the qualified and experimental scope.
2. Complete deployment-safety review and obtain the production decision for that concrete change.
3. Freeze current main, run its release-candidate scope, and publish a fresh immutable RC. The
   failed RC.8 publication and old rc.7 live health are not qualification for the next candidate.
4. Deploy that RC through the reviewed path and execute the bounded household check. Record source
   SHA, image digest, server/schema version, client source/build/hash, device/driver identity,
   time interval, corpus/renditions, exact commands, raw-report locations/hashes, and each result
   on #1512. Unknown, skipped, and failed observations must remain distinct from passes.
5. After all required rows pass, publish final beta.8 from the qualified source under the existing
   exact-main guard. If source or artifact identity changes, explicitly account for the change and
   repeat affected qualification; a new runtime image needs fresh installed evidence. Verify final
   deployment identity/readiness and playback, then close the release workstream. Keep broader
   qualification issues open.

Scope approval does not itself approve a particular production mutation. The deployment PR has a
separate recorded review-only boundary; present its reviewed result for the production decision.
