package demolibrary

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path"
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

// migratedTitles are real titles, brands, people and ids that #1592 removed from the migrated
// paths below. Unmigrated fixtures still carry many of them, so they cannot join realTitles yet
// without listing those files; until then they are enforced on the migrated paths only. Add to
// it; never remove from it. When legacyFixtures is empty, fold it into realTitles.
var migratedTitles = []string{
	"Sunny D", "Gushers", "Fruit by the Foot", "Kellogg", "Bran Flakes", "Grrreat", "Springfield",
	"Bart the Mother", "Lisa Gets an A", "Kidney Trouble", "Radioactive Man", "Milhouse", "Homediddly",
	"Treehouse of Horror", "Neutral Zone", "Best of Both Worlds", "First Contact", "TNG", "Michael Mann",
	"Ada Lovelace", "Lovelace", "Grace Hopper", "Katherine Johnson", "Alan Turing", "Mary Jackson",
	"Dorothy Vaughan", "Margaret Hamilton", "IaEBrNaHfUs", "saturday-morning-cartoons-v-110", "Heat",
	"Predator", "The Rock", "Breaking Bad", "Harry Potter", "Philosopher's Stone", "Chamber of Secrets",
	"Prisoner of Azkaban", "Goblet of Fire", "Schwarzenegger", "Great Indian Family",
	// The Go API tests and testkit (slice 5).
	"South Park", "King of the Hill", "Indiana Jones", "Next Generation", "Deep Space Nine", "Campbell",
	"McDonald", "Tootsie", "HP Sauce", "Fanta", "Hopper", "Ronin", "Full House", "Family Matters",
	"Bob's Burgers", "Last Action Hero", "Killing Game", "Moaning Lisa", "Bart the Genius", "In Flames",
	// The suggest date-execution and reference/network tests (slice 7).
	"TGIF", "Frasier", "Modern Marvels", "Roosevelts", "History Channel", "Discovery Channel", "Toonami",
	"Sabrina the Teenage Witch", "Boy Meets World", "Camp Wilder", "Mr. Cooper", "Teen Angel", "Iron Man",
	"MCU",
	// The remaining suggest tests (slice 8). Common words and phrases these titles share ("Big",
	// "Speed", "step by step", "criterion") are left out: they would match ordinary prose.
	"Gremlins", "Back to the Future", "Goonies", "Raiders of the Lost Ark", "Temple of Doom", "Last Crusade",
	"Dial of Destiny", "Toy Story", "Castle in the Sky", "Shrunk the Kids", "Beetlejuice", "Ferris Bueller",
	"Coming to America", "Teen Wolf", "Weekend at Bernie's", "Sister Act", "To Be or Not to Be",
	"Lover Come Back", "Casablanca", "The Expanse", "Matrix Reloaded", "Matrix Revolutions", "Father Brown",
	"with Children", "MST3K", "DS9", "Tom Hanks", "Meg Ryan", "Nora Ephron", "Keanu Reeves", "Wachowski",
	"Tiffani Thiessen", "Jeff Franklin", "David Simon", "Idris Elba", "HBO", "PG Tips",
	// The integration, tmdb, catalog, binder and library tests (slice 9). "Terminator" and "Raiders"
	// alone are left out: they would match ordinary prose.
	"RoboCop", "Homer at the Bat", "Zero Theorem", "Space Odyssey", "70s Show",
	// The web component tests and stories (slice 10). "Elf", "Voyager" and "Criterion" alone are
	// left out: they would match ordinary prose.
	"Home Alone", "Rugrats", "Coca-Cola", "Pepsi", "Toys R Us",
	// The schedule, playout and recurate tests (slice 11). Episode titles that are ordinary phrases
	// ("Chain of Command", "Ship in a Bottle", "The Inner Light", "Night Moves") are left out.
	"Battlestar Galactica", "Mad Max", "Marge vs. the Monorail", "Wings of the Dope", "Dog of Death",
	"Canine Mutiny", "Halloween of Horror", "Life on the Fast Lane", "Farpoint", "Tribble", "Realm of Fear",
	"Truffaut", "Jules & Jim", "Top Goon", "Halloweeners",
	// The filler, fillerenrichment and fillerstore tests (slice 12). Brands that are ordinary words
	// ("Ford", "Fox", "Puma", "Coke", "Mustang", "Total", "Five") are left out: they would match
	// ordinary prose.
	"Swiffer", "Aqua Globes", "Walgreens", "WKRP", "Smokey Bear", "Jersey Mike", "Turbo Teen",
	"Highway Patrol", "Sega Genesis", "TMNT", "Capri Sun", "La Llave", "TV Puls", "NatWest", "Rumbelows",
	"KCPQ", "WNYW", "WAGA", "Sony", "Lego", "NFL", "CBC",
	// The web app, core and player tests, and the e2e mock backend (slice 14). "Speed" alone is
	// left out: it would match ordinary prose.
	"Wizarding World", "Crimson Tide", "Air Force One", "True Lies", "Enemy of the State", "Animaniacs",
	"Gargoyles", "Warner Bros",
}

