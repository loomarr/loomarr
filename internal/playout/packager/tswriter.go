package packager

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	mp4codecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	tscodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts/codecs"
)

// TSWriter is the channel's MPEG-TS sink (#1512 phase 2b): it turns the packaged fMP4 timeline,
// segment by segment, into one continuous transport stream for media-server tuners. It adds no
// timing of its own: the packager's timeline is already gapless, so one writer (one continuity
// counter per PID, one PCR) for a viewer's whole session carries that across every item
// boundary. Parameter sets go in band at every IDR, so a tuner can join at any segment.
type TSWriter struct {
	w            *mpegts.Writer
	video, audio *mpegts.Track
	params       [][]byte // SPS/PPS (H.264) or VPS/SPS/PPS (HEVC), prepended to every IDR
	hevc         bool
}

// NewTSWriter starts a transport stream for the channel's init segment (H.264 or HEVC, AAC).
func NewTSWriter(w io.Writer, init []byte) (*TSWriter, error) {
	var in fmp4.Init
	if err := in.Unmarshal(bytes.NewReader(init)); err != nil {
		return nil, fmt.Errorf("packager ts: init: %w", err)
	}
	t := &TSWriter{}
	for _, tr := range in.Tracks {
		switch c := tr.Codec.(type) {
		case *mp4codecs.H264:
			t.video, t.params = &mpegts.Track{Codec: &tscodecs.H264{}}, [][]byte{c.SPS, c.PPS}
		case *mp4codecs.H265:
			t.video, t.params, t.hevc = &mpegts.Track{Codec: &tscodecs.H265{}}, [][]byte{c.VPS, c.SPS, c.PPS}, true
		case *mp4codecs.MPEG4Audio:
			t.audio = &mpegts.Track{Codec: &tscodecs.MPEG4Audio{Config: c.Config}}
		}
	}
	if t.video == nil || t.audio == nil {
		return nil, errors.New("packager ts: init needs an H.264 or HEVC track and an AAC track")
	}
	t.w = &mpegts.Writer{W: w, Tracks: []*mpegts.Track{t.video, t.audio}}
	if err := t.w.Initialize(); err != nil {
		return nil, err
	}
	return t, nil
}

// tsUnit is one access unit on the 90 kHz transport clock.
type tsUnit struct {
	dts, pts int64
	video    bool
	payload  []byte
	sync     bool
}

// WriteSegment muxes one packaged segment (moof+mdat), interleaving its video and audio by time.
func (t *TSWriter) WriteSegment(seg []byte) error {
	parts, err := unmarshalFragment(seg)
	if err != nil {
		return fmt.Errorf("packager ts: %w", err)
	}
	var units []tsUnit
	for _, part := range parts {
		for _, tr := range part.Tracks {
			at := int64(tr.BaseTime)
			for _, s := range tr.Samples {
				switch tr.ID {
				case videoTrack: // already 90 kHz
					units = append(units, tsUnit{dts: at, pts: at + int64(s.PTSOffset), video: true, payload: s.Payload, sync: !s.IsNonSyncSample})
				case audioTrack: // 48 kHz: 1024 samples are exactly 1920 ticks
					units = append(units, tsUnit{dts: at * videoRate / audioRate, pts: at * videoRate / audioRate, payload: s.Payload})
				}
				at += int64(s.Duration)
			}
		}
	}
	sort.SliceStable(units, func(i, j int) bool { return units[i].dts < units[j].dts })
	for _, u := range units {
		if err := t.write(u); err != nil {
			return err
		}
	}
	return nil
}

func (t *TSWriter) write(u tsUnit) error {
	if !u.video {
		return t.w.WriteMPEG4Audio(t.audio, u.pts, [][]byte{u.payload})
	}
	var au h264.AVCC
	if err := au.Unmarshal(u.payload); err != nil {
		return fmt.Errorf("packager ts: video sample: %w", err)
	}
	if u.sync {
		au = append(append([][]byte{}, t.params...), au...)
	}
	if t.hevc {
		if u.sync && !h265.IsRandomAccess(au) {
			return errors.New("packager ts: a sync sample without an IRAP picture")
		}
		return t.w.WriteH265(t.video, u.pts, u.dts, au)
	}
	return t.w.WriteH264(t.video, u.pts, u.dts, au)
}
