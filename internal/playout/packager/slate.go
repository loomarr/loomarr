package packager

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4/seekablebuffer"
)

// Slate is a pre-encoded house-format card: one closed GOP of video and a pool of silent AAC frames,
// encoded by the channel's own pipeline so its sample descriptions match the items'. It fills any
// length of slot by repeating the GOP and cycling the silence, with exactly the audio each fragment's
// video end owes, so a long slate never drifts.
type Slate struct {
	init  []byte
	video []*fmp4.Sample
	audio []*fmp4.Sample
}

// NewSlate parses an encoder's fMP4 output (ftyp+moov and at least one fragment). The first
// fragment's first audio frame is AAC priming and is dropped; its video must start with an IDR.
func NewSlate(stream []byte) (*Slate, error) {
	s := &Slate{}
	r := bufio.NewReader(bytes.NewReader(stream))
	var moof []byte
	for s.video == nil {
		typ, b, err := readBox(r)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, errors.New("packager: slate has no fragment")
			}
			return nil, err
		}
		switch typ {
		case "ftyp", "moov":
			s.init = append(s.init, b...)
		case "moof":
			moof = b
		case "mdat":
			parts, err := unmarshalFragment(append(moof, b...))
			if err != nil {
				return nil, fmt.Errorf("packager: slate: %w", err)
			}
			for _, part := range parts {
				for _, tr := range part.Tracks {
					switch tr.ID {
					case videoTrack:
						s.video = append(s.video, tr.Samples...)
					case audioTrack:
						s.audio = append(s.audio, tr.Samples...)
					}
				}
			}
			if len(s.audio) > 0 {
				s.audio = s.audio[1:]
			}
		}
	}
	if len(sampleDescriptions(s.init)) == 0 || len(s.video) == 0 || len(s.audio) == 0 || s.video[0].IsNonSyncSample {
		return nil, errors.New("packager: slate needs an init, an IDR-led GOP and audio")
	}
	return s, nil
}

// fillSlate fills a whole slot with slate, one GOP per segment.
func (p *Packager) fillSlate(ctx context.Context, slot Slot) error {
	p.count(func(s *Stats) { s.Slates++ })
	if !p.acceptInit(p.slate.init) {
		p.count(func(s *Stats) { s.DecoderMismatch++ })
		p.cfg.Log.Error("packager: slate's decoder configuration differs from the channel's")
	}
	var nv, na int64
	for nv < slot.Frames && ctx.Err() == nil {
		frames := min(int64(len(p.slate.video)), slot.Frames-nv)
		audio := min(p.audioOwed(p.v+frames*p.frameDur), slot.AudioFrames-na)
		if nv+frames == slot.Frames {
			audio = slot.AudioFrames - na // the slot's last fragment settles its audio exactly
		}
		b, err := p.slateFragment(frames, max(audio, 0))
		if err != nil {
			return err
		}
		c := map[uint32]int64{videoTrack: frames, audioTrack: max(audio, 0)}
		if err := p.appendSegment(ctx, b, c); err != nil {
			return err
		}
		nv += frames
		na += c[audioTrack]
	}
	return nil
}

func (p *Packager) slateFragment(frames, audio int64) ([]byte, error) {
	video := make([]*fmp4.Sample, frames)
	for i := range video {
		s := *p.slate.video[i]
		s.Duration, s.PTSOffset = uint32(p.frameDur), 0
		video[i] = &s
	}
	sound := make([]*fmp4.Sample, audio)
	for i := range sound {
		s := *p.slate.audio[i%len(p.slate.audio)]
		s.Duration = aacFrame
		sound[i] = &s
	}
	part := &fmp4.Part{SequenceNumber: p.seq, Tracks: []*fmp4.PartTrack{
		{ID: videoTrack, BaseTime: uint64(p.v), Samples: video},
		{ID: audioTrack, BaseTime: uint64(p.a), Samples: sound},
	}}
	var w seekablebuffer.Buffer
	if err := part.Marshal(&w); err != nil {
		return nil, fmt.Errorf("packager: marshal slate: %w", err)
	}
	return w.Bytes(), nil
}
