package app

import (
	"bytes"
	"context"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/channeldatastore"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/playout/resolve"
)

type fakeUpcomingGuide []resolve.UpcomingProgramme

func (f fakeUpcomingGuide) UpcomingProgrammes(context.Context, string, time.Time, time.Time) ([]resolve.UpcomingProgramme, error) {
	return f, nil
}

type fakeUpcomingWriter struct {
	wrote    bool
	revision int64
	airings  []channeldatastore.UpcomingAiring
}

func (f *fakeUpcomingWriter) ReplaceUpcomingAirings(_ context.Context, _ string, revision int64, airings []channeldatastore.UpcomingAiring) (bool, error) {
	f.revision, f.airings = revision, airings
	return f.wrote, nil
}

func TestUpcomingIndex_StoresTheGuidesProgrammes(t *testing.T) {
	from := time.Unix(1_800_000_000, 0).UTC()
	guide := fakeUpcomingGuide{{
		Start: from, Stop: from.Add(time.Hour), LibraryItemID: "lib-1", Key: "movie:tmdb:90000001", Forecast: true,
	}}
	writer := &fakeUpcomingWriter{wrote: true}
	u := upcomingIndex{guide: guide, store: writer}

	if err := u.IndexUpcoming(context.Background(), channelstore.Channel{ID: "ch-1", Revision: 7}, from, from.Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	want := []channeldatastore.UpcomingAiring{{
		ChannelID: "ch-1", Start: from, End: from.Add(time.Hour), LibraryItemID: "lib-1",
		TitleKey: "movie:tmdb:90000001", Forecast: true,
	}}
	if writer.revision != 7 || !reflect.DeepEqual(writer.airings, want) {
		t.Fatalf("wrote revision %d, %+v; want revision 7, %+v", writer.revision, writer.airings, want)
	}
}

// A write that loses its revision guard is not an error; it is logged at debug.
func TestUpcomingIndex_LostGuardIsLoggedAtDebug(t *testing.T) {
	var out bytes.Buffer
	u := upcomingIndex{
		guide: fakeUpcomingGuide{},
		store: &fakeUpcomingWriter{wrote: false},
		log:   slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}

	if err := u.ClearUpcoming(context.Background(), channelstore.Channel{ID: "ch-1", Revision: 7}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "level=DEBUG") || !strings.Contains(got, "channel=ch-1") {
		t.Fatalf("log = %q, want one debug line naming the channel", got)
	}
}
