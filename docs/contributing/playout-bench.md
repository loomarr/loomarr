# Playout bench

**For:** contributors changing playout, or checking a new host's encoder.
**You'll get:** how to run the bench, and whether this host still meets the playout numbers.

The playout bench answers one question on any host: **does Loomarr's real playout pipeline still meet
the beta.8 numbers here?** It runs a generated, redistributable corpus through `playout.Build`, the
same pipeline builder the live chain uses, measures it, judges the result against the thresholds and
diffs it against the last accepted report for that hardware family.

```sh
make playout-bench                       # detect the host as the app does at boot
PLAYOUT_BENCH_FAMILY=software make playout-bench   # force a family (a GPU host, GPU ignored)
```

It needs `ffmpeg` and `ffprobe` on `PATH` (or `PLAYOUT_BENCH_FFMPEG`), with `libx264`, `libx265` and,
for the VMAF number, `libvmaf`. Run it on a quiet machine: it measures speed and CPU, so another heavy
job on the host makes the numbers meaningless.

## What it measures

| Metric | How | Threshold (hardware families) |
| --- | --- | --- |
| `start_p95_ms/<class>` | **encoder start (warm)**: fresh process per run, seeking to a different second, until the first second of media (one segment) is produced. Time to media, never bytes. Source files are page-cached, so cold network-share reads are excluded; those are host-specific and belong to supervised household runs, not CI. | H.264 1080p ≤ 400 ms, HEVC 1080p ≤ 500 ms, 4K HDR ≤ 1800 ms |
| `speed_x/<class>` | one whole clip, unpaced | ≥ 15x (1080p), ≥ 2x (4K HDR) |
| `cores_per_stream/<class>` | child CPU time (rusage) per second of media, so the cores one stream costs at 1x | reported |
| `concurrency/max_streams` | largest number of simultaneous 1080p H.264 streams that each hold ≥ 1.2x | ≥ 1 (every family) |
| `concurrency/total_cores` | CPU of those streams at 1x | ≤ 1 core (hardware families) |
| `break/pts_gaps`, `break/audio_off_grid` | video PTS off the frame grid, audio PTS off the AAC frame grid, across every corpus clip | 0 |
| `break/sps_variants` | distinct H.264 SPS across every clip's output, so items of any geometry, cadence or codec splice into one decoder | 1 |
| `break/loudness_dev_lu` | worst integrated-loudness deviation from −23 LUFS on the generated clips, no `loudnorm` | ≤ 1 LU |
| `break/first_packet_ms` | worst time to first output byte across the commercial spots | ≤ 250 ms |
| `vmaf_mean/<class>` | VMAF of the output against the source scaled to the rung | tracked against the baseline only |

Classes are `h264-1080p`, `hevc-1080p` and `hdr-4k`. A source the builder refuses on this host (a
software host that cannot tone-map 4K HDR in real time) is reported as **refused**, its metrics are
marked skipped and it is not a failure: the slate covers that slot.

**Known failures.** A check that fails until referenced work lands is listed in `expectedFailures`
(`internal/playoutbench/compare.go`) and reported as *expected fail* with that reference, never skipped.
Once the fix lands the check passes, which the bench reports as an *unexpected pass* and fails on, so the
entry is deleted in the same change. The list is empty: `break/sps_variants` (an 854×480 source scaled to
SAR 1281:1280 in the SPS) passes since the packager's items encode with `setsar=1` (#1526).

The software family has no GPU thresholds; it is judged on sustaining a stream at 1.2x and on
correctness; the channel capacity it measures (`concurrency/max_streams`) is reported with every run.
`PLAYOUT_BENCH_THRESHOLDS=correctness` judges only the exact checks (gaps, SPS,
loudness). CI uses it on the virtualised macOS runner. `off` skips judging.

## The corpus

Everything is synthetic, made with ffmpeg `lavfi`, and generated on first use into
`$LOOMARR_ARTIFACT_DIR/playout-bench-corpus`: H.264 (23.976, 25 and 29.97 fps, interlaced, 10-bit),
HEVC 10-bit, a 4K HDR10-tagged HEVC clip, and AAC, AC-3, E-AC-3 and TrueHD 5.1 audio. Four
commercial spots (720p, 1080p, 480p, HEVC) with fades, all built at −23 LUFS, form the break sequence.
Changing any clip changes `corpus`, which invalidates every baseline.

