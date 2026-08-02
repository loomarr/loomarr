package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/clipfetch"
	"github.com/mantonx/loomarr/internal/store"
)

// recordingSources captures what got registered, and can fail on demand.
type recordingSources struct {
	upserted []store.FillerSource
	err      error
}

func (r *recordingSources) ListFillerSources(context.Context) ([]store.FillerSource, error) {
	return r.upserted, nil
}
func (r *recordingSources) UpsertFillerSource(_ context.Context, s store.FillerSource) error {
	if r.err != nil {
		return r.err
	}
	r.upserted = append(r.upserted, s)
	return nil
}
func (r *recordingSources) DeleteFillerSource(context.Context, string) error { return nil }
func (r *recordingSources) MarkFillerSourceFetched(context.Context, string, time.Time) error {
	return nil
}
func (r *recordingSources) SetFillerSourceEnabled(context.Context, string, bool) error { return nil }

func TestRememberSources(t *testing.T) {
	fixed := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	newAdapter := func(rs *recordingSources) fillerServiceAdapter {
		return fillerServiceAdapter{sources: rs, now: func() time.Time { return fixed }}
	}

	t.Run("registers an archive item by its identifier", func(t *testing.T) {
		rs := &recordingSources{}
		newAdapter(rs).rememberSources(context.Background(),
			[]string{"https://archive.org/details/classic_tv_commercials"})

		if len(rs.upserted) != 1 {
			t.Fatalf("registered %d sources, want 1", len(rs.upserted))
		}
		got := rs.upserted[0]
		// Keyed by IDENTIFIER, not the URL: re-adding the same collection spelled a
		// different way must update one row rather than growing a second.
		if got.ID != "classic_tv_commercials" {
			t.Errorf("id = %q, want the archive identifier", got.ID)
		}
		if got.Kind != "archive" {
			t.Errorf("kind = %q, want archive", got.Kind)
		}
		if !got.CreatedAt.Equal(fixed) {
			t.Errorf("CreatedAt = %v, want the injected clock", got.CreatedAt)
		}
	})

	// ⚠ A YouTube URL is a VIDEO, not a source with state to remember. Nothing about it needs
	// a licence, a fetch cursor or a name someone chose, and a row per video would turn the
	// registry into a second, worse clip list.
	t.Run("ignores non-archive urls", func(t *testing.T) {
		rs := &recordingSources{}
		newAdapter(rs).rememberSources(context.Background(), []string{
			"https://www.youtube.com/watch?v=abc123",
			"https://www.youtube.com/playlist?list=PL123",
		})
		if len(rs.upserted) != 0 {
			t.Errorf("registered %v, want nothing for YouTube urls", rs.upserted)
		}
	})

	// ⚠ THE property this is best-effort for: a registry failure must not be able to stop a
	// download. The method returns nothing, so this asserts it does not panic and leaves the
	// caller free to proceed.
	t.Run("survives a store failure", func(t *testing.T) {
		rs := &recordingSources{err: errors.New("disk full")}
		newAdapter(rs).rememberSources(context.Background(),
			[]string{"https://archive.org/details/x"})
		// Reaching here without a panic IS the assertion.
	})

	t.Run("does nothing when no store is wired", func(t *testing.T) {
		fillerServiceAdapter{}.rememberSources(context.Background(),
			[]string{"https://archive.org/details/x"})
	})
}

func TestArchiveIDFrom(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"https://archive.org/details/classic_tv_commercials", "classic_tv_commercials"},
		{"http://archive.org/details/item-1/", "item-1"},
		{"https://archive.org/details/item-2?start=3", "item-2"},
		{"https://archive.org/details/item-3#play", "item-3"},
		{"https://www.youtube.com/watch?v=abc", ""},
		{"not a url", ""},
		{"", ""},
	} {
		if got := archiveIDFrom(tc.in); got != tc.want {
			t.Errorf("archiveIDFrom(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ⚠ The omit-vs-zero rule, tested against the REAL mapping rather than through the API's fake
// — which implements the rule itself, so asserting there proves the fake works, not the code.
// Found by sabotage: deleting the rule from the adapter broke nothing.
//
// 0 means UNKNOWN. archive.org has not probed every item, and a client renders a present 0 as
// "0:00" — claiming the clip is empty, which is a different and false statement from "we don't
// know how long this is".
func TestDiscoveredStats_OmitsAnItemItLearnedNothingAbout(t *testing.T) {
	stats := discoveredStats([]clipfetch.DiscoveredItem{
		{ID: "probed", DurationMS: 91_090, Height: 960},
		{ID: "unprobed"},
		// Partial knowledge is still knowledge: a height with no runtime must survive, or a
		// "known nothing" rule written as AND-vs-OR silently drops half the answers.
		{ID: "height-only", Height: 480},
		{ID: "duration-only", DurationMS: 30_000},
	})

	if _, present := stats["unprobed"]; present {
		t.Errorf("unprobed item present as %+v, want absent", stats["unprobed"])
	}
	for _, id := range []string{"probed", "height-only", "duration-only"} {
		if _, present := stats[id]; !present {
			t.Errorf("%s is missing — anything learned must survive", id)
		}
	}
	if stats["probed"].DurationMS != 91_090 || stats["probed"].Height != 960 {
		t.Errorf("probed = %+v, want the values it carried", stats["probed"])
	}
}
