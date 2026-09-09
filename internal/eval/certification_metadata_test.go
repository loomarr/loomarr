//go:build eval

package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/loomarr/loomarr/internal/provision"
)

func TestV8MetadataStrictDateScopes(t *testing.T) {
	tests := []struct {
		name  string
		scope string
		valid bool
	}{
		{"explicit none", `{}`, true},
		{"independent boundary axes", `{"movieRelease":[{"from":1900,"to":1900}],"seriesPremiere":[{"from":2099,"to":2099}]}`, true},
		{"canonical disjoint", `{"movieRelease":[{"from":1900,"to":1901},{"from":2099,"to":2099}]}`, true},
		{"null scope", `null`, false},
		{"null axis", `{"movieRelease":null}`, false},
		{"empty axis", `{"movieRelease":[]}`, false},
		{"wrong shape", `[]`, false},
		{"unknown axis", `{"release":[{"from":1990,"to":1990}]}`, false},
		{"unknown range field", `{"movieRelease":[{"from":1990,"to":1990,"extra":1}]}`, false},
		{"missing from", `{"movieRelease":[{"to":1990}]}`, false},
		{"missing to", `{"movieRelease":[{"from":1990}]}`, false},
		{"noninteger", `{"movieRelease":[{"from":1990.5,"to":1991}]}`, false},
		{"null bound", `{"movieRelease":[{"from":null,"to":1991}]}`, false},
		{"zero", `{"movieRelease":[{"from":0,"to":1991}]}`, false},
		{"out of range", `{"movieRelease":[{"from":1899,"to":1991}]}`, false},
		{"inverted", `{"movieRelease":[{"from":1992,"to":1991}]}`, false},
		{"overlapping", `{"movieRelease":[{"from":1990,"to":1992},{"from":1992,"to":1993}]}`, false},
		{"adjacent unnormalized", `{"movieRelease":[{"from":1990,"to":1991},{"from":1992,"to":1993}]}`, false},
		{"unsorted", `{"movieRelease":[{"from":2000,"to":2001},{"from":1990,"to":1991}]}`, false},
		{"too many ranges", `{"movieRelease":[{"from":1900,"to":1900},{"from":1910,"to":1910},{"from":1920,"to":1920},{"from":1930,"to":1930},{"from":1940,"to":1940}]}`, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			files := v8MetadataFS(t, func(manifest, _, _ map[string]any) {
				manifest["dateScopes"].(map[string]any)["date-movie-adjacent"] = json.RawMessage(test.scope)
			})
			_, err := certificationCases(files)
			if test.valid && err != nil {
				t.Fatalf("certificationCases() error = %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("certificationCases() error = nil, want metadata rejection")
			}
		})
	}
}

func TestV8MetadataRejectsInvalidReferencesBeforeProjection(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(manifest, base, fixture map[string]any)
	}{
		{
			name: "unknown family",
			mutate: func(manifest, _, _ map[string]any) {
				manifest["proposalKeys"].(map[string]any)["unknown"] = []any{"movie:tmdb:11001"}
			},
		},
		{
			name: "unknown fixture",
			mutate: func(_, base, _ map[string]any) {
				caseByID(base, "date-movie-adjacent")["fixtureCase"] = "missing"
			},
		},
		{
			name: "illegal key",
			mutate: func(manifest, _, _ map[string]any) {
				manifest["proposalKeys"].(map[string]any)["date-movie-adjacent"] = []any{"not-a-key"}
			},
		},
		{
			name: "duplicate key",
			mutate: func(manifest, _, _ map[string]any) {
				manifest["proposalKeys"].(map[string]any)["date-movie-adjacent"] = []any{"movie:tmdb:11001", "movie:tmdb:11001"}
			},
		},
		{
			name: "key outside resolved fixture",
			mutate: func(manifest, base, _ map[string]any) {
				caseByID(base, "date-movie-adjacent")["fixtureCase"] = "date-title-year-none"
				manifest["proposalKeys"].(map[string]any)["date-movie-adjacent"] = []any{"movie:tmdb:11001"}
			},
		},
		{
			name: "malformed fixture candidate",
			mutate: func(_, _, fixture map[string]any) {
				candidateByCase(fixture, "date-movie-adjacent")["tmdbId"] = float64(0)
			},
		},
		{
			name: "terminal conflicts with scope",
			mutate: func(manifest, _, _ map[string]any) {
				manifest["proposalTerminals"].(map[string]any)["date-movie-adjacent"] = "constraints_conflict"
			},
		},
		{
			name: "terminal conflicts with keys",
			mutate: func(manifest, _, _ map[string]any) {
				delete(manifest["dateScopes"].(map[string]any), "date-movie-adjacent")
				manifest["proposalTerminals"].(map[string]any)["date-movie-adjacent"] = "constraints_conflict"
			},
		},
		{
			name: "terminal requires abstaining family",
			mutate: func(manifest, _, _ map[string]any) {
				delete(manifest["dateScopes"].(map[string]any), "date-movie-adjacent")
				delete(manifest["proposalKeys"].(map[string]any), "date-movie-adjacent")
				manifest["proposalTerminals"].(map[string]any)["date-movie-adjacent"] = "constraints_conflict"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := certificationCases(v8MetadataFS(t, test.mutate))
			if err == nil {
				t.Fatal("certificationCases() error = nil, want validation failure")
			}
		})
	}
}

