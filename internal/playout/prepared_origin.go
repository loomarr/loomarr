package playout

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/prepared"
)

const preparedManifestSegments = 3

// PreparedAiring binds one immutable source rendition to its authoritative place on a Channel's
// wall clock. Offset is meaningful for Current; previous Airings contribute their trailing media.
type PreparedAiring struct {
	Specification prepared.Specification
	StartedAt     time.Time
	Offset        time.Duration
}

// PreparedWindow is the live edge plus the immediately preceding Airings needed to render a stable
// three-segment HLS window across a programme boundary. Previous is chronological, oldest first.
type PreparedWindow struct {
	Previous []PreparedAiring
	Current  PreparedAiring
}

// PreparedResolver maps a tune request onto the authoritative Airings and prepared identities. It
// is implemented at composition, where the accepted schedule and source catalogue already meet.
type PreparedResolver interface {
	ResolvePrepared(context.Context, TuneRequest) (PreparedWindow, bool, error)
}

// PreparedOrigin renders short live HLS manifests over immutable shared publications. It owns no
// encoder and cannot create media; absent or invalid current publications are misses for Origin's
// live fallback.
type PreparedOrigin struct {
	library  *prepared.Library
	resolver PreparedResolver
}

func newPreparedOrigin(library *prepared.Library, resolver PreparedResolver) *PreparedOrigin {
	return &PreparedOrigin{library: library, resolver: resolver}
}

// NewPreparedOrigin creates the prepared implementation consumed by Origin. Construction does not
// start background work: publications are produced by the separate readiness control plane.
func NewPreparedOrigin(library *prepared.Library, resolver PreparedResolver) *PreparedOrigin {
	return newPreparedOrigin(library, resolver)
}

func (o *PreparedOrigin) Tune(ctx context.Context, request TuneRequest) (Presentation, bool, error) {
	if request.Delivery != DeliveryHLS || o == nil || o.library == nil || o.resolver == nil {
		return Presentation{}, false, nil
	}
	window, ok, err := o.resolver.ResolvePrepared(ctx, request)
	if err != nil || !ok {
		return Presentation{}, false, err
	}
	current, ok, err := o.load(window.Current)
	if err != nil || !ok {
		return Presentation{}, false, err
	}
	previous := make([]preparedMedia, 0, len(window.Previous))
	for _, airing := range window.Previous {
		media, hit, loadErr := o.load(airing)
		if loadErr != nil || !hit {
			continue
		}
		previous = append(previous, media)
	}
	manifest, err := renderPreparedManifest(current, previous)
	if err != nil {
		return Presentation{}, false, err
	}
	return Presentation{Manifest: manifest, Release: func() {}}, true, nil
}

func (o *PreparedOrigin) load(airing PreparedAiring) (preparedMedia, bool, error) {
	pub, ok, err := o.library.Lookup(airing.Specification)
	if err != nil || !ok {
		return preparedMedia{}, false, err
	}
	asset, ok, err := o.library.Open(pub.Key, prepared.MediaManifestName)
	if err != nil || !ok {
		if err == nil {
			err = prepared.ErrIncomplete
		}
		return preparedMedia{}, false, err
	}
	body, readErr := io.ReadAll(asset.Content)
	closeErr := asset.Content.Close()
	if readErr != nil {
		return preparedMedia{}, false, fmt.Errorf("playout: read prepared manifest: %w", readErr)
	}
	if closeErr != nil {
		return preparedMedia{}, false, fmt.Errorf("playout: close prepared manifest: %w", closeErr)
	}
	media, err := parsePreparedManifest(body, pub.Key, pub.Files, airing)
	return media, err == nil, err
}

// OpenAsset opens a publication-keyed URI emitted by Tune. The key binds a follow-up request to
// the immutable publication that authored its manifest, even across an Airing boundary.
func (o *PreparedOrigin) OpenAsset(_ string, _ EncodePlan, rel string) (Asset, bool, error) {
	if o == nil || o.library == nil {
		return Asset{}, false, nil
	}
	key, name, ok := strings.Cut(rel, "/")
	if !ok || key == "" || name == "" {
		return Asset{}, false, nil
	}
	asset, ok, err := o.library.Open(key, name)
	if err != nil || !ok {
		return Asset{}, ok, err
	}
	return Asset{Content: asset.Content, Modified: asset.Modified, Immutable: true}, true, nil
}

type preparedSegment struct {
	duration time.Duration
	extinf   string
	uri      string
	startsAt time.Time
}

type preparedMedia struct {
	airing      PreparedAiring
	key         string
	version     string
	independent string
	init        string
	segments    []preparedSegment
}

type preparedSegmentRef struct {
	media   preparedMedia
	segment preparedSegment
}

