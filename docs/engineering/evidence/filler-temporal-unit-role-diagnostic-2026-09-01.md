# Filler temporal unit/role diagnostic — 2026-09-01

Issue: #786. This is development evidence, not corpus truth or production certification.

## Question

Can two independent multimodal model families apply a factored temporal contract reliably enough to
distinguish standalone filler from compilations and programme excerpts before the 300-case corpus is
relabeled?

The diagnostic asks `UnitAssessment` first. A second, independent `RoleAssessment` call is made only
when the unit answer is `standalone`. Models receive the identity-blind package's ordered frames,
OCR, card hints, and transcript segments. They do not receive source filenames, private maps, prior
labels, or candidate predictions.

## Locked inputs and outputs

External root: `/Users/matthewpanton/LoomarrData/filler-development-2026-08-30`

| Artifact | SHA-256 |
| --- | --- |
| `temporal-role-readjudication-v3-ocr-card/review/package.json` | `31b1a43fefcd35dc0002ebaced28ace5ca12b9d2c361905cc9e337c1fdef67a2` |
| `temporal-unit-role-assessment-gemma-v4.json` | `36d7330e9cb2ee361352f6678a264df92a8bf9e8c49340ec79874bc08f49ee00` |
| `temporal-unit-role-assessment-qwen35-v4.json` | `86d79e5ece720c0e053d38222880fa3ab33247e9acb091145e3b1a382dca78a3` |
| `temporal-unit-role-comparison-gemma-qwen-v4-stratified.json` | `516c9664dc3c5d437e10ce19189c04d2b20fcb59719df75fbdb269e8a3fe92e7` |

The comparison was generated again at
`temporal-unit-role-comparison-gemma-qwen-v4-replay.json`; it had the same
`516c9664dc3c5d437e10ce19189c04d2b20fcb59719df75fbdb269e8a3fe92e7` digest.

Both runs used prompt `filler-temporal-unit-role-ollama-v4` and the exact package above:

| Family | Concrete model | Model digest | Prompt tokens | Completion tokens | Total latency | Operational failures |
| --- | --- | --- | ---: | ---: | ---: | ---: |
| Gemma 4 | `gemma4:8b-q4_k_m` | `c6eb396dbd5992bbe3f5cdb947e8bbc0ee413d7c17e2beaae69f5d569cf982eb` | 231,852 | 4,134 | 356,635 ms | 0 |
| Qwen 3.5 | `qwen3.5:9b` | `6488c96fa5faab64bb65cbd30d4289e20e6130ef535a93ef9a49f42eda893ea7` | 241,018 | 4,578 | 473,037 ms | 0 |

## Result

| Measure | Result |
| --- | ---: |
| Cases with two complete unit assessments | 32/32 |
| Unit agreement | 22/32 (68.8%) |
| Cases where both models said standalone | 22 |
| Role agreement on jointly standalone cases | 7/22 (31.8%) |
| Exact unit-plus-role agreement | 7/32 (21.9%) |
| Cases requiring adjudication | 25/32 (78.1%) |
| Stop rule | **Triggered: systemic failure** |

Gemma classified 28 spans as standalone and four as programme excerpts. Qwen classified 25 as
standalone, four as compilations, and three as programme excerpts. The ten unit disagreements were
distributed across four confusion directions; no single direction explains the problem.

Role disagreement is larger. The eight observed directions are commercial→promo,
interstitial→bumper, interstitial→commercial, station ID→promo, trailer→promo, unclear→bumper,
unclear→commercial, and unclear→promo. The largest single direction contains three cases, so the
failure is not one typo or one dominant mapping that can be silently rewritten.

The deterministic report selects 15 calibration candidates: one sorted representative for each of
the 12 unit/role confusion directions plus one exact-agreement control for each of the three observed
agreement strata (commercial, promo, and trailer). The report retains all 25 disputed aliases in its
case comparisons and confusion rows; sampling does not hide disagreement.

## Qwen 3.8 local feasibility

