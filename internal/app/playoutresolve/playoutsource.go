package playoutresolve

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/library/inventory"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/playout"
)

// LibraryFileReader is the store's read of the files a Library item plays.
type LibraryFileReader interface {
	LibraryPlayingParts(ctx context.Context, itemID string) ([]librarystore.LibraryPlayingPart, error)
}

// itemInput is the input ffmpeg reads for a programme from its start: its first file. What is
// measured of a movie in parts (its codec, its keyframes, its fades) is measured of that.
func (r *Resolver) itemInput(ctx context.Context, itemID string) string {
	return r.programmeInput(ctx, itemID, 0).Path
}

// programmeInput is the file a programme airs offset into it: the library item's own playing
// file, or, for a movie in parts, the part offset falls in. Playout reads only files Loomarr's
// scan found (#2151 step 16), while every one of them is readable: an id the Library does not
// hold, or a movie with a file gone, part or not, has nothing to play (Path "").
func (r *Resolver) programmeInput(ctx context.Context, itemID string, offset time.Duration) programmeInput {
	return inputAt(r.programmeFiles(ctx, itemID), offset)
}

// programmeFiles is every file a programme airs, in order, each where it airs in the programme;
// none when any of them is unreadable.
func (r *Resolver) programmeFiles(ctx context.Context, itemID string) []programmeInput {
	if r.libraryFile == nil {
		return nil
	}
	parts, err := r.libraryFile.LibraryPlayingParts(ctx, itemID)
	if err != nil && r.log != nil {
		r.log.Debug("playout: library file unavailable", "item", itemID, "err", err)
	}
	for _, p := range parts {
		if !readableFile(p.Path) {
			return nil
		}
	}
	return filesOf(parts)
}

// programmeInput is where in a programme one of its files airs: Path from Start into the
// programme to End; End is 0 for its last (or only) file, which airs to the end of its slot.
type programmeInput struct {
	Path       string
	Start, End time.Duration
}

// filesOf is where each of parts airs, each as long as its header says. Parts whose lengths are
// not all read yet air as the first alone: the runtime the schedule gave the programme is then
// the catalog's, not theirs.
func filesOf(parts []librarystore.LibraryPlayingPart) []programmeInput {
	for _, p := range parts {
		if p.DurationMs <= 0 {
			return []programmeInput{{Path: parts[0].Path}}
		}
	}
	out := make([]programmeInput, len(parts))
	var start time.Duration
	for i, p := range parts {
		end := start + time.Duration(p.DurationMs)*time.Millisecond
		out[i] = programmeInput{Path: p.Path, Start: start, End: end}
		start = end
	}
	if len(out) > 0 {
		out[len(out)-1].End = 0
	}
	return out
}

// inputAt is the file of files that airs offset into the programme; zero for none.
func inputAt(files []programmeInput, offset time.Duration) programmeInput {
	for _, f := range files {
		if f.End == 0 || offset < f.End {
			return f
		}
	}
	return programmeInput{}
}

// readableFile reports whether path is a regular file this process can open. A directory, a
// missing path or an unreadable file is false.
func readableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

// LibraryVideoReader is the store's read of the video a Library item plays.
type LibraryVideoReader interface {
	LibraryPlayingVideo(ctx context.Context, itemID string) (librarystore.LibraryStream, bool, error)
}

// libraryFormat is a library item's video format as the scanner read its playing file's header
// (#2151 step 15): what a channel's format and a still's tone mapping need. ok is false for any
// other id, which the inventory answers.
func (r *Resolver) libraryFormat(ctx context.Context, itemID string) (playout.MediaFormat, bool) {
	if r.libraryVideo == nil {
		return playout.MediaFormat{}, false
	}
	v, ok, err := r.libraryVideo.LibraryPlayingVideo(ctx, itemID)
	if err != nil || !ok {
		if err != nil && r.log != nil {
			r.log.Debug("playout: library video unavailable", "item", itemID, "err", err)
		}
		return playout.MediaFormat{}, false
	}
	return playoutFormatOf(inventory.SourceFacts{Streams: []inventory.Stream{{
		Index: v.Index, Kind: inventory.StreamVideo, Codec: v.Codec, Width: v.Width, Height: v.Height,
		FrameRate: v.FrameRate, PixelFormat: v.PixelFormat, ColorTransfer: v.ColorTransfer,
	}}}), true
}

