package prepared

import "testing"

func TestPreparedVideoReorderingRequiresExplicitZero(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, observation string
		valid             bool
	}{
		{"zero", `{"streams":[{"codec_type":"video","has_b_frames":0}]}`, true},
		{"reordered", `{"streams":[{"codec_type":"video","has_b_frames":2}]}`, false},
		{"unknown", `{"streams":[{"codec_type":"video"}]}`, false},
		{"null", `{"streams":[{"codec_type":"video","has_b_frames":null}]}`, false},
		{"missing video", `{"streams":[]}`, false},
		{"wrong stream", `{"streams":[{"codec_type":"audio","has_b_frames":0}]}`, false},
		{"multiple", `{"streams":[{"codec_type":"video","has_b_frames":0},{"codec_type":"video","has_b_frames":0}]}`, false},
		{"malformed", `{`, false},
		{"wrong type", `{"streams":[{"codec_type":"video","has_b_frames":"0"}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateVideoReordering([]byte(tc.observation)); (err == nil) != tc.valid {
				t.Fatalf("observation accepted = %v, want %v: %v", err == nil, tc.valid, err)
			}
		})
	}
}