// migrated are the paths (prefixes) #1592 has moved onto the demo catalogue. They must be free of
// realTitles AND migratedTitles. Each later #1592 slice adds its paths here.
var migrated = []string{
	"web/packages/fixtures/src/",
	// The stories that render the shared fixtures' guide and surf channels with their artwork.
	"web/apps/web/src/design-system/guide-surface.stories.tsx",
	"web/apps/web/src/design-system/programme-card.stories.tsx",
	"web/apps/web/src/design-system/surf-rail.stories.tsx",
	"web/native-stories/guide-surface.stories.tsx",
	// The AI proposal stories.
	"web/apps/web/src/components/loomarr/ai/proposal-edit/",
	"web/apps/web/src/components/loomarr/ai/refine-panel/",
	"web/apps/web/src/components/loomarr/ai/refine-review/",
	"web/apps/web/src/components/loomarr/feedback/generation-progress/",
	// The web component stories (slice 10). Their directories' other stories are still legacy.
	"web/apps/web/src/components/loomarr/channels/channel-icon-field/channel-icon-field.stories.tsx",
	"web/apps/web/src/components/loomarr/dashboard/playout-panel/playout-panel.stories.tsx",
	"web/apps/web/src/components/loomarr/people/import-dialog/import-dialog.stories.tsx",
	"web/apps/web/src/components/loomarr/people/invitation-dialog/invitation-dialog.stories.tsx",
	"web/apps/web/src/components/loomarr/people/invitation-join/invitation-join.stories.tsx",
	"web/apps/web/src/components/loomarr/people/invitation-roster/invitation-roster.stories.tsx",
	"web/apps/web/src/components/loomarr/shell/app-shell/app-shell.stories.tsx",
	"web/apps/web/src/components/ui/card/card.stories.tsx",
	"web/apps/web/src/components/ui/disclosure/disclosure.stories.tsx",
	"web/apps/web/src/components/ui/label/label.stories.tsx",
	// The layout and incoming stories and the e2e mock backend (slice 14).
	"web/apps/web/src/design-system/layout.stories.tsx",
	"web/apps/web/src/filler/incoming/incoming.stories.tsx",
	"web/apps/web/tests/e2e/mock-backend.ts",
}