// LibraryAudioReader is the store's one Library read audio selection makes.
type LibraryAudioReader interface {
	LibraryAudioTracks(ctx context.Context, path string, sizeBytes, modifiedNs int64) ([]librarystore.LibraryStream, bool, error)
}

// libraryAudioTracks is a local file's audio tracks as the Library lists them, when the scanner
// read the file as it is now (#2151 decision 47).
func (r *Resolver) libraryAudioTracks(ctx context.Context, streamURL string) ([]playout.AudioTrack, bool) {
	if r.libraryAudio == nil {
		return nil, false
	}
	_, info, ok := localInventoryOrigin(streamURL)
	if !ok {
		return nil, false
	}
	streams, ok, err := r.libraryAudio.LibraryAudioTracks(ctx, streamURL, info.Size(), info.ModTime().UnixNano())
	if err != nil || !ok {
		return nil, false
	}
	tracks := make([]playout.AudioTrack, 0, len(streams))
	for _, s := range streams {
		tracks = append(tracks, playout.AudioTrack{SourceIndex: s.Index, Language: s.Language})
	}
	return tracks, true
}

// inventoryAudioTracks is a local file's audio tracks as the Library lists them, else as Loomarr
// measured the file's current bytes. A file neither has read falls through to one probe.
func (r *Resolver) inventoryAudioTracks(ctx context.Context, streamURL string) ([]playout.AudioTrack, bool) {
	if tracks, ok := r.libraryAudioTracks(ctx, streamURL); ok {
		return tracks, true
	}
	if r.inventory == nil {
		return nil, false
	}
	if localOrigin, ok := r.ensureLocalInventorySource(ctx, streamURL); ok {
		source, found, err := r.inventory.ResolveSource(ctx, inventory.SourceRequest{
			Item: inventory.ItemRef{Origin: &localOrigin}, Now: r.inventoryNow(),
			Kinds: []inventory.SourceKind{inventory.SourceLocalFile}, RequiredCoverage: []string{"audioStreams"},
		})
		if err == nil && found {
			return playoutAudioTracks(source.Observation.Facts.Streams), true
		}
	}
	// A local file's stat revision is the authoritative freshness boundary, so a miss falls
	// directly through to one probe.
	return nil, false
}

// recordSourceMeasurement files a probe of a local file under its current bytes, so the next
// programme of the same file reads it back instead of probing again.
func (r *Resolver) recordSourceMeasurement(ctx context.Context, streamURL string, observed playout.SourceObservation) {
	if r.inventory == nil {
		return
	}
	localOrigin, ok := r.ensureLocalInventorySource(ctx, streamURL)
	if !ok {
		return
	}
	source, ok, err := r.inventory.ResolveSource(ctx, inventory.SourceRequest{Item: inventory.ItemRef{Origin: &localOrigin}})
	if err != nil || !ok {
		return
	}
	measurement := inventory.NewFactsMeasurement(source.ID, source.Revision, r.inventoryNow(), InventoryFactsOf(observed))
	if err := r.inventory.RecordMeasurement(ctx, measurement); err != nil && r.log != nil {
		r.log.Debug("playout: measured audio observation not persisted", "err", err)
	}
}

// localInventoryOrigin is the inventory identity of a local file: its path's origin key and the
// stat revision of its current bytes.
func localInventoryOrigin(input string) (inventory.OriginKey, os.FileInfo, bool) {
	info, err := os.Stat(input)
	if err != nil || info.IsDir() || !info.Mode().IsRegular() {
		return inventory.OriginKey{}, nil, false
	}
	digest := sha256.Sum256([]byte(input))
	return inventory.OriginKey{Authority: "local-playout:v1", ExternalItemID: hex.EncodeToString(digest[:16])}, info, true
}

// knownLocalSource reads, never registers, the inventory source of a local file's current bytes.
func (r *Resolver) knownLocalSource(ctx context.Context, input string) (inventory.Source, bool) {
	if r.inventory == nil {
		return inventory.Source{}, false
	}
	origin, info, ok := localInventoryOrigin(input)
	if !ok {
		return inventory.Source{}, false
	}
	revision := localInventoryRevision(info)
	item, found, err := r.inventory.Item(ctx, inventory.ItemRef{Origin: &origin})
	if err != nil || !found {
		return inventory.Source{}, false
	}
	for _, source := range item.Sources {
		if source.Kind == inventory.SourceLocalFile && source.Revision == revision {
			return source, true
		}
	}
	return inventory.Source{}, false
}