func parsePreparedManifest(body []byte, key string, files []string, airing PreparedAiring) (preparedMedia, error) {
	declared := make(map[string]struct{}, len(files))
	for _, name := range files {
		declared[name] = struct{}{}
	}
	lines := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")
	media := preparedMedia{airing: airing, key: key}
	startsAt := airing.StartedAt
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		switch {
		case strings.HasPrefix(line, "#EXT-X-VERSION:"):
			media.version = line
		case line == "#EXT-X-INDEPENDENT-SEGMENTS":
			media.independent = line
		case strings.HasPrefix(line, "#EXT-X-MAP:"):
			var err error
			media.init, err = prefixMapURI(line, key, declared)
			if err != nil {
				return preparedMedia{}, err
			}
		case strings.HasPrefix(line, "#EXTINF:"):
			raw := strings.TrimSuffix(strings.TrimPrefix(line, "#EXTINF:"), ",")
			if comma := strings.IndexByte(raw, ','); comma >= 0 {
				raw = raw[:comma]
			}
			seconds, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
			if err != nil || seconds <= 0 {
				return preparedMedia{}, fmt.Errorf("playout: invalid prepared EXTINF %q", line)
			}
			if i+1 >= len(lines) || strings.TrimSpace(lines[i+1]) == "" || strings.HasPrefix(strings.TrimSpace(lines[i+1]), "#") {
				return preparedMedia{}, errors.New("playout: prepared EXTINF has no media URI")
			}
			i++
			uri, err := preparedAssetURI(strings.TrimSpace(lines[i]), key, declared)
			if err != nil {
				return preparedMedia{}, err
			}
			duration := time.Duration(seconds * float64(time.Second))
			media.segments = append(media.segments, preparedSegment{
				duration: duration, extinf: line, uri: uri, startsAt: startsAt,
			})
			startsAt = startsAt.Add(duration)
		}
	}
	if media.init == "" || len(media.segments) == 0 {
		return preparedMedia{}, errors.New("playout: prepared manifest has no initialisation or media")
	}
	return media, nil
}

func renderPreparedManifest(current preparedMedia, previous []preparedMedia) ([]byte, error) {
	offset := current.airing.Offset
	if offset < 0 {
		offset = 0
	}
	index := 0
	for index < len(current.segments) && offset >= current.segments[index].duration {
		offset -= current.segments[index].duration
		index++
	}
	if index == len(current.segments) {
		return nil, errors.New("playout: prepared window is beyond its media")
	}

	refs := []preparedSegmentRef{{media: current, segment: current.segments[index]}}
	for i := index - 1; i >= 0 && len(refs) < preparedManifestSegments; i-- {
		refs = append(refs, preparedSegmentRef{media: current, segment: current.segments[i]})
	}
	for p := len(previous) - 1; p >= 0 && len(refs) < preparedManifestSegments; p-- {
		for i := len(previous[p].segments) - 1; i >= 0 && len(refs) < preparedManifestSegments; i-- {
			refs = append(refs, preparedSegmentRef{media: previous[p], segment: previous[p].segments[i]})
		}
	}
	reversePrepared(refs)

	longest := time.Duration(0)
	for _, ref := range refs {
		if ref.segment.duration > longest {
			longest = ref.segment.duration
		}
	}
	cadence := refs[0].media.airing.Specification.Rendition.SegmentDurationMS
	sequence := int64(0)
	if cadence > 0 {
		sequence = refs[0].segment.startsAt.UnixMilli() / int64(cadence)
	}
	if sequence < 0 {
		sequence = 0
	}
	out := []string{
		"#EXTM3U", current.version,
		fmt.Sprintf("#EXT-X-TARGETDURATION:%d", int(math.Ceil(longest.Seconds()))),
		fmt.Sprintf("#EXT-X-MEDIA-SEQUENCE:%d", sequence),
	}
	if current.independent != "" {
		out = append(out, current.independent)
	}
	out = append(out, "#EXT-X-PROGRAM-DATE-TIME:"+refs[0].segment.startsAt.UTC().Format(time.RFC3339Nano))
	previousBoundary := ""
	for _, ref := range refs {
		boundary := fmt.Sprintf("%s/%d", ref.media.key, ref.media.airing.StartedAt.UnixNano())
		if boundary != previousBoundary {
			if previousBoundary != "" {
				out = append(out, "#EXT-X-DISCONTINUITY")
			}
			out = append(out, ref.media.init)
			previousBoundary = boundary
		}
		out = append(out, ref.segment.extinf, ref.segment.uri)
	}
	return []byte(strings.Join(compactPreparedLines(out), "\n") + "\n"), nil
}

func reversePrepared[T any](values []T) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func compactPreparedLines(lines []string) []string {
	out := lines[:0]
	for _, line := range lines {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func prefixMapURI(line, key string, declared map[string]struct{}) (string, error) {
	const marker = `URI="`
	start := strings.Index(line, marker)
	if start < 0 {
		return "", errors.New("playout: prepared map has no URI")
	}
	uriStart := start + len(marker)
	end := strings.Index(line[uriStart:], `"`)
	if end < 0 {
		return "", errors.New("playout: prepared map has an unterminated URI")
	}
	uri, err := preparedAssetURI(line[uriStart:uriStart+end], key, declared)
	if err != nil {
		return "", err
	}
	return line[:uriStart] + uri + line[uriStart+end:], nil
}

func preparedAssetURI(raw, key string, declared map[string]struct{}) (string, error) {
	name := strings.TrimSpace(raw)
	clean := path.Clean(name)
	if name == "" || clean == "." || path.IsAbs(clean) || clean == ".." ||
		strings.HasPrefix(clean, "../") || strings.ContainsAny(name, `\\?#`) {
		return "", fmt.Errorf("playout: unsafe prepared asset URI %q", raw)
	}
	if _, ok := declared[clean]; !ok {
		return "", fmt.Errorf("playout: prepared manifest references undeclared asset %q", clean)
	}
	return key + "/" + clean, nil
}