// migratedTests are the directories whose unit tests and e2e specs #1592 has moved onto the demo
// catalogue. isFixture skips tests, so the migrated guard reads their *.test.* and *.spec.* files
// from here; each entry must hold at least one. Keep in step with the matching case in
// scripts/ci-impact.sh.
var migratedTests = []string{
	"web/apps/web/src/components/loomarr/ai/approval-queue-item/",
	"web/apps/web/src/components/loomarr/ai/episode-selection-label/",
	"web/apps/web/src/components/loomarr/ai/proposal-edit/",
	"web/apps/web/src/components/loomarr/ai/proposal-review/",
	"web/apps/web/src/components/loomarr/ai/refine-panel/",
	"web/apps/web/src/components/loomarr/ai/refine-review/",
	"web/apps/web/src/components/loomarr/feedback/generation-progress/",
	"web/apps/web/src/discovery-feedback/",
	"web/apps/web/src/suggest/approval-queue/",
	"web/apps/web/src/suggest/intent-form/",
	"web/apps/web/src/suggest/live-proposal-outlook/",
	"web/apps/web/src/suggest/movie-collection-choices/",
	"web/apps/web/src/suggest/suggestion-expansion/",
	"web/apps/web/src/suggest/suggestion-language/",
	"web/apps/web/src/suggest/use-channel-refine/",
	"web/apps/web/src/suggest/use-proposal-review-edit/",
	"web/apps/web/src/suggest/use-suggestion-run/",
	// The web component tests, policy overrides and request detail (slice 10).
	"web/apps/web/src/components/loomarr/channels/channel-card/",
	"web/apps/web/src/components/loomarr/channels/channel-collections-scope/",
	"web/apps/web/src/components/loomarr/channels/channel-cycle-preview/",
	"web/apps/web/src/components/loomarr/channels/channel-ident/",
	"web/apps/web/src/components/loomarr/channels/channel-lineup-editor/",
	"web/apps/web/src/components/loomarr/channels/channel-policy-fields/",
	"web/apps/web/src/components/loomarr/channels/channel-rules-editor/",
	"web/apps/web/src/components/loomarr/channels/channel-series-scope/",
	"web/apps/web/src/components/loomarr/channels/now-next-strip/",
	"web/apps/web/src/components/loomarr/filler/clip-card/",
	"web/apps/web/src/components/loomarr/filler/clip-player/",
	"web/apps/web/src/components/loomarr/filler/clip-row/",
	"web/apps/web/src/components/loomarr/filler/pod-timeline/",
	"web/apps/web/src/components/loomarr/filler/split-review-editor/",
	"web/apps/web/src/components/loomarr/people/import-dialog/",
	"web/apps/web/src/components/loomarr/people/invitation-join/",
	"web/apps/web/src/components/loomarr/shell/search-command/",
	"web/apps/web/src/components/ui/panel-row/",
	"web/apps/web/src/components/ui/sheet/",
	"web/apps/web/src/components/ui/video-player/",
	"web/apps/web/src/lib/policy-overrides/",
	"web/apps/web/src/queue/request-detail/",
	// The rest of the web app's tests, its e2e specs, and the core and player packages (slice 14).
	"web/apps/web/src/channels/use-channel-lineup/",
	"web/apps/web/src/channels/use-channel-save/",
	"web/apps/web/src/filler/channel-filler/",
	"web/apps/web/src/filler/clip-details/",
	"web/apps/web/src/filler/incoming/",
	"web/apps/web/src/filler/sources-panel/",
	"web/apps/web/src/queue/request-lists/",
	"web/apps/web/src/suggest/channel-suggest-panel/",
	"web/apps/web/src/suggest/manual-channel-builder/",
	"web/apps/web/src/test/",
	"web/apps/web/tests/e2e/",
	"web/packages/core/src/",
	"web/packages/core/tests/",
	"web/packages/player/tests/",
}

