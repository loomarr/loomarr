package header

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/loomarr/loomarr/internal/library/inventory"
)

// ErrMatroskaIndex reports a Matroska file whose index is present but does not parse.
var ErrMatroskaIndex = errors.New("media header: matroska index does not parse")

// Header is what a file's container says about it. Streams carry ffprobe's global stream index;
// tracks that are neither video, audio nor subtitle (timecode, attachments) take an index and are
// left out, as ffprobe's observation leaves them out.
type Header struct {
	Container      string
	DurationMillis int64
	Bitrate        int64
	Streams        []inventory.Stream
	// Chapters are the first edition's top-level chapters, nil when the file has none. Facts from
	// the ffprobe fallback carry none: ChaptersKnown says whether nil means "none" or "not read".
	Chapters      []Chapter
	ChaptersKnown bool
}

// Chapter is one chapter mark, as the inventory stores it.
type Chapter = inventory.Chapter

// SourceFacts is the header as the inventory stores a source's facts.
func (h Header) SourceFacts() inventory.SourceFacts {
	return inventory.SourceFacts{
		Container: h.Container, DurationMillis: h.DurationMillis, Bitrate: h.Bitrate, Streams: h.Streams,
		Chapters: h.Chapters,
	}
}

// Version changes when Read or Probe learns a fact it did not report before, so a header read by
// an older reader is read once more.
//
//	1: containers, durations, streams, colour and chapters.
//	2: commentary, hearing-impaired and described tracks, and a video track's Dolby Vision profile.
//	3: a file's chapters are kept with its facts.
//	4: each track's bitrate and bit depth (#3228).
const Version = 4

// Read returns a file's header facts from its container. ok is false for a container this package
// does not read (AVI, MPEG-TS, …), one whose header does not parse, or one where a fact playout
// relies on (pixel format, HDR transfer, field order, frame rate, duration) needs decoding.
func Read(r io.ReaderAt, size int64) (Header, bool) {
	if size <= 0 {
		return Header{}, false
	}
	if h, ok := readMatroska(r, size); ok {
		h.ChaptersKnown = true
		return h, true
	}
	if h, ok := readMP4(r, size); ok {
		h.ChaptersKnown = true
		return h, true
	}
	return Header{}, false
}

// ChannelLayout is the layout ffmpeg's decoder states for an audio codec's channel count, which
// neither container records, so this reader leaves a stream's ChannelLayout empty: "" where it
// depends on the bitstream.
func ChannelLayout(codec string, channels int) string {
	if strings.HasPrefix(codec, "pcm_") {
		return "" // a PCM decoder states no layout for a container that records none
	}
	switch channels {
	case 1:
		return "mono"
	case 2:
		return "stereo"
	}
	switch codec {
	case "ac3", "eac3", "dts", "truehd":
		switch channels {
		case 6:
			return "5.1(side)"
		case 7:
			return "6.1"
		case 8:
			return "7.1"
		}
	case "aac", "opus", "flac", "vorbis":
		if channels == 6 {
			return "5.1"
		}
	}
	return ""
}

// Keyframes reads a file's keyframe index from its container's own index: Matroska Cues or the
// MP4 sample tables. ok is false when the container has none for a video track; err reports an
// index that is present but does not parse.
func Keyframes(r io.ReaderAt, size int64) ([]inventory.Keyframe, bool, error) {
	frames, ok, err := matroskaKeyframes(r, size)
	if ok || err != nil {
		return frames, ok, err
	}
	return mp4Keyframes(r, size)
}

// Method says which reader produced a Header.
type Method string

const (
	MethodHeader  Method = "header"
	MethodFFprobe Method = "ffprobe"
)

// Fallback reads a file the native readers cannot: the shared ffprobe observation in production.
type Fallback func(ctx context.Context, path string) (Header, error)

// Probe reads a local file's header facts: natively when the container allows, otherwise through
// fallback. The file is opened for random access (the kernel does no readahead), because each read
// here is one seek on a spinning disk and readahead would only cost more of them.
func Probe(ctx context.Context, path string, fallback Fallback) (Header, Method, error) {
	if err := ctx.Err(); err != nil {
		return Header{}, "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return Header{}, "", err
	}
	adviseRandom(f)
	info, statErr := f.Stat()
	var h Header
	ok := false
	if statErr == nil {
		h, ok = Read(f, info.Size())
	}
	closeErr := f.Close()
	if statErr != nil {
		return Header{}, "", statErr
	}
	if ok {
		return h, MethodHeader, nil
	}
	if closeErr != nil {
		return Header{}, "", closeErr
	}
	if fallback == nil {
		return Header{}, "", fmt.Errorf("media header: %w", ErrUnreadable)
	}
	h, err = fallback(ctx, path)
	if err != nil {
		return Header{}, "", err
	}
	return h, MethodFFprobe, nil
}

// ErrUnreadable reports a file the native readers cannot describe when no fallback is configured.
var ErrUnreadable = errors.New("container header not readable natively")
