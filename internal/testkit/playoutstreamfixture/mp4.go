package playoutstreamfixture

import (
	"testing"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4/seekablebuffer"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
)

// FragmentedMP4 makes a structurally valid encoder init and one fragment without running ffmpeg.
// Samples carry inert payloads: this tests container accounting, not codec decoding.
func FragmentedMP4(t testing.TB, fps, frames int) []byte {
	t.Helper()
	init := fmp4.Init{Tracks: []*fmp4.InitTrack{
		{ID: 1, TimeScale: 90000, Codec: &codecs.H264{
			SPS: []byte{0x67, 0x42, 0xc0, 0x0a, 0xd9, 0x04, 0x26, 0xc0, 0x44, 0x00, 0x00, 0x03, 0x00, 0x04, 0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x48, 0x99, 0x20},
			PPS: []byte{0x68, 0xcb, 0x83, 0xcb, 0x20},
		}},
		{ID: 2, TimeScale: 48000, Codec: &codecs.MPEG4Audio{Config: mpeg4audio.AudioSpecificConfig{
			Type: mpeg4audio.ObjectTypeAACLC, SampleRate: 48000, ChannelConfig: 2,
		}}},
	}}
	var output seekablebuffer.Buffer
	if err := init.Marshal(&output); err != nil {
		t.Fatal(err)
	}
	video, audio := &fmp4.PartTrack{ID: 1}, &fmp4.PartTrack{ID: 2}
	for i := range frames {
		video.Samples = append(video.Samples, &fmp4.Sample{Duration: uint32(90000 / fps), IsNonSyncSample: i != 0, Payload: []byte{1}})
	}
	// One priming sample, followed by the rounded audio owed by the video.
	for range (frames*48000/fps+512)/1024 + 1 {
		audio.Samples = append(audio.Samples, &fmp4.Sample{Duration: 1024, Payload: []byte{1}})
	}
	fragment := fmp4.Part{Tracks: []*fmp4.PartTrack{video, audio}}
	if err := fragment.Marshal(&output); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