// migratedGo are the Go trees and files #1592 has moved onto the demo catalogue: the API tests,
// the shared testkit, and the tests that consume the testkit's mock TMDB catalogue. Their
// *_test.go files, and every .go file of the testkit, are scanned like a migrated fixture. The
// API's own route examples are not tests and are not scanned yet. Captured provider responses
// under internal/testkit/fixtures stay out of scope, as they do for isFixture.
var migratedGo = []string{
	"internal/api/", "internal/testkit/",
	// The mock TMDB catalogue's consumers (slice 6).
	"internal/app/adapters_tmdb_test.go",
	"internal/app/iconadapter_test.go",
	"internal/suggest/suggester_progress_test.go",
	"internal/suggest/suggester_test.go",
	"internal/suggest/worker_callsite_test.go",
	"internal/suggest/worker_llmcause_test.go",
	"internal/suggest/worker_test.go",
	// The suggest date-execution candidate's callers and the reference/network internals (slice 7).
	"internal/suggest/cache_prefix_test.go",
	"internal/suggest/date_execution_test.go",
	"internal/suggest/date_source_order_test.go",
	"internal/suggest/date_tool_retirement_test.go",
	"internal/suggest/finalization_state_test.go",
	"internal/suggest/lean_final_test.go",
	"internal/suggest/network_epoch_internal_test.go",
	"internal/suggest/network_epoch_test.go",
	"internal/suggest/query_expansion_regression_test.go",
	"internal/suggest/reference_discovery_internal_test.go",
	"internal/suggest/reference_discovery_test.go",
	// The remaining suggest tests (slice 8).
	"internal/suggest/approval_size_test.go",
	"internal/suggest/approve_test.go",
	"internal/suggest/candidate_key_test.go",
	"internal/suggest/channel_name_test.go",
	"internal/suggest/date_retrieval_test.go",
	"internal/suggest/diagnostic_test.go",
	"internal/suggest/editorial_role_test.go",
	"internal/suggest/example_fill_test.go",
	"internal/suggest/failure_test.go",
	"internal/suggest/named_example_titles_test.go",
	"internal/suggest/parse_test.go",
	"internal/suggest/policy_extract_test.go",
	"internal/suggest/prepare_approval_test.go",
	"internal/suggest/progress_stream_test.go",
	"internal/suggest/proposal_size_internal_test.go",
	"internal/suggest/ranker_test.go",
	"internal/suggest/score_era_test.go",
	"internal/suggest/score_intent_test.go",
	"internal/suggest/tools_internal_test.go",
	// The integration, tmdb, catalog, binder, library and collection tests, and the last suggest
	// leftovers (slice 9).
	"internal/binder/", "internal/catalog/", "internal/ideas/", "internal/integration/", "internal/library/",
	"internal/moviecollections/", "internal/recommend/", "internal/tmdb/",
	"internal/suggest/date_meaning_test.go",
	"internal/suggest/intent_policy_test.go",
	"internal/suggest/scopenarrows_test.go",
	"internal/suggest/worker_cache_internal_test.go",
	// The schedule, playout and recurate tests (slice 11).
	"internal/playout/", "internal/recurate/", "internal/schedule/",
	// The filler, fillerenrichment and fillerstore tests (slice 12).
	"internal/filler/", "internal/fillerenrichment/", "internal/fillerstore/",
}

// pendingGo are the files under migratedGo not yet migrated, both owned by an open PR (#1917).
// This list may only SHRINK: a listed file that is already clean fails (delete its line).
var pendingGo = []string{
	"internal/api/channelideas_test.go",
	"internal/api/proposaljourneys_test.go",
}

// realDataGo are Go tests that exercise real external data on purpose, so #1592 never migrates
// them and migratedGo skips them even when a slice adds their whole package. Each carries its
// reason; a file listed here must exist.
var realDataGo = map[string]string{
	"internal/eval/schedule_live_test.go":             "queries live data on purpose",
	"internal/fillerenrichment/deterministic_test.go": "pins the production brand dictionary in deterministic.go, whose phrases are real brands",
	"internal/suggest/live_latency_test.go":           "drives a real LLM, whose world knowledge of real titles is what it measures",
	"internal/suggest/live_named_titles_test.go":      "drives a real LLM on requests that name real titles",
	"internal/tmdb/exists_contract_test.go":           "pins the live-captured /movie/603 response (testkit/fixtures), real id and title",
	"internal/tmdb/fixture_test.go":                   "parses a live-captured TMDB response (testkit/fixtures), real ids and all",
}

