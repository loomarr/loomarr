package connectiontest

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/library/mediaserver"
)

func TestMediaServerConnectionTestUsesSharedLibraryClient(t *testing.T) {
	providerCalls := 0
	client := mediaserver.NewDynamic(func() mediaserver.Connection {
		providerCalls++
		return mediaserver.Connection{Flavor: mediaserver.Emby}
	}, "stable-install-device")

	ok, detail := New(Config{Library: client})["media_server"](context.Background())
	if ok || detail != "set the media server URL" {
		t.Fatalf("media-server Test = %v, %q; want shared client's missing-URL result", ok, detail)
	}
	if providerCalls != 1 {
		t.Fatalf("connection provider calls = %d, want 1", providerCalls)
	}
}
