# Portable visual-sensitive-content lane

Issue: [#951](https://github.com/loomarr/loomarr/issues/951)  
Date: 2026-09-04  
Status: research decision only. This note grants no quarantine, ingestion, scheduling, training, or broadcast authority.

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

1. Add the generic portable-worker protocol and model/runtime capability authority without choosing a
   threshold or enabling production behavior.
2. Add a single-pass exact-frame decoder that proves every planned timestamp and complete source decode;
   do not reuse the four-frame taxonomy extractor.
3. Build reproducible Marqo and Freepik ONNX exports outside the application, then lock their hashes.
4. Run conversion parity and a small rights-cleared development diagnostic. Stop a candidate early on
   parity failure, unacceptable CPU throughput, or obvious positive misses/clean false positives.
5. Only then construct and lock the independent certification authority. Keep the current
   `visual_safety_not_certified` production hold until both certification and issue #947's separate
   release authority are complete.