// realProviderID matches a TMDB/TVDB reference in source: a provisioning key, a Go field or a JSON
// field (`movie:tmdb:603`, `TMDBID: 603`, `"tmdbId": 603`, a media server's
// `"ProviderIds":{"Tmdb":"603"}`); a positional id after a media type
// (`libItem("movie", 603, …)`, `Exists(ctx, provision.Movie, 603)`) or first in a testkit TMDB
// call (`AddMovie(603, …)`); a provider path (`/tv/1396/season/1`); or a bare id (`"id": 603`,
// or a title-shaped `ID: 603, Name:` literal). Digits may carry Go's `_` separators. Any id
// from 1 to below ProviderIDBase names a real title; 0 means "no id". The bare and path forms
// cannot tell a provider id from any other small number, so a scanned file gives every such
// id, provider or not, a value from ProviderIDBase up.
var realProviderID = regexp.MustCompile(`(?i)(?:` +
	`(?:movie|series):(?:tmdb|tvdb):|(?:tmdb|tvdb)_?id"?\s*[:=]\s*"?|"(?:tmdb|tvdb)"\s*:\s*"?|` +
	`"(?:movie|series|tv)",\s*|provision\.(?:movie|series),\s*|` +
	`(?:AddKeywordMovie|AddMovie|AddSeries|AddPerson|AddNetwork)\(\s*|` +
	`/(?:movie|tv|collection|find)/|(?-i:"id")\s*:\s*` +
	`)(\d[\d_]*)|(?-i:\bID):\s*(\d[\d_]*),\s*Name:`)

// guarded must be entirely free of real titles and ids.
var guarded = []string{"cmd/seed", "cmd/demo-library", "internal/demolibrary"}

