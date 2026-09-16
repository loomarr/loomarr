package tmdb_test

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/tmdb"
)

func TestDiscover_NetworkEditorialEpochFindsOlderPoolIncludingEarlierPremieres(t *testing.T) {
	mock := testkit.NewTMDB(t)
	mock.AddNetwork(49, "History", "US")
	for _, title := range []struct {
		id, year int
		name     string
	}{
		{20001, 1988, "Earlier documentary"},
		{20002, 1993, "Era documentary"},
		{20003, 2009, "Later reality show"},
	} {
		mock.AddSeries(title.id, title.name, title.year, []int{99}, "A network title.")
		mock.SetSeriesNetwork(title.id, 49)
	}
	client := tmdb.NewWithBase(mock.URL, "key")
	got, err := client.Discover(context.Background(), catalog.DiscoveryQuery{MediaType: provision.Series, Network: "History", EditorialEpochEnd: 1999}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("epoch pool = %+v, want earlier and era titles only", got)
	}
	for _, candidate := range got {
		if candidate.Year > 1999 || len(candidate.Networks) != 1 || candidate.Networks[0] != "History" {
			t.Fatalf("unbacked epoch candidate: %+v", candidate)
		}
	}
	for _, request := range mock.Requests() {
		if request.Path != "/discover/tv" {
			continue
		}
		query, err := url.ParseQuery(request.RawQuery)
		if err != nil {
			t.Fatal(err)
		}
		if query.Get("first_air_date.lte") != "1999-12-31" || query.Get("first_air_date.gte") != "" {
			t.Fatalf("wrong editorial coverage projection: %v", query)
		}
	}
}

func TestDiscover_UnmatchedNetworkBrandOffersSourceNameWithoutResolvingAlias(t *testing.T) {
	mock := testkit.NewTMDB(t)
	mock.AddNetwork(49, "History", "US")
	_, err := tmdb.NewWithBase(mock.URL, "key").Discover(context.Background(), catalog.DiscoveryQuery{MediaType: provision.Series, Network: "History Channel"}, 20)
	if err == nil || !strings.Contains(err.Error(), `source catalog names to try: ["History"]`) {
		t.Fatalf("source-backed name hint missing: %v", err)
	}
	for _, request := range mock.Requests() {
		if request.Path == "/discover/tv" {
			t.Fatal("alias hint automatically dispatched a guessed identity")
		}
	}
}

func TestDiscover_NetworkEditorialEpochRejectsUnsupportedContextBeforeHTTP(t *testing.T) {
	for _, query := range []catalog.DiscoveryQuery{
		{MediaType: provision.Movie, Network: "History", EditorialEpochEnd: 1999},
		{MediaType: provision.Series, EditorialEpochEnd: 1999},
		{MediaType: provision.Series, Network: "History", EditorialEpochEnd: 2100},
	} {
		mock := testkit.NewTMDB(t)
		_, err := tmdb.NewWithBase(mock.URL, "key").Discover(context.Background(), query, 20)
		if err == nil || mock.RequestCount() != 0 {
			t.Fatalf("unsupported context reached source: err=%v requests=%d", err, mock.RequestCount())
		}
	}
}
