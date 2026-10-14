// Package mediatools is the ffmpeg / ffprobe / whisper layer (§10, §14.2): the exec calls, the
// parsers for what those binaries print, and the shapes they return.
//
// It was carved out of internal/filler, which had grown to 10,320 non-test lines of which only a
// fraction was the commercial-clip domain it is named for. The rest included this — a driver
// layer with no scheduling concepts in it at all.
//
// ⚠ **The dependency runs one way: filler imports mediatools, never the reverse.** Nothing here
// knows what a clip, a pod or a channel is. If a change wants a domain type in this package, the
// change is wrong — pass the primitive in, or do the domain part on the filler side.
//
// ⚠ **The types here are TOOL OUTPUT, and that is why they moved.** Interval, Chapter, Probed and
// TranscriptSegment lived in filler because the splitter needed them first, but their own doc
// comments always described them as ffprobe/whisper results rather than domain concepts. They are
// re-exported as aliases from internal/filler (mediatypes.go) so the domain keeps its vocabulary
// without a second definition.
//
// ⚠ **Unit tests never execute the binaries** (AGENTS.md §19). The exec paths are exercised only
// against real media, by hand; what is tested here is the PARSING — given this ffmpeg stderr,
// these intervals — which is where the bugs have actually been. FFmpegTools is faked through the
// MediaTools interface everywhere else.
package mediatools
