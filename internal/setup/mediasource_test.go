package setup_test

import (
	"context"
	"errors"
	"testing"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/setup"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/libraryfixture"
)

func TestMediaSourceConnect_SnapshotsConnectionAndUsersOnce(t *testing.T) {
	primary := libraryfixture.MediaSourceLibrary{
		ConnectionValue: library.Connection{
			Flavor: library.Jellyfin, BaseURL: "http://jellyfin-a:8096", Token: "token-a",
		},
		Users: []library.User{{ID: "admin-a", IsAdmin: true}},
	}
	rotated := libraryfixture.MediaSourceLibrary{
		ConnectionValue: library.Connection{
			Flavor: library.Emby, BaseURL: "http://emby-b:8096", Token: "token-b",
		},
		ListUsersErr: errors.New("rotated server must not supply users to this operation"),
	}
	var snapshots int
	programmer := testkit.NewTunarr()
	connector := setup.NewMediaSourceConnector(func() setup.MediaSourceLibrary {
		snapshots++
		if snapshots == 1 {
			return primary
		}
		return rotated
	}, programmer)

	sourceID, enabled, err := connector.Connect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshots != 1 {
		t.Fatalf("library snapshots = %d, want exactly 1 for Connect", snapshots)
	}
	if sourceID == "" || enabled != 2 {
		t.Fatalf("Connect = source %q, enabled %d; want source and 2 libraries", sourceID, enabled)
	}
	if programmer.MediaSourceToken != "token-a" {
		t.Fatalf("media source token = %q, want snapshot token-a", programmer.MediaSourceToken)
	}
}
