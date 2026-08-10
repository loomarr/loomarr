package docs_test

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/mantonx/loomarr/docs"
	"github.com/mantonx/loomarr/internal/config"
	"github.com/mantonx/loomarr/internal/settings"
)

// The help set ships INSIDE the binary and is read as instructions. embed_test.go already
// proves a deep-link LANDS; nothing proved the sentence at the other end was still true.
//
// It was not. §9.1 made internal playout the default backend, and for months afterwards three
// embedded pages told every operator that "Loomarr doesn't stream or transcode — Tunarr does
// that", while docs/help/quickstart.md listed Tunarr as a hard prerequisite the wizard had
// stopped requiring. The design doc was correct the whole time. The derived documentation had
// rotted away from it with no gate in between — the same failure scripts/check-retired.sh
// exists for, one directory over.
//
// These tests assert a small set of facts the help set may not contradict, DERIVED from code
// rather than restated. The rule: a claim about behaviour belongs next to a test, or it belongs
// in prose that does not assert. Adding a fourth claim is cheaper than finding a fourth wrong page.
//
// ⚠ SCOPE IS docs/help/ ONLY — deliberately. design.md and the engineering notes quote wrong
// claims in order to correct them ("this used to say X"), which is exactly how the repo records
// its own history; banning the phrase everywhere would ban the correction too.

// ---------------------------------------------------------------------------
// Claim 1: the playout default
// ---------------------------------------------------------------------------

// contradictsInternalPlayout are phrases that assert something ELSE does the streaming. Each is
// recorded with the page it was actually found on, so a future reader can tell a real guard from
// a speculative one.
var contradictsInternalPlayout = []string{
	"doesn't stream or transcode",  // was docs/help/concepts.md
	"does not stream or transcode", // the same claim, reworded
	"Tunarr streams it",            // was docs/help/integrations.md
	"Tunarr owns playout",          // was docs/help/troubleshooting.md
}

