package filler

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// On-file loudness normalisation (§10 V42) — the opt-in behind
// `filler.autofile.normalize_loudness`, surfaced as the Tune panel's second toggle.
//
// ⚠ **DEFAULT OFF, and everything here assumes an operator asked for it.** V40 chose
// playout-only normalisation and that is still the default path: at playout the correction is one
// filter on a stream already being encoded, it is reversible, and changing the target later simply
// works. This rewrites the operator's file — the original is unrecoverable — which is why it is a
// setting rather than a behaviour.
//
// ⚠ **Playout keeps normalising too, and that is not redundancy.** A file normalised on disk
// already measures at target, so the playout filter is a no-op for it; clips that arrived before
// the toggle was switched on, or by a route that skipped auto-file, are still corrected there. The
// promise "every break plays at a consistent level" must not depend on which clips happened
// through one optional step.

// normalizeToleranceLU is how close to target counts as "already done".
//
// ⚠ Not zero, because loudnorm lands NEAR the target rather than exactly on it — V40 measured
// −26.8 → −23.4 and −32.6 → −23.1 against a −23 target, both correct results. An exact comparison
// would re-normalise every clip on every pass, which is the failure the marker exists to prevent.
// 1.0 LU is comfortably wider than loudnorm's own error and far narrower than the ~11 dB spread
// the feature addresses.
const normalizeToleranceLU = 1.0

// NormalizeInPlace rewrites `path`'s audio to `targetLUFS`, in place, and records the marker.
//
// ⚠ **THE ONLY function in this package that modifies a media file.** Everything else — intake,
// scan, artwork, sidecar writes — leaves the operator's bytes alone. Reached only from the
// auto-file step and only when `filler.autofile.normalize_loudness` is on.
//
// Returns (true, nil) when it normalised, (false, nil) when it skipped as already-normalised.
// A skip is an ordinary outcome, not a failure.
func NormalizeInPlace(ctx context.Context, ffmpegPath, path string, targetLUFS float64) (bool, error) {
	// ⚠ THE IDEMPOTENCY GATE, and the pass is a bug without it. A normalised file is
	// indistinguishable from any other file by inspection, so every re-scan would normalise an
	// already-normalised clip and walk its loudness down run after run. Read from the sidecar so
	// the answer survives a catalog rebuild.
	if tags, ok := ReadSidecarTags(path); ok {
		if tags.NormalizedLUFS != 0 && math.Abs(tags.NormalizedLUFS-targetLUFS) <= normalizeToleranceLU {
			return false, nil
		}
	}

	// ⚠ Write to a TEMPORARY file beside the original, then rename over it. ffmpeg cannot read
	// and write the same path — pointed at its own input it produces a truncated or empty file —
	// and the rename makes the replacement atomic, so a crash mid-encode leaves the original
	// intact rather than a half-written clip in the catalog.
	tmp := path + ".loudnorm.tmp" + filepath.Ext(path)
	defer func() { _ = os.Remove(tmp) }()

	// ⚠ TWO-PASS here, unlike playout's single-pass. Playout is single-pass because measuring the
	// whole file before emitting a frame would stall a live stream; this is a batch job with
	// nobody waiting, so it can afford the accurate form. Inheriting playout's compromise would
	// mean accepting its first-second ramp for no reason.
	//
	// ⚠ VIDEO IS COPIED, never re-encoded. Re-encoding to adjust audio would degrade the picture
	// on every pass for no benefit — and this feature is about loudness alone.
	target := strconv.FormatFloat(targetLUFS, 'f', -1, 64)
	cmd := exec.CommandContext(ctx, ffmpegOr(ffmpegPath),
		"-nostdin", "-v", "error",
		"-i", path,
		"-af", "loudnorm=I="+target+":TP=-1:LRA=11",
		"-c:v", "copy",
		"-c:a", "aac", "-b:a", "192k",
		"-y", tmp)
	if out, err := cmd.CombinedOutput(); err != nil {
		return false, fmt.Errorf("normalize %s: %w: %s", filepath.Base(path), err, truncate(string(out), 200))
	}

	// ⚠ Refuse to replace the original with something implausibly small. ffmpeg can exit 0 having
	// written a header and no samples (a truncated input, an unreadable audio stream), and that
	// file would sail through as "normalised" — the same class as the 2.9KB/33ms download V40's
	// duration floor was added for, except here it would DESTROY the good copy.
	info, err := os.Stat(tmp)
	if err != nil {
		return false, fmt.Errorf("normalize %s: no output written: %w", filepath.Base(path), err)
	}
	if orig, oerr := os.Stat(path); oerr == nil && info.Size() < orig.Size()/10 {
		return false, fmt.Errorf(
			"normalize %s: output is %d bytes against an original of %d — refusing to replace it",
			filepath.Base(path), info.Size(), orig.Size())
	}

	if err := os.Rename(tmp, path); err != nil {
		return false, fmt.Errorf("normalize %s: %w", filepath.Base(path), err)
	}

	// ⚠ The marker is written AFTER the rename. Written before, a failed encode would leave a
	// file marked as normalised that never was — and nothing would ever revisit it.
	tags, _ := ReadSidecarTags(path)
	tags.NormalizedLUFS = targetLUFS
	if err := WriteSidecarTags(path, tags, false); err != nil {
		// The audio IS normalised at this point; only the marker failed. Report it so the next
		// pass re-does the work rather than silently claiming success it cannot prove.
		return true, fmt.Errorf("normalize %s: normalised, but the marker could not be written: %w",
			filepath.Base(path), err)
	}
	return true, nil
}
