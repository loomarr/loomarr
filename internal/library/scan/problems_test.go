package scan

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/library/inventory"
	"github.com/loomarr/loomarr/internal/media/header"
)

// A movie whose file doesn't parse is still a title, and needs a look; so does a movie with an
// extra copy, sized as its page marks it. Needs you counts them with the titles to answer.
func TestProblemsAreTitlesThatNeedALook(t *testing.T) {
	h := newHarness(t)
	h.im.Header = func(_ context.Context, path string) (header.Header, error) {
		if strings.Contains(path, "Broken") {
			return header.Header{}, errors.New("not a matroska file")
		}
		w, ht := 1920, 1080
		if strings.Contains(path, "DVD") {
			w, ht = 720, 480
		}
		return header.Header{Container: "matroska,webm", DurationMillis: 5_000_000, Streams: []inventory.Stream{
			{Index: 0, Kind: inventory.StreamVideo, Codec: "h264", Width: w, Height: ht},
		}}, nil
	}
	h.file("Broken (1999) {tmdb-90000641}/Broken.mkv", 10)
	h.file("Golf (2003) {tmdb-90000642}/Golf.mkv", 10)
	h.file("Golf (2003) {tmdb-90000642}/Golf DVD.mkv", 30)
	h.file("Hotel (2005) {tmdb-90000643}/Hotel.mkv", 10)
	h.run()

	lib := &Library{ItemStore: h.st, NeedStore: h.st, Inventory: h.inv}
	ctx := context.Background()
	problems, err := lib.Problems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 2 {
		t.Fatalf("problems = %+v, want Broken and Golf", problems)
	}
	broken, golf := problems[0], problems[1]
	if broken.Unplayable != 1 || broken.ExtraBytes != 0 {
		t.Errorf("broken = %+v, want one file that can't be played", broken)
	}
	if golf.Unplayable != 0 || golf.ExtraBytes != 30 {
		t.Errorf("golf = %+v, want the DVD copy's 30 bytes extra", golf)
	}
	if n, err := lib.NeedCount(ctx); err != nil || n != 2 {
		t.Errorf("needs you = %d, %v; want the two problems", n, err)
	}
}
