package playout

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// TestChannelHDR10_SEIBytes pins the exact prefix SEI NAL. The mastering display's minimum
// luminance (0x00000001) follows zero bytes, so the NAL needs an emulation prevention byte; the
// pinned bytes show where.
func TestChannelHDR10_SEIBytes(t *testing.T) {
	want := "00000001" + "4e01" + // start code, prefix SEI header
		"8918" + // payload 137, 24 bytes
		"2134" + "9baa" + "1996" + "08fc" + "8a48" + "3908" + // G, B, R
		"3d13" + "4042" + // D65
		"00989680" + // 1000 nits
		"000003" + "0001" + // 0.0001 nits, escaped
		"9004" + "03e8" + "0190" + // payload 144, 4 bytes: 1000, 400
		"80"
	if got := hex.EncodeToString(ChannelHDR10.SEI()); got != want {
		t.Fatalf("SEI bytes\n got %s\nwant %s", got, want)
	}
}

// TestEscapeRBSP_NeverImitatesAStartCode: no 00 00 0x (x <= 3) survives, and unescaping restores
// the input.
func TestEscapeRBSP_NeverImitatesAStartCode(t *testing.T) {
	for _, in := range [][]byte{
		{0, 0, 0}, {0, 0, 1}, {0, 0, 2, 0, 0, 3}, {0, 0, 4}, {1, 0, 0, 0, 0, 0}, {0x80},
	} {
		out := escapeRBSP(in)
		for i := 0; i+2 < len(out); i++ {
			if out[i] == 0 && out[i+1] == 0 && out[i+2] <= 3 && out[i+2] != 3 {
				t.Errorf("escapeRBSP(%x) = %x: start-code prefix at %d", in, out, i)
			}
		}
		if back := unescapeRBSP(out); !bytes.Equal(back, in) {
			t.Errorf("unescape(escape(%x)) = %x", in, back)
		}
	}
}

func unescapeRBSP(b []byte) []byte {
	var out []byte
	zeros := 0
	for _, c := range b {
		if zeros >= 2 && c == 3 {
			zeros = 0
			continue
		}
		out = append(out, c)
		if c == 0 {
			zeros++
		} else {
			zeros = 0
		}
	}
	return out
}
