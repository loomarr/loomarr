package filler

import (
	"bytes"
	"testing"
)

// splitJPEGs is the seam between ffmpeg's image2pipe output (concatenated JPEGs)
// and the decoders — in-package because it is unexported and exercised only by the
// exec-backed Keyframes, which unit tests never run. A synthetic stream is enough:
// SOI markers are what it splits on, and real JPEG payload is irrelevant to that.
func TestSplitJPEGs(t *testing.T) {
	soi := []byte{0xFF, 0xD8}
	// Three "frames" — each begins with SOI and carries a distinct body byte so we
	// can assert the boundaries landed in the right place.
	a := append(append([]byte{}, soi...), 0x01, 0x02)
	b := append(append([]byte{}, soi...), 0x03)
	c := append(append([]byte{}, soi...), 0x04, 0x05, 0x06)

	tests := []struct {
		name   string
		stream []byte
		want   [][]byte
	}{
		{
			name:   "three concatenated frames split on SOI",
			stream: bytes.Join([][]byte{a, b, c}, nil),
			want:   [][]byte{a, b, c},
		},
		{
			name:   "a single frame comes back whole",
			stream: a,
			want:   [][]byte{a},
		},
		{
			// No SOI anywhere: no video decoded, no frames — not an error, an empty
			// result the caller reads as "nothing to look at".
			name:   "no marker yields nothing",
			stream: []byte{0x11, 0x22, 0x33},
			want:   nil,
		},
		{
			name:   "empty stream yields nothing",
			stream: nil,
			want:   nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := splitJPEGs(tc.stream)
			if len(got) != len(tc.want) {
				t.Fatalf("split into %d frames, want %d", len(got), len(tc.want))
			}
			for i := range got {
				if !bytes.Equal(got[i], tc.want[i]) {
					t.Errorf("frame %d = %x, want %x", i, got[i], tc.want[i])
				}
			}
		})
	}
}
