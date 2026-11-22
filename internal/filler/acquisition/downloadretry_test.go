package acquisition

import (
	"testing"
	"time"
)

// Three attempts fit inside a day, and the third failure starts the rest.
func TestDownloadRetryAt_ThreeAttemptsOverADayThenRests(t *testing.T) {
	at := time.Unix(1_800_000_000, 0).UTC()

	// The attempts: the first at the start, the second after the first failure's wait, the third
	// after the second's. They all land inside 24 hours.
	second := DownloadRetryAt(1, at)
	third := DownloadRetryAt(2, second)
	if span := third.Sub(at); span > 24*time.Hour || span < 12*time.Hour {
		t.Fatalf("the third attempt comes %v after the first, want within the day and not crowded into the first hours", span)
	}
	if !second.After(at) || !third.After(second) {
		t.Fatalf("attempts are not spread out: %v %v %v", at, second, third)
	}

	for _, failures := range []int{DownloadAttempts, DownloadAttempts + 1, 10} {
		if rest := DownloadRetryAt(failures, at).Sub(at); rest != 30*24*time.Hour {
			t.Fatalf("after %d failures the item rests %v, want 30 days", failures, rest)
		}
	}
}

// A one-off URL has no provider id, so its URL is its identity; an item with an id is the same item
// whatever URL the provider now serves it at.
func TestDownloadItemKey(t *testing.T) {
	byID := DownloadItem{Kind: "youtube", SourceID: "9000a1", RemoteID: "abc", URL: "https://example.invalid/a"}
	moved := byID
	moved.URL = "https://example.invalid/b"
	if byID.Key() != moved.Key() {
		t.Fatal("an item with a provider id changed identity with its URL")
	}
	oneOffA := DownloadItem{Kind: "youtube", URL: "https://example.invalid/a"}
	oneOffB := DownloadItem{Kind: "youtube", URL: "https://example.invalid/b"}
	if oneOffA.Key() == oneOffB.Key() {
		t.Fatal("two one-off URLs share an identity")
	}
	if oneOffA.Key() == byID.Key() {
		t.Fatal("a one-off URL shares an identity with a source item")
	}
}