func TestHelpDoesNotContradictPlayoutDefault(t *testing.T) {
	backend, ok := settings.NewRegistry().Get("playout.backend")
	if !ok {
		t.Fatal("playout.backend is not a declared setting — this guard has lost its subject")
	}
	// ⚠ A hard failure, NOT t.Skip. If the default ever flips to tunarr these phrases become
	// TRUE and the guard must be re-authored in the same change — a silently skipped test is
	// the "green that proves nothing" this repo keeps re-learning about.
	if backend.Default != "internal" {
		t.Fatalf("playout.backend now defaults to %q, not \"internal\".\n"+
			"These phrase guards encode the internal-default world and must be re-authored, "+
			"not deleted: the help set has to stop saying Loomarr does the streaming.", backend.Default)
	}

	for _, page := range docs.Pages() {
		for _, phrase := range contradictsInternalPlayout {
			if strings.Contains(page.Markdown, phrase) {
				t.Errorf("docs/help/%s.md says %q, but playout.backend defaults to %q.\n"+
					"  Loomarr serves its own streams on the default path (§9.1); Tunarr is an "+
					"alternative backend, not the streamer.", page.Slug, phrase, backend.Default)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Claim 2: every env var named in help actually exists
// ---------------------------------------------------------------------------

// envVarPattern matches SCREAMING_SNAKE_CASE with at least one underscore. The underscore is
// what keeps the false-positive rate at zero: it excludes prose acronyms the help set is full of
// (TMDB, LLM, GET, HLS, TV) while matching every real pin, all of which are compound.
var envVarPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]*(_[A-Z0-9]+)+$`)

// backtickSpan captures inline-code spans, so only things the docs PRESENT as identifiers are
// checked. A capitalised phrase in prose is not a claim about an env var.
var backtickSpan = regexp.MustCompile("`([^`\n]+)`")

// envVarsOutsideTheRegistry are real, documented pins that the registry legitimately does not
// declare. Each needs a reason; this list is the place a reader looks to find out why a variable
// is "real but absent", and it must not become a junk drawer for typos.
var envVarsOutsideTheRegistry = map[string]string{
	"PLAYOUT_RENDER_DEVICE": "compose-only — it drives the `devices:` mapping; the app never reads it",
	"PLAYOUT_FONT_PATH":     "read directly via os.Getenv in internal/playout/font.go",
	"FILLER_DROP_DIR":       "compose-only — selects the host path bound at /filler",
	"MEDIA_SERVER_IP":       "compose-only — dev Tunarr's extra_hosts entry",
}

func TestHelpEnvVarsExist(t *testing.T) {
	valid := knownEnvVars(t)
	var checked int

	for _, page := range docs.Pages() {
		for _, m := range backtickSpan.FindAllStringSubmatch(page.Markdown, -1) {
			// Split on "=" so `LLM_PROVIDER=ollama` is checked as a variable, matching how
			// the pages actually write examples.
			token := m[1]
			if i := strings.IndexByte(token, '='); i >= 0 {
				token = token[:i]
			}
			token = strings.TrimSpace(token)
			if !envVarPattern.MatchString(token) {
				continue
			}
			checked++
			if _, ok := valid[token]; ok {
				continue
			}
			if why, ok := envVarsOutsideTheRegistry[token]; ok {
				t.Logf("docs/help/%s.md names %s (%s)", page.Slug, token, why)
				continue
			}
			t.Errorf("docs/help/%s.md documents %s, which is not a declared setting, "+
				"a bootstrap config key, or a recorded exception.\n"+
				"  An operator following this page would set a variable nothing reads.\n"+
				"  Fix the page, or add %s to envVarsOutsideTheRegistry WITH a reason.",
				page.Slug, token, token)
		}
	}

	// Without this, a broken regex scans nothing and reports success — the failure mode that
	// makes a gate worse than no gate, because it also reports confidence. The Integrations
	// page alone names well over a dozen pins.
	if checked < 10 {
		t.Errorf("only %d env-var token(s) examined across the help set; expected the "+
			"Integrations page alone to name more. The scanner is probably broken, not the docs.", checked)
	}
}

// knownEnvVars is the union of the two places a pin can legitimately come from: the typed
// settings registry (env > database > default) and the env-only bootstrap struct. Both are read
// from code, so a rename moves this set automatically — which is the entire point.
func knownEnvVars(t *testing.T) map[string]struct{} {
	t.Helper()
	out := map[string]struct{}{}

	for _, s := range settings.NewRegistry().All() {
		if s.EnvVar != "" {
			out[s.EnvVar] = struct{}{}
		}
	}

	// internal/config declares its keys as `env:"..."` struct tags rather than registry rows,
	// so reflection is the only way to read them without duplicating the list here.
	cfg := reflect.TypeOf(config.Config{})
	for i := range cfg.NumField() {
		if tag := cfg.Field(i).Tag.Get("env"); tag != "" {
			out[strings.Split(tag, ",")[0]] = struct{}{}
		}
	}

	if len(out) == 0 {
		t.Fatal("no env vars discovered — the check would pass vacuously against any page")
	}
	return out
}

// ---------------------------------------------------------------------------
// Claim 3: shown commands reference files that exist
// ---------------------------------------------------------------------------

var composeFileFlag = regexp.MustCompile(`docker compose\s+(?:[^\n]*?\s)?-f\s+(\S+)`)

func TestHelpComposeCommandsResolve(t *testing.T) {
	// The help pages live one directory below the repo root, and the commands they show are
	// written to be run FROM that root.
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	var checked int
	for _, page := range docs.Pages() {
		for _, m := range composeFileFlag.FindAllStringSubmatch(page.Markdown, -1) {
			path := m[1]
			checked++
			if _, err := os.Stat(filepath.Join(root, path)); err != nil {
				t.Errorf("docs/help/%s.md runs `docker compose -f %s`, which does not exist.\n"+
					"  A copy-pasted quickstart command would fail on the first line.", page.Slug, path)
			}
		}
	}
	// A quickstart that shows no compose command at all is a bigger problem than a wrong path,
	// and would otherwise make this test pass by having nothing to check.
	if checked == 0 {
		t.Error("no `docker compose -f …` command found in the help set — the Quickstart " +
			"should show one, and this guard is inert without it")
	}
}

// ---------------------------------------------------------------------------
// Coverage: the guard set itself
// ---------------------------------------------------------------------------

// TestClaimsCoverEveryHelpPage does not check content — it checks that this file's assumption
// (that it can see the whole embedded set) still holds. A page added under docs/help/ is
// automatically in scope above; this fails loudly if the embed ever stops returning pages,
// which would make all three claims pass against nothing.
func TestClaimsCoverEveryHelpPage(t *testing.T) {
	pages := docs.Pages()
	if len(pages) < 2 {
		t.Fatalf("only %d help page(s) embedded — the claim guards would be near-vacuous", len(pages))
	}
	var slugs []string
	for _, p := range pages {
		slugs = append(slugs, p.Slug)
	}
	sort.Strings(slugs)
	t.Logf("claims checked across %d pages: %s", len(slugs), strings.Join(slugs, ", "))

	// Fail if a page is empty: an empty file satisfies every "must not contain" check.
	for _, p := range pages {
		if len(strings.TrimSpace(p.Markdown)) < 100 {
			t.Errorf("docs/help/%s.md is nearly empty; it would satisfy every guard here vacuously", p.Slug)
		}
	}
}
