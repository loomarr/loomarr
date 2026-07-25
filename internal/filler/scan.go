package filler

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Scanning FILLER_DIR directly (§10, revised by §9.1).
//
// Loomarr walks its own drop-folder and probes each clip with ffprobe. This replaces asking
// Tunarr to scan the folder and reporting its program ids back — see Clip for why the old
// arrangement could not serve internal playout, and why the dependency ran the wrong way.
//
// The premise that justified the detour is also gone: it existed so "probing stays out of
// loomarr entirely", and §14 now bundles ffmpeg AND ffprobe as core runtime dependencies
// precisely because internal playout owns duration and cut points. Scanning locally spends a
// dependency we already have rather than requiring a service we made optional.

// clipExtensions are the container formats a filler clip may use.
//
// An ALLOWLIST rather than "probe everything and see": a drop-folder accumulates
// `.DS_Store`, `.nfo`, partial `.part` downloads and cover art, and probing each one costs an
// ffprobe exec plus a log line. Extensions are the cheap first filter; ffprobe is still the
// authority on whether a file has a usable stream.
var clipExtensions = map[string]bool{
	".mp4": true, ".mkv": true, ".avi": true, ".mov": true,
	".m4v": true, ".webm": true, ".ts": true, ".mpg": true, ".mpeg": true,
}

// Prober reads a media file's duration. Satisfied by FFprobeDuration; injected so the scanner
// is testable without executing a binary.
type Prober func(ctx context.Context, path string) (int64, error)

// ScanDir walks dir and returns one RawClip per playable file found.
//
// Errors on individual files are SKIPPED, not fatal. A drop-folder is operator-managed and
// will contain junk, a half-copied file, or something with no video stream; refusing to build
// a catalog because one file is bad would mean one stray download silently costs a channel all
// of its commercials. What is skipped is reported so the caller can log a count.
func ScanDir(ctx context.Context, dir string, probe Prober) (clips []RawClip, skipped int, err error) {
	if dir == "" {
		return nil, 0, nil // filler not configured — an empty catalog, not an error
	}
	if probe == nil {
		probe = FFprobeDuration
	}

	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// An error on the ROOT is fatal; one on a child is not, and telling them apart
			// matters. WalkDir reports a missing/unreadable root by calling this callback once
			// with path == dir, so swallowing every error here made a misconfigured FILLER_DIR
			// return an empty catalog and no error — "filler mysteriously does nothing", the
			// exact failure the check below was written to prevent. (Verified by executing
			// WalkDir against a missing directory; the test asserted the intent and caught it.)
			if path == dir {
				return err
			}
			// A child: an unreadable subdirectory shouldn't abort the whole scan.
			skipped++
			return nil //nolint:nilerr // deliberate: skip and continue
		}
		if d.IsDir() {
			return nil
		}
		if !clipExtensions[strings.ToLower(filepath.Ext(path))] {
			return nil // not a media container; not counted as skipped (it is not a clip)
		}

		// The identity: path RELATIVE to dir, with forward slashes so the id is identical on
		// every platform. A Windows-authored catalog and a Linux one must agree, because the
		// pod seed hashes clip ids and a differing separator would silently change pod
		// contents rather than failing.
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			skipped++
			return nil
		}
		rel = filepath.ToSlash(rel)

		durMs, probeErr := probe(ctx, path)
		if probeErr != nil || durMs <= 0 {
			// No usable duration means the pod assembler cannot place it: it fills a break to
			// a target length, so a zero-duration clip would either be skipped downstream or
			// break the arithmetic. Rejecting here keeps that invariant at the boundary.
			skipped++
			return nil
		}

		name := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
		clips = append(clips, RawClip{
			Path: rel,
			Name: name,
			// Kind + Era from the filename — the cheapest tagging tier (§10). Without this a
			// clip lands as a generic interstitial the pod assembler can never place, so
			// filler would silently never build unless AI tagging is on.
			Kind:       KindFromName(name),
			Era:        EraFromName(name),
			DurationMs: durMs,
		})
		return nil
	})
	if walkErr != nil {
		// The directory itself is unreadable or missing. That IS worth reporting: it is
		// almost always a misconfigured FILLER_DIR, and silently returning an empty catalog
		// would present as "filler mysteriously does nothing".
		return nil, skipped, fmt.Errorf("scan filler dir %s: %w", dir, walkErr)
	}
	return clips, skipped, nil
}

