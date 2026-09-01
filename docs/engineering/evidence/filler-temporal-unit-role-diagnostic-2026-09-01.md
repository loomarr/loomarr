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

## Decision

Do not start the 300-case development relabel yet. The factored interface is operationally sound—it
strictly validates every signal, preserves every model call, completes with zero provider failures,
and reproduces the comparison byte-for-byte—but the current local families do not share a calibrated
role boundary.

The next experiment should send only the 15 stratified candidates to one stronger, separately
versioned multimodal adjudicator, preferably hosted Qwen 3.8 or another top vision family through the
existing bounded OpenRouter controls. That run needs an explicit request and spend ceiling. Its job
is to identify which definitions or evidence gaps cause the confusion, not to vote the old labels
into truth. Fine-tuning is premature until this contract-level error is repaired and a larger clean,
group-separated label set exists.
