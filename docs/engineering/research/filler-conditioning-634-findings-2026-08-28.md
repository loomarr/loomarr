# Issue #634 conditioning findings — 2026-08-28

This note records the final measured fixture observations for the reviewed split-to-air journey.
The journey uses synthetic, redistributable media generated locally from ffmpeg lavfi sources;
it contains no downloaded or third-party media.

The reviewed child carries its parent content hash and intended `[start,end)` interval. The
existing `internal/mediatools` packet seam measures actual-minus-intended start and end errors per
presented audio/video stream, and the persisted sidecar retains those measurements with the
before/after conditioning evidence. Missing or mismatched edge evidence remains review-only.
The real-ffmpeg journey also verifies measured stream timing, cadence, loudness, true peak, and
the viewer-visible return from a mid-break child to scheduled program content.

In the generated 320x180, 30000/1001-fps compilation, the reviewed 12-second child measured
12,012 ms before rewrite. Packet-matched pre-rewrite edge errors were video start 0 ms/end +12 ms
and audio start 0 ms/end +10 ms. The rewritten mezzanine measured 12,100 ms with +88 ms ending
A/V skew; its direct post-rewrite cut edges were explicitly unavailable because packet identity
changed. Measured integrated loudness was -21.8 LUFS before and -23.0 LUFS after, with finite true
peak values on both artifacts. The final production mux measured about 7.0 seconds for the two
program blocks and the 3-second filler remainder, with the generated cadence preserved. The
fixture also supplies black, silence, and freeze cases for the detector seam; those observations
are persisted as bounded intervals rather than inferred from stream presence.

These are observations, not policy: this issue deliberately proposes no boundary-error threshold,
automatic correction, admission rule, or source rewrite. Any such policy requires a separate
measured study and explicit design decision.