// FFprobeDuration returns a media file's duration in milliseconds.
//
// JSON output rather than the `-show_entries … -of csv` form: csv silently yields "N/A" for a
// file with no duration metadata, which parses to 0 and is indistinguishable from a real
// zero-length result. JSON gives an absent field we can tell apart.
func FFprobeDuration(ctx context.Context, path string) (int64, error) {
	return ffprobeDurationWith(ctx, "ffprobe", path)
}

// FFprobeDurationNextTo returns a Prober using the ffprobe that sits ALONGSIDE the given ffmpeg
// binary.
//
// It takes the ffmpeg path deliberately, because that is the setting operators actually have
// (`playout.ffmpeg_path`), and derives ffprobe from it: the two ship together, so an operator
// who moved one moved both. Adding a second path setting would be a knob whose only correct
// value is derivable from the first.
//
// ⚠ It takes the FFMPEG path and returns an FFPROBE prober, which is worth stating loudly: an
// earlier version was named FFprobeDurationUsing(bin) and was handed `playout.ffmpeg_path` at
// the call site. That ran `ffmpeg -show_entries format=duration -of json`, which is not a valid
// ffmpeg invocation, so EVERY probe failed — and because ScanDir skips unprobeable files by
// design, the catalog came back silently empty with no error anywhere. The name now says which
// binary it wants.
func FFprobeDurationNextTo(ffmpegPath string) Prober {
	bin := "ffprobe"
	if ffmpegPath != "" && ffmpegPath != "ffmpeg" {
		// Same directory, ffmpeg → ffprobe. Handles /opt/ffmpeg/bin/ffmpeg and a
		// ffmpeg-with-suffix build alike, since only the basename's leading token changes.
		dir, base := filepath.Split(ffmpegPath)
		bin = filepath.Join(dir, strings.Replace(base, "ffmpeg", "ffprobe", 1))
	}
	return func(ctx context.Context, path string) (int64, error) {
		return ffprobeDurationWith(ctx, bin, path)
	}
}

func ffprobeDurationWith(ctx context.Context, bin, path string) (int64, error) {
	// -show_format, not -show_streams: container duration is what a pod needs (how long the
	// clip occupies a break), and a stream's duration can differ from it — an audio stream
	// running a few frames past the video is normal and would give a subtly wrong answer.
	out, err := exec.CommandContext(ctx, bin,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "json",
		path,
	).Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe %s: %w", filepath.Base(path), err)
	}

	var probed struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &probed); err != nil {
		return 0, fmt.Errorf("ffprobe %s: parse output: %w", filepath.Base(path), err)
	}
	if probed.Format.Duration == "" {
		return 0, fmt.Errorf("ffprobe %s: no duration in the container", filepath.Base(path))
	}
	secs, err := strconv.ParseFloat(probed.Format.Duration, 64)
	if err != nil {
		return 0, fmt.Errorf("ffprobe %s: duration %q: %w", filepath.Base(path), probed.Format.Duration, err)
	}
	if secs <= 0 {
		return 0, fmt.Errorf("ffprobe %s: duration %v is not positive", filepath.Base(path), secs)
	}
	return int64(secs * 1000), nil
}

