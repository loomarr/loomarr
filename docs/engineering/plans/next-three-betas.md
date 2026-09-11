# Beta release roadmap

Roadmap owner: [#1105](https://github.com/loomarr/loomarr/issues/1105). Beta.6 release
execution: [#1204](https://github.com/loomarr/loomarr/issues/1204). Planning baseline:
2026-09-11, accepted main `2f73c5ff9790d4bd05975729e4ce5d6479f51596`.
`PROGRESS.md` owns current phase status; this document owns the release sequence and exit criteria.
Existing design documents and issue acceptance criteria retain their authority.

## Release baseline and intent

The latest published server release is
[v0.2.0-beta.5](https://github.com/loomarr/loomarr/releases/tag/v0.2.0-beta.5), tagged at
`0019257f42556101fa0a16743e70b6c459725c40` on 2026-09-10. The default desktop Guide repair
[#1202](https://github.com/loomarr/loomarr/pull/1202) is accepted after that tag. The idle encoder
capability repair [#1201](https://github.com/loomarr/loomarr/pull/1201) remains in protected
integration at this baseline. Neither repair is present in the published beta.5 image.

The remaining planned server releases are **v0.2.0-beta.6 and beta.7**; the beta.5 section below
preserves its accepted release contract and explicit carry-forwards. Android TV keeps its separate
version authority in `web/apps/tv/android-release.json` (currently `0.1.0-beta.5`); each release
candidate records the exact server/client pairing and follows the existing protected promotion
workflow. This plan neither edits versions nor publishes a release.

The household outcome remains: create a Channel that matches the request, understand what will
happen, watch reliably, and improve it without constant intervention. Use readiness gates rather
than unsubstantiated calendar dates. A failed required row holds that candidate; an optional item
can move without weakening the release's promised outcome.

| Release | Viewer/operator outcome | Primary gate owners |
| --- | --- | --- |
| **beta.5 — Trustworthy creation and playback** | Understand the request, explain uncertainty and launch readiness, recover cleanly, and watch the accepted changes on the declared household setup. | #1103, #1044, #1098/#1068, #1015/#494, #1097/#1037; approval safety #955 and applicable release prerequisites #688/#661/#1162 |
| **beta.6 — Complete certified filler journey** | Take an intentional source through exact approval, preparation, complete screening, review and deterministic admission into a real eligible break, with useful provenance and enrichment. | #945/#548, #955, #952/#1111, #908/#909/#951/#1086, #947, #946/#1110, #965/#1112, #741 |
| **beta.7 — Sustained household use** | Preserve explicit preferences through repeated curation and ordinary maintenance, replenish filler from measured gaps, and complete the remaining Web parity work. | #498/#497, #749, #970, integrated acceptance #1104; carry forward the exact #1037/#1097 and filler certificates |

These are release outcomes, not a serial queue of one PR per issue. The many filler issues include
parents, delivered tooling and independent evidence tasks. Reuse accepted code; complete the
remaining clauses instead of rebuilding the old stack or treating every open issue as missing code.

## beta.6 execution checkpoint — 2026-09-11

Beta.6 is the active release goal. [#1204](https://github.com/loomarr/loomarr/issues/1204) groups the
43 milestone issues into five release-evidence lanes without replacing their existing acceptance
criteria. Work proceeds in this order where dependencies require it and concurrently where source,
truth, safety, structure, enrichment, UI and release seams do not overlap:

1. Land the post-beta.5 Guide and encoder-reporting repairs and bind the beta.6 branch to their
   accepted merge commits.
2. Reconcile the retained filler worktrees and immutable artifacts before recovering or replacing
   any work. Old local work is evidence, not authority to overwrite current owners or reuse burned
   holdouts.
3. Complete the source and truth prerequisites for a fresh production envelope, led by
   #899/#1085/#1089/#1093/#1100/#1102/#1106/#1108/#1113.
4. Run the applicable structure, spoken, written, visual, audience and suitability certification
   gates. Missing, stale, operationally failed or unsupported evidence keeps media held.
5. Finish grounded enrichment, the coherent operator lifecycle and the 32-clip product-acceptance
   cohort, then freeze one exact candidate for source-to-air, browser, homelab and Shield acceptance.

Provider runs, acquisition and human adjudication use their existing bounded authorities. This
checkpoint grants no model spend, truth decision, media clearance or admission shortcut.

## What the delivery already provides

| Accepted work | Evidence | Consequence for this plan |
| --- | --- | --- |
| Named membership, source-backed scores, date meaning and request recovery | #1151, #1156, #1158 | Start with current main; the old planner integration backlog is no longer the frontier. Remaining score semantics and partial-Intent clarification have separate owners. |
| Discovery lifecycle and outcome attribution | #964 | Reuse its measurements for #498 and #1104; repeated household qualification remains. |
| Continuous raw/live AAC and strict playout certification | #1157; full protected queue `34341681693` | Reuse the accepted harness and repaired finite-output/epoch handling. The generated 106-Channel, capacity-four result does not establish deployment capacity. Prepared HLS remains process-free. |
| Exact acquisition, quarantine, derivatives, structure, safety projections and sole admission/publication authority | #1130–#1139, #1142/#1143 | The replacement pipeline is integrated. Missing/uncertified evidence holds media; there is no legacy admission fallback. Real certificates, written projection, enrichment and activation still need applicable acceptance. |
| Incoming workspace and holdout/quarantine authority | #1141/#1144 | Extend the existing interfaces for the complete source-to-air journey; do not recreate a second workbench. Tooling and proposed labels are not independent truth. |
| React Native Shield acceptance, Internal distribution and Linux Docker discovery repairs | #970, #1078, #1150; current client plan | Keep the accepted React Native client. Test each new candidate on the actual Shield/LAN; Web migration remains open. Do not resurrect the retired Compose path. |
| Proportional verification, dependency patches and concurrent delivery workflow | #1154/#1155, #1120 | Keep affected PR checks, protected integration gates and parallel disjoint work. #1050 remains a measured build-speed improvement. |

## beta.5 — Trustworthy creation and playback

**Status:** published as `v0.2.0-beta.5` on 2026-09-10. This section retains the accepted release
contract. Open parent and evidence issues continue under their assigned later-release scope; their
open state does not revoke the published beta.5 acceptance packet.

Complete these lanes concurrently, with one owner for overlapping planner/scorer/UI interfaces:

1. **Reference journeys first — #1103.** Freeze named-block, genre/era movies, family, single-show
   classics and seasonal examples through existing testkit/evaluation seams. Assert concrete
   identities, episode/order/date semantics, availability and repetitions. This complements all
   existing certification families; five attractive demonstrations do not replace the full suite.
2. **Honest creation — #1044, #1098 and #1068.** Preserve useful terse requests while exposing
   partial understanding or asking for clarification. Separate Intent adherence, optional diversity
   and unknown evidence; do not reward off-era spread as alignment. Ship the coordinated launch,
   acquisition and repeat-runway outlook. Reuse #1156 recovery and #1158 date semantics. For bare
   named blocks such as TGIF, automatically discover trustworthy source evidence through the existing
   grounded retrieval path; model memory does not establish membership. Missing or conflicting
   evidence remains explicit uncertainty. Qualify this through #1015/#494.
3. **Prepared readiness and playback — #1097/#1037.** Reconcile the implemented current/next planner
   and its 100-Channel seam test with every remaining clause: default storage budget, convergence,
   restart/schedule changes, capacity-scaled preparation and foreground preemption. Qualify the
   exact declared hardware/profile with the existing 100+ configured-Channel HLS/raw churn harness,
   measured active capacity, cleanup and percentile artifacts. The maintainer deferred only raw
   MPEG-TS prepared startup p95 targets (below 100 ms to first transport byte and 500 ms to first
   decoded frame) to beta.7. Keep the original certifier, thresholds and failed reports. Raw validity,
   continuity, admission, capacity, recovery and cleanup remain required. Prepared HLS timing and
   the existing shipping-browser budgets remain beta.5 requirements.
4. **Approval and login correctness — #955/#688.** Finish durable atomic pull approval/queueing,
   including concurrent decisions, persistence failures and restart. Admission atomicity from
   #1142 does not satisfy pull approval. The Emby header/offline-verifier repair already exists;
   #688's remaining release/homelab login acceptance needs evidence rather than another implementation.
5. **Release preparation.** Qualify the supported current provider/build with #1015 and #494;
   retain unchanged semantic, latency and resource gates. Complete applicable #661 exact-artifact
   redistribution evidence and documented source distribution mechanics; a separate qualified
   legal/NOTICE reviewer sign-off is not required following the maintainer's beta.5 scope decision.
   Resolve the newly tracked Metro/image-size dependency exposure
   and compatible remedy or explicit release-scoped disposition in #1162; GitHub currently lists no
   patched version. This is a locked-dependency finding, not a demonstrated deployed-service exploit.
   Prepare the installed Shield/browser acceptance packet.

**Exit:** the five journeys exercise creation through actual scheduled playback with honest
sparse/acquisition/clarification states; the complete current-provider gate passes; declared-profile
readiness and every non-deferred churn requirement pass; concurrent pull approval is durable; applicable login,
artifact and installed-client acceptance is recorded. The candidate identifies precisely which
filler capabilities remain held. It cannot present uncertified media as admitted or substitute a
legacy pipeline to populate the catalog.

The exact release packet must distinguish deferred raw startup performance from required evidence.
A raw performance failure remains an uncertified report with its original unsuccessful exit;
record it as an explicit beta.5 limitation, never as a green certification. Missing samples or any
non-deferred failure still hold release. The accepted scope decision does not qualify the existing
synthetic diagnostics as shipping-browser, codec, restart or physical-device acceptance.

**Parallel improvements:** #1040 can improve trustworthy progress on the same workflow states;
#1050 should measure the actual full Android job and pursue its existing 15-minute target without
reducing ABI, signing, alignment or correctness coverage. Neither visual delight nor build speed
replaces the required correctness rows. #970 Web parity foundations can start independently.

## beta.6 — Complete certified filler journey

Run corpus/rights/truth work alongside production integration and the operator journey:

- **Finish source and truth authority.** Reuse #899/#1144 tooling; settle challenged truth (#1085),
  transition classes (#1079), replacement-source/exposure authority (#1089/#1108), soundtrack
  preflight (#1100) and exact source lanes (#1102/#1106/#1113). Reuse delivered Met tooling
  (#993/#995/#1011). Refresh and byte-verify required artifacts; burned, unavailable or changed
  sources do not qualify a new run. #1093 retains its rights/quarantine disposition requirements.
- **Certify the supported production envelope.** Close applicable structure/automatic-split clauses
  (#952/#1111, #959/#961/#962/#963/#1069), complete-source spoken, written and visual safety
  (#908/#909/#951), source suitability/audience authority (#1109/#1019/#1086), and admission
  projection/activation (#947). Include exact route portability/accounting requirements where used
  (#1070/#960). A certificate is bound to its media/profile/route/audience scope; short test clips
  or one model family cannot replace required coverage or independent assessment.
- **Finish enrichment after screening — #946/#1110.** Produce evidence-grounded entities,
  categories and campaign/creative relationships from supported source evidence. Preserve unknowns;
  useful metadata never grants rights, safety, child-role or admission authority.
- **Complete one operator lifecycle — #965/#1112.** Extend accepted Incoming/Sources/Library
  surfaces so planning, exact approval, quarantine, preparation, screening, exact-child review,
  admission and on-air usage are understandable as one journey. Retain typed retries, holds and
  rights remediation; routine confirmations must not replace automated deterministic authority.
- **Qualify exact reference outputs — #741.** Record the human-reviewed 32-clip cohort and an
  eligible-break playback journey through the normal authority. This is product acceptance in
  addition to the full applicable #549/#555/#548 certification and shadow gates.

**Exit:** an exact, rights-traceable source reaches an eligible break through the sole certified
admission path; missing/stale evidence, provider outage, retries/concurrent approval, restart,
quarantine and withdrawal remain safe and explainable. Required corpus, complete-source safety,
structure, audience, enrichment and shadow results pass their existing thresholds. Record exact
source/profile/certificate/build identities, operator burden and supported categories. A small
accepted cohort cannot waive the broader corpus or unattended-admission gates.

This is the largest qualification dependency in the three-release plan. Start independent truth,
source and written-text work while beta.5 is being stabilized; do not wait for its tag to begin.
If applicable certification is missing, beta.6 holds its promised filler outcome.

## beta.7 — Sustained household use

- **Complete raw startup performance — #1037/#1097.** Meet the original prepared raw MPEG-TS p95
  targets below 100 ms to first transport byte and 500 ms to first decoded frame on the exact
  candidate and declared hardware/profile. Reuse retained beta.5 failures and pursue a bounded,
  sample-correct design if needed; preserve media, lifecycle, capacity and cleanup requirements.

- **Maintenance and repeated curation — #498/#1104.** For all five journeys, exercise baseline,
  sparse/missing Library, failed acquisition, restart, new arrivals and at least three controlled
  re-curation cycles with a pinned clock. Preserve keep/never, scheduled items, approved Policy,
  Channel identity and paused/detached states. An unchanged Channel is a legitimate outcome.
- **Measured discovery improvement — #497.** Add inspectable exposure memory without treating
  rejection, inactivity or playback stops as inferred taste. Separate relevance and explicit
  preference fidelity from novelty; qualify against the existing repeated-release evidence.
- **Coverage-driven filler — #749.** Use actual Channel/break gaps to propose bounded acquisition
  through the beta.6 authority. Never turn a gap into an automatic download or admission shortcut.
- **Finish shared Web parity — #970.** Deliver remaining P6/P7 route cohorts and P8 retirement
  through existing visual, authorization, form, responsive and accessibility gates. Delete replaced
  surface implementations with their accepted replacements. Device evidence from one Shield does
  not establish iOS, Android-touch or Apple TV product readiness.
- **Close integrated household acceptance — #1104.** Bind the beta.5 readiness/capacity profile,
  beta.6 certificates, repeated-curation results and exact installed clients into one go/hold
  packet. Predeclare trial duration and any missing burden/freshness thresholds before runs;
  obtain maintainer acceptance of the exact build and supported envelope.

**Exit:** all #1104 rows have applicable evidence and explicit acceptance; ordinary maintenance
and repeated use preserve intent and resources; the promised Web migration is complete; unsupported
platforms/categories remain identified. Supporting work such as #1061 streaming presentation and
#935 certification-draft assistance may ship when their own contracts pass.

## Recognition improvement over these releases

Use #549/#555/#787, #1085/#1086 and the source/holdout owners as one evidence loop:

1. Preserve disagreements and failures as development cases with exact source and context.
2. Obtain independent truth/adjudication; proposals and model agreement alone are not truth.
3. Keep development/calibration cases separate from untouched held-out source families and exposure
   records. Rebuild unavailable or burned evidence before claiming a new comparison.
4. Compare candidates per task/category: missed hazards, false approvals/rejections, boundary/role
   errors, abstention/coverage, human-review burden, latency and cost. Apply existing fixed gates;
   do not replace them with one favorable average accuracy number.
5. Promote only a versioned route/model/profile that passes the full applicable regression and
   shadow criteria. Keep unsupported material held, and preserve re-evaluation/rollback authority.

Beta.5 prepares this evidence; beta.6 qualifies the first supported production set; beta.7 uses
observed errors and expanded independent truth to improve it. The three-pillar program #856 remains
independent: no online weight updates or implicit household taste inference. Stock-model evidence
must establish a repeatable gap before the #828/#831–#837 custom-model release program is justified.

## Common release checklist and execution

For each candidate, bind commit, image/artifact digests, configured provider/model, corpus/scorer/
prompt/tool versions, configuration, hardware and client versions, commands, denominators and
results. Public summaries exclude private household prompts/titles/paths, credentials, signed URLs
and private media/legal artifacts; retain sensitive evidence privately. Run affected local gates and the full applicable protected integration/release gates;
retain source/notice/SBOM evidence and existing signing/version/ABI/alignment checks. Follow the
protected Android promotion path; a CI artifact is not proof of installation or Play publication.

Record current client pairing/playback/Guide/Surf/recovery and actual Linux Docker discovery for
the proposed build. Reuse #688's first-login/offline evidence and #661's redistribution requirements.
No release is inferred from the fact that earlier betas were published. Record the operational
rollback/recovery method under the existing distribution contract; do not reintroduce superseded
cross-channel pairing-preservation or Compose fallback requirements from #727.

One delivery owner integrates each PR. Start reference fixtures, isolated approval fixes, corpus
preparation and Web parity on disjoint seams; share one owner for scorer/outlook DTO changes and
one for admission/publication authority. Review frozen dependent patches early and merge in actual
base order. Use #1116's retained-work dispositions before recovering local work; a protected tree
or old branch is neither a missing implementation nor authority to overwrite its owner.

This roadmap authorizes planning and issue organization. It does not itself execute provider spend,
media acquisition, household trials, deployments, tags, stores or releases. At execution time use
existing session authority and the exact applicable run/profile/budget; request only genuinely
missing decisions. Never run `make smoke*` from an agent session.

## Issue reconciliation and complete allocation

Snapshot: **79 open issues** on 2026-09-09 before roadmap reconciliation. The table allocates every
one, plus the newly discovered dependency follow-up #1162. A target is the earliest planned completion window, not a claim that code is absent or that an
open parent can close when only one child ships. Verify remaining clauses against current main and
retain original assignees and acceptance criteria. Milestone metadata follows beta.5/.6/.7; supporting
optimizations can move without relaxing required release gates.

- Track #1162 before beta.5: Dependabot alerts 40/41 affect locked `image-size@1.2.1` through
  `metro@0.87.0`; establish the actual input/consumer exposure and a compatible remedy. Earlier
  dependency patches #1154 cover different package families.
- Keep #688 open for its remaining homelab acceptance; its scheme/verifier code exists in
  `internal/library/auth.go`, `internal/library/client_test.go` and `internal/auth/auth_test.go`.
- Keep #955 open: `internal/api/fillerpullapproval.go` still starts ingestion before persisting the
  decision and explicitly tracks concurrent atomicity there. #1142 protects a different boundary.
- Keep #1098 open: general theme matching still credits any matching term, while general era
  scoring measures decade spread. #1151's named-membership/unknown-episode repairs do not satisfy
  all of this issue's counterexamples (`internal/suggest/score.go`, Proposal review UI).
- Keep #1097/#1037 open for their remaining convergence and declared-profile evidence; current
  planner classes and `TestPlannerPublicSeamScalesOneHundredChannelPriorityAndPreemption` are
  reusable implementation evidence, not the complete qualification result.
- Reconcile #727 as superseded by #970's accepted single-device adoption/retirement contract,
  recorded in the shared-client plan, completeness ledger and platform inventory. Retain those
  historical references; do not restart the old Compose-versus-React-Native decision.
- Keep #970 open for remaining Web/shared-client migration and each new artifact's acceptance.
  The old issue text about a shipping Kotlin app is historical, not the current architecture.
- #1105 remains the cross-release map; #1104 owns final integrated acceptance. #856 retains the
  wider model program. No training, new provider or additional platform is an unconditional gate
  for these three betas. #1020/#966 become release work only after a measured gap requires them;
  #495's context contract and optional refactors remain separately scoped.

| Issue | Existing owner / required outcome | Target |
| --- | --- | --- |
| [#1162](https://github.com/loomarr/loomarr/issues/1162) | Resolve image-size parser advisories in the Metro dependency graph | beta.5 |
| [#1103](https://github.com/loomarr/loomarr/issues/1103) | Define and exercise five reference Channel journeys | beta.5 |
| [#1098](https://github.com/loomarr/loomarr/issues/1098) | Make Proposal theme and era scores reflect the requested Intent | beta.5 |
| [#1068](https://github.com/loomarr/loomarr/issues/1068) | Show a calm Channel outlook for launch readiness, repeat runway, and serendipity | beta.5 |
| [#1044](https://github.com/loomarr/loomarr/issues/1044) | Partially understood Intent produces a confident Proposal instead of asking for clarification | beta.5 |
| [#1015](https://github.com/loomarr/loomarr/issues/1015) | Add a local pre-release gate for channel-generation quality and latency | beta.5 |
| [#494](https://github.com/loomarr/loomarr/issues/494) | Preserve candidate relevance and enrich grounded retrieval evidence | beta.5 |
| [#1097](https://github.com/loomarr/loomarr/issues/1097) | Make prepared readiness converge across 50–100 channels | beta.5; support raw startup performance beta.7 |
| [#1037](https://github.com/loomarr/loomarr/issues/1037) | Certify many-channel playout capacity and channel-surf churn | beta.5; raw startup performance beta.7 |
| [#955](https://github.com/loomarr/loomarr/issues/955) | Make filler pull approval atomically idempotent | beta.5 |
| [#688](https://github.com/loomarr/loomarr/issues/688) | fix(auth): Emby successful login fails when client scheme is missing | beta.5 |
| [#661](https://github.com/loomarr/loomarr/issues/661) | Complete binary source, notice and distribution evidence | beta.5 |
| [#1050](https://github.com/loomarr/loomarr/issues/1050) | perf(android): reduce React Native Play AAB CI build time | beta.5 |
| [#1040](https://github.com/loomarr/loomarr/issues/1040) | Make channel-generation progress calm, trustworthy, and delightful | beta.5 |
| [#1113](https://github.com/loomarr/loomarr/issues/1113) | Add exact NPS video corpus source authority | beta.6 |
| [#1112](https://github.com/loomarr/loomarr/issues/1112) | Overhaul filler UX around one source-to-air lifecycle | beta.6 |
| [#1111](https://github.com/loomarr/loomarr/issues/1111) | Certify compilation identification and automatic split boundaries | beta.6 |
| [#1110](https://github.com/loomarr/loomarr/issues/1110) | Add evidence-grounded filler entity and campaign enrichment | beta.6 |
| [#1109](https://github.com/loomarr/loomarr/issues/1109) | Build source-level filler suitability and audience authority | beta.6 |
| [#1108](https://github.com/loomarr/loomarr/issues/1108) | Bridge qualified filler corpus into temporal holdout replacement planning | beta.6 |
| [#1106](https://github.com/loomarr/loomarr/issues/1106) | Add exact Blender Open Movie corpus source authority | beta.6 |
| [#1102](https://github.com/loomarr/loomarr/issues/1102) | Add exact USGS corpus source authority | beta.6 |
| [#1100](https://github.com/loomarr/loomarr/issues/1100) | Preflight expected soundtrack before replacement-corpus download | beta.6 |
| [#1093](https://github.com/loomarr/loomarr/issues/1093) | Bind development rights review to quarantine disposition | beta.6 |
| [#1089](https://github.com/loomarr/loomarr/issues/1089) | Acquire unexposed truth sources for the replacement filler holdout | beta.6 |
| [#1086](https://github.com/loomarr/loomarr/issues/1086) | Certify multimodal broadcast-suitability recall before filler admission | beta.6 |
| [#1085](https://github.com/loomarr/loomarr/issues/1085) | Add targeted adjudication authority for challenged filler anchors | beta.6 |
| [#1079](https://github.com/loomarr/loomarr/issues/1079) | Add measured transition-class authority to the temporal-structure holdout | beta.6 |
| [#1070](https://github.com/loomarr/loomarr/issues/1070) | Make the direct-video structure protocol portable to Gemini reasoning routes | beta.6 |
| [#1069](https://github.com/loomarr/loomarr/issues/1069) | Separate compilation disposition from final child-role authority | beta.6 |
| [#1019](https://github.com/loomarr/loomarr/issues/1019) | Filler: make Airworthiness audience-aware across violence and other suitability flags | beta.6 |
| [#1011](https://github.com/loomarr/loomarr/issues/1011) | Filler: build and certify the visual clean-control cohort | beta.6 |
| [#995](https://github.com/loomarr/loomarr/issues/995) | Filler: bridge approved media into visual-corpus nominations | beta.6 |
| [#993](https://github.com/loomarr/loomarr/issues/993) | Filler: add a Met Museum corpus inventory lane and private materializer | beta.6 |
| [#965](https://github.com/loomarr/loomarr/issues/965) | Make Filler one coherent source-to-admission workflow | beta.6 |
| [#963](https://github.com/loomarr/loomarr/issues/963) | Certify complete-coverage structure assessment for long compilation reels | beta.6 |
| [#962](https://github.com/loomarr/loomarr/issues/962) | Bind structure certification to the production media profile and source envelope | beta.6 |
| [#961](https://github.com/loomarr/loomarr/issues/961) | Run certified independent structure assessments in production shadow | beta.6 |
| [#960](https://github.com/loomarr/loomarr/issues/960) | Record OpenRouter media over-reservation charges honestly | beta.6 |
| [#959](https://github.com/loomarr/loomarr/issues/959) | Certify conservative multi-signal compilation decisions | beta.6 |
| [#952](https://github.com/loomarr/loomarr/issues/952) | Certify commercial-compilation identification and segmentation | beta.6 |
| [#951](https://github.com/loomarr/loomarr/issues/951) | Certify complete-source visual sensitive-content safety for filler | beta.6 |
| [#947](https://github.com/loomarr/loomarr/issues/947) | Apply certified filler screens and enrichment at the admission boundary | beta.6 |
| [#946](https://github.com/loomarr/loomarr/issues/946) | Add post-screen multimodal entity and taxonomy enrichment for filler | beta.6 |
| [#945](https://github.com/loomarr/loomarr/issues/945) | Finish intentional, playback-ready, deeply enriched filler ingestion | beta.6 |
| [#932](https://github.com/loomarr/loomarr/issues/932) | Assemble spoken-safety cohorts into one reviewable certification draft | beta.6 |
| [#930](https://github.com/loomarr/loomarr/issues/930) | Prepare VCTK clean controls for spoken-safety certification | beta.6 |
| [#909](https://github.com/loomarr/loomarr/issues/909) | Certify full-duration written-text safety for filler | beta.6 |
| [#908](https://github.com/loomarr/loomarr/issues/908) | Certify complete-source spoken-language safety for filler | beta.6 |
| [#899](https://github.com/loomarr/loomarr/issues/899) | Build a source-diverse 36-case temporal-structure holdout | beta.6 |
| [#870](https://github.com/loomarr/loomarr/issues/870) | Separate filler media integrity from presentation and suitability defects | beta.6 |
| [#865](https://github.com/loomarr/loomarr/issues/865) | Add a separate airworthiness gate for filler content | beta.6 |
| [#787](https://github.com/loomarr/loomarr/issues/787) | Build model-led labeling and sampled calibration for the 300-case filler corpus | beta.6 |
| [#741](https://github.com/loomarr/loomarr/issues/741) | Build the 32-clip production-ready filler reference cohort | beta.6 |
| [#555](https://github.com/loomarr/loomarr/issues/555) | Run the bounded OpenRouter bakeoff and shadow-certify filler automation | beta.6 |
| [#549](https://github.com/loomarr/loomarr/issues/549) | Build a versioned filler certification corpus and replayable evaluator (certification pending) | beta.6 |
| [#548](https://github.com/loomarr/loomarr/issues/548) | Certify automatic filler admission with evidence, abstention, and exception-only UX | beta.6 |
| [#1104](https://github.com/loomarr/loomarr/issues/1104) | Record integrated household-journey qualification and acceptance | beta.7 |
| [#1061](https://github.com/loomarr/loomarr/issues/1061) | Stream Catalog-grounded titles while a Proposal Job is running | beta.7 |
| [#970](https://github.com/loomarr/loomarr/issues/970) | refactor(clients): complete Shield and Web shared-design-system migration | beta.7 |
| [#749](https://github.com/loomarr/loomarr/issues/749) | Acquire filler against Channel coverage gaps | beta.7 |
| [#498](https://github.com/loomarr/loomarr/issues/498) | Measure discovery quality in production without inferring taste | beta.7 |
| [#497](https://github.com/loomarr/loomarr/issues/497) | Remember recommendation exposure to prevent repetitive discovery | beta.7 |
| [#935](https://github.com/loomarr/loomarr/issues/935) | Automate independent model review of spoken-safety certification drafts | beta.7 |
| [#1020](https://github.com/loomarr/loomarr/issues/1020) | Prefer direct filesystem sources for internal playout when media is locally mounted | Later / conditional |
| [#966](https://github.com/loomarr/loomarr/issues/966) | Normalize local NFO and optional metadata-provider evidence for AI retrieval | Later / conditional |
| [#907](https://github.com/loomarr/loomarr/issues/907) | Prototype Apple SCA capability for source-level filler screening | Later / conditional |
| [#837](https://github.com/loomarr/loomarr/issues/837) | Add the curated planner-model install, update, rollback, and offline UX | Later / conditional |
| [#836](https://github.com/loomarr/loomarr/issues/836) | Verify planner-model compatibility and integrity before activation | Later / conditional |
| [#835](https://github.com/loomarr/loomarr/issues/835) | Publish immutable planner-model releases with provenance and licenses | Later / conditional |
| [#834](https://github.com/loomarr/loomarr/issues/834) | Merge, quantize, package, and certify the selected planner model | Later / conditional |
| [#832](https://github.com/loomarr/loomarr/issues/832) | Build a privacy-safe corpus of reviewed Loomarr tool-use traces | Later / conditional |
| [#831](https://github.com/loomarr/loomarr/issues/831) | Benchmark stock local planner models and select at most two finalists | Later / conditional |
| [#828](https://github.com/loomarr/loomarr/issues/828) | Evaluate and distribute a Loomarr-specific local planner model | Later / conditional |
| [#779](https://github.com/loomarr/loomarr/issues/779) | Modularize the authoritative design record and archive shipped plans | Later / conditional |
| [#777](https://github.com/loomarr/loomarr/issues/777) | Split the high-churn filler vertical into cohesive source files | Later / conditional |
| [#495](https://github.com/loomarr/loomarr/issues/495) | Curate new and existing channels from explicit ambient context | Later / conditional |
| [#1105](https://github.com/loomarr/loomarr/issues/1105) | Roadmap: complete and qualify the household Channel and filler experience | Cross-release program |
| [#856](https://github.com/loomarr/loomarr/issues/856) | Track the three-pillar Loomarr AI model program | Cross-release program |
| [#727](https://github.com/loomarr/loomarr/issues/727) | decision(clients): record the native vertical-slice adoption outcome | Superseded decision |

## Review scope

This plan combines live GitHub release/issue/milestone snapshots, accepted commits since beta.4,
current phase/design/release documents, and targeted code/test inspection. Two bounded Terra readers
returned partial code-evidence reports at their work cutoffs; a Luna reader inventoried the other
43 issue contracts. The supervisor checked the critical score, pull-approval, auth, prepared-planner
and client-retirement seams directly. This is a release-roadmap review, not a fresh exhaustive audit
of every accepted implementation or a new runtime/corpus certification result.