// legacyFixtures still carry real titles and are migrated off them in #1592. They render the
// Storybook stories behind the visual baselines, so each migration re-shoots baselines; until
// then this list may only SHRINK: a new file with a real title fails, and so does a listed file
// that is already clean (delete its line).
var legacyFixtures = []string{
	"web/apps/web/src/components/loomarr/channels/channel-breaks/channel-breaks.stories.tsx",
	"web/apps/web/src/components/loomarr/channels/channel-card/channel-card.stories.tsx",
	"web/apps/web/src/components/loomarr/channels/channel-collections-scope/channel-collections-scope.stories.tsx",
	"web/apps/web/src/components/loomarr/channels/channel-cycle-preview/channel-cycle-preview.stories.tsx",
	"web/apps/web/src/components/loomarr/channels/channel-ident/channel-ident.stories.tsx",
	"web/apps/web/src/components/loomarr/channels/channel-lineup-editor/channel-lineup-editor.stories.tsx",
	"web/apps/web/src/components/loomarr/channels/channel-rules-editor/channel-rules-editor.stories.tsx",
	"web/apps/web/src/components/loomarr/channels/channel-series-scope/channel-series-scope.stories.tsx",
	"web/apps/web/src/components/loomarr/channels/now-next-strip/now-next-strip.stories.tsx",
	"web/apps/web/src/components/loomarr/filler/segment-filmstrip/segment-filmstrip.stories.tsx",
	"web/apps/web/src/components/ui/caption/caption.stories.tsx",
	"web/apps/web/src/components/ui/video-player/timeline-scrubber/timeline-scrubber.stories.tsx",
	"web/apps/web/src/design-system/identity.stories.tsx",
	"web/apps/web/src/design-system/overlay.stories.tsx",
	"web/native-stories/identity.stories.tsx",
	"web/native-stories/overlay.stories.tsx",
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

func denylistRE(titles []string) *regexp.Regexp {
	parts := make([]string, len(titles))
	for i, t := range titles {
		parts[i] = regexp.QuoteMeta(t)
	}
	return regexp.MustCompile(`(?i)\b(?:` + strings.Join(parts, "|") + `)\b`)
}

// realHits lists the real titles and real provider ids in one file's text.
func realHits(text string) []string {
	hits := denylistRE(realTitles).FindAllString(text, -1)
	for _, m := range realProviderID.FindAllStringSubmatch(text, -1) {
		digits := m[1] + m[2] // exactly one group matched
		if id, err := strconv.Atoi(strings.ReplaceAll(digits, "_", "")); err == nil && id > 0 && id < ProviderIDBase {
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
		`"ProviderIds":{"Tmdb":"603","Tvdb":"81189"}`,
		`name: "point break"`,
		// Positional ids: after a media type, and first in a testkit TMDB call.
		`ok, err := c.Exists(ctx, provision.Movie, 603)`,
		`mock.SetRating(provision.Series, 1396, "TV-MA")`,
		`mock.AddMovie(671, "Some Film", 2001, nil, "")`,
		`mock.AddKeywordMovie(77_001, "Winter Story", 2020, nil, "", "Christmas")`,
		// Bare ids: a provider path, a JSON id, a title-shaped literal.
		`case "/tv/1396/season/1/episode/1":`,
		`{"id":85,"belongs_to_collection":{"id":84}}`,
		`100: {ID: 100, Name: "Some Film", Year: 1994},`,
	} {
		if len(realHits(bad)) == 0 {
			t.Errorf("guard missed a real reference in %q", bad)
		}
	}
	for _, ok := range []string{
		`key: "movie:tmdb:90000001"`, `"The Clockmaker's Umbrella"`, `Bladerunner-free text: blades, bladed`,
		`mock.AddKeywordMovie(90_077_001, "Winter Story", 2020, nil, "", "Christmas")`,
		`c.Exists(ctx, provision.Movie, 0)`, `{ID: 1, Title: "Uncited"}`, `want := map[string]int{two.ID: 2}`,
		`{id: 3, name: "Kettle Kids"}`, `"ProviderIds":{"Tmdb":"90000603"}`,
	} {
		if hits := realHits(ok); len(hits) != 0 {
			t.Errorf("guard flagged %q in invented text %q", hits, ok)
		}
	}
	strict := denylistRE(migratedTitles)
	for _, bad := range []string{`name: "Sunny D — Dude!"`, `series: "Star Trek: TNG"`, `approvedByName: "Ada Lovelace"`, `title: "Harry Potter and the Goblet of Fire"`, `Name: "King of the Hill"`} {
		if !strict.MatchString(bad) {
			t.Errorf("migrated-path guard missed a real reference in %q", bad)
		}
	}
	for _, ok := range []string{`name: "Fizzbolt — Dude!"`, `series: "Cold Relay"`, `approvedByName: "Adela Hale"`} {
		if hits := strict.FindAllString(ok, -1); len(hits) != 0 {
			t.Errorf("migrated-path guard flagged %q in invented text %q", hits, ok)
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

// seedFiles lists every file under the guarded directories, repo-relative.
func seedFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	for _, dir := range guarded {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			files = append(files, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return files
}

// fixtureFiles lists every web seed or fixture the guard scans, repo-relative.
func fixtureFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
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
		if rel = filepath.ToSlash(rel); isFixture(rel) {
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// migratedTestFiles lists the *.test.* and *.spec.* files under migratedTests, repo-relative. An
// entry with no test file fails, so a moved or renamed directory cannot leave the guard reading
// nothing.
func migratedTestFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	for _, dir := range migratedTests {
		before := len(files)
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.Contains(d.Name(), ".test.") && !strings.Contains(d.Name(), ".spec.") {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			files = append(files, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(files) == before {
			t.Errorf("migratedTests lists %s, which holds no *.test.* file", dir)
		}
	}
	return files
}

// migratedGoFiles lists the Go files under migratedGo the guard scans, repo-relative.
func migratedGoFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	for _, dir := range migratedGo {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			if rel = filepath.ToSlash(rel); d.IsDir() && rel == "internal/testkit/fixtures" {
				return filepath.SkipDir
			}
			if _, real := realDataGo[rel]; real {
				return nil
			}
			if !d.IsDir() && (strings.HasSuffix(rel, "_test.go") || strings.HasPrefix(rel, "internal/testkit/") && strings.HasSuffix(rel, ".go")) {
				files = append(files, rel)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return files
}

func TestMigratedGoTestsStayInvented(t *testing.T) {
	root := repoRoot(t)
	strict := denylistRE(migratedTitles)
	files := migratedGoFiles(t, root)
	if len(files) == 0 {
		t.Fatalf("no Go file under %q; the guard is scanning nothing", migratedGo)
	}
	var dirty []string
	for _, rel := range files {
		text := mustRead(t, filepath.Join(root, rel))
		hits := append(realHits(text), strict.FindAllString(text, -1)...)
		if len(hits) == 0 {
			continue
		}
		dirty = append(dirty, rel)
		if !slices.Contains(pendingGo, rel) {
			t.Errorf("%s was moved onto the demo catalogue (#1592) but carries real titles, people or ids %q", rel, hits)
		}
	}
	for _, rel := range pendingGo {
		if !slices.Contains(dirty, rel) {
			t.Errorf("%s no longer carries real titles; delete it from pendingGo", rel)
		}
	}
	for rel := range realDataGo {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Errorf("realDataGo lists %s, which no longer exists; delete its line", rel)
		}
	}
}

func TestSeedsCarryNoRealTitles(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range seedFiles(t, root) {
		if path.Base(rel) == "guard_test.go" { // holds the denylist itself
			continue
		}
		if hits := realHits(mustRead(t, filepath.Join(root, rel))); len(hits) != 0 {
			t.Errorf("%s carries real titles or ids: %q", rel, hits)
		}
	}
}

// CI runs this guard in the path-filtered Go contracts job (privacy-verify), so the ci-impact
// classifier must select that job for every file the guard scans, Go and web alike.
func TestCIRunsTheGuardWheneverAScannedFileChanges(t *testing.T) {
	root := repoRoot(t)
	scanned := slices.Concat(seedFiles(t, root), fixtureFiles(t, root), migratedTestFiles(t, root), migratedGoFiles(t, root))
	for _, rel := range scanned {
		cmd := exec.Command("./scripts/ci-impact.sh", rel)
		cmd.Dir = root
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("ci-impact %s: %v", rel, err)
		}
		if !slices.Contains(strings.Fields(string(out)), "contracts=true") {
			t.Errorf("ci-impact skips the Go contracts job for %s, so a real title there would merge unchecked; route it in scripts/ci-impact.sh", rel)
		}
	}
}

func TestFixturesCarryNoNewRealTitles(t *testing.T) {
	root := repoRoot(t)
	var dirty []string
	for _, rel := range fixtureFiles(t, root) {
		if len(realHits(mustRead(t, filepath.Join(root, rel)))) != 0 {
			dirty = append(dirty, rel)
		}
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

func TestMigratedFixturesStayInvented(t *testing.T) {
	root := repoRoot(t)
	strict := denylistRE(migratedTitles)
	var files []string
	for _, rel := range fixtureFiles(t, root) {
		if slices.ContainsFunc(migrated, func(prefix string) bool { return strings.HasPrefix(rel, prefix) }) {
			files = append(files, rel)
		}
	}
	if len(files) == 0 {
		t.Fatalf("no fixture file matched the migrated paths %q; the guard is scanning nothing", migrated)
	}
	for _, rel := range append(files, migratedTestFiles(t, root)...) {
		if slices.Contains(legacyFixtures, rel) {
			t.Errorf("%s is migrated (#1592) but still listed in legacyFixtures", rel)
		}
		text := mustRead(t, filepath.Join(root, rel))
		if hits := append(realHits(text), strict.FindAllString(text, -1)...); len(hits) != 0 {
			t.Errorf("%s was moved onto the demo catalogue (#1592) but carries real titles, people or ids %q", rel, hits)
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