For realistic content, fetch Blender's *Tears of Steel* (CC BY 3.0) with a pinned hash and pass the
directory in. Household media must never enter the bench.

```sh
scripts/playout-bench-open-films.sh "$LOOMARR_ARTIFACT_DIR/films" 720p    # add 1080p
PLAYOUT_BENCH_FILMS="$LOOMARR_ARTIFACT_DIR/films" make playout-bench
```

Each film contributes a `film-<name>` class over its first 30 seconds.

## Reading a report

`make playout-bench` prints markdown and writes `playout-bench-<family>.json` and `.md` to
`$LOOMARR_ARTIFACT_DIR`. It has three parts:

1. **Thresholds**: each beta.8 limit with the measured value. A threshold whose metric is absent
   fails; only a metric the host declares skipped (no `libvmaf`, refused HDR) is excused.
2. **Against the accepted baseline**: every metric's change. A change is a regression only if it is
   worse than the baseline by more than the tolerance (15% by default, `PLAYOUT_BENCH_TOLERANCE`)
   **and** by more than a small absolute slack per unit (25 ms, 0.3x, 0.01 cores), so a timer tick on
   a fast start is not a failure. Correctness counts must not change at all. A metric that vanishes
   without a skip reason is a regression.
3. **Metrics** and **Clips**: everything measured, and each clip's outcome and any stage that left the
   GPU (`decode: …`, `tonemap: …`).

The command exits non-zero on a failed threshold, a regression or a clip that failed to encode.

## Accepting a new baseline

Only from a supervised run on the real hardware, on a commit you have read the numbers for:

```sh
PLAYOUT_BENCH_ACCEPT=1 make playout-bench
git add docs/engineering/playout-bench/<family>.json
```

The bench refuses to accept a report that fails a threshold or regresses. Fix the cause, or change
the threshold in `internal/playoutbench/compare.go` in the same PR that explains why. A baseline is
tied to the schema and corpus version; bumping either forces a re-accept.

## CI

| Job | When | What |
| --- | --- | --- |
| `Playout bench — software-only pipeline` (`ci-playout-bench.yml`) | every PR that touches playout, the bench or its scripts (classifier gate `playout_bench`, part of `CI`) | 720p software family, pinned production ffmpeg, against `docs/engineering/playout-bench/ci/` |
| `macos-15 — VideoToolbox` (`playout-bench.yml`) | `workflow_dispatch` and nightly | VideoToolbox family; correctness thresholds only, since the runner is virtualised |
| `GitHub T4 GPU — NVENC` | `workflow_dispatch` and nightly, **only when the repository variable `PLAYOUT_BENCH_T4_RUNNER` is set** | needs a paid GPU larger runner (about $0.052/min); set the variable to its label to enable |
| `Self-hosted — Intel Arc`, `Self-hosted — NVIDIA GeForce` | `workflow_dispatch` only | the maintainer's machines |

`macos-15` arm64 runners expose VideoToolbox hardware encode; `macos-14` does not.

The macOS job installs a SHA-256-pinned static arm64 ffmpeg (VideoToolbox included) with the same bounded
retry as `scripts/ci-ffmpeg.sh`, never an unpinned Homebrew bottle. It is ffmpeg 9.0.2 because no
older macOS arm64 build is published; that is not the production 8.1 pin. ffmpeg 9 breaks the concat
advance over a chunked HTTP body, which the bench never exercises: it drives `playout.Build` pipelines
and reads the output directly. The bench films are Tears of Steel 720p and 1080p only; the 4K class
runs on generated HDR10 clips.

### Self-hosted runners

Register each machine as a repository runner with the custom label `loomarr-playout-arc` or
`loomarr-playout-geforce`, with `ffmpeg`, `ffprobe`, `go` and `make` on its `PATH`. The workflow
`playout-bench.yml` must never gain a `pull_request`, `pull_request_target` or `merge_group` trigger:
the repository is public, and a pull request must not be able to schedule work on these machines. A
test (`TestPlayoutBenchWorkflowNeverRunsOnPullRequest`) enforces that. Dispatch the workflow from the
Actions tab, tick `run_arc` or `run_geforce`, and tick `accept` to have the run write the family
baseline into the uploaded artifact for you to commit.
