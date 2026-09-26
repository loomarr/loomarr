package inventory

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
)

// AnalysisSchemaVersion is the version of the durable source analysis encoding below. A reader that
// sees a newer version treats the analysis as absent rather than guessing.
const AnalysisSchemaVersion = 1

// Keyframe is one seekable video sync point: presentation time and the byte offset of its packet.
// Offset is -1 when the container does not expose packet positions (some MPEG-TS and fragmented
// files); PTSMs is always present.
type Keyframe struct {
	PTSMs  int64 `json:"ptsMs"`
	Offset int64 `json:"offset"`
}

// Break is a natural break candidate: a place where black video and silent audio coincide, so a
// break inserted there never lands mid-scene. AtMs is the coincidence midpoint; KeyframeMs is the
// nearest keyframe at or after it (0 when the source has no keyframe index), which is where a
// packager can cut without re-encoding. Confidence is in (0, 1].
type Break struct {
	AtMs       int64   `json:"atMs"`
	KeyframeMs int64   `json:"keyframeMs,omitempty"`
	OverlapMs  int64   `json:"overlapMs"`
	Confidence float64 `json:"confidence"`
}

// Analysis is everything Loomarr measures about one source revision beyond its stream facts: the
// keyframe index, integrated loudness with true peak, and natural break candidates. It is written
// once per revision by the measurement job and read by playout, the packager and the scheduler
// without ever touching the file or the media server.
type Analysis struct {
	SourceID SourceID `json:"sourceId"`
	Revision string   `json:"revision"`
	// Keyframes is ordered by PTSMs.
	Keyframes []Keyframe `json:"-"`
	// IntegratedLUFS and TruePeakDBTP are nil when the source has no audio (or peak was -inf).
	IntegratedLUFS *float64  `json:"integratedLufs,omitempty"`
	TruePeakDBTP   *float64  `json:"truePeakDbtp,omitempty"`
	Breaks         []Break   `json:"breaks,omitempty"`
	AnalyzedAt     time.Time `json:"analyzedAt"`
}

// AnalysisStore is the durable seam for Analysis. It is separate from Repository so the six-table
// inventory aggregate and its fakes stay unchanged.
type AnalysisStore interface {
	AnalysisReader
	RecordInventoryAnalysis(context.Context, Analysis) error
}

// ErrKeyframeEncoding reports a keyframe blob that does not decode.
var ErrKeyframeEncoding = errors.New("inventory: keyframe index does not decode")

// EncodeKeyframes packs the index as varint deltas: PTS delta in ms, then the offset delta
// zig-zagged (offsets are monotone in practice, but a container may reorder). A two-hour film with
// a 2 s GOP is about 3,600 entries at roughly 4 bytes each. Offsets of -1 are stored as a flag so
// a whole index without positions stays one byte per entry for the offset column.
func EncodeKeyframes(frames []Keyframe) []byte {
	buf := make([]byte, 0, len(frames)*5+8)
	buf = binary.AppendUvarint(buf, uint64(len(frames)))
	var prevPTS, prevOff int64
	for _, frame := range frames {
		buf = binary.AppendVarint(buf, frame.PTSMs-prevPTS)
		prevPTS = frame.PTSMs
		if frame.Offset < 0 {
			buf = binary.AppendUvarint(buf, 0)
			continue
		}
		buf = binary.AppendUvarint(buf, 1)
		buf = binary.AppendVarint(buf, frame.Offset-prevOff)
		prevOff = frame.Offset
	}
	return buf
}

// DecodeKeyframes reverses EncodeKeyframes.
func DecodeKeyframes(data []byte) ([]Keyframe, error) {
	count, n := binary.Uvarint(data)
	if n <= 0 || count > uint64(len(data)) {
		return nil, ErrKeyframeEncoding
	}
	data = data[n:]
	frames := make([]Keyframe, 0, count)
	var prevPTS, prevOff int64
	for i := uint64(0); i < count; i++ {
		delta, n := binary.Varint(data)
		if n <= 0 {
			return nil, fmt.Errorf("%w: truncated at entry %d", ErrKeyframeEncoding, i)
		}
		data = data[n:]
		prevPTS += delta
		flag, n := binary.Uvarint(data)
		if n <= 0 || flag > 1 {
			return nil, fmt.Errorf("%w: bad offset flag at entry %d", ErrKeyframeEncoding, i)
		}
		data = data[n:]
		frame := Keyframe{PTSMs: prevPTS, Offset: -1}
		if flag == 1 {
			od, n := binary.Varint(data)
			if n <= 0 {
				return nil, fmt.Errorf("%w: truncated offset at entry %d", ErrKeyframeEncoding, i)
			}
			data = data[n:]
			prevOff += od
			frame.Offset = prevOff
		}
		frames = append(frames, frame)
	}
	if len(data) != 0 {
		return nil, fmt.Errorf("%w: %d trailing bytes", ErrKeyframeEncoding, len(data))
	}
	return frames, nil
}

// ValidateAnalysis trims and bounds an Analysis before it is stored.
func ValidateAnalysis(a Analysis) (Analysis, error) {
	if a.SourceID == "" || a.Revision == "" || a.AnalyzedAt.IsZero() {
		return Analysis{}, ErrInvalid
	}
	for i, frame := range a.Keyframes {
		if frame.PTSMs < 0 || (i > 0 && frame.PTSMs < a.Keyframes[i-1].PTSMs) {
			return Analysis{}, ErrInvalid
		}
	}
	for _, v := range []*float64{a.IntegratedLUFS, a.TruePeakDBTP} {
		if v != nil && (math.IsNaN(*v) || math.IsInf(*v, 0)) {
			return Analysis{}, ErrInvalid
		}
	}
	for _, b := range a.Breaks {
		if b.AtMs < 0 || b.Confidence <= 0 || b.Confidence > 1 {
			return Analysis{}, ErrInvalid
		}
	}
	return a, nil
}

// AnalysisReader is the consumer seam for phase 2 (packager) and phase 3 (scheduler): everything
// Loomarr measured about a source, from the database alone.
type AnalysisReader interface {
	InventoryAnalysis(context.Context, SourceID) (Analysis, bool, error)
}

// KeyframeAtOrBefore returns the last keyframe at or before ptsMs: where a seek to ptsMs must
// start reading. ok is false when the index is empty or begins after ptsMs.
func (a Analysis) KeyframeAtOrBefore(ptsMs int64) (Keyframe, bool) {
	i := sort.Search(len(a.Keyframes), func(i int) bool { return a.Keyframes[i].PTSMs > ptsMs })
	if i == 0 {
		return Keyframe{}, false
	}
	return a.Keyframes[i-1], true
}

// KeyframeAtOrAfter returns the first keyframe at or after ptsMs: the earliest place a stream copy
// can begin without re-encoding.
func (a Analysis) KeyframeAtOrAfter(ptsMs int64) (Keyframe, bool) {
	i := sort.Search(len(a.Keyframes), func(i int) bool { return a.Keyframes[i].PTSMs >= ptsMs })
	if i == len(a.Keyframes) {
		return Keyframe{}, false
	}
	return a.Keyframes[i], true
}

// BreaksBetween returns the break candidates in [fromMs, toMs) with at least minConfidence, in
// time order.
func (a Analysis) BreaksBetween(fromMs, toMs int64, minConfidence float64) []Break {
	var out []Break
	for _, b := range a.Breaks {
		if b.AtMs >= fromMs && b.AtMs < toMs && b.Confidence >= minConfidence {
			out = append(out, b)
		}
	}
	return out
}
