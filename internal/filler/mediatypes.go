package filler

import "github.com/mantonx/loomarr/internal/mediatools"

// The media-tool vocabulary, re-exported as ALIASES rather than re-declared (§14.2).
//
// ⚠ **These are deliberate, not a migration leftover.** `internal/mediatools` owns the ffmpeg /
// ffprobe / whisper layer — the exec calls, the output parsers, and the shapes those tools
// return. The splitter and the ingest stages are the domain that CONSUMES them, and they read
// naturally in the domain's own vocabulary: `[]Interval`, `[]TranscriptSegment`. An alias gives
// the domain that vocabulary while leaving one definition, in the package that produces the
// values.
//
// The alternative was qualifying ~110 references across 11 files to say `mediatools.Interval`,
// which buys nothing: it is the same type either way, and Go aliases are exactly the feature for
// moving a type between packages without churning its callers. What matters architecturally is
// that the DEFINITION moved, so the driver layer no longer lives inside a 10,000-line domain
// package.
//
// ⚠ Do not add an alias for a type the domain does not genuinely speak. The test is whether a
// reader of `split.go` would expect the word without a package qualifier; these four pass it,
// and `FFmpegTools` (a concrete exec implementation the domain never names) does not.
type (
	// Interval is a [StartMs, EndMs) span inside a media file.
	Interval = mediatools.Interval
	// Chapter is one embedded chapter from ffprobe.
	Chapter = mediatools.Chapter
	// TranscriptSegment is one whisper utterance, offset within the probed span.
	TranscriptSegment = mediatools.TranscriptSegment
	// MediaTools is the split pipeline's exec boundary — the interface the domain depends on
	// and `mediatools.FFmpegTools` implements.
	MediaTools = mediatools.MediaTools
	// Probed is what one ffprobe pass learns about a clip; Prober is the injected seam that
	// produces it. The scanner and the probe stage are written in terms of both.
	Probed = mediatools.Probed
	Prober = mediatools.Prober
	// MezzanineProfile is the transcode target the transcode stage carries around.
	MezzanineProfile = mediatools.MezzanineProfile
	// MediaQuality is the black/content-silent/frozen evidence measured while decoding a clip.
	MediaQuality = mediatools.MediaQuality
)