// ClipPath resolves a clip's identity back to an absolute path ffmpeg can read.
//
// ⚠ THIS IS A SECURITY BOUNDARY, not just a join. A clip id reaches here from the database,
// and the pod assembler and the API both accept ids from callers — so a crafted id containing
// `../` would otherwise make playout read (and stream) an arbitrary file outside FILLER_DIR.
// Containment is verified against the CLEANED path rather than by rejecting ".." textually,
// which misses encodings and symlink-shaped tricks.
func ClipPath(dir, clipID string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("filler: no FILLER_DIR configured")
	}
	if clipID == "" {
		return "", fmt.Errorf("filler: empty clip id")
	}
	// An absolute id is never legitimate — ids are relative by construction (see Clip.Path) —
	// and filepath.Join would silently honour it.
	if filepath.IsAbs(clipID) || strings.HasPrefix(clipID, "/") {
		return "", fmt.Errorf("filler: clip id %q is absolute", clipID)
	}

	base := filepath.Clean(dir)
	full := filepath.Clean(filepath.Join(base, filepath.FromSlash(clipID)))

	// The containment check. filepath.Rel gives a path starting with ".." exactly when `full`
	// escapes `base`, which covers `../`, nested traversal, and a cleaned path that lands
	// outside for any other reason.
	rel, err := filepath.Rel(base, full)
	if err != nil {
		return "", fmt.Errorf("filler: clip id %q is not under the filler dir", clipID)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("filler: clip id %q escapes the filler dir", clipID)
	}
	return full, nil
}

// DirSource is the FillerSource that scans FILLER_DIR directly (§9.1).
//
// It optionally consults Tunarr to learn each clip's program uuid, so Tunarr-backed channels
// can still build filler-lists. THE SCAN IS THE SOURCE OF TRUTH for what exists — Tunarr only
// annotates. That ordering is the fix: previously Tunarr's knowledge determined the catalog, so
// no Tunarr meant no clips at all.
type DirSource struct {
	// Dir is FILLER_DIR. Read through a func so a settings change applies on the next sync
	// without a restart (config-design §3 hot-apply).
	Dir func() string
	// Probe reads durations; nil ⇒ FFprobeDuration.
	Probe Prober
	// Tunarr, when non-nil, supplies program uuids for clips it knows. Nil is a fully
	// supported configuration — an install with no Tunarr — not a degraded one.
	Tunarr TunarrClipSource
}

// TunarrClipSource is the narrow slice of Tunarr the scan uses: register the drop-folder as a
// `local` media source, and report the program ids it assigned.
type TunarrClipSource interface {
	EnsureLocalSource(ctx context.Context, dir string) error
	// LocalClipIDsByName maps a clip's file NAME to its Tunarr program uuid.
	//
	// By name rather than by path because that is all Tunarr's scan reports back, and it is
	// the only join key available. Imperfect: two clips with the same basename in different
	// subfolders collide, so one may get the other's uuid. That is tolerable precisely because
	// the uuid is no longer identity — a wrong uuid degrades a Tunarr filler-list, it cannot
	// corrupt the catalog or internal playout.
	LocalClipIDsByName(ctx context.Context) (map[string]string, error)
}

// EnsureLocalSource registers the drop-folder with Tunarr when one is configured.
//
// Best-effort by design: with no Tunarr there is nothing to register, and that is not an
// error — it is the configuration §9.1 exists to support.
func (d DirSource) EnsureLocalSource(ctx context.Context, dir string) error {
	if d.Tunarr == nil {
		return nil
	}
	return d.Tunarr.EnsureLocalSource(ctx, dir)
}

// ListLocalClips scans FILLER_DIR, then annotates with Tunarr uuids where available.
func (d DirSource) ListLocalClips(ctx context.Context) ([]RawClip, error) {
	dir := ""
	if d.Dir != nil {
		dir = d.Dir()
	}
	clips, _, err := ScanDir(ctx, dir, d.Probe)
	if err != nil {
		return nil, err
	}
	if d.Tunarr == nil || len(clips) == 0 {
		return clips, nil
	}

	// Tunarr's uuids are a BONUS, so a failure here must not fail the scan: the catalog is
	// already complete and internal playout needs nothing more. Only Tunarr-backed channels
	// lose anything, and they lose it until the next sync rather than permanently.
	byName, err := d.Tunarr.LocalClipIDsByName(ctx)
	if err != nil {
		return clips, nil //nolint:nilerr // deliberate: annotation is optional, see above
	}
	for i := range clips {
		if id, ok := byName[clips[i].Name]; ok {
			clips[i].TunarrProgramID = id
		}
	}
	return clips, nil
}