`qwen3.8:27b-mlx` was installed at digest
`5642e97495e1a088883805981563dcdc4a040c2f53388b7a41d1f24d3622cf7e`. A one-frame structured-output
probe succeeded only after the exact JSON shape was stated in the prompt; the MLX runner did not
enforce the supplied JSON schema by itself. On the full evidence request the model occupied about
18 GB, left roughly 6% system memory free on the 24 GB host, and had not completed the first case
after more than five minutes. The run was stopped before its ten-minute case deadline and published
no partial artifact. Qwen 3.8 is therefore a hosted or exceptional escalation candidate on this
machine, not the routine local first rung.

## Hosted calibration

The maintainer authorized at most 30 serial requests, $0.10 per request, and $3 total for an exact
Qwen 3.8 27B route with zero-data retention and provider fallback disabled. A fresh OpenRouter
schema-v2 snapshot resolved `qwen/qwen3.8-27b` to canonical model
`qwen/qwen3.8-27b-20260814`. The selected AkashML FP8 endpoint was active in that snapshot, declared
ZDR, accepted image and text input, and advertised strict structured output.

The route failed operationally. All 15 unit requests returned provider-side HTTP 502 errors; no unit
or role claim was accepted and no role request was attempted. OpenRouter did not return exact charge
values, so the ledger correctly retained all 15 conservative $0.10 reservations: $1.50 consumed,
rather than recording an unsupported zero cost. Fallback remained disabled throughout.

| Artifact | SHA-256 |
| --- | --- |
| `openrouter/qwen38-27b-temporal-calibration-snapshot-2026-09-02.json` | `3dceb05f7587821170c6ea481b242165d13a3d188589a6b04e81fec32dcf8f34` |
| Snapshot contract identity carried by the result | `31d18a171a894d02c98b0be9283ca92f559db5e9bcb1e797bb731b41e9f4cd71` |
| `temporal-unit-role-assessment-qwen38-openrouter-v1.json` | `d5a44f79b619a6543871a9474d2561cfb4d074608267c5cedd071659f2a4eaad` |
| `temporal-unit-role-calibration-report-qwen38-openrouter-v1.json` | `63b81a356a243e5ce0c2e486704d06f884c05088895892b20c5628ec3a424c16` |

The predeclared report disposition was `repeat_bounded_hosted_calibration`, for reason
`hosted_operational_failure`; it recorded 15/15 operational failures, 0/3 assessable agreement
controls, and `fullCorpusRelabelAllowed: false`. This result says nothing about Qwen's semantic
quality. It says only that the pinned AkashML route did not execute the experiment. The runner now
retains non-2xx status as a typed provider failure so a body-bearing 502 cannot be misreported as
non-retryable model JSON.

The same immutable selection then ran through the snapshot-proven CoreWeave FP8 Qwen route and the
Amazon Bedrock Claude Opus 5 route. Route probing found three portability defects before accepting a
full result:

- CoreWeave's grammar compiler rejected JSON Schema `uniqueItems` despite advertising structured
  output, so the provider schema uses the portable subset and the ordinary validator still enforces
  set-like evidence references after normalization;
- Qwen spent its whole completion allowance on hidden reasoning until the route was explicitly sent
  `reasoning.enabled=false`; and
- Claude routes emitted malformed JSON, HTML fragments, and trailing braces inside a free-form
  explanation despite advertised strict output. The hosted seam now accepts only the closed class
  and one to four package-owned signal IDs, then derives the required audit sentence locally.

The final prompt-v6 hosted runs were operationally clean:

| Assessor | Cases | Requests | Operational failures | Exact charge |
| --- | ---: | ---: | ---: | ---: |
| Qwen 3.8 27B / CoreWeave FP8 | 15 | 22 | 0 | $0.0578282 |
| Claude Opus 5 / Amazon Bedrock | 15 | 22 | 0 | $0.8114200 |