func TestV8MetadataResolvesFamilyFixtureIndependently(t *testing.T) {
	files := v8MetadataFS(t, func(manifest, base, _ map[string]any) {
		caseByID(base, "date-movie-adjacent")["fixtureCase"] = "date-title-year-none"
		manifest["proposalKeys"].(map[string]any)["date-movie-adjacent"] = []any{"movie:tmdb:10001"}
	})
	cases, err := certificationCases(files)
	if err != nil {
		t.Fatal(err)
	}
	caseValue := certificationCaseByName(t, cases, "date-movie-adjacent")
	if caseValue.ExpectedDateScope == nil || len(caseValue.ExpectedProposalKeys) != 1 || caseValue.ExpectedProposalKeys[0] != provision.Key("movie:tmdb:10001") {
		t.Fatalf("resolved case = %+v, want date scope and resolved fixture key", caseValue)
	}
}

func TestV8CaseOwnership(t *testing.T) {
	files := v8MetadataFS(t, func(manifest, _, _ map[string]any) {
		expectations := manifest["scheduleExpectations"].(map[string]any)
		expectations["date-movie-disjoint"].(map[string]any)["requiredSequence"] = []any{"movie:tmdb:11003", "movie:tmdb:11004"}
	})
	cases, err := certificationCases(files)
	if err != nil {
		t.Fatal(err)
	}
	base := certificationCaseByName(t, cases, "date-movie-adjacent")
	variant := certificationCaseByName(t, cases, "date-movie-adjacent--direct")
	terminal := certificationCaseByName(t, cases, "date-same-axis-conflict")
	if base.ExpectedDateScope == nil || variant.ExpectedDateScope == nil || terminal.ExpectedProposalTerminal == "" {
		t.Fatalf("v8 metadata did not project expected fields: base=%+v variant=%+v terminal=%+v", base, variant, terminal)
	}
	base.ExpectedDateScope.MovieRelease[0].From = 2001
	base.ExpectedProposalKeys[0] = "movie:tmdb:99999"
	terminal.ExpectedProposalTerminal = ""
	if variant.ExpectedDateScope.MovieRelease[0].From != 1990 || variant.ExpectedProposalKeys[0] != "movie:tmdb:11001" {
		t.Fatalf("variant shared base metadata: %+v", variant)
	}
	fresh, err := certificationCases(files)
	if err != nil {
		t.Fatal(err)
	}
	freshBase := certificationCaseByName(t, fresh, "date-movie-adjacent")
	freshTerminal := certificationCaseByName(t, fresh, "date-same-axis-conflict")
	if freshBase.ExpectedDateScope.MovieRelease[0].From != 1990 || freshBase.ExpectedProposalKeys[0] != "movie:tmdb:11001" || freshTerminal.ExpectedProposalTerminal == "" {
		t.Fatalf("fresh projection retained caller mutation: base=%+v terminal=%+v", freshBase, freshTerminal)
	}
	legacy := certificationCaseByName(t, fresh, "ambiguous-one-word")
	if legacy.ExpectedDateScope != nil || legacy.ExpectedProposalAbstention {
		t.Fatalf("legacy family behavior changed: %+v", legacy)
	}

	scheduled := certificationCaseByName(t, cases, "date-movie-disjoint")
	scheduledVariant := certificationCaseByName(t, cases, "date-movie-disjoint--direct")
	scheduled.RequireScheduledPrograms[0] = "mutated-required"
	scheduled.ForbidScheduledPrograms[0] = "mutated-forbidden"
	scheduled.RequireScheduledSequence[0] = "mutated-sequence"
	if scheduledVariant.RequireScheduledPrograms[0] != "movie:tmdb:11003" ||
		scheduledVariant.ForbidScheduledPrograms[0] != "movie:tmdb:11005" ||
		scheduledVariant.RequireScheduledSequence[0] != "movie:tmdb:11003" {
		t.Fatalf("variant shared scheduled metadata: %+v", scheduledVariant)
	}
	freshScheduled := certificationCaseByName(t, fresh, "date-movie-disjoint")
	if freshScheduled.RequireScheduledPrograms[0] != "movie:tmdb:11003" ||
		freshScheduled.ForbidScheduledPrograms[0] != "movie:tmdb:11005" ||
		freshScheduled.RequireScheduledSequence[0] != "movie:tmdb:11003" {
		t.Fatalf("fresh projection retained scheduled metadata mutation: %+v", freshScheduled)
	}
}

