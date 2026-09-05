package playoutcert

import "testing"

func TestParseProbeRequiresExactlyOneVideoAndAudio(t *testing.T) {
	shape, err := parseProbe([]byte(`{"streams":[{"codec_type":"video","codec_name":"h264"},{"codec_type":"audio","codec_name":"aac"}]}`))
	if err != nil || shape.VideoStreams != 1 || shape.AudioStreams != 1 || shape.VideoCodec != "h264" || shape.AudioCodec != "aac" {
		t.Fatalf("parseProbe = %+v, %v", shape, err)
	}
	if _, err := parseProbe([]byte(`{"streams":[{"codec_type":"video","codec_name":"h264"}]}`)); err == nil {
		t.Fatal("video-only media passed validation")
	}
	if _, err := parseProbe([]byte(`{"streams":[{"codec_type":"video"},{"codec_type":"video"},{"codec_type":"audio"}]}`)); err == nil {
		t.Fatal("two-video media passed validation")
	}
}
