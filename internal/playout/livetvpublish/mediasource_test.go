package livetvpublish_test

import (
	"context"
	"errors"
	"testing"

	"github.com/loomarr/loomarr/internal/library/mediaserver"
	"github.com/loomarr/loomarr/internal/playout/livetvpublish"
	"github.com/loomarr/loomarr/internal/playout/tunarr/tunarrtest"
	"github.com/loomarr/loomarr/internal/testkit/libraryfixture"
)

func TestMediaSourceConnect_SnapshotsConnectionAndUsersOnce(t *testing.T) {
	primary := libraryfixture.MediaSourceLibrary{
		ConnectionValue: mediaserver.Connection{
			Flavor: mediaserver.Jellyfin, BaseURL: "http://jellyfin-a:8096", Token: "token-a",
		},
		Users: []mediaserver.User{{ID: "admin-a", IsAdmin: true}},
	}
	rotated := libraryfixture.MediaSourceLibrary{
		ConnectionValue: mediaserver.Connection{
			Flavor: mediaserver.Emby, BaseURL: "http://emby-b:8096", Token: "token-b",
		},
		ListUsersErr: errors.New("rotated server must not supply users to this operation"),
	}
	var snapshots int
	programmer := tunarrtest.NewTunarr()
	connector := livetvpublish.NewMediaSourceConnector(func() livetvpublish.MediaSourceLibrary {
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

func embyWithToken(token string) libraryfixture.MediaSourceLibrary {
	return libraryfixture.MediaSourceLibrary{
		ConnectionValue: mediaserver.Connection{Flavor: mediaserver.Emby, BaseURL: "http://emby:8096", Token: token},
		Users:           []mediaserver.User{{ID: "admin", IsAdmin: true}},
	}
}

// A library token that changes without a settings save (an environment-pinned LIBRARY_TOKEN
// rotated across a restart) reaches Tunarr on the next maintenance pass, not never.
func TestMediaSourceConverge_PushesATokenThisProcessHasNotWired(t *testing.T) {
	ctx := context.Background()
	programmer := tunarrtest.NewTunarr()
	before := livetvpublish.NewMediaSourceConnector(func() livetvpublish.MediaSourceLibrary {
		return embyWithToken("token-old")
	}, programmer)
	if _, _, err := before.Connect(ctx); err != nil {
		t.Fatal(err)
	}

	restarted := livetvpublish.NewMediaSourceConnector(func() livetvpublish.MediaSourceLibrary {
		return embyWithToken("token-new")
	}, programmer)
	if err := restarted.Converge(ctx); err != nil {
		t.Fatal(err)
	}
	if programmer.MediaSourceToken != "token-new" {
		t.Fatalf("Tunarr media source token = %q after maintenance, want token-new", programmer.MediaSourceToken)
	}
}

// A save whose auto-wire failed (Tunarr unreachable at that moment) is retried by maintenance:
// the failure does not leave Tunarr streaming with the previous token.
func TestMediaSourceConverge_RetriesAfterAFailedConnect(t *testing.T) {
	ctx := context.Background()
	token := "token-old"
	programmer := tunarrtest.NewTunarr()
	connector := livetvpublish.NewMediaSourceConnector(func() livetvpublish.MediaSourceLibrary {
		return embyWithToken(token)
	}, programmer)
	if _, _, err := connector.Connect(ctx); err != nil {
		t.Fatal(err)
	}

	token = "token-new"
	programmer.EnsureSourceErr = errors.New("dial tcp: connection refused")
	if _, _, err := connector.Connect(ctx); err == nil {
		t.Fatal("Connect succeeded although Tunarr was unreachable")
	}
	programmer.EnsureSourceErr = nil

	if err := connector.Converge(ctx); err != nil {
		t.Fatal(err)
	}
	if programmer.MediaSourceToken != "token-new" {
		t.Fatalf("Tunarr media source token = %q after maintenance, want token-new", programmer.MediaSourceToken)
	}
}

// A failed Connect clears what this process counts as wired even when the library connection is
// unchanged: a tunarr.url save whose wiring failed points at a Tunarr that never got the token.
func TestMediaSourceConverge_RetriesAFailedConnectForTheSameConnection(t *testing.T) {
	ctx := context.Background()
	programmer := tunarrtest.NewTunarr()
	connector := livetvpublish.NewMediaSourceConnector(func() livetvpublish.MediaSourceLibrary {
		return embyWithToken("token")
	}, programmer)
	if _, _, err := connector.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	programmer.EnsureSourceErr = errors.New("dial tcp: connection refused")
	if _, _, err := connector.Connect(ctx); err == nil {
		t.Fatal("Connect succeeded although Tunarr was unreachable")
	}
	programmer.EnsureSourceErr = nil

	if err := connector.Converge(ctx); err != nil {
		t.Fatal(err)
	}
	if programmer.MediaSourceEnsures != 3 {
		t.Fatalf("EnsureEmbySource calls = %d, want 3: maintenance did not retry the failed wiring", programmer.MediaSourceEnsures)
	}
}

// A connection this process already wired is not rewritten on every maintenance tick.
func TestMediaSourceConverge_SkipsAConnectionAlreadyWired(t *testing.T) {
	ctx := context.Background()
	programmer := tunarrtest.NewTunarr()
	connector := livetvpublish.NewMediaSourceConnector(func() livetvpublish.MediaSourceLibrary {
		return embyWithToken("token")
	}, programmer)
	if _, _, err := connector.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	if err := connector.Converge(ctx); err != nil {
		t.Fatal(err)
	}
	if programmer.MediaSourceEnsures != 1 {
		t.Fatalf("EnsureEmbySource calls = %d, want 1: the wired connection was pushed again", programmer.MediaSourceEnsures)
	}
}

// An unconfigured media server is nothing to wire, not a maintenance failure.
func TestMediaSourceConverge_UnconfiguredLibraryIsANoOp(t *testing.T) {
	programmer := tunarrtest.NewTunarr()
	connector := livetvpublish.NewMediaSourceConnector(func() livetvpublish.MediaSourceLibrary {
		return libraryfixture.MediaSourceLibrary{}
	}, programmer)
	if err := connector.Converge(context.Background()); err != nil {
		t.Fatalf("Converge with no media server = %v, want nil", err)
	}
	if programmer.MediaSourceEnsures != 0 {
		t.Fatalf("EnsureEmbySource calls = %d, want none", programmer.MediaSourceEnsures)
	}
}
