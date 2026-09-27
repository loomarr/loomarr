package packager

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	mp4codecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
)

// CodecsAttr is the HLS CODECS attribute of a channel init: the RFC 6381 name of each track, read
// from the init itself rather than assumed from the output profile. It is "" when a track has no
// name here; the master then omits the attribute and players probe.
func CodecsAttr(init []byte) string {
	var in fmp4.Init
	if err := in.Unmarshal(bytes.NewReader(init)); err != nil {
		return ""
	}
	names := make([]string, 0, len(in.Tracks))
	for _, tr := range in.Tracks {
		switch c := tr.Codec.(type) {
		case *mp4codecs.H264:
			if len(c.SPS) < 4 {
				return ""
			}
			// profile_idc, constraint flags, level_idc.
			names = append(names, fmt.Sprintf("avc1.%02x%02x%02x", c.SPS[1], c.SPS[2], c.SPS[3]))
		case *mp4codecs.MPEG4Audio:
			names = append(names, fmt.Sprintf("mp4a.40.%d", c.Config.Type))
		default:
			return ""
		}
	}
	return strings.Join(names, ",")
}