func TestV8MustIncludeOwnershipAndVariantOverride(t *testing.T) {
	for _, tc := range []struct {
		name     string
		override []any
		want     []string
	}{
		{"inherited", nil, []string{"Synthetic Repair Date (1990)"}},
		{"replaced", []any{"variant include"}, []string{"variant include"}},
		{"cleared", []any{}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := v8MetadataFS(t, func(_, base, _ map[string]any) {
				if tc.override != nil {
					family := caseByID(base, "date-repair")
					family["variants"].([]any)[0].(map[string]any)["mustInclude"] = tc.override
				}
			})
			cases, err := certificationCases(files)
			if err != nil {
				t.Fatal(err)
			}
			base := certificationCaseByName(t, cases, "date-repair")
			variant := certificationCaseByName(t, cases, "date-repair--direct")
			if !slices.Equal(variant.Intent.MustInclude, tc.want) {
				t.Fatalf("variant includes = %v, want %v", variant.Intent.MustInclude, tc.want)
			}
			base.Intent.MustInclude[0] = "mutated base"
			if !slices.Equal(variant.Intent.MustInclude, tc.want) {
				t.Fatal("base mutation changed variant includes")
			}
			if len(variant.Intent.MustInclude) > 0 {
				variant.Intent.MustInclude[0] = "mutated variant"
			}
			if base.Intent.MustInclude[0] != "mutated base" {
				t.Fatal("variant mutation changed base includes")
			}
			fresh, err := certificationCases(files)
			if err != nil {
				t.Fatal(err)
			}
			if got := certificationCaseByName(t, fresh, "date-repair").Intent.MustInclude; !slices.Equal(got, []string{"Synthetic Repair Date (1990)"}) {
				t.Fatalf("fresh base retained mutation: %v", got)
			}
			if got := certificationCaseByName(t, fresh, "date-repair--direct").Intent.MustInclude; !slices.Equal(got, tc.want) {
				t.Fatalf("fresh variant includes = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestV8MetadataRejectsScheduleAndPlayableAmbiguity(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(manifest, base, fixture map[string]any)
	}{
		{"sequence forbidden", func(manifest, _, _ map[string]any) {
			expectations := manifest["scheduleExpectations"].(map[string]any)
			expectations["date-movie-disjoint"].(map[string]any)["requiredSequence"] = []any{"movie:tmdb:11005"}
		}},
		{"mismatched library identity", func(_, _, fixture map[string]any) {
			playableByCase(fixture, "date-movie-disjoint")["libraryItemId"] = "fixture-wrong"
		}},
		{"unowned candidate", func(_, _, fixture map[string]any) {
			candidate := candidateByCase(fixture, "date-movie-disjoint")
			candidate["inLibrary"] = false
			delete(candidate, "libraryItemId")
		}},
		{"duplicate episode coordinate", func(_, _, fixture map[string]any) {
			episodes := playableByCase(fixture, "date-series-premiere-airing")["episodes"].([]any)
			episodes[1].(map[string]any)["season"] = episodes[0].(map[string]any)["season"]
			episodes[1].(map[string]any)["episode"] = episodes[0].(map[string]any)["episode"]
		}},
		{"duplicate episode library identity", func(_, _, fixture map[string]any) {
			episodes := playableByCase(fixture, "date-series-premiere-airing")["episodes"].([]any)
			episodes[1].(map[string]any)["libraryItemId"] = episodes[0].(map[string]any)["libraryItemId"]
		}},
		{"empty expectation", func(manifest, _, _ map[string]any) {
			delete(manifest["scheduleExpectations"].(map[string]any), "date-movie-disjoint")
			manifest["scheduleExpectations"].(map[string]any)["date-movie-disjoint"] = map[string]any{}
		}},
		{"abstaining family", func(manifest, _, _ map[string]any) {
			manifest["scheduleExpectations"].(map[string]any)["season-window-conflict"] = map[string]any{"requiredPrograms": []any{"movie:tmdb:10001"}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := certificationCases(v8MetadataFS(t, test.mutate)); err == nil {
				t.Fatal("certificationCases() error = nil, want metadata rejection")
			}
		})
	}
}

func v8MetadataFS(t *testing.T, mutate func(manifest, base, fixture map[string]any)) fs.FS {
	t.Helper()
	read := func(name string) []byte {
		blob, err := certificationFiles.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		return blob
	}
	var manifest, base, fixture map[string]any
	for _, item := range []struct {
		name   string
		target *map[string]any
	}{
		{certificationManifestPath, &manifest},
		{"testdata/planner-certification-v8-base.json", &base},
		{"testdata/planner-catalog-v3.json", &fixture},
	} {
		if err := json.Unmarshal(read(item.name), item.target); err != nil {
			t.Fatal(err)
		}
	}
	if mutate != nil {
		mutate(manifest, base, fixture)
	}
	fixtureBlob := marshalMetadata(t, fixture)
	base["fixture"].(map[string]any)["sha256"] = metadataSHA256(fixtureBlob)
	baseBlob := marshalMetadata(t, base)
	manifest["base"].(map[string]any)["sha256"] = metadataSHA256(baseBlob)
	return fstest.MapFS{
		certificationManifestPath:                     &fstest.MapFile{Data: marshalMetadata(t, manifest)},
		"testdata/planner-certification-v8-base.json": &fstest.MapFile{Data: baseBlob},
		"testdata/planner-catalog-v3.json":            &fstest.MapFile{Data: fixtureBlob},
	}
}

func marshalMetadata(t *testing.T, value any) []byte {
	t.Helper()
	blob, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return blob
}

func metadataSHA256(blob []byte) string {
	digest := sha256.Sum256(blob)
	return hex.EncodeToString(digest[:])
}

func caseByID(base map[string]any, id string) map[string]any {
	for _, raw := range base["cases"].([]any) {
		value := raw.(map[string]any)
		if value["id"] == id {
			return value
		}
	}
	panic("missing case " + id)
}

func candidateByCase(fixture map[string]any, id string) map[string]any {
	for _, raw := range fixture["cases"].([]any) {
		value := raw.(map[string]any)
		if value["id"] != id {
			continue
		}
		responses := value["responses"].([]any)
		return responses[0].(map[string]any)["candidates"].([]any)[0].(map[string]any)
	}
	panic("missing fixture case " + id)
}

func playableByCase(fixture map[string]any, id string) map[string]any {
	for _, raw := range fixture["cases"].([]any) {
		value := raw.(map[string]any)
		if value["id"] == id {
			return value["playables"].([]any)[0].(map[string]any)
		}
	}
	panic("missing playable fixture case " + id)
}

func certificationCaseByName(t *testing.T, cases []Case, name string) Case {
	t.Helper()
	for _, caseValue := range cases {
		if caseValue.Name == name {
			return caseValue
		}
	}
	t.Fatalf("missing projected case %q", name)
	return Case{}
}