func (r *Resolver) ensureLocalInventorySource(ctx context.Context, input string) (inventory.OriginKey, bool) {
	origin, info, ok := localInventoryOrigin(input)
	if !ok {
		return inventory.OriginKey{}, false
	}
	revision := localInventoryRevision(info)
	if _, known := r.knownLocalSource(ctx, input); known {
		return origin, true
	}
	at := r.inventoryNow()
	_, err := r.inventory.ApplySnapshot(ctx, inventory.Snapshot{
		Origin: origin, Kind: "unknown",
		Observation: inventory.Observation[inventory.ItemFacts]{
			SchemaVersion: 1, ObservedAt: at,
			Coverage: map[string]inventory.Coverage{"sources": inventory.CoveragePresent},
		},
		Sources: []inventory.SourceSnapshot{{
			ExternalSourceID: "file", Kind: inventory.SourceLocalFile,
			Revision: revision,
			Locator:  inventory.Locator{Path: input},
			Observation: inventory.Observation[inventory.SourceFacts]{
				SchemaVersion: 1, ObservedAt: at,
				Coverage: map[string]inventory.Coverage{"sourceStat": inventory.CoveragePresent},
				Facts: inventory.SourceFacts{
					SizeBytes: info.Size(), ModifiedUnixNano: info.ModTime().UnixNano(),
				},
			},
		}},
	})
	return origin, err == nil
}

func localInventoryRevision(info os.FileInfo) string {
	return fmt.Sprintf("stat:%d:%d", info.Size(), info.ModTime().UnixNano())
}

func InventoryFactsOf(observed playout.SourceObservation) inventory.SourceFacts {
	facts := inventory.SourceFacts{
		Container: observed.Container, DurationMillis: observed.DurationMillis, Bitrate: observed.Bitrate,
		UnsafePreroll: observed.UnsafePreroll,
	}
	for _, stream := range observed.Streams {
		facts.Streams = append(facts.Streams, inventory.Stream{
			Index: stream.Index, Kind: inventory.StreamKind(stream.Kind), Codec: stream.Codec,
			Profile: stream.Profile, Level: stream.Level, Language: stream.Language, Title: stream.Title,
			Disposition: inventory.Disposition{Default: stream.Default, Forced: stream.Forced, Comment: stream.Comment,
				HearingImpaired: stream.HearingImpaired, VisualImpaired: stream.VisualImpaired},
			Channels: stream.Channels, ChannelLayout: stream.ChannelLayout, SampleRate: stream.SampleRate,
			Width: stream.Width, Height: stream.Height, FrameRate: stream.FrameRate,
			PixelFormat: stream.PixelFormat, ColorSpace: stream.ColorSpace,
			ColorTransfer: stream.ColorTransfer, ColorPrimaries: stream.ColorPrimaries,
			HDR: stream.HDR, DolbyVision: stream.DolbyVision, Interlaced: stream.Interlaced,
			SubtitleFormat: func() string {
				if stream.Kind == string(inventory.StreamSubtitle) {
					return stream.Codec
				}
				return ""
			}(),
		})
	}
	return facts
}

func playoutFormatOf(facts inventory.SourceFacts) playout.MediaFormat {
	format := playout.MediaFormat{
		Container:    strings.ToLower(strings.TrimSpace(facts.Container)),
		Duration:     float64(facts.DurationMillis) / float64(time.Second/time.Millisecond),
		Bitrate:      facts.Bitrate,
		VideoPreroll: facts.UnsafePreroll,
	}
	for _, stream := range facts.Streams {
		switch stream.Kind {
		case inventory.StreamVideo:
			if format.VideoCodec != "" {
				continue
			}
			format.VideoCodec = strings.ToLower(strings.TrimSpace(stream.Codec))
			format.Width = stream.Width
			format.Height = stream.Height
			format.FrameRate = playout.ParseFrameRate(stream.FrameRate)
			format.PixelFormat = strings.ToLower(strings.TrimSpace(stream.PixelFormat))
			format.ColorTransfer = strings.ToLower(strings.TrimSpace(stream.ColorTransfer))
			format.Interlaced = stream.Interlaced
		case inventory.StreamAudio:
			if format.AudioCodec != "" {
				continue
			}
			format.AudioCodec = strings.ToLower(strings.TrimSpace(stream.Codec))
			format.AudioChannels = stream.Channels
			format.AudioSampleRate = stream.SampleRate
		}
	}
	return format
}

