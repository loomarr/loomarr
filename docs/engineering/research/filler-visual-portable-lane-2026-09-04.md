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
   timeout, runtime error, or unresolved disagreement without a positive is a hold. No majority vote
   clears a source and no negative can override a valid positive.
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

The Freepik comparator now has the same conversion evidence. Its exact 172,725,672-byte safetensors
object is SHA-256 `024a9d4818fae2656403bf626c9f8c9e7789c2da274749fbebb1060d8fdaa7ab`.
The fixed 1×3×448×448 opset-17 graph is 358,173,524 bytes at SHA-256
`264295c492943791338f3ef544e10bae1fe1da807c9ea899de59acc86d940121`.
The EVA implementation's fused attention was not accepted through an exporter workaround: the
export recipe first recorded native fused output, switched all twelve attention modules to the
equivalent explicit-attention path for ONNX tracing, and compared fused PyTorch, explicit PyTorch,
and ONNX Runtime on three deterministic tensors. All argmax decisions agreed; the largest fused-to-
ONNX raw-logit delta was 0.000004291534423828125. Two clean container exports produced byte-identical
graphs and parity reports. The parity report is SHA-256
`0d3ba1dd0ba451d0c53cddf3c8202356ab161dfde44a4ee5067925bad087ef8f`.

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

The report applies either declared single-label softmax or ordered cumulative softmax to exact raw logits,
rejecting a cumulative boundary at the first output because that would tautologically score every frame
as one. This permits Freepik's `medium + high` interpretation without putting thresholds in the worker.
It scores every
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

## Candidate-blind review evidence and the first decisive weakness

Independent truth review no longer requires handing a reviewer either a raw source path or the candidate's
answer. One `BuildCandidateBlindReviewBundle` operation snapshots the exact source, performs the complete
coverage decode, and atomically publishes a private bundle containing the exact complete source plus every
planned full-resolution frame as a lossless PNG. The shareable reviewer directory contains only a fresh
opaque alias, the exact validated development-only policy bytes, policy/profile identities, the complete
plan and coverage evidence, and the media assets. It
contains no provider metadata, source name, candidate identity, inference, score, threshold, or proposed
verdict. The policy asset is a private bounded regular file whose digest must match the source authority;
there is no out-of-band prompt paraphrase. A separate owner map restores the exact source authority, family,
rights, and selection origin only
after review. Reopening the bundle rejects extra files, symlinks, non-private permissions, byte or pixel
drift, policy drift, incomplete topology, and owner-map drift. An incomplete marker makes a partially written or crashed
bundle unreadable, while atomic directory creation prevents an existing output from being overwritten.

Two real candidate-blind bundles now reproduce. The 128-frame positive-candidate package is 22 MiB with
package SHA-256 `5d54967f716b64ea48261763f0a865e827b60aaba7ed5af8e2fd7f9d28278606`
and owner-map identity SHA-256 `6941ebfc73fc9a4ea920f58e3480098b317dbecd22a3e3904fff4f959809e07b`.
The 92-frame Mary Hartline package is 14 MiB with package SHA-256
`355f425ebb54d8c7e1676602a1177264fa96746fc24c547ea1d4c1dca192d037` and owner-map
identity SHA-256 `14a2daf4113d6059c57573bc09a9aff915fb26a58f74e17a845319cd7f88a53e`.
Both are explicitly `targeted_diagnostic`, so even future agreeing reviews cannot silently count them as
independently selected certification families.

The earlier locked 300-case semantic corpus supplied candidate-preceding evidence for the trailer. Its two
independent reviews disagreed in specificity: the first described the relevant scenes without an explicit
nudity flag, while the second explicitly flagged prohibited visual content near 40 and 108 seconds; the
independent adjudicator selected the second label set. A narrow source inspection at two frames per second,
limited to those independently nominated windows, confirmed development intervals `[39,500, 41,500)` and
`[107,500, 111,500)`. The private finding has SHA-256
`dab97507ed713d5b5b719cb9f879107be2a19451aa3efae7bf1a93d7127efd05`. The reviewer had already seen the
candidate diagnostic and the older reviews were not authored under the visual-safety truth contract, so
this finding is deliberately barred from certification truth.

