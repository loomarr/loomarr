package app

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/library"
)

func TestMediaServerConnectionTestUsesSharedLibraryClient(t *testing.T) {
	providerCalls := 0
	client := library.NewDynamic(func() library.Connection {
		providerCalls++
		return library.Connection{Flavor: library.Emby}
	}, "stable-install-device")

	ok, detail := connectionTests(resolved{}, client, nil)["media_server"](context.Background())
	if ok || detail != "set the media server URL" {
		t.Fatalf("media-server Test = %v, %q; want shared client's missing-URL result", ok, detail)
	}
	if providerCalls != 1 {
		t.Fatalf("connection provider calls = %d, want 1", providerCalls)
	}
}
