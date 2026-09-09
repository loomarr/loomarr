# Declared-profile playout qualification

This procedure supplies the declared-profile portion of [#1037](https://github.com/loomarr/loomarr/issues/1037).
The [design contract](../design.md#production-path-playout-load-certification-1037) owns the thresholds.
Readiness lifecycle acceptance remains tracked in [#1097](https://github.com/loomarr/loomarr/issues/1097).

Build the command from a clean, committed checkout on the candidate's target platform. Its embedded
full VCS revision identifies the code under test. Set `LOOMARR_ARTIFACT_DIR` to a private evidence
directory and retain the command's hash, exact source commit, FFmpeg/ffprobe hashes, container image,
CPU/memory limits, GPU/device access and selected quality tier with the run.

```sh
go build -o "$LOOMARR_ARTIFACT_DIR/playout-load-cert" ./cmd/playout-load-cert
"$LOOMARR_ARTIFACT_DIR/playout-load-cert" \
  --synthetic --quality-tier balanced --certify \
  --manifest /private/qualification/channels.json \
  --synthetic-scope beta5-qualified-target \
  --disposable-target beta5-qualified-target \
  --fault-profile child_failure --fault-profile parent_failure \
  --ffmpeg /opt/ffmpeg/ffmpeg --ffprobe /opt/ffmpeg/ffprobe \
  --programme-boundary-timeout 2m --suite-timeout 30m
```

Run the terminal shutdown drill in a fresh disposable target: repeat the command with only
`--fault-profile shutdown`, retaining the matching scope/acknowledgement and choosing a different
`--out` report path under `LOOMARR_ARTIFACT_DIR`. Shutdown cannot be combined with the other fault
profiles. Retain both reports; one drill does not substitute for another.

Use the existing private ordered Channel manifest format. It must cover at least 100 configured
Channels, a declared `prepared` cohort, enough deliberately cold transcode Channels for the measured
capacity and overload attempt, and copy/codec roles required by the intended qualification. A role
label alone never proves a codec. The generated sources cover H.264/AAC; use the existing
`--operator-cohort` mode instead of `--synthetic` for additional independently characterized codec inputs.
Keep signatures, paths and media private. Both isolated modes accept the same explicit quality tier.

The command probes the tier's highest live rendition and uses the resulting admission capacity.
It rejects `--synthetic-capacity` alongside a declared tier. Live profiles follow the production
load-dependent ladder; prepared output uses the tier's canonical rendition. Generated source sizes
and frame rates follow that tier. The report's `target.profile` records the selected encoder,
measured admission budget, probe dimensions and prepared dimensions. These fields describe the
workload; only the complete passing run supplies concurrent-load evidence.

The prepared cohort converges through the production runtime resolver, persistent readiness index,
planner, source access and packager under the shared encode pool. Software-only or single-slot
targets cannot satisfy this background-preparation lane and hold qualification. Missing readiness
also fails setup. The planner retains its normal ten-minute deadline drain reserve, so use the
documented 30-minute suite budget. Periodic preparation is cancelled and joined on shutdown.

The small default software fixture remains useful for harness regression tests. It does not qualify
a household profile. The declared-profile run initially uses two repeated generated sources across
many Channels; it proves configured scale and shared-publication readiness, not unique-source
throughput or real-media codec diversity. Keep restart, source/schedule changes, retention,
foreground-preemption and installed-device evidence with the final release packet. No isolated
target starts or discovers the maintainer's smoke stack.
