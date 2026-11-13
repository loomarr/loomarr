package app

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/channels/ideas"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/proposals"
)

type recordingSuggest struct {
	api.SuggestService
	built proposals.Proposal
	by    string
}

func (r *recordingSuggest) SubmitBuilt(_ context.Context, _ proposals.Intent, p proposals.Proposal, by string) (string, error) {
	r.built, r.by = p, by
	return "job-1", nil
}

type demoItems []ideas.Item

func (d demoItems) IdeaItems(context.Context) ([]ideas.Item, error) { return d, nil }

// The demo's Submit never reaches a model: it records a proposal built from the library as the
// guest's own request (#2729).
func TestDemoSuggestBuildsFromTheLibraryInsteadOfAskingAModel(t *testing.T) {
	var items demoItems
	for i := 1; i <= 6; i++ {
		items = append(items, ideas.Item{
			Keys:      []provision.Key{provision.Key("movie:tmdb:" + string(rune('0'+i)))},
			MediaType: provision.Movie, Name: "Film", Year: 1980 + i, Genres: []string{"Comedy"},
		})
	}
	rec := &recordingSuggest{}
	d := demoSuggest{SuggestService: rec, library: items, now: time.Now}
	if !d.Canned() {
		t.Fatal("the demo submitter must say it is canned")
	}
	job, err := d.Submit(context.Background(), proposals.Intent{Description: "funny films"}, "guest")
	if err != nil {
		t.Fatal(err)
	}
	if job != "job-1" || rec.by != "guest" || rec.built.ChannelName == "" || len(rec.built.Lineup) == 0 {
		t.Fatalf("job %q by %q built %+v, want a library-built proposal recorded for the guest", job, rec.by, rec.built)
	}
}