| Artifact | SHA-256 |
| --- | --- |
| `temporal-unit-role-assessment-qwen38-coreweave-v5.json` | `f5ade68f0591bc37ec6caf7acc24da11ab928183757dba236b9102c9c04ab5bd` |
| `temporal-unit-role-calibration-report-qwen38-coreweave-v5.json` | `a46b991aa6547c256739652c7e0779746765896a4d3274e6f4b71047fe328135` |
| `openrouter/claude-opus5-temporal-calibration-snapshot-2026-09-02.json` | `9c5b0596724b7fa560b9be0d006186fbb00df7f08d65bc1bcd320b9525e4c1f4` |
| `temporal-unit-role-assessment-claude-opus5-bedrock-v4.json` | `8da6c5df8ca72f9064ed36cef21df7f8f05fff56d576d052e161616b2c670386` |
| `temporal-unit-role-calibration-report-claude-opus5-bedrock-v4.json` | `90ce43a4a82325b49bfc4c9b07eedcb2f96f19862f14673f1c2b4e7ad4952cab` |

Qwen and Opus agreed on unit structure for 12/15 selected cases. Both produced roles on six common
standalone cases and agreed on four. This is substantially stronger than the local families, but it
is not a truth lock. Both stronger families independently rejected the supposed standalone/promo
agreement control `temporal-review-6c94724b1623af2f541d0bfe` as a programme excerpt. Qwen also
rejected the supposed commercial control as promo while Opus retained commercial. Therefore two of
the three controls were never safe controls; counting them against hosted models would preserve the
old local error rather than measure quality.

Across all probes and completed runs in this investigation, exact charges plus conservative unknown
reservations total $3.7715090 of the maintainer's $20 ceiling. No fallback route was enabled.

## Prompt-v7 local repair check

The five hosted-family disagreements were inspected, not the whole corpus. They exposed bounded
contract gaps: the threshold for unusable recaptures, a trailer missing closing evidence, a programme
opening mistaken for interstitial filler, political/propaganda material outside the role vocabulary,
and commercial-versus-promo precedence for branded sponsorship. Prompt v7 named those rules and both
pinned local families reran all 32 cases with zero operational failures.

| Artifact | SHA-256 |
| --- | --- |
| `temporal-unit-role-assessment-gemma-v7.json` | `5e66c36bc5de37882d6d773ea98019689739d357da3c0c382dc0248b793bd34f` |
| `temporal-unit-role-assessment-qwen35-v7.json` | `34a78fafebe04b4e9662baaeca975269495c120629a7fa484a3c5fa52a1f4c1c` |
| `temporal-unit-role-comparison-gemma-qwen-v7.json` | `786a264af58ffe87bd419b6465b07eb0b7782ccaf23127460d317a6328e485a2` |

Exact agreement improved from 7/32 to 11/32 and cases requiring adjudication fell from 25 to 21, but
unit agreement worsened from 22/32 to 19/32. The dominant confusion is now 13 cases where Gemma says
`standalone` and Qwen says `programme_excerpt`. The stop remains a systemic failure. Further tuning
on this same diagnostic would overfit rather than establish truth.

## Decision

Do not start the 300-case development relabel yet. The transport and evidence interfaces are now
operationally sound, and the stronger hosted families reduced disagreement enough to identify five
specific error classes. The remaining blocker is the diagnostic's truth/control layer: the two small
local families are not reliable temporal authorities, and two of the three old agreement controls
are contradicted by the stronger evidence.

Stop prompt-tuning this 32-case set. Replace the invalid controls and commission the already planned
small independent review for the disputed/high-risk slice. Use Qwen 3.8 and Opus 5 as independent
model proposals, not votes that become truth. When a 64 GB Apple-silicon host is available, rerun the
exact package with a pinned Qwen 3.8 27B MLX model and compare it with the hosted Qwen result; if it
reproduces the stronger-family behavior, it can become the routine local assessor. Fine-tuning
remains premature until the repaired, group-separated labeled set is materially larger.