It nevertheless exposes the exact weakness the next independent constituent must address. Marqo scored the
two observations inside the first confirmed interval only 0.2299 and 0.2214, while observations in the
second interval reached 0.9400. Any threshold above 0.2299 misses the first interval; at or below that value,
four of the six unresolved clean candidates signal. Because those six are not yet clean truth, this is not a
certified false-positive rate. It is sufficient to stop treating Marqo as a plausible solo lane and to make
low-contrast and varied-skin-tone sensitivity, without archival advertising false positives, the explicit
incremental requirement for the second portable constituent.

## Hosted semantic review result

The candidate-blind OpenRouter adapter makes at most one fallback-disabled request per invocation. It
requires a fresh capability snapshot, exact canonical model revision, one exact ZDR provider route,
strict structured output, and an already authorized per-request charge ceiling. Before network I/O it
durably writes a reservation. It preserves the raw response and closes the attempt as accepted, failed,
or unsettled; an ambiguous charged attempt is never retried. Its hosted input contains a complete H.264
video carrier with audio removed plus chronological contact sheets covering every exact planned frame.
Neither source identity nor candidate score is sent. Accepted results still keep truth creation, training,
and production admission false.

Qwen 3.8 27B on the pinned CoreWeave FP8 ZDR route supplied useful positive evidence only when reasoning
was disabled. The reasoning-enabled attempt consumed all 2,048 completion tokens without returning the
required structure, cost $0.0135004, and was retained as a failed non-result. The accepted positive review
cost $0.008791 and nominated eight broad `explicit_nudity_v1` ranges, including both the low-Marqo first
interval and the high-Marqo second interval. It marked coverage insufficient, so its positives are useful
but it cannot establish absence. Its sealed result identity is
`676a24a249065e2a106e74253666c72a788e0e559655beb833b5b8868c7bd574`.
The Mary Hartline review cost $0.0037822, returned no match, and also marked coverage insufficient; it is
therefore a hold, not clean evidence. Its sealed result identity is
`dd0e41e83340daee5cfa3bd4a1bbabe7f773d3297dc461e00d75652650cbff9a`.

Seed 2.1 Turbo reached the exact pinned Seed FP8 ZDR route but the provider returned `content_filter` with
no assessment or usage record on the positive source. The attempt remains unsettled with unknown charge
and was not retried. The Mary request was deliberately not sent: a reviewer unavailable on the known
positive cannot be a qualifying panel member. Known accepted spend across these calls is $0.0260736, plus
that one unsettled Seed attempt. This is decisive enough to retain Qwen as a semantic positive/escalation
detector while rejecting both Qwen and Seed as negative-adjudication authorities in this slice.

## Independent Freepik comparator result

The comparator runs as a separate offline, network-disabled worker so its capability and failures remain
independently attributable from Marqo. Its launcher pins the same read-only Linux-arm64 runtime image,
verifies the mounted worker-script digest before startup, and exposes capability SHA-256
`2ba07e792eda9b8a5bbc5f31aba5947b5488601b6ccc5c1f39daaf6875d95d82`. Loomarr retains all four raw
logits and computes cumulative `medium + high` scores outside the worker.

Eight rights-approved sources completed 518/518 planned frames without worker or coverage failure. The
known positive source's two observations inside the Marqo-missed interval scored 0.3892 and 0.3510; its
second confirmed interval also scored above the development sweep. A threshold through 0.35 detected both
confirmed development intervals, while 0.40 and above missed the first. The six unresolved clean candidates
had source maxima from 0.0149 through 0.1699, and none signaled at 0.20 through 0.90. The disputed Old Spice
source reached 0.7832 and would be escalated, which is appropriate because it remains unresolved rather than
clean truth. At the non-authorizing 0.30 corpus-expansion candidate, both confirmed intervals were detected,
zero of six unresolved clean candidates signaled, and Old Spice signaled.

This is a decisive improvement over Marqo alone: Freepik finds the exact weak interval without colliding
with the current clean-candidate range. It is not a threshold-selection or false-positive result because
there is only one targeted positive family and zero clean-truth families. The private complete summary is
SHA-256 `fb0ebe8ad3f61080e45412a5332cea91819f0c2535ad5a6924d23b15384f401e`.
CPU model time averaged roughly 2.04–2.10 seconds per frame, about twenty times Marqo's measured time, so
the present worker is suitable for offline acquisition/admission screening rather than a playback hot path.

