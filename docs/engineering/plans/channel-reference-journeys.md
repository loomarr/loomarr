# Reference Channel journeys v1

Tracking: [#1103](https://github.com/loomarr/loomarr/issues/1103), within the
[three-beta roadmap](next-three-betas.md). The initial integration exposed
[#1163](https://github.com/loomarr/loomarr/issues/1163): an exclusive seasonal Channel rejected a
series by its title before inspecting playable holiday episodes.

## Evidence boundary

`internal/testkit/channeljourney` owns `channel-reference-journeys-v1`. Its five fresh fixtures start
with controlled submitted Proposals and use a fixed 2026-12-24 18:00 UTC clock. Fictional movie and
episode facts are public synthetic inputs, not historical or provider truth. The named collection
reuses the Full House, Family Matters, Step by Step and lexical-neighbor identity contract in
`TestSuggest_NamedCollectionAdmitsOnlyEnumeratedCatalogMembers`, accepted for #968/#1022. It does
not invent a second historically complete TGIF roster.

The API integration calls ordinary admin approval, the real binder, SQL availability, the real
Channel reconcile engine and the same cycle preview used by playback. It checks persisted concrete
program identities against preview identities and the frozen expected order. No test seeds an
approved Channel or writes an available title directly. The fixture's deliberately out-of-policy
items exercise scheduler defense; their presence is not a claim that production generation may
produce unsafe or off-topic Proposals.

These are complementary seams. Existing Suggester tests and the complete current planner corpus
retain ownership of generation, identity grounding, named-set admission and semantic qualification.
The new tests do not run inference, decode video, access a household library, or certify filler.

## Assertion matrix

| Journey | Concrete integration assertion | Generation / negative evidence |
| --- | --- | --- |
| Named block | The three governed member identities reach episode slots; pre-1990 series premieres retain their supported 1990s episode, while the 1989 episode cannot air. | `TestSuggest_NamedCollectionAdmitsOnlyEnumeratedCatalogMembers` actually surfaces and rejects the non-member neighbor; reference ambiguity and model-only membership negatives remain in the Suggester suite. |
| Genre and era movies | Only the two science-fiction movies released in 1990–1999 air in approved order; later-release and wrong-genre movies do not. | Existing planner genre/date cases retain their full thresholds; cross-decade spread is not evidence of adherence (#1098). |
| Family | The classified family movie airs; unknown-rating and above-ceiling movies do not. | Existing audience corpus and auth tests remain required. Real filler eligibility/admission remains separately qualified under #947/#1019. |
| Single show | Season-one and season-two episodes air in sequence; season twelve is excluded. | Existing `schedule_classic_simpsons_highlights` and `TestRunnerChecksConcreteProgramsAfterScheduleFiltering` retain the different curated-highlight contract and wrong-concrete-identity negative. |
| Seasonal | At the pinned Christmas clock, exclusive policy and the named holiday episode selector survive approval; only the Christmas episode reaches the persisted schedule. | `TestSeasonalExclusiveEpisodeEvidenceAndCalendar` covers unavailable editorial evidence, ordinary/wrong-holiday/unknown/above-ceiling episodes, off-season loop/dark, zero clock and unchanged auto/off behavior. |

`TestReferenceChannelAcquisitionJourney` adds two sparse-library outcomes through the same approval
boundary. A wholly acquisition-dependent Channel stays non-live with no program; the real
provisioning reconciler submits the request through the shared Requester fixture. A later library
confirmation yields the exact program, while a missed deadline yields unavailable state and
cancellation. A fresh availability adapter and reconcile reconstruct both results from SQL.

Seasonal multipart and metadata tests preserve a whole supported story unit, accept title/overview/
tag evidence, reject a holiday-named series whose episodes lack evidence, and keep exclusions
distinct from pending acquisition. The minimal seasonal test failed on the accepted base with
`Reason:out_of_season` for the entire series; the full API journey failed with `status = empty`.

The separation acceptance extension reuses those same fixtures and approval boundary. A mixed
named-member pool has two 22-minute episodes per series, a two-hour episode repeat window,
44-minute series gap and one-episode block limit. Every concrete episode must appear exactly once,
and the persisted cycle and preview must honor both spacing rules through the last-to-first wrap
without relaxation. A sparse single-show control retains its explicit season exclusions and
records the existing 48-hour to 24-hour repeat-window relaxation; its actual 44-minute cycle is
not evidence that either requested repeat window is attainable. The genre/era movie variant's
two one-hour movies satisfy an explicit two-hour movie repeat window without relaxation; the
wrong-genre and out-of-era movies remain excluded. Submitted policies remain intact while the
reconciler's applied notes explain any shortfall.

Eligibility controls change one relevant source fact or explicit season limit on a fresh copy of
each reference fixture. The previously excluded item must then reach the schedule. These controls
show that the original negative was present and eligible for binding, rather than absent from the
fixture. They do not change production policy, invent named-block membership, or certify model
interpretation. Known duplicate, missing, adjacent-series and cycle-wrap defects also challenge
the separation assertions directly.

## Reproduction and remaining acceptance

Capture machine-readable test events through Go's existing harness; do not create another evaluator:

```sh
go test -json -race -count=1 -run '^TestReferenceChannel' ./internal/api > reference-journeys.jsonl
go test -race -run '^TestSeasonalExclusive' ./internal/schedule
go test -race -run '^TestSuggest_NamedCollectionAdmitsOnlyEnumeratedCatalogMembers$' ./internal/suggest
make verify BASE=origin/main
```

The delivery receipt binds source/accepted commits, fixture version, commands, results and artifact
hashes. This deterministic suite has no provider, model, prompt or tool execution; those identities
are explicitly inapplicable here and must be recorded for the separate #1015/#494 provider run.
Current evaluator scorer/prompt/tool versions and all existing full-corpus gates remain authoritative.

This first integration contribution does not close #1103 by itself. Remaining release acceptance
includes reviewed complete named-block hard-negative coverage, a complete mapping of generation
and schedule assertions to all five journeys, remaining causal negative and applicable filler
expectations, and exact-build scheduled playback alongside
the full provider and installed-client qualification. Do not call a fixture PASS a beta release PASS.
