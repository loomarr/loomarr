package library

import (
	"context"
	"strconv"
	"testing"

	"github.com/mantonx/loomarr/internal/testkit"
)

func metaClient(t *testing.T) (*Client, *testkit.MediaServer) {
	t.Helper()
	ms := testkit.NewMediaServer(t)
	t.Cleanup(ms.Close)
	return New(Emby, ms.URL, ms.AdminToken, "loomarr-test-device"), ms
}

// The fields the guide needs, round-tripped from the media server.
func TestItemMetadataByID_ReturnsDisplayMetadata(t *testing.T) {
	c, ms := metaClient(t)
	ms.ItemMetadata = map[string]testkit.ItemMetadata{
		"641360": {
			Overview:       "After his father's death, a young boy finds solace in action movies.",
			Genres:         []string{"Fantasy", "Action", "Comedy"},
			Year:           1993,
			OfficialRating: "PG-13",
		},
	}

	got, err := c.ItemMetadataByID(context.Background(), []string{"641360"})
	if err != nil {
		t.Fatal(err)
	}
	m, ok := got["641360"]
	if !ok {
		t.Fatalf("no metadata returned: %+v", got)
	}
	if m.Overview == "" {
		t.Error("no overview — the guide's detail pane would be empty")
	}
	if len(m.Genres) != 3 {
		t.Errorf("genres = %v, want 3", m.Genres)
	}
	if m.Year != 1993 || m.OfficialRating != "PG-13" {
		t.Errorf("year/rating = %d/%q", m.Year, m.OfficialRating)
	}
}

// ONE REQUEST FOR MANY ITEMS is the whole reason this is affordable on a route a media server
// polls. Per-item lookups would be a round trip per programme and would force a cache.
func TestItemMetadataByID_FetchesInBulk(t *testing.T) {
	c, ms := metaClient(t)
	ms.ItemMetadata = map[string]testkit.ItemMetadata{}
	ids := make([]string, 0, 40)
	for i := 0; i < 40; i++ {
		id := "item-" + strconv.Itoa(i)
		ids = append(ids, id)
		ms.ItemMetadata[id] = testkit.ItemMetadata{Overview: "desc " + id, Year: 2000 + i}
	}

	got, err := c.ItemMetadataByID(context.Background(), ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 40 {
		t.Fatalf("resolved %d of 40 items", len(got))
	}
	// All 40 in ONE request — bulk fetching is the point.
	if n := ms.MetadataRequests(); n != 1 {
		t.Errorf("%d requests for 40 items; the guide polls this route, so it must be bulk", n)
	}
}

// A duplicate id must not be asked for twice: a channel airing the same film twice in a cycle,
// or several channels sharing a title, would otherwise inflate the request.
func TestItemMetadataByID_DeduplicatesIDs(t *testing.T) {
	c, ms := metaClient(t)
	ms.ItemMetadata = map[string]testkit.ItemMetadata{"a": {Overview: "x"}}

	got, err := c.ItemMetadataByID(context.Background(), []string{"a", "a", "a", "", "a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("got %d entries for one distinct id: %+v", len(got), got)
	}
}

// An id the server does not know is simply absent, not an error: a guide must render even when
// a title has been removed from the library since the lineup was built.
func TestItemMetadataByID_MissingIDsAreAbsentNotFatal(t *testing.T) {
	c, ms := metaClient(t)
	ms.ItemMetadata = map[string]testkit.ItemMetadata{"here": {Overview: "present"}}

	got, err := c.ItemMetadataByID(context.Background(), []string{"here", "gone"})
	if err != nil {
		t.Fatalf("a missing item failed the whole lookup: %v", err)
	}
	if _, ok := got["here"]; !ok {
		t.Error("lost the item that does exist")
	}
	if _, ok := got["gone"]; ok {
		t.Error("invented metadata for an item the server does not have")
	}
}

// No ids is not a request at all.
func TestItemMetadataByID_EmptyInputMakesNoRequest(t *testing.T) {
	c, ms := metaClient(t)
	got, err := c.ItemMetadataByID(context.Background(), nil)
	if err != nil || len(got) != 0 {
		t.Errorf("got %d entries, err %v — want empty and no error", len(got), err)
	}
	if n := ms.MetadataRequests(); n != 0 {
		t.Errorf("made %d requests for zero ids", n)
	}
}
