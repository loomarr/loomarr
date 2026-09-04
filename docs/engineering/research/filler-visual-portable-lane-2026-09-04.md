# Portable visual-sensitive-content lane

Issue: [#951](https://github.com/loomarr/loomarr/issues/951)  
Date: 2026-09-04  
Status: development implementation and measurements. This note grants no quarantine, ingestion,
scheduling, training, or broadcast authority.

## Decision

Build the first **development-only** portable bakeoff around two ordinary image classifiers, not a
general-purpose LLM or VLM:

1. Use [`Marqo/nsfw-image-detection-384`](https://huggingface.co/Marqo/nsfw-image-detection-384/blob/0c26ec22111b83f106d72a55f611ec35962bcb65/README.md)
   as the primary candidate. It is the smallest credible candidate inspected, has Apache-2.0 model
   metadata, accepts fixed 384×384 inputs, and returns two logits (`NSFW`, `SFW`). Its exact upstream
   revision is `0c26ec22111b83f106d72a55f611ec35962bcb65`; the safetensors object is 22,404,720 bytes.
2. Use [`Freepik/nsfw_image_detector`](https://huggingface.co/Freepik/nsfw_image_detector/blob/15b85477e4fd2000db76ae9aae0f89a72f95e2e3/README.md)
   as the independent comparator. Its model card and base-model metadata declare MIT, it accepts
   fixed 448×448 inputs, and returns cumulative-severity-compatible `neutral`, `low`, `medium`, and
   `high` logits. Its exact upstream revision is
   `15b85477e4fd2000db76ae9aae0f89a72f95e2e3`; the safetensors object is 172,725,672 bytes.
3. Export both exact revisions to ONNX in a locked development environment and run them with one
   pinned ONNX Runtime CPU build. The exported graph, converter environment, preprocessing recipe,
   runtime executable/library, thresholds, policy mapping, and every input frame remain separate
   content-addressed authorities. A community-converted graph is not accepted as the upstream model.
4. Treat the two models as candidate constituents of one portable lane. Any locked constituent's
   valid positive may produce a portable positive. A portable `no_signal` requires every locked
   constituent to finish on every planned frame. A conversion mismatch, missing output, NaN/Inf,
   timeout, runtime error, or constituent disagreement below a positive threshold is a hold unless
   the locked policy says otherwise. No majority vote clears a source.
5. Do not select a production model, ship weights, add a runtime dependency, or train/fine-tune yet.
   First run the source-family-disjoint development matrix. The observed misses and false positives
   must justify the final constituent set and thresholds; upstream accuracy claims cannot do so.

The portable classifier answers only the visual nudity/sexual-content part of the operator's private
policy. Spoken prohibited language belongs to the complete-source speech lane, and visible prohibited
language belongs to the complete-source OCR lane. A general VLM remains eligible only for a declared
direct-video escalation; it cannot turn classifier negatives into a clear result.

## Why these candidates

| Candidate | Primary-source facts | Loomarr decision |
| --- | --- | --- |
| Marqo NSFW 384 | Apache-2.0 metadata; ViT-tiny, 5.6M parameters; fixed 384×384 input; two classes; proprietary 220,000-image corpus including photographs, drawings, memes, AI images, and illustrated sexual material; model card reports 98.56% on its own balanced test set and explicitly recommends use-case-specific threshold testing. [Card](https://huggingface.co/Marqo/nsfw-image-detection-384/blob/0c26ec22111b83f106d72a55f611ec35962bcb65/README.md), [config](https://huggingface.co/Marqo/nsfw-image-detection-384/blob/0c26ec22111b83f106d72a55f611ec35962bcb65/config.json) | **Primary development candidate.** Its small graph makes dense complete-source screening plausible on CPU, while its declared content mix is relevant to archival advertising and animation. Its binary label is not Loomarr's policy, so the raw NSFW probability and a Loomarr-owned threshold must be retained. |
| Freepik NSFW detector | MIT metadata; EVA02-base, 87.1M parameters; fixed 448×448 input; four severity labels; 100,000-image training claim. The card recommends cumulative `medium + high` style decisions and reports only an in-domain underprediction statistic. It recommends BF16 and reports NVIDIA RTX 3090 timings rather than portable CPU timings. [Card](https://huggingface.co/Freepik/nsfw_image_detector/blob/15b85477e4fd2000db76ae9aae0f89a72f95e2e3/README.md), [config](https://huggingface.co/Freepik/nsfw_image_detector/blob/15b85477e4fd2000db76ae9aae0f89a72f95e2e3/config.json), [base model](https://huggingface.co/timm/eva02_base_patch14_448.mim_in22k_ft_in22k_in1k/blob/81063ecfe9c381a16a19d06f396d6c7011aa426a/README.md) | **Independent comparator.** The ordinal outputs could help set conservative policy thresholds, but it is about 7.7× larger than Marqo's weights and has no published Loomarr-domain evidence. Advance only if it finds positives that Marqo misses without breaking clean-slice bounds. |
| Falconsai NSFW image detection | Apache-2.0 metadata; ViT-base, fixed 224×224 binary classifier; proprietary 80,000-image corpus; 343,223,968-byte safetensors graph. Its reported 98.04% evaluation is on its own unspecified split. [Card](https://huggingface.co/Falconsai/nsfw_image_detection/blob/04367978d3474804ab1a00a9bd6548b741764069/README.md) | **Do not run initially.** It offers the same binary output as Marqo at roughly 15× the weight size and supplies less domain detail. Reopen only if the first matrix exposes a slice for which its architecture or data provides a concrete hypothesis. |
| NudeNet v3 | The upstream project supplies 12.2 MB and 103.5 MB ONNX YOLO detectors with eighteen anatomical/exposure classes and localized boxes. Its repository and container declare AGPL-3.0, and the README says the maintainer is seeking help; it does not publish a held-out calibration table or training-corpus description. [Repository](https://github.com/notAI-tech/NudeNet/tree/6ccc81c6c305cccfd46d92b414f8a5c0a816574d), [v3.4 weights](https://github.com/notAI-tech/NudeNet/releases/tag/v3.4-weights), [license](https://github.com/notAI-tech/NudeNet/blob/6ccc81c6c305cccfd46d92b414f8a5c0a816574d/LICENSE) | **Reject as a Loomarr dependency.** The localized taxonomy is attractive, but the licence is materially different from Loomarr's MIT application and the published evidence is too thin to compensate. Do not copy its preprocessing or postprocessing implementation into Loomarr. |

These are upstream facts, not comparative Loomarr results. Training corpora are proprietary, their
label definitions are not inspectable, and no upstream test set represents Loomarr's historical
advertising/programme mix. The only meaningful winner is the one measured against the locked private
corpus and policy.

## Runtime decision

Use ONNX Runtime for the development comparison, behind an exec-isolated private adapter rather than
linked into the Loomarr server. ONNX Runtime exposes a C inference API, accepts on-disk or in-memory
models, and publishes CPU archives for Linux x64, Linux arm64, and macOS arm64. The current v1.29.0
release publishes those three artifacts with GitHub-supplied SHA-256 digests; the project is MIT but
its `ThirdPartyNotices.txt` must accompany any future distribution. [C API](https://onnxruntime.ai/docs/get-started/with-c.html),
[v1.29.0 release](https://github.com/microsoft/onnxruntime/releases/tag/v1.29.0),
[license](https://github.com/microsoft/onnxruntime/blob/v1.29.0/LICENSE),
[third-party notices](https://github.com/microsoft/onnxruntime/blob/v1.29.0/ThirdPartyNotices.txt).

The first comparison must use the CPU execution provider on every platform. CoreML can be measured
later as a distinct capability: ONNX Runtime documents that its CoreML provider requires macOS 10.15+
and can use the Apple Neural Engine, but a CoreML result is not assumed numerically identical to CPU.
[CoreML execution provider](https://onnxruntime.ai/docs/execution-providers/CoreML-ExecutionProvider.html)

The adapter should eventually be an executable with a length-delimited, versioned stdin/stdout
protocol. Loomarr supplies one decoded frame at a time plus its evidence identity; the worker returns
only finite raw logits and timing. Loomarr—not the worker—owns thresholding, interval construction,
policy-match identifiers, observation sealing, reduction, and all source/derivative effects. This
keeps model code and native libraries out of the server process, makes crashes fail closed, and lets
the exact worker binary be hashed like `ffmpeg` and `whisper-cli`.

## Reproducible primary-model export

The first Marqo export is sealed outside the repository at
`LoomarrData/filler-development-2026-09-04/visual-safety-portable-v1`. Its
`artifact-manifest.json` has SHA-256
`ba58cfc360efc83c8f5263e316b8c1cd5af1e2e3d83f55186537fe665a58a01c` and explicitly grants only
development use. It does not grant production admission or training authority.

The export used `python:3.12.11-slim-bookworm` at image digest
`sha256:519591d6871b7bc437060736b9f7456b8731f1499a57e22e6c285135ae657bf7` on Linux arm64. The
exact upstream weights are 22,404,720 bytes at SHA-256
`6bf2e0f64a1d20169736c2836e3a787b12379fdc08ba87f7d94a7a3d58eeefce`. The fixed-shape
1×3×384×384 opset-17 graph is 22,489,943 bytes at SHA-256
`c0d0078642236cf50a80bdbecbc296598d87bd7c6f2f976d383b516a6ae327f5`. The export script and full
package freeze are separately hashed in the manifest. The environment pins PyTorch 2.14.0+cpu,
torchvision 0.29.0+cpu, timm 1.0.29, ONNX 1.22.0, ONNX Runtime 1.29.0, and safetensors 0.8.0. The
CPU-only PyTorch index is deliberate: the ordinary Linux arm64 wheel attempted to introduce CUDA 13
packages and was rejected rather than accepted into the export environment.

Two exports in fresh containers produced byte-identical ONNX graphs, parity reports, and dependency
freezes. On three deterministic generated tensors, PyTorch and ONNX Runtime returned the same argmax
and had a maximum absolute raw-logit delta of 0.000001430511474609375. This proves repeatable model
conversion only. Generated tensors are neither positive controls nor clean controls and provide no
evidence about Loomarr-policy recall, false positives, threshold choice, archival-video behavior, or
broadcast suitability.

## Real worker diagnostic

The development worker now exercises the repository's exact decoder-to-logit path around that graph.
The launcher is SHA-256 `fa92a8abbbce9aad706ff339dbf35fa966db07f6d9a772739b25a9e97d415215`
and pins an offline, read-only Linux-arm64 container at image digest
`sha256:aa26efeda8f4035dea9ffdd58c0dbe2d449ed22647478318dbe5983467944c76`.
The worker derives and verifies its own model/runtime/preprocessor capability before reading requests;
that capability is SHA-256 `f2246d86eb6761ae9c0131c212c233440cae9521c410dcc4ee6b48d9cc7dc8e7`.
It uses the model card's exact `timm.data.create_transform` recipe and the pinned CPU execution
provider. It returns ordered raw `NSFW` and `SFW` logits only.

One generated 640×360, 10 fps, three-second FFV1 transport control exercised the real FFmpeg
decoder, all four planned frames at 0, 1,000, 2,000, and 2,900 ms, the framed worker process, raw
ONNX inference, response validation, complete-decode validation, and aggregate evidence sealing.
Ten fresh worker-container runs all passed and returned byte-for-byte identical raw logits for every
frame. Reported model time was 94–104 ms per frame; total process startup, decode, inference, and
shutdown took 1.87–1.91 seconds per source. The private development report has SHA-256
`21f5365eff7e19be37a44725457850e3a5114f4e79e6102c1972ed8ca3b979f0`.

This generated pattern is deliberately called a transport control rather than a clean control. Its
stronger `SFW` logits prove that values have the expected label order, but the pattern has no private
policy label and contributes nothing to accuracy or certification. Likewise, stable logits on one
generated video do not establish stability across architectures, codecs, archival material, or real
sensitive content.

The first real archival positive candidate is the rights-approved development source
`archive.org/movie_trailers_picfixer/OrgyOfTheDeadTrailer`. Its 127,454 ms Theora artifact was
decoded completely at a one-second target interval. The first attempt held because an actual 167 ms
early-stream timestamp gap exceeded the provisional 40 ms drift assumption. After measuring the
source's irregular PTS, a 300 ms development drift ceiling and 1,700 ms claimed display floor were
locked. A second attempt exposed a planner error: the final 127,060 ms frame was inside the preceding
regular grid point's tolerance window, making two requested observations resolve to one physical
frame. The planner now collapses only such overlapping terminal grid points and retains the measured
terminal edge; the profile's existing `interval + 2 × drift` bound covers the resulting gap.

The corrected run completed 128 distinct frames with a maximum observed gap of 1,235 ms. Marqo's
summary-only softmax NSFW score ranged from 0.0564 to 0.9415, with a median of 0.3458; 27 frames were
at or above the illustrative 0.90 level and 54 were at or above 0.50. The strongest frame occurred at
92,025 ms. Model time was 100–205 ms per frame. The decoder-corrected complete private report has
SHA-256 `6b9917d2c3713a5a6599418a5ced0d5c77268b1302394e4aaa88efd936371dfb`; its private summary has
SHA-256 `3e95b8ecb2b6621dcd89e5862e28321653ffc490447d2ad307091170376048f2`. The summary binds and
supersedes the initial run; its scores reproduce the earlier result while two selected timestamps now
match the canonical rounded-millisecond timeline exactly.

This is encouraging sensitivity evidence, not truth or certification. The source's prior model flags
and corpus policy metadata are not an independently locked visual label. No threshold was selected,
and no clean false-positive or recall estimate follows from this one source. Its purpose is to show
that the off-the-shelf portable model produces a strong, temporally distributed signal on relevant
real media before we spend effort constructing the independent corpus.

## Development threshold scorer and disagreement diagnostic

The candidate can now be evaluated without allowing its output to create its own truth. One
`EvaluatePortableDiagnostic` operation consumes an authority authored before execution, the exact
capability and coverage profile, and one complete or explicitly failed run per case. The authority
requires unique source content and source-family identities, locks rights and pre-existing truth
authority digests, accepts unresolved cases only without a truth digest, and predeclares up to 32
strictly ordered thresholds. Positive intervals must each meet the coverage profile's minimum exposure
floor. Its slice vocabulary is closed over the positive and clean slices declared in V68.

The report applies the candidate's declared softmax transformation to exact raw logits, scores every
positive interval, reports source-family recall with a one-sided exact 95% lower bound, reports clean
false positives overall and by slice, and retains incomplete executions as operational holds. It does
not choose a threshold. Only an unresolved signal, a positive miss at the lowest tested threshold, a
clean signal at the highest tested threshold, or an operational failure enters the targeted-review
worklist. The report explicitly keeps blind audit, candidate-created truth, training, and production
admission false. Reproduction validation recomputes the complete report rather than accepting a
matching self-digest.

The second rights-approved real source was the disputed Old Spice 1992 advertisement at exact source
SHA-256 `026550f27351d832e997ea787d43b2a76b4b9f7970d6f923ddf89cbb85df02bf`.
The real worker completed all 30 planned frames over 28,746 ms with a 1,001 ms maximum observed gap.
Marqo's summary-only score ranged from 0.0400 to 0.8946, crossed 0.50 on three frames, crossed 0.85
once, and never crossed 0.90. The strongest frame was at 13,013 ms rather than near the previously
alleged 22-second interval. A targeted inspection of only the three model-selected frames did not
establish that allegation and instead exposed a plausible archival color/skin/framing false-positive
pattern. Because model-selected frame inspection cannot establish absence elsewhere, the case remains
unresolved and contributes to neither the positive nor clean denominator.

The decoder-corrected private report has SHA-256
`b0727bfe03d6e3c1486c28e800c93a331d38982101e5ecf1b57c2cbe9d5badb2`; its private summary has
SHA-256 `30cec79a31d9e4edf9bc696b9a8543f685d33d583b706f00fd6d3c05fdcf5330`.
The raw scores reproduced exactly; only measured execution-time evidence changed.
This is already useful: a threshold at 0.85 would surface this case while 0.90 would not, so neither
value may be selected from the positive candidate alone. Independent labeled controls must decide the
tradeoff.

The next six rights-approved candidates deliberately span distinct Archive items and source families:
Peanut Butter, Mary Hartline Doll, Muppet games, AARP, animated skin protection, and Air Buddies.
All six complete-source runs succeeded after the decoder corrections below, covering 360 exact frames
with maximum observed gaps from 1,000 to 1,040 ms. No source crossed the illustrative 0.50 level. The
highest source maximum was 0.3855; individual maxima ranged from 0.1241 to 0.3855. These are promising
clean-control candidates, not clean truth. Five had a coverage hold from one earlier full-video VLM,
while Mary Hartline was the only candidate with complete no-signal observations from both independent
VLM reviews and Marqo. That makes Mary Hartline the first targeted clean-truth nominee; model agreement
still cannot author its label.

The private six-source summary is SHA-256
`d4f633ddd2df2955ab595830130965cc898671355e630bb47fd35db197a42599`. It contains no machine-local
paths, locks every source/capability/profile/evidence/report digest, marks all six truth labels unresolved,
and grants no threshold, certification, training, or admission authority.

Expanding beyond the irregular-PTS trailer found two decoder defects that the generated 10 fps control
could not expose. First, a 25 fps source had distinct frames at 113,000 and 113,040 ms. The planner
correctly omitted the overlapping 113,000 ms grid point, but FFmpeg independently regenerated it and
emitted 115 frames for a 114-point plan. The filter now caps cadence selection at the exact count of
planned non-terminal points. Second, a 29.97 fps source ended at 90.990991 seconds, which is the locked
90,991 ms authority timestamp; comparing the unrounded seconds to `90.991` dropped the terminal frame.
Selection and `showinfo` validation now share the same rounded-millisecond timeline. Separate hermetic
real-FFmpeg regressions reproduced both failures before their fixes, and the original Peanut Butter and
Mary Hartline sources then passed end to end.

## Required development measurements

Before either model may become a locked portable constituent, freeze and publish the hashes of:

- source repository/revision, original weights, exported ONNX graph, and all config files;
- converter code, dependency lock, container image, opset, and export command;
- inference worker executable, ONNX Runtime archive/library, licence, and notices;
- color decoding, orientation, alpha handling, resize/crop/pad, interpolation, tensor layout,
  channel order, normalization constants, dtype, batch size, thread count, and provider options;
- raw-output ordering, softmax/cumulative transformation, finite-number validation, thresholds,
  private policy mapping, and the coverage profile;
- hardware architecture, OS, CPU/provider, wall time, peak RSS, and frames/second.

First prove conversion parity on clean and positive controls: the upstream framework and exported
ONNX graph must make the same threshold-side decision on every locked image, and raw-score deltas must
stay under a predeclared tolerance. Then run the complete-source challenge from design V68:

- at least 59 independent positive source families, with zero missed sources and a one-sided 95%
  exact lower recall bound of at least 95%; derivatives and generated transforms do not increase the
  independent denominator;
- short exposure, cuts, crop/letterbox, transcode, VFR/CFR, animation, monochrome, low light,
  multiple people, compilation placement, and damaged-tail positive slices;
- programme, advertising, animation, historical graphics, skin-tone/medical/beach/underwear, and
  visually busy clean controls, with a predeclared per-slice false-positive ceiling;
- exact repeats on each supported release architecture; record score drift even when decisions agree;
- cold/warm latency, sustained complete-source throughput, timeout/error/abstention rate, and memory;
- per-frame candidate outputs retained privately, with only opaque public policy-match ids and hashes;
- full source quarantine projection for every positive and a hold for every decode, worker, model,
  threshold, or identity failure.

The 59-source gate is a certification minimum, not a training set. It is far too small and too policy-
specific to justify fine-tuning a vision model. Reconsider training only after the frozen off-the-shelf
comparison identifies repeatable false-negative clusters, enough separately rights-cleared training
families can be acquired, and a second untouched certification corpus remains available. Until then,
an LLM fine-tune would add cost and nondeterminism without fixing the coverage or label-authority
problem.

## Next implementation slice

1. The generic portable-worker protocol, model/runtime capability authority, and single-pass exact-frame
   decoder are implemented without choosing a threshold or enabling production behavior.
2. The primary Marqo ONNX export, exact worker process, decoder-to-logit transport, response identity,
   resource limits, and repeated generated-control execution are implemented and measured.
3. The source-family-disjoint threshold scorer, one disagreement run, and a six-source candidate-clean
   expansion are implemented. Lock source-level truth for the top-ranked cases, then add independently
   labeled positives and difficult clean controls. Stop Marqo early on obvious positive misses or clean
   false positives; do not derive truth from model agreement or model-selected frame inspection.
4. Export and measure Freepik only after the Marqo diagnostic establishes which independent evidence a
   second, much larger constituent must add.
5. Only then construct and lock the independent certification authority. Keep the current
   `visual_safety_not_certified` production hold until both certification and issue #947's separate
   release authority are complete.
