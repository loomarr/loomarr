package playoutcert

import (
	"bufio"
	"bytes"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func parsePreparedHLS(body []byte) ([]preparedHLSSegment, error) {
	return parseHLSMedia(body, true)
}

// parseHLSMedia binds each URI to its own time interval and decoder epoch.
// Tags between EXTINF and the URI still describe that pending segment.
func parseHLSMedia(body []byte, requireMap bool) ([]preparedHLSSegment, error) {
	const maxSequence = 1<<63 - 1
	invalid := func() ([]preparedHLSSegment, error) { return nil, errors.New("invalid_hls") }
	var out []preparedHLSSegment
	var sequence, index, epoch int64
	var sequenceSet, epochSet, header, pending, anchored bool
	var init string
	var start time.Time
	var duration time.Duration
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 1024), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !header {
			if line != "#EXTM3U" {
				return invalid()
			}
			header = true
			continue
		}
		switch {
		case strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"):
			v, err := strconv.ParseInt(strings.TrimPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"), 10, 64)
			if err != nil || v < 0 || sequenceSet || pending || len(out) > 0 {
				return invalid()
			}
			sequence, sequenceSet = v, true
		case strings.HasPrefix(line, "#EXT-X-DISCONTINUITY-SEQUENCE:"):
			v, err := strconv.ParseInt(strings.TrimPrefix(line, "#EXT-X-DISCONTINUITY-SEQUENCE:"), 10, 64)
			if err != nil || v < 0 || epochSet || pending || len(out) > 0 {
				return invalid()
			}
			epoch, epochSet = v, true
		case strings.HasPrefix(line, "#EXT-X-MAP:"):
			init = mapURI(line)
			if !validHLSAssetReference(init) {
				return invalid()
			}
		case line == "#EXT-X-DISCONTINUITY":
			if epoch == maxSequence || pending {
				return invalid()
			}
			epoch++
			epochSet = true
			anchored = false
		case strings.HasPrefix(line, "#EXT-X-PROGRAM-DATE-TIME:"):
			value := strings.TrimPrefix(line, "#EXT-X-PROGRAM-DATE-TIME:")
			var err error
			start, err = parseHLSProgramDateTime(value)
			if err != nil || start.IsZero() {
				return invalid()
			}
			anchored = true
		case strings.HasPrefix(line, "#EXTINF:"):
			value, _, _ := strings.Cut(strings.TrimPrefix(line, "#EXTINF:"), ",")
			if pending || value == "" || strings.IndexFunc(value, func(r rune) bool { return (r < '0' || r > '9') && r != '.' }) >= 0 {
				return invalid()
			}
			var err error
			duration, err = time.ParseDuration(value + "s")
			if err != nil || duration <= 0 {
				return invalid()
			}
			pending = true
		case strings.HasPrefix(line, "#"):
			// Other production playlist tags do not change segment coordinates.
		default:
			if !pending || !anchored || (requireMap && init == "") || !validHLSAssetReference(line) || index > maxSequence-sequence {
				return invalid()
			}
			out = append(out, preparedHLSSegment{
				uri: line, init: init, id: strconv.FormatInt(sequence+index, 10), boundary: strconv.FormatInt(epoch, 10),
				startedAt: start, duration: duration,
			})
			index++
			start = start.Add(duration)
			pending = false
		}
	}
	if scanner.Err() != nil || pending || len(out) == 0 {
		return invalid()
	}
	return out, nil
}

func parseHLSProgramDateTime(value string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed, nil
	}
	// The pinned FFmpeg HLS muxer writes ISO-8601 offsets without a colon.
	return time.Parse("2006-01-02T15:04:05.999999999-0700", value)
}

func validHLSAssetReference(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Path != "" && !u.IsAbs() && u.Host == "" && u.User == nil && u.Fragment == "" && !strings.Contains(u.Path, "..")
}
