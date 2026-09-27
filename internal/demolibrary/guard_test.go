package demolibrary

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The repo is public: seeds, fixtures and the demo library must never carry a real media title,
// a real brand, or a real provider id that would fetch a real poster (#1587).

// realTitles is the denylist: every title and brand `make seed` shipped before #1587, plus
// franchises that recur in this repo's older fixtures. Matched case-insensitively on word
// boundaries. Add to it; never remove from it.
var realTitles = []string{
	"The Matrix", "Terminator 2", "Judgment Day", "Lock, Stock and Two Smoking Barrels", "Point Break",
	"Blade", "Face/Off", "Con Air", "Frosted Flakes", "Super Nintendo", "Nintendo", "Nike", "Kellogg's",
	"Just Do It", "They're Grrreat", "Simpsons", "Star Wars", "Star Trek", "Batman", "Seinfeld", "Futurama",
	"X-Files", "Twin Peaks", "Die Hard", "Jurassic Park", "Ghostbusters", "Scooby-Doo", "Looney Tunes",
	"Transformers", "Pokémon", "Pokemon",
}

// realProviderID matches a TMDB/TVDB reference in source: a provisioning key, a Go field, a JSON
// field, or a positional id in a seed helper call (`libItem("movie", 603, …)`). Any id below
// ProviderIDBase names a real title.
var realProviderID = regexp.MustCompile(`(?i)(?:(?:movie|series):(?:tmdb|tvdb):|(?:tmdb|tvdb)_?id"?\s*[:=]\s*"?|Item\("(?:movie|series)",\s*)(\d+)`)

// guarded must be entirely free of real titles and ids.
var guarded = []string{"cmd/seed", "cmd/demo-library", "internal/demolibrary"}

// legacyFixtures still carry real titles and are migrated off them in #1592. They render the
// Storybook stories behind the visual baselines, so each migration re-shoots baselines; until
// then this list may only SHRINK: a new file with a real title fails, and so does a listed file
// that is already clean (delete its line).
var legacyFixtures = []string{
	"web/apps/web/src/components/loomarr/ai/proposal-edit/proposal-edit.stories.tsx",
	"web/apps/web/src/components/loomarr/ai/refine-panel/refine-panel.stories.tsx",
	"web/apps/web/src/components/loomarr/ai/refine-review/refine-review.stories.tsx",
	"web/apps/web/src/components/loomarr/channels/channel-breaks/channel-breaks.stories.tsx",
	"web/apps/web/src/components/loomarr/channels/channel-card/channel-card.stories.tsx",
	"web/apps/web/src/components/loomarr/channels/channel-collections-scope/channel-collections-scope.stories.tsx",
	"web/apps/web/src/components/loomarr/channels/channel-cycle-preview/channel-cycle-preview.stories.tsx",
	"web/apps/web/src/components/loomarr/channels/channel-ident/channel-ident.stories.tsx",
	"web/apps/web/src/components/loomarr/channels/channel-lineup-editor/channel-lineup-editor.stories.tsx",
	"web/apps/web/src/components/loomarr/channels/channel-rules-editor/channel-rules-editor.stories.tsx",
	"web/apps/web/src/components/loomarr/channels/channel-series-scope/channel-series-scope.stories.tsx",
	"web/apps/web/src/components/loomarr/channels/now-next-strip/now-next-strip.stories.tsx",
	"web/apps/web/src/components/loomarr/feedback/generation-progress/generation-progress.stories.tsx",
	"web/apps/web/src/components/loomarr/filler/segment-filmstrip/segment-filmstrip.stories.tsx",
	"web/apps/web/src/components/loomarr/guide/guide-detail-card/guide-detail-card.stories.tsx",
	"web/apps/web/src/components/loomarr/guide/guide-grid/guide-grid.stories.tsx",
	"web/apps/web/src/components/ui/caption/caption.stories.tsx",
	"web/apps/web/src/components/ui/video-player/timeline-scrubber/timeline-scrubber.stories.tsx",
	"web/apps/web/src/design-system/guide-surface.stories.tsx",
	"web/apps/web/src/design-system/identity.stories.tsx",
	"web/apps/web/src/design-system/overlay.stories.tsx",
	"web/apps/web/src/design-system/surf-rail.stories.tsx",
	"web/apps/web/tests/e2e/mock-backend.ts",
	"web/native-stories/identity.stories.tsx",
	"web/native-stories/overlay.stories.tsx",
	"web/packages/fixtures/src/client-platform/programmes/programmes.ts",
	"web/packages/fixtures/src/client-platform/surf/surf.ts",
	"web/packages/fixtures/src/testcard/testcard.ts",
	"web/scripts/tv-emulator-fixture-server.mjs",
}