func (r *Resolver) inventoryNow() time.Time {
	if r.now != nil {
		return r.now()
	}
	return time.Now()
}

func playoutAudioTracks(streams []inventory.Stream) []playout.AudioTrack {
	tracks := make([]playout.AudioTrack, 0)
	for _, stream := range streams {
		if stream.Kind == inventory.StreamAudio {
			tracks = append(tracks, playout.AudioTrack{SourceIndex: stream.Index, Language: stream.Language})
		}
	}
	return tracks
}

// Tracks probes the audio + subtitle tracks of what's airing on a channel right now (§9.1 Watch,
// V46). It resolves the current program's source URL exactly as the broadcast path does, then
// probes it.
//
// BEST-EFFORT, like AudioTrackFor: nothing airing, no prober wired, or a failed probe all return
// empty tracks rather than an error, so the Watch pickers degrade to "just the channel default"
// instead of erroring. The one thing it does NOT swallow is a genuine store/resolve error (a
// missing channel), which the handler needs to render a 404 rather than an empty list.
func (r *Resolver) Tracks(ctx context.Context, channelID string) (playout.MediaTracks, error) {
	airing, streamURL, err := r.AiringNow(ctx, channelID)
	if err != nil {
		return playout.MediaTracks{}, err
	}
	if r.probeTracks == nil || !airing.Playable() || streamURL == "" {
		return playout.MediaTracks{}, nil
	}
	tracks, err := r.probeTracks(ctx, streamURL)
	if err != nil {
		if r.log != nil {
			r.log.Debug("playout: tracks not probed for Watch pickers", "channel", channelID, "err", err)
		}
		return playout.MediaTracks{}, nil // degrade, don't fail the UI request
	}
	return tracks, nil
}

// FormatOf probes an input once and returns everything the probe measured: geometry, framerate,
// pixel format, colour transfer, audio shape, duration, bitrate (playout.MediaFormat).
//
// BEST-EFFORT, and it fails SAFE: no prober wired, no input, or a probe error all return the zero
// MediaFormat, which reads as SDR/unknown rather than as anything asserted, so the item transcodes.
// A copy of an unprobed source could ship a codec the output cannot play — a black frame — whereas
// an unnecessary transcode is merely slow.
//
// A library item's colour transfer comes from the scanner's header read of its playing file when
// that names one (#2173): an HDR10 Matroska file can signal PQ only in the HEVC parameter set, which
// the header reader parses and ffprobe's stream summary reports as unknown, so the airing would skip
// tone mapping. Any other id keeps the probe's.
func (r *Resolver) FormatOf(ctx context.Context, libraryItemID, input string) playout.MediaFormat {
	format := r.probedFormat(ctx, input)
	if libraryItemID == "" {
		return format
	}
	if header, ok := r.libraryFormat(ctx, libraryItemID); ok && header.ColorTransfer != "" && header.ColorTransfer != "unknown" {
		format.ColorTransfer = header.ColorTransfer
	}
	return format
}

func (r *Resolver) probedFormat(ctx context.Context, input string) playout.MediaFormat {
	if input == "" {
		return playout.MediaFormat{}
	}
	if r.inventory != nil {
		if origin, ok := r.ensureLocalInventorySource(ctx, input); ok {
			source, found, err := r.inventory.ResolveSource(ctx, inventory.SourceRequest{
				Item: inventory.ItemRef{Origin: &origin}, Now: r.inventoryNow(),
				Kinds: []inventory.SourceKind{inventory.SourceLocalFile}, RequiredCoverage: []string{"streams"},
			})
			if err == nil && found {
				format := playoutFormatOf(source.Observation.Facts)
				format.Active = r.activePicture(ctx, source.ID)
				r.submitAnalysis(source.ID, source.Revision, input, source.Observation.Facts)
				return format
			}
			if format, ok := r.measureFirstPlay(ctx, origin, input); ok {
				return format
			}
		}
	}
	if r.probeFormat == nil {
		return playout.MediaFormat{}
	}
	f, err := r.probeFormat(ctx, input)
	if err != nil {
		if r.log != nil {
			r.log.Debug("playout: format not probed, transcoding", "input", input, "err", err)
		}
		return playout.MediaFormat{}
	}
	return f
}
