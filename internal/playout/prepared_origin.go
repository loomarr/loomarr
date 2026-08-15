package playout

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/prepared"
)

const (
	preparedManifestSegments = 4
)

// PreparedWindow is the authoritative wall-clock slice a prepared presentation must render.
// Specification selects immutable bytes; StartedAt and Offset place those bytes on the Channel's
// timeline. The resolver must derive all three from the same Airing used by live playout and guide.
type PreparedWindow struct {
	Specification prepared.Specification
	StartedAt     time.Time
	Offset        time.Duration
}

// PreparedResolver maps a tune request onto the authoritative Airing and prepared identity. It is
// implemented at composition, where the accepted schedule and source catalogue already meet.
type PreparedResolver interface {
	ResolvePrepared(context.Context, TuneRequest) (PreparedWindow, bool, error)
}

// PreparedOrigin renders short live HLS manifests over immutable shared publications. It owns no
// encoder and cannot create media; absent or invalid publications are misses for Origin's live
// fallback.
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
	pub, ok, err := o.library.Lookup(window.Specification)
	if err != nil || !ok {
		return Presentation{}, false, err
	}
	asset, ok, err := o.library.Open(pub.Key, prepared.MediaManifestName)
	if err != nil || !ok {
		if err == nil {
			err = prepared.ErrIncomplete
		}
		return Presentation{}, false, err
	}
	body, readErr := io.ReadAll(asset.Content)
	closeErr := asset.Content.Close()
	if readErr != nil {
		return Presentation{}, false, fmt.Errorf("playout: read prepared manifest: %w", readErr)
	}
	if closeErr != nil {
		return Presentation{}, false, fmt.Errorf("playout: close prepared manifest: %w", closeErr)
	}
	manifest, err := renderPreparedManifest(body, pub.Key, pub.Files, window)
	if err != nil {
		return Presentation{}, false, err
	}
	return Presentation{Manifest: manifest, Release: func() {}}, true, nil
}

// OpenAsset opens a publication-keyed URI emitted by Tune. The key binds a follow-up request to
// the immutable publication that authored its manifest, even if the Channel crosses an Airing
// boundary before the request arrives.
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
	return Asset{Content: asset.Content, Modified: asset.Modified}, true, nil
}

type preparedSegment struct {
	duration time.Duration
	extinf   string
	uri      string
}

func renderPreparedManifest(body []byte, key string, files []string, window PreparedWindow) ([]byte, error) {
	declared := make(map[string]struct{}, len(files))
	for _, name := range files {
		declared[name] = struct{}{}
	}
	lines := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")
	version, target, independent, init := "", "", "", ""
	segments := make([]preparedSegment, 0)
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		switch {
		case strings.HasPrefix(line, "#EXT-X-VERSION:"):
			version = line
		case strings.HasPrefix(line, "#EXT-X-TARGETDURATION:"):
			target = line
		case line == "#EXT-X-INDEPENDENT-SEGMENTS":
			independent = line
		case strings.HasPrefix(line, "#EXT-X-MAP:"):
			var err error
			init, err = prefixMapURI(line, key, declared)
			if err != nil {
				return nil, err
			}
		case strings.HasPrefix(line, "#EXTINF:"):
			raw := strings.TrimSuffix(strings.TrimPrefix(line, "#EXTINF:"), ",")
			if comma := strings.IndexByte(raw, ','); comma >= 0 {
				raw = raw[:comma]
			}
			seconds, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
			if err != nil || seconds <= 0 {
				return nil, fmt.Errorf("playout: invalid prepared EXTINF %q", line)
			}
			if i+1 >= len(lines) || strings.TrimSpace(lines[i+1]) == "" || strings.HasPrefix(strings.TrimSpace(lines[i+1]), "#") {
				return nil, errors.New("playout: prepared EXTINF has no media URI")
			}
			i++
			uri, err := preparedAssetURI(strings.TrimSpace(lines[i]), key, declared)
			if err != nil {
				return nil, err
			}
			segments = append(segments, preparedSegment{
				duration: time.Duration(seconds * float64(time.Second)), extinf: line,
				uri: uri,
			})
		}
	}
	if len(segments) == 0 {
		return nil, errors.New("playout: prepared manifest has no media")
	}

	offset := window.Offset
	if offset < 0 {
		offset = 0
	}
	index := 0
	for index < len(segments) && offset >= segments[index].duration {
		offset -= segments[index].duration
		index++
	}
	if index == len(segments) {
		return nil, errors.New("playout: prepared window is beyond its media")
	}
	end := min(index+preparedManifestSegments, len(segments))

	cadence := int64(window.Specification.Rendition.SegmentDurationMS)
	sequence := int64(0)
	if cadence > 0 {
		sequence = window.StartedAt.UnixMilli()/cadence + int64(index)
	}
	if sequence < 0 {
		sequence = 0
	}
	discontinuity := window.StartedAt.Unix()
	if discontinuity < 0 {
		discontinuity = 0
	}
	out := []string{"#EXTM3U"}
	for _, header := range []string{version, target, independent} {
		if header != "" {
			out = append(out, header)
		}
	}
	out = append(out,
		fmt.Sprintf("#EXT-X-MEDIA-SEQUENCE:%d", sequence),
		fmt.Sprintf("#EXT-X-DISCONTINUITY-SEQUENCE:%d", discontinuity),
	)
	if init != "" {
		out = append(out, init)
	}
	for _, segment := range segments[index:end] {
		out = append(out, segment.extinf, segment.uri)
	}
	return []byte(strings.Join(out, "\n") + "\n"), nil
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
