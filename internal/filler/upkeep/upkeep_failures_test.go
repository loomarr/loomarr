package upkeep

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/quarantine"
	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/media/tools"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// readFailsStore makes reading one clip fail.
type readFailsStore struct {
	*fakeStore
	err error
}

func (s readFailsStore) GetClip(ctx context.Context, hash string) (clipstore.Clip, bool, error) {
	if s.err != nil {
		return clipstore.Clip{}, false, s.err
	}
	return s.fakeStore.GetClip(ctx, hash)
}

// A rejected clip whose files cannot be set aside is retried on every pass, so each way it can fail
// to be set aside says so, once per pass, with its cause. The ordinary shapes of a clip (a sidecar
// that is absent, or a valid one with nothing of Loomarr's in it) are not failures and stay quiet.
func TestUpkeepSaysWhyARejectedClipStaysPut(t *testing.T) {
	master := filepath.ToSlash(filepath.Join(split.MediaAssetRootName, "masters", "aa", "bb", "m.mkv"))
	for name, tc := range map[string]struct {
		prepare   func(t *testing.T, f *fixture, playback string) Store
		warn      []string // substrings of the one WARN; none = no WARN at all
		wantMoved bool
	}{
		"the clip cannot be read": {
			prepare: func(t *testing.T, f *fixture, _ string) Store {
				return readFailsStore{f.store, errors.New("catalog unavailable")}
			},
			warn: []string{"could not be read", "catalog unavailable"},
		},
		"the clip has no catalog row": {
			prepare: func(t *testing.T, f *fixture, _ string) Store {
				delete(f.store.clips, hashOf("old"))
				return f.store
			},
			warn: []string{"no catalog row"},
		},
		"the sidecar is corrupt": {
			prepare: func(t *testing.T, f *fixture, playback string) Store {
				if err := os.WriteFile(tools.SidecarPathFor(playback), []byte("{not json"), 0o644); err != nil {
					t.Fatal(err)
				}
				return f.store
			},
			warn: []string{"sidecar", "left where they are"},
		},
		"the sidecar cannot be read": {
			prepare: func(t *testing.T, f *fixture, playback string) Store {
				side := tools.SidecarPathFor(playback)
				if err := os.Remove(side); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(side, 0o755); err != nil { // a read of it fails with something other than not-exist
					t.Fatal(err)
				}
				return f.store
			},
			warn: []string{"sidecar", "left where they are"},
		},
		"Loomarr's part of the sidecar is malformed": {
			prepare: func(t *testing.T, f *fixture, playback string) Store {
				if err := os.WriteFile(tools.SidecarPathFor(playback), []byte(`{"loomarr":null}`), 0o644); err != nil {
					t.Fatal(err)
				}
				return f.store
			},
			warn: []string{"sidecar", "left where they are"},
		},
		"no sidecar at all is an ordinary clip": {
			prepare: func(t *testing.T, f *fixture, playback string) Store {
				if err := os.Remove(tools.SidecarPathFor(playback)); err != nil {
					t.Fatal(err)
				}
				return f.store
			},
			wantMoved: true,
		},
		"a valid sidecar with nothing of Loomarr's in it is ordinary": {
			prepare: func(t *testing.T, f *fixture, playback string) Store {
				if err := os.WriteFile(tools.SidecarPathFor(playback), []byte(`{"title":"someone else's"}`), 0o644); err != nil {
					t.Fatal(err)
				}
				return f.store
			},
			wantMoved: true,
		},
		"a clip whose sidecar names its master is ordinary": {
			prepare:   func(t *testing.T, f *fixture, _ string) Store { return f.store },
			wantMoved: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			old := f.reject(t, hashOf("old"), "run-1", master, t0.Add(-RejectRest-1))
			store := tc.prepare(t, f, old)
			now := func() time.Time { return t0 }
			var logs bytes.Buffer
			log := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

			res, err := New(store, quarantine.New(f.dir, now), nil, f.dir, now, log).Run(context.Background())
			if err != nil {
				t.Fatalf("Run = %v", err)
			}
			out := logs.String()
			if len(tc.warn) == 0 {
				if strings.Contains(out, "level=WARN") {
					t.Fatalf("an ordinary clip warned: %s", out)
				}
			} else {
				if got := strings.Count(out, "level=WARN"); got != 1 {
					t.Fatalf("want one WARN, got %d: %s", got, out)
				}
				for _, want := range tc.warn {
					if !strings.Contains(out, want) {
						t.Errorf("log does not mention %q: %s", want, out)
					}
				}
			}
			if moved := res.Quarantined == 1; moved != tc.wantMoved {
				t.Fatalf("quarantined = %d, want moved=%v", res.Quarantined, tc.wantMoved)
			}
			if !tc.wantMoved && (!exists(old) || !exists(filepath.Join(f.dir, master))) {
				t.Fatal("a clip that could not be judged had its media moved: the master would be orphaned")
			}
		})
	}
}

// A corrupt sidecar is the one a clip is held back for, because it may name the master and the
// evidence copy: moving the sidecar away first would leave them where nothing points. Once the
// sidecar is readable again the next pass sets the whole clip aside, master included.
func TestACorruptSidecarHoldsTheClipUntilItIsReadable(t *testing.T) {
	f := newFixture(t)
	master := filepath.ToSlash(filepath.Join(split.MediaAssetRootName, "masters", "aa", "bb", "m.mkv"))
	old := f.reject(t, hashOf("old"), "run-1", master, t0.Add(-RejectRest-1))
	side := tools.SidecarPathFor(old)
	good, err := os.ReadFile(side)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(side, []byte("{truncated"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return t0 }
	bin := quarantine.New(f.dir, now)

	first, err := New(f.store, bin, nil, f.dir, now, nil).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.LetGo != 1 || first.Quarantined != 0 || !exists(old) || !exists(side) || !exists(filepath.Join(f.dir, master)) {
		t.Fatalf("first pass = %+v; want the clip retired and every file left in place", first)
	}

	if err := os.WriteFile(side, good, 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := New(f.store, bin, nil, f.dir, now, nil).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second.Quarantined != 1 || exists(old) || exists(side) || exists(filepath.Join(f.dir, master)) {
		t.Fatalf("second pass = %+v; want the playback file, the sidecar and the master all set aside", second)
	}
}
