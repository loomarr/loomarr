package identity

import (
	"context"

	"github.com/loomarr/loomarr/internal/media/creative"
	"github.com/loomarr/loomarr/internal/media/tools"
)

// spanTools is a media-tool stub whose every method does nothing, so a test embeds it and
// overrides only what it exercises (here, Fingerprint).
type spanTools struct{}

func (spanTools) Chapters(context.Context, string) ([]tools.Chapter, error) { return nil, nil }
func (spanTools) Boundaries(context.Context, string, int64, int64) ([]tools.Interval, []tools.Interval, error) {
	return nil, nil, nil
}
func (spanTools) Transcribe(context.Context, string, int64, int64) ([]tools.TranscriptSegment, error) {
	return nil, nil
}
func (spanTools) Fingerprint(context.Context, string, int64, int64) (creative.Fingerprint, error) {
	return creative.Fingerprint{}, nil
}
func (spanTools) Keyframes(context.Context, string, int) ([][]byte, error) { return nil, nil }
func (spanTools) KeyframesIn(context.Context, string, int64, int64, int) ([][]byte, error) {
	return nil, nil
}
func (spanTools) VisionKeyframesIn(context.Context, string, int64, int64, int) ([][]byte, error) {
	return nil, nil
}
func (spanTools) Cut(context.Context, string, int64, int64, string) error { return nil }
