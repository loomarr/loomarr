package mediaserver_test

import (
	"context"
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/library/mediaserver"
	"github.com/loomarr/loomarr/internal/testkit"
)

// MediaFolders reads every video library's folders from the captured VirtualFolders array: mixed
// libraries (no CollectionType) are kept, music and collections are not.
func TestMediaFolders_ListsVideoLibraryFolders(t *testing.T) {
	ms := testkit.NewMediaServer(t)
	c := mediaserver.New(mediaserver.Emby, ms.URL, ms.AdminToken, "dev-1")

	got, err := c.MediaFolders(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"/data/tv", "/data/movies", "/config/data/top-picks"} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %s in %v", want, got)
		}
	}
	if slices.Contains(got, "/data/music") {
		t.Errorf("a music library was listed: %v", got)
	}
	if name, err := c.FlavorName(); err != nil || name != "emby" {
		t.Fatalf("FlavorName = %q, %v", name, err)
	}
}