// isFixture reports whether a repo path is a seed or fixture the guard scans. Captured provider
// responses under internal/testkit/fixtures are deliberately out of scope: they are recorded
// evidence of real servers' wire shapes, not demo content, and never render anywhere.
func isFixture(rel string) bool {
	switch {
	case strings.HasSuffix(rel, ".stories.tsx"):
		return true
	case strings.HasPrefix(rel, "web/packages/fixtures/src/") && !strings.Contains(rel, ".test."):
		return true
	case rel == "web/apps/web/tests/e2e/mock-backend.ts", rel == "web/scripts/tv-emulator-fixture-server.mjs":
		return true
	}
	return false
}

func denylistRE() *regexp.Regexp {
	parts := make([]string, len(realTitles))
	for i, t := range realTitles {
		parts[i] = regexp.QuoteMeta(t)
	}
	return regexp.MustCompile(`(?i)\b(?:` + strings.Join(parts, "|") + `)\b`)
}

// realHits lists the real titles and real provider ids in one file's text.
func realHits(text string) []string {
	hits := denylistRE().FindAllString(text, -1)
	for _, m := range realProviderID.FindAllStringSubmatch(text, -1) {
		if id, err := strconv.Atoi(m[1]); err == nil && id < ProviderIDBase {
			hits = append(hits, m[0])
		}
	}
	return hits
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the test directory")
		}
		dir = parent
	}
}

func TestGuardCatchesRealTitlesAndIDs(t *testing.T) {
	for _, bad := range []string{
		`{libItem("movie", 603, "Some Film", 1999, "lib-x"), 1}`,
		`key: "movie:tmdb:78"`,
		`TMDBID: 9738,`,
		`"tvdbId": 71663`,
		`name: "point break"`,
	} {
		if len(realHits(bad)) == 0 {
			t.Errorf("guard missed a real reference in %q", bad)
		}
	}
	for _, ok := range []string{`key: "movie:tmdb:90000001"`, `"The Clockmaker's Umbrella"`, `Bladerunner-free text: blades, bladed`} {
		if hits := realHits(ok); len(hits) != 0 {
			t.Errorf("guard flagged %q in invented text %q", hits, ok)
		}
	}
}

func TestDemoCatalogueIsInvented(t *testing.T) {
	seen := map[string]bool{}
	for _, ti := range Catalogue {
		if hits := realHits(ti.Name + " " + ti.Overview); len(hits) != 0 {
			t.Errorf("demo title %q matches real titles %q", ti.Name, hits)
		}
		if seen[ti.ID()] {
			t.Errorf("duplicate demo id %s", ti.ID())
		}
		seen[ti.ID()] = true
	}
	for _, c := range Channels {
		for _, id := range c.Titles {
			if _, ok := ByID(id); !ok {
				t.Errorf("channel %d lists unknown title %s", c.Number, id)
			}
		}
	}
}

func TestSeedsCarryNoRealTitles(t *testing.T) {
	root := repoRoot(t)
	for _, dir := range guarded {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if filepath.Base(p) == "guard_test.go" { // holds the denylist itself
				return nil
			}
			raw, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if hits := realHits(string(raw)); len(hits) != 0 {
				rel, _ := filepath.Rel(root, p)
				t.Errorf("%s carries real titles or ids: %q", rel, hits)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestFixturesCarryNoNewRealTitles(t *testing.T) {
	root := repoRoot(t)
	var dirty []string
	err := filepath.WalkDir(filepath.Join(root, "web"), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); n == "node_modules" || n == "dist" || n == "storybook-static" || strings.HasPrefix(n, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if !isFixture(filepath.ToSlash(rel)) {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if len(realHits(string(raw))) != 0 {
			dirty = append(dirty, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range dirty {
		if !slices.Contains(legacyFixtures, rel) {
			hits := realHits(mustRead(t, filepath.Join(root, rel)))
			t.Errorf("%s adds real titles or ids %q; use the demo catalogue (internal/demolibrary)", rel, hits)
		}
	}
	for _, rel := range legacyFixtures {
		if !slices.Contains(dirty, rel) {
			t.Errorf("%s no longer carries real titles; delete it from legacyFixtures", rel)
		}
	}
}

func mustRead(t *testing.T, p string) string {
	t.Helper()
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// The generator's palette is a copy of the brand contract (the backend does not ship the web
// tree); this pins the two together.
func TestPaletteMatchesBrandContract(t *testing.T) {
	var brand struct {
		Chroma     []string `json:"chroma"`
		Ground     string   `json:"ground"`
		Foreground string   `json:"foreground"`
	}
	raw := mustRead(t, filepath.Join(repoRoot(t), "web/packages/design-system/src/tokens/brand-contract.json"))
	if err := json.Unmarshal([]byte(raw), &brand); err != nil {
		t.Fatal(err)
	}
	want := make([]string, len(brand.Chroma))
	for i, c := range brand.Chroma {
		want[i] = strings.TrimPrefix(c, "#")
	}
	if !slices.Equal(chroma, want) || ground != strings.TrimPrefix(brand.Ground, "#") || foreground != strings.TrimPrefix(brand.Foreground, "#") {
		t.Fatalf("demo palette %v/%s/%s differs from brand-contract.json %v/%s/%s", chroma, ground, foreground, want, brand.Ground, brand.Foreground)
	}
}
