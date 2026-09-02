package recommend_test

import (
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/recommend"
)

func TestDecodeSnapshotRejectsIdentityAndViewingHistory(t *testing.T) {
	for name, raw := range map[string]string{
		"operator identity": `{"id":"private","signals":[],"userId":"user-123"}`,
		"viewing history":   `{"id":"private","signals":[],"viewerHistory":["movie:tmdb:603"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := recommend.DecodeSnapshot([]byte(raw))
			if err == nil || !strings.Contains(err.Error(), "snapshot field") {
				t.Fatalf("privacy decode error = %v", err)
			}
		})
	}
}

func TestDecodeSnapshotAcceptsOnlyBoundedRecommendationContext(t *testing.T) {
	raw := []byte(`{"id":"safe","signals":[{"id":"library:genre:comedy","kind":"library_genre","value":"Comedy"}]}`)
	snapshot, err := recommend.DecodeSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ID != "safe" || len(snapshot.Signals) != 1 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}
