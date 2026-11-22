package playout

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/testkit"
)

// tracksResolver answers the airing programme's probed tracks.
type tracksResolver struct {
	PlayoutResolver
	tracks playout.MediaTracks
}

func (r tracksResolver) Tracks(context.Context, string) (playout.MediaTracks, error) {
	return r.tracks, nil
}

// The tracks route marks the image subtitle tracks a channel can burn in, and leaves the field off
// text tracks and audio, which stay each viewer's choice.
func TestChannelTracksMarksBurnableSubtitles(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	if _, err := st.SaveChannel(t.Context(), channelstore.Channel{Channel: schedule.Channel{
		ID: "ch1", Name: "Films", Number: 1, Status: schedule.StatusLive,
	}}); err != nil {
		t.Fatal(err)
	}
	s := &Handlers{log: testkit.Logger(), store: st, playoutResolver: tracksResolver{tracks: playout.MediaTracks{
		Audio: []playout.Track{{Index: 0, Language: "eng", Codec: "truehd"}},
		Subtitles: []playout.Track{
			{Index: 0, Language: "eng", Codec: "subrip"},
			{Index: 1, Language: "eng", Title: "Forced", Codec: "hdmv_pgs_subtitle"},
			{Index: 2, Language: "spa", Codec: "dvd_subtitle"},
		},
	}}}

	out, err := s.channelTracks(t.Context(), &httpcore.ChannelIDInput{ID: "ch1"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(out.Body)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"audio":[{"index":0,"language":"eng"}],"subtitles":[` +
		`{"index":0,"language":"eng"},` +
		`{"index":1,"language":"eng","title":"Forced","burnable":true},` +
		`{"index":2,"language":"spa"}]}`
	if string(raw) != want {
		t.Errorf("tracks\n got %s\nwant %s", raw, want)
	}
}