## Certification-corpus acquisition decision

There is no trustworthy ready-made corpus that can be adopted as the V68 certificate. Existing sensitive-
content datasets generally label the broader concepts `porn` or `NSFW`, do not provide policy-exact visible-
anatomy intervals, were assembled from web media whose product-evaluation and redistribution rights are
unclear, or use model-authored labels. Conversely, a missing annotation or a generic `safe` label does not
prove complete absence. Open Images explicitly says its image-level labeling is non-exhaustive, so even its
9-million-image scale cannot create clean truth. [Open Images evaluation protocol](https://storage.googleapis.com/openimages/web/evaluation.html),
[Open Images V7 description](https://storage.googleapis.com/openimages/web/factsfigures_v7.html).

The practical source pool is rights-reviewed cultural open access, not a pornography benchmark:

- The Smithsonian exposes more than 5.1 million open-access assets and makes assets carrying its CC0
  designation available for reuse, including commercial reuse. It also warns that CC0 addresses copyright,
  not every possible privacy, publicity, trademark, or third-party right. This is the preferred reproducible
  clean-candidate pool and a supplementary historical-art positive pool. [Open Access](https://www.si.edu/OpenAccess),
  [FAQ](https://www.si.edu/openaccess/faq), [terms](https://www.si.edu/termsofuse).
- The Met collection API exposes public-domain status and public high-resolution image URLs without an API
  key. The Art Institute of Chicago API can filter `is_public_domain=true` and exposes IIIF image identities.
  Both are preferred over an aggregator because the institution supplies the work identity and rights signal.
  [Met API](https://metmuseum.github.io/), [Art Institute API](https://api.artic.edu/docs/).
- The Rijksmuseum data service publishes item-level rights identifiers and says most digitised objects are in
  the public domain. It is another institutionally identified source when an exact item carries PDM or CC0.
  [Data service](https://data.rijksmuseum.nl/), [information and data policy](https://data.rijksmuseum.nl/policy/information-and-data-policy).
- Wikimedia Commons has enough discovery breadth for positives: its `Nude women in art` category currently
  contains 179 direct files and 58 subcategories, while its general `Nudity` category exposes only 11 direct
  videos plus subcategories. Every file has its own licence, and Commons warns that depicted-person rights can
  still restrict reuse. Use it only as an index to adult historical artworks with an exact acceptable file
  licence; exclude contemporary people, minors, AI-generated work, and uncertain files.
  [Art category](https://commons.wikimedia.org/wiki/Category:Nude_women_in_art),
  [nudity category](https://commons.wikimedia.org/wiki/Category:Nudity),
  [reuse guide](https://commons.wikimedia.org/wiki/Commons:Reusing_content_outside_Wikimedia/en).

Public Domain Mark and CC0 must not be treated as interchangeable evidence. Creative Commons describes PDM
as an informational mark for work believed already free of copyright restrictions, whereas CC0 is a legal
waiver/dedication by a rights holder; neither silently clears privacy or publicity rights. The private rights
authority must retain the exact item-level claim and the acquisition-time source record rather than reducing
both to `public_domain`. [Creative Commons public-domain tools](https://creativecommons.org/public-domain/).

The available direct-video pool cannot presently supply 59 defensible policy-positive families. Research
datasets such as NPDI/Pornography-2k have ample video, but their broad pornography labels do not map to
`explicit_nudity_v1`, their official distribution is restricted by request and responsibility agreement,
and the underlying web-video copyright and consent posture is not suitable as Loomarr product truth.
[Official RECOD dataset page](https://recodbr.wordpress.com/code-n-data/). Pexels is also excluded absent
written permission because its current terms prohibit using API content to build or evaluate ML/AI systems.
[Pexels API terms explanation](https://help.pexels.com/hc/en-us/articles/900005880463-What-are-the-Terms-and-Conditions).

### One-maintainer protocol

Use automation to remove blind viewing, not to manufacture the answer:

1. Build a metadata-only candidate manifest of 120–150 adult historical-art positives and 300–350 clean
   candidates. Bind institution, object/work id, creator, collection, source URL, item-level rights statement,
   acquisition time, original byte hash, and a near-duplicate fingerprint before any candidate model runs.
2. Group by the underlying source work. One work may contribute at most one independent family. Different
   encodes, crops, pans, cuts, exposure lengths, or other derivatives of that work remain in the same family
   and exercise slices without increasing the denominator.
3. Let local classifiers and VLMs propose policy matches, deduplicate candidates, identify likely rights or
   age hazards, and prioritize disagreements. Do not show those answers in the truth-authoring view.
4. Present the maintainer one private keyboard-driven review board: the exact policy, the complete still or
   complete deterministic contact sheet, item-level rights evidence, and `positive`, `clean`, `uncertain`, or
   `exclude` actions. This is bounded adjudication of a frozen shortlist, not a blind real-time viewing pass.
   Uncertain items become replacements and never enter either denominator.
5. Lock at least 59 positive families and 100 clean families before opening model output. The 100-clean floor
   only gives a one-sided 95% upper family false-positive bound of about 3% when zero fire. If Loomarr wants a
   statistically supported 1% family-level upper bound, lock at least 299 clean families with zero fires;
   per-slice confidence claims require their own predeclared populations. Preserve additional replacement and
   development families outside the untouched certificate.
6. After the truth lock, run Marqo and Freepik over every exact planned frame, use Qwen only for positive
   escalation, and generate a short disagreement queue. Thresholds may be selected on a separate development
   partition; the final certificate is opened once.

The immediate transport plan needs one explicit design decision before it can certify. Each independently
sourced museum work can be losslessly wrapped in a deterministic video carrier and retain one family root;
transport variants would never create more families. That gives lawful, policy-exact visual material and
complete known source timelines without generating new semantic content. V68 also says generated controls
and transformed derivatives do not inflate the independent-source denominator and requires a decodable video
source. Until the design states whether one lossless carrier per independent underlying work counts, this
pool is authorized only for development and corpus-construction rehearsal—not the 59-family certificate.

Natural archival video remains a separate domain challenge even if the carrier decision is accepted. Add
rights-approved programme, advertising, animation, historical graphics, beach/swimwear, medical, low-light,
and visually busy sources, using complete contact sheets plus only the model-proposed full-resolution windows
for maintainer attention. A clean natural video still requires deterministic complete-source coverage; one
thumbnail or one model's negative cannot establish it.

This approach is consistent with NIST's recommendation to document test sets, metrics, measurement tools,
processes, and materials so evaluation is repeatable. It deliberately separates discovery models from the
locked evaluation population. [NIST AI RMF Measure 2.1](https://airc.nist.gov/airmf-resources/playbook/measure/).

Do not train an LLM or vision model during this acquisition slice. The initial corpus is a certificate, not a
training set. Training becomes rational only after production-like development data exposes a repeatable
error cluster, rights-cleared training families exist outside the certificate, and a second untouched
source-family-disjoint certificate remains available. A local Mac can later make discovery, dense rescoring,
deduplication, and private taxonomy extraction cheap; it does not change this truth-separation requirement.

### Corpus-draft implementation checkpoint

The development-only preparation module now makes that acquisition protocol executable without resolving the
carrier question by implication. `PrepareVisualCorpusDraft` accepts one already sealed, pre-model authority
plus private source, policy, and alias-seed locations. Behind that single interface it reopens every source
work and rights-review byte, accepts only complete PNG or JPEG images within fixed byte/pixel ceilings,
requires at least the authority's 120-positive/300-clean candidate-pool targets, rejects source-work, family,
independence-group, exact-byte, positive-creator, rights-evidence, and normalized difference-hash collisions,
and verifies a work-specific acquisition review. The rights review permits private retention and model
evaluation only; training and production broadcast must be false.

The module publishes atomically into a mode-`0700` root with mode-`0600` files. Its shareable review directory
uses keyed opaque aliases and contains the exact policy, source works, rights evidence, a path-free manifest,
and a self-contained keyboard review board. The board offers positive, clean, uncertain, and exclude actions,
restores local progress when the browser permits private-file storage, and downloads an alias-only decision
document. It contains no candidate id, family, creator, nomination, model output, truth, or admission result.
The separate owner map restores those identities after review. Reopening the draft reproduces the board and
every image, rights, policy, manifest, and owner-map byte and rejects permissions drift, image/pixel drift,
malformed rights, missing or extra files, and model-result sidecars. A command wrapper supplies the same deep
interface to private request files.

This checkpoint uses generated test images only. It acquired no external media, authored no real truth, made
no provider call, selected no threshold, wrapped no video carrier, and granted no certification, training,
ingestion, scheduling, or broadcast authority.

### Institutional-source transport probe

A bounded no-cost probe confirmed that the Met can supply the institutional still-art lane without adding a
second acquisition framework. Met search does not reliably apply `isPublicDomain=true` as a server-side
filter, so an adapter must use explicit search terms, retrieve candidate object records serially within a
predeclared request ceiling, and admit only records whose object response says `isPublicDomain: true` and
whose image URL uses the exact `images.metmuseum.org` host. It must freeze the original object response before
rights review; the search result alone is not rights evidence.

Object `195733`, _Venus_ by Massimiliano Soldani, proved the complete transport path. The object response is
SHA-256 `8bffbf95ed2574d48322cb67c4ef27cd6d536b7c6e1e1b147a1eb91f1d00032c`; its declared primary image
downloaded as a complete 2,989-by-4,000 JPEG of 1,156,190 bytes with SHA-256
`87392d74fc09c959ba3a0adf9b53eb8b47e77114a103e7967997beb4ea523f3b`. Both artifacts remain in the
private development area with mode `0600`. This proves transport and reproducibility only. It is not a rights
approval, truth label, independent-family admission, model result, or broadcast authorization.

The Art Institute API remains useful for metadata discovery: an exact public-domain/image-backed query found
324 records, and three sampled object responses were preserved privately. However, the corresponding IIIF
image requests currently receive a Cloudflare browser challenge (`403`), including with an identifying user
agent. The institution's public repositories are tracking the same image-service failure
([API data #9](https://github.com/art-institute-of-chicago/api-data/issues/9),
[data aggregator #151](https://github.com/art-institute-of-chicago/data-aggregator/issues/151),
[data aggregator #157](https://github.com/art-institute-of-chicago/data-aggregator/issues/157)). Loomarr must
not bypass that challenge or pretend metadata-only records are acquired images. Keep this source disabled
until the documented non-browser image route works again.

The ownership boundary is therefore:

- `fillercorpus` owns bounded discovery, raw metadata evidence, immutable source/work identity, representation
  facts, inventory merge, and the human rights worksheet;
- the shared private materialization step downloads only rights-approved representations and verifies their
  exact bytes, declared media type, decodability, and resource ceilings;
- `fillervisualsafety` owns opaque review, policy truth, family/creator/near-duplicate independence checks,
  model-result separation, and certification measurements.

The existing `fillercorpus.SourceClient` should be reused for public metadata requests. It should not cache
sensitive image bytes under its metadata cache contract. Neither acquisition concern belongs inside
`PrepareVisualCorpusDraft`, and neither changes the unresolved deterministic-carrier decision.

That boundary is now executable. `CaptureMetInventory` consumes one canonical term set plus exact request,
object-lookup, item, metadata-byte, predicted-media-byte, delay, and wall-time ceilings. It queries only the
exact Met API and image hosts, freezes each raw object response in the existing source cache, and rejects a
search hit unless the individual object record has `isPublicDomain: true`, a trusted original image, a stable
object URL, a named creator, and one of the term set's required source-authored subject tags. The committed
adapter also canonicalizes the Met creator display name and rejects a repeated creator before probing the
candidate image. That is a useful within-source independence screen, not proof that differently spelled names
or cross-institution records describe different people; the later locked-corpus checks remain authoritative.
The positive seed searches `adam and eve`, `aphrodite`, `bathers`, `nude`, and `venus`, but requires `Female Nudes`
or `Male Nudes` and rejects the exact Met subject terms `Adolescents`, `Boys`, `Children`, `Girls`, and
`Infants`. Both required and excluded sets are part of the capture identity. Search term, all returned subject
terms, creator, source work, image facts, and raw-metadata identity survive into the source-neutral inventory
and rights worksheet. They remain discovery evidence, not truth, an adult-status decision, or a reusable
taxonomy assertion.

A current-code real probe admitted three candidates from 22 serial requests in 4.730 seconds: _Naval Battle_,
_Woman Sitting Half-Dressed beside a Stove_, and _Allegory of America_. All three carried a required Met nude
subject tag, no excluded subject tag, three distinct creators, and three distinct source-work families. The
response budget used 122,311 of 16,777,216 bytes and predicted 9,500,798 bytes of media without downloading
it. The exact inventory is SHA-256 `17d8df0f2cf10f7540535d2b951e5e8c9c6bb0e17a910cd45c9ddb407e036f0a`;
its directory, cache, and inventory are mode `0700`, `0700`, and `0600`, respectively. Preceding diagnostic
probes admitted a decorative vase from the `adam and eve` search and a nude-tagged work whose Met terms also
included `Infants`; the bound required/excluded subject rules removed both weak or unsafe nominations before
any image request. The current inventory also produced a schema-4 inert rights worksheet and CSV at SHA-256
`fd783f7f662fc123b3ef0c92bcab79f4ef1c2824af8b3f7e41ebad58a2b2731c` and
`56d2d74b05eb41607108e9f1692a833f0cbb6cdea2de56410c41bbdc109add47`; every reviewer and decision field
remains blank. No external image was fetched in these adapter probes.

The existing rights-bound downloader is now the one shared materializer rather than a second visual-only
implementation. Its shared schema-3 ledger supports MP4, JPEG, and PNG; derives the local extension from the admitted
MIME type; enforces the response MIME type, exact byte count, optional source hashes, complete image decode,
terminal JPEG/PNG boundary, dimensions, and a maximum 50-million-pixel image; rejects duplicate exact media;
retains capture, role, creator, subject-term, campaign, source-family, profile, and processor identity; and
publishes non-overwriting mode-`0600` files beneath a mode-`0700` directory. The downloader and downstream
consumers use one strict `fillercorpus` decoder and inventory-bound validator rather than reconstructing the
command's former private ledger shape. The visual draft independently
repeats complete-image and pixel checks at its trust boundary. Materialization still requires an independently
locked rights decision tied to the exact inventory and metadata. Creating visual-corpus subject status,
generated status, policy nomination, diagnostic slices, or truth from source tags remains forbidden.

The next handoff is also executable without widening that authority. One deep visual nomination workflow
reconstructs the exact inventory and materialization ledger, reopens every private image, and prepares an inert
worksheet whose CSV has only four editable per-case fields: nomination, subject status, generated status, and
diagnostic slices. Its current closed adapter accepts only Met JPEGs whose independent rights rationale starts
with `met_cc0_open_access_object_reviewed_v1: ` plus a non-empty explanation and has no restriction. Every source, creator, family, object, approval, file, media, and perceptual
identity is immutable. Locking re-runs preparation and rejects a changed cell, unresolved or contradictory
judgment, symlink/path escape, changed bytes, duplicate exact or normalized image, repeated work/family, or repeated
positive creator. It atomically publishes a mode-`0700` source root with mode-`0600` assets, rights evidence, and
draft-compatible candidates. Rights evidence schema 2 now binds the exact inventory, materialization, approval,
case, and content SHA-256 identities; no hand-authored provenance re-entry is needed. Both worksheet and locked set
state that candidate-model output, truth authority, training, and production use are false. This eliminates the
large mechanical part of review while preserving the few decisions that genuinely require a maintainer.

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
3. The source-family-disjoint scorer, exact-policy review bundles, one-request OpenRouter adapter, and first
   candidate-blind Qwen/Seed checks are implemented. Qwen is retained for positive semantic escalation;
   neither hosted model may establish a negative result from the observed evidence.
4. Freepik is reproducibly exported and passes the exact incremental development requirement across 518
   frames. Retain it as the second local constituent and predeclare the 0.20–0.35 threshold region before
   expanding into independently selected truth families. Continue to treat any positive as a nomination and
   every incomplete or no-signal disagreement as a hold; do not use majority voting.
5. Construct and lock the source-family-disjoint certification authority before the larger run. Keep the current
   `visual_safety_not_certified` production hold until both certification and issue #947's separate
   release authority are complete.
