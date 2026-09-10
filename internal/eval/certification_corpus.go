//go:build eval

package eval

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/suggest"
)

const certificationManifestPath = "testdata/planner-certification-v8.json"

//go:embed testdata/planner-certification-v1.json testdata/planner-certification-v2.json testdata/planner-certification-v3.json testdata/planner-certification-v4.json testdata/planner-certification-v5.json testdata/planner-certification-v6.json testdata/planner-certification-v7.json testdata/planner-certification-v8.json testdata/planner-certification-v6-base.json testdata/planner-certification-v8-base.json testdata/planner-catalog-v1.json testdata/planner-catalog-v2.json testdata/planner-catalog-v3.json
var certificationFiles embed.FS

// CertificationCorpus is the immutable, held-out planner-model corpus contract.
// It names the exact fixture bytes used by every candidate model.
type CertificationCorpus struct {
	SchemaVersion         int                     `json:"schemaVersion"`
	Version               string                  `json:"version"`
	Split                 string                  `json:"split"`
	PromptVersion         string                  `json:"promptVersion"`
	ToolSchemaVersion     string                  `json:"toolSchemaVersion"`
	ScorerVersion         string                  `json:"scorerVersion"`
	HardMetrics           []string                `json:"hardMetrics"`
	QualityMetrics        []string                `json:"qualityMetrics"`
	Thresholds            CertificationThresholds `json:"thresholds"`
	Selection             CertificationSelection  `json:"selection"`
	AllowedTrainingSplits []string                `json:"allowedTrainingSplits"`
	Fixture               CertificationFixture    `json:"fixture"`
	Cases                 []CertificationCase     `json:"cases"`
}

// CertificationRunnerConfig binds Runner output to the exact corpus, fixture,
// prompt/tool contract, scorer, and pre-registered metric sets.
func CertificationRunnerConfig(config RunnerConfig) (RunnerConfig, error) {
	corpus, err := LoadEmbeddedCertificationCorpus()
	if err != nil {
		return RunnerConfig{}, err
	}
	if strings.TrimSpace(config.Generator.Provider) == "" || buildScorecardRunSnapshot(Scorecard{
		CorpusVersion: corpus.Version, GeneratedAt: time.Unix(1, 0).UTC(),
		Profile: config.Profile, Generator: config.Generator,
	}, false) == nil {
		return RunnerConfig{}, fmt.Errorf("certification scorecard requires identifier-shaped profile and generator provider/model")
	}
	config.Contract = &CertificationContract{
		CorpusVersion:        corpus.Version,
		CatalogFixtureSHA256: corpus.Fixture.SHA256,
		PromptVersion:        corpus.PromptVersion,
		ToolSchemaVersion:    corpus.ToolSchemaVersion,
		ScorerVersion:        corpus.ScorerVersion,
		HardMetrics:          append([]string(nil), corpus.HardMetrics...),
		QualityMetrics:       append([]string(nil), corpus.QualityMetrics...),
		Thresholds:           corpus.Thresholds,
		Selection:            corpus.Selection,
	}
	return config, nil
}

type CertificationFixture struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type certificationCorpusExtension struct {
	SchemaVersion        int                                         `json:"schemaVersion"`
	Version              string                                      `json:"version"`
	Base                 CertificationFixture                        `json:"base"`
	PromptVersion        string                                      `json:"promptVersion"`
	ToolSchemaVersion    string                                      `json:"toolSchemaVersion"`
	ScorerVersion        string                                      `json:"scorerVersion"`
	QualityMetrics       []string                                    `json:"qualityMetrics"`
	Thresholds           CertificationThresholds                     `json:"thresholds"`
	Selection            CertificationSelection                      `json:"selection"`
	ProposalExpectation  string                                      `json:"proposalExpectation"`
	PolicyCeilings       map[string]string                           `json:"policyCeilings"`
	RecoveryCases        []string                                    `json:"recoveryCases"`
	RepairCases          []string                                    `json:"repairCases"`
	DateScopes           map[string]json.RawMessage                  `json:"dateScopes"`
	ProposalKeys         map[string][]provision.Key                  `json:"proposalKeys"`
	ProposalTerminals    map[string]string                           `json:"proposalTerminals"`
	ScheduleExpectations map[string]certificationScheduleExpectation `json:"scheduleExpectations"`
}

type CertificationCase struct {
	ID              string                 `json:"id"`
	Split           string                 `json:"split"`
	FixtureCase     string                 `json:"fixtureCase"`
	Axes            []string               `json:"axes"`
	Description     string                 `json:"description"`
	MustInclude     []string               `json:"mustInclude,omitempty"`
	AllowAbstention bool                   `json:"allowAbstention,omitempty"`
	Variants        []CertificationVariant `json:"variants,omitempty"`
}

type CertificationVariant struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	MustInclude []string `json:"mustInclude,omitempty"`
}

type certificationCatalogFixture struct {
	SchemaVersion int                        `json:"schemaVersion"`
	FixtureID     string                     `json:"fixtureId"`
	Cases         []certificationFixtureCase `json:"cases"`
}

type certificationFixtureCase struct {
	ID                 string                         `json:"id"`
	DateQuerySensitive bool                           `json:"dateQuerySensitive,omitempty"`
	Responses          []certificationFixtureResponse `json:"responses"`
	Playables          []certificationFixturePlayable `json:"playables"`
}

// certificationFixturePlayable is deliberately separate from a catalog
// candidate: these are the pinned facts needed to turn an owned title into a
// real scheduler input, not inferred catalog metadata.
type certificationFixturePlayable struct {
	Key           provision.Key                 `json:"key"`
	LibraryItemID string                        `json:"libraryItemId"`
	DurationMs    int64                         `json:"durationMs"`
	Episodes      []certificationFixtureEpisode `json:"episodes"`
}

type certificationFixtureEpisode struct {
	LibraryItemID string `json:"libraryItemId"`
	Title         string `json:"title"`
	DurationMs    int64  `json:"durationMs"`
	Season        int    `json:"season"`
	Episode       int    `json:"episode"`
	Year          int    `json:"year"`
}

type certificationScheduleExpectation struct {
	RequiredPrograms  []string `json:"requiredPrograms"`
	ForbiddenPrograms []string `json:"forbiddenPrograms"`
	RequiredSequence  []string `json:"requiredSequence"`
}

type certificationFixtureResponse struct {
	Operation  string              `json:"operation"`
	Candidates []catalog.Candidate `json:"candidates"`
	Error      string              `json:"error"`
}

type certificationCorpusMetadata struct {
	corpus    CertificationCorpus
	extension certificationCorpusExtension
	fixture   certificationCatalogFixture
}

// LoadEmbeddedCertificationCorpus verifies and returns the corpus manifest.
// A digest mismatch or missing fixture case fails before any provider is called.
func LoadEmbeddedCertificationCorpus() (CertificationCorpus, error) {
	metadata, err := loadCertificationCorpus(certificationFiles)
	if err != nil {
		return CertificationCorpus{}, err
	}
	return metadata.corpus, nil
}

func loadCertificationCorpus(files fs.FS) (certificationCorpusMetadata, error) {
	extensionBlob, err := fs.ReadFile(files, certificationManifestPath)
	if err != nil {
		return certificationCorpusMetadata{}, fmt.Errorf("read certification manifest: %w", err)
	}
	var extension certificationCorpusExtension
	if err := json.Unmarshal(extensionBlob, &extension); err != nil {
		return certificationCorpusMetadata{}, fmt.Errorf("decode certification manifest: %w", err)
	}
	manifestBlob, err := fs.ReadFile(files, extension.Base.Path)
	if err != nil {
		return certificationCorpusMetadata{}, fmt.Errorf("read certification base manifest: %w", err)
	}
	baseDigest := sha256.Sum256(manifestBlob)
	if hex.EncodeToString(baseDigest[:]) != extension.Base.SHA256 {
		return certificationCorpusMetadata{}, fmt.Errorf("certification base manifest digest mismatch")
	}
	var corpus CertificationCorpus
	if err := json.Unmarshal(manifestBlob, &corpus); err != nil {
		return certificationCorpusMetadata{}, fmt.Errorf("decode certification base manifest: %w", err)
	}
	if extension.PromptVersion != suggest.PlannerPromptVersion || extension.ToolSchemaVersion != suggest.PlannerToolSchemaVersion {
		return certificationCorpusMetadata{}, fmt.Errorf("certification prompt/tool identity differs from production Suggester")
	}
	corpus.SchemaVersion = extension.SchemaVersion
	corpus.Version = extension.Version
	corpus.PromptVersion = extension.PromptVersion
	corpus.ToolSchemaVersion = extension.ToolSchemaVersion
	corpus.ScorerVersion = extension.ScorerVersion
	corpus.QualityMetrics = append([]string(nil), extension.QualityMetrics...)
	corpus.Thresholds = extension.Thresholds
	corpus.Selection = extension.Selection
	fixtureBlob, err := fs.ReadFile(files, corpus.Fixture.Path)
	if err != nil {
		return certificationCorpusMetadata{}, fmt.Errorf("read certification fixture: %w", err)
	}
	digest := sha256.Sum256(fixtureBlob)
	if hex.EncodeToString(digest[:]) != corpus.Fixture.SHA256 {
		return certificationCorpusMetadata{}, fmt.Errorf("certification fixture digest mismatch")
	}
	var fixture certificationCatalogFixture
	if err := json.Unmarshal(fixtureBlob, &fixture); err != nil {
		return certificationCorpusMetadata{}, fmt.Errorf("decode certification fixture: %w", err)
	}
	fixtureCases := make(map[string]map[provision.Key]bool, len(fixture.Cases))
	for _, c := range fixture.Cases {
		keys, keyErr := fixtureCandidateKeys(c)
		if keyErr != nil {
			return certificationCorpusMetadata{}, fmt.Errorf("certification fixture case %q has invalid candidate: %w", c.ID, keyErr)
		}
		fixtureCases[c.ID] = keys
	}
	for _, c := range corpus.Cases {
		if _, ok := fixtureCases[c.FixtureCase]; !ok {
			return certificationCorpusMetadata{}, fmt.Errorf("certification case %q references missing fixture case %q", c.ID, c.FixtureCase)
		}
	}
	if err := validateCertificationExtension(extension, corpus, fixtureCases); err != nil {
		return certificationCorpusMetadata{}, err
	}
	if err := validateScheduleExpectations(extension, corpus, fixture); err != nil {
		return certificationCorpusMetadata{}, err
	}
	return certificationCorpusMetadata{corpus: corpus, extension: extension, fixture: fixture}, nil
}

func validateScheduleExpectations(extension certificationCorpusExtension, corpus CertificationCorpus, fixture certificationCatalogFixture) error {
	families := make(map[string]CertificationCase, len(corpus.Cases))
	for _, family := range corpus.Cases {
		families[family.ID] = family
	}
	byCase := make(map[string]certificationFixtureCase, len(fixture.Cases))
	for _, fixtureCase := range fixture.Cases {
		byCase[fixtureCase.ID] = fixtureCase
	}
	for familyID, expectation := range extension.ScheduleExpectations {
		family, ok := families[familyID]
		if !ok {
			return fmt.Errorf("certification schedule expectations reference unknown case %q", familyID)
		}
		if len(expectation.RequiredPrograms) == 0 && len(expectation.ForbiddenPrograms) == 0 && len(expectation.RequiredSequence) == 0 {
			return fmt.Errorf("certification schedule expectations for %q are empty", familyID)
		}
		if family.AllowAbstention {
			return fmt.Errorf("certification schedule expectations for abstaining case %q are not allowed", familyID)
		}
		titles, err := fixturePlayableTitles(byCase[family.FixtureCase])
		if err != nil {
			return fmt.Errorf("certification schedule fixture for %q: %w", familyID, err)
		}
		known := playableProgramIdentities(titles)
		seenRequired := make(map[string]bool, len(expectation.RequiredPrograms))
		for _, identity := range expectation.RequiredPrograms {
			if identity == "" || !known[identity] || seenRequired[identity] {
				return fmt.Errorf("certification schedule required program %q for %q is invalid, absent, or duplicated", identity, familyID)
			}
			seenRequired[identity] = true
		}
		forbidden := make(map[string]bool, len(expectation.ForbiddenPrograms))
		for _, identity := range expectation.ForbiddenPrograms {
			if identity == "" || !known[identity] || forbidden[identity] || seenRequired[identity] {
				return fmt.Errorf("certification schedule forbidden program %q for %q is invalid, absent, duplicated, or required", identity, familyID)
			}
			forbidden[identity] = true
		}
		for _, identity := range expectation.RequiredSequence {
			if identity == "" || !known[identity] || forbidden[identity] {
				return fmt.Errorf("certification schedule sequence program %q for %q is invalid or absent", identity, familyID)
			}
		}
	}
	return nil
}

func fixturePlayableTitles(fixture certificationFixtureCase) (map[provision.Key]FixtureTitle, error) {
	candidates, err := fixtureCandidateFacts(fixture)
	if err != nil {
		return nil, err
	}
	titles := make(map[provision.Key]FixtureTitle, len(fixture.Playables))
	for _, playable := range fixture.Playables {
		candidate, ok := candidates[playable.Key]
		if _, _, _, validKey := provision.ParseKey(playable.Key); !validKey || !ok || !candidate.InLibrary || candidate.LibraryItemID != playable.LibraryItemID || playable.LibraryItemID == "" {
			return nil, fmt.Errorf("playable %q has an unknown key or blank library identity", playable.Key)
		}
		if _, duplicate := titles[playable.Key]; duplicate {
			return nil, fmt.Errorf("playable %q is duplicated", playable.Key)
		}
		title := FixtureTitle{LibraryItemID: playable.LibraryItemID, DurationMs: playable.DurationMs}
		if playable.Key.IsSeries() {
			if len(playable.Episodes) == 0 {
				return nil, fmt.Errorf("series playable %q has no episodes", playable.Key)
			}
			for _, episode := range playable.Episodes {
				if episode.LibraryItemID == "" || episode.Title == "" || episode.DurationMs <= 0 || episode.Season <= 0 || episode.Episode <= 0 {
					return nil, fmt.Errorf("series playable %q has incomplete episode metadata", playable.Key)
				}
				for _, existing := range title.Episodes {
					if existing.Season == episode.Season && existing.Episode == episode.Episode || existing.LibraryItemID == episode.LibraryItemID {
						return nil, fmt.Errorf("series playable %q has duplicate episode identity", playable.Key)
					}
				}
				title.Episodes = append(title.Episodes, schedule.ResolvedProgram{LibraryItemID: episode.LibraryItemID, Title: episode.Title, DurationMs: episode.DurationMs, Season: episode.Season, Episode: episode.Episode, Year: episode.Year})
			}
		} else if playable.DurationMs <= 0 {
			return nil, fmt.Errorf("movie playable %q has no positive duration", playable.Key)
		}
		titles[playable.Key] = title
	}
	return titles, nil
}

func playableProgramIdentities(titles map[provision.Key]FixtureTitle) map[string]bool {
	known := make(map[string]bool)
	for key, title := range titles {
		if key.IsSeries() {
			for _, episode := range title.Episodes {
				known[fmt.Sprintf("%s:s%02de%02d", key, episode.Season, episode.Episode)] = true
			}
			continue
		}
		known[string(key)] = true
	}
	return known
}

func validateCertificationExtension(extension certificationCorpusExtension, corpus CertificationCorpus, fixtureCases map[string]map[provision.Key]bool) error {
	if extension.SchemaVersion <= 0 || extension.Version == "" || extension.PromptVersion == "" || extension.ToolSchemaVersion == "" || extension.ScorerVersion == "" {
		return fmt.Errorf("certification extension identity is incomplete")
	}
	if extension.ProposalExpectation != "exact_fixture_candidates_or_declared_abstention" {
		return fmt.Errorf("unsupported proposal expectation %q", extension.ProposalExpectation)
	}
	if len(extension.QualityMetrics) == 0 {
		return fmt.Errorf("certification extension quality metrics are empty")
	}
	if err := validateSelection(extension.Selection); err != nil {
		return fmt.Errorf("certification extension selection: %w", err)
	}
	known := make(map[string]bool, len(corpus.Cases))
	families := make(map[string]CertificationCase, len(corpus.Cases))
	for _, c := range corpus.Cases {
		known[c.ID] = true
		families[c.ID] = c
	}
	dateScopes, err := strictDateScopes(extension.DateScopes)
	if err != nil {
		return err
	}
	for id, scope := range dateScopes {
		if !known[id] {
			return fmt.Errorf("certification extension date scopes references unknown case %q", id)
		}
		if !normalizedDateScope(&scope) {
			return fmt.Errorf("certification extension date scope for %q is not canonical", id)
		}
	}
	for id, keys := range extension.ProposalKeys {
		if !known[id] {
			return fmt.Errorf("certification extension proposal keys references unknown case %q", id)
		}
		if len(keys) == 0 {
			return fmt.Errorf("certification extension proposal keys for %q are empty", id)
		}
		candidates := fixtureCases[knownFamilyFixture(corpus, id)]
		seen := make(map[provision.Key]bool, len(keys))
		for _, key := range keys {
			if _, _, _, ok := provision.ParseKey(key); !ok || seen[key] || !candidates[key] {
				return fmt.Errorf("certification extension proposal key %q for %q is invalid, duplicated, or absent from its fixture", key, id)
			}
			seen[key] = true
		}
	}
	for id, terminal := range extension.ProposalTerminals {
		if !known[id] {
			return fmt.Errorf("certification extension proposal terminal references unknown case %q", id)
		}
		if terminal != suggest.TerminalConstraintsConflict && terminal != suggest.TerminalDateSemanticsUnclear {
			return fmt.Errorf("certification extension proposal terminal for %q is unsupported", id)
		}
		if _, ok := dateScopes[id]; ok || len(extension.ProposalKeys[id]) != 0 || !families[id].AllowAbstention {
			return fmt.Errorf("certification extension proposal terminal for %q conflicts with date scope or proposal keys", id)
		}
	}
	validateIDs := func(label string, ids []string) error {
		seen := make(map[string]bool, len(ids))
		for _, id := range ids {
			if !known[id] {
				return fmt.Errorf("certification extension %s references unknown case %q", label, id)
			}
			if seen[id] {
				return fmt.Errorf("certification extension %s duplicates case %q", label, id)
			}
			seen[id] = true
		}
		return nil
	}
	policyIDs := make([]string, 0, len(extension.PolicyCeilings))
	for id := range extension.PolicyCeilings {
		policyIDs = append(policyIDs, id)
	}
	for label, ids := range map[string][]string{
		"policy ceilings": policyIDs, "recovery cases": extension.RecoveryCases, "repair cases": extension.RepairCases,
	} {
		if err := validateIDs(label, ids); err != nil {
			return err
		}
	}
	return nil
}

func knownFamilyFixture(corpus CertificationCorpus, id string) string {
	for _, c := range corpus.Cases {
		if c.ID == id {
			return c.FixtureCase
		}
	}
	return ""
}

func fixtureCandidateKeys(c certificationFixtureCase) (map[provision.Key]bool, error) {
	facts, err := fixtureCandidateFacts(c)
	if err != nil {
		return nil, err
	}
	keys := make(map[provision.Key]bool, len(facts))
	for key := range facts {
		keys[key] = true
	}
	return keys, nil
}

func fixtureCandidateFacts(c certificationFixtureCase) (map[provision.Key]catalog.Candidate, error) {
	keys := make(map[provision.Key]catalog.Candidate)
	for _, response := range c.Responses {
		for _, candidate := range response.Candidates {
			key, err := candidate.Key()
			if err != nil {
				return nil, err
			}
			if existing, ok := keys[key]; !ok || (!existing.InLibrary && candidate.InLibrary) {
				keys[key] = candidate
			}
		}
	}
	return keys, nil
}

func strictDateScopes(raw map[string]json.RawMessage) (map[string]schedule.DateScope, error) {
	result := make(map[string]schedule.DateScope, len(raw))
	for id, blob := range raw {
		if string(blob) == "null" {
			return nil, fmt.Errorf("certification extension date scope for %q is null", id)
		}
		var axes map[string]json.RawMessage
		if err := json.Unmarshal(blob, &axes); err != nil || axes == nil {
			return nil, fmt.Errorf("certification extension date scope for %q must be an object", id)
		}
		var scope schedule.DateScope
		for axis, rangesBlob := range axes {
			var target *[]schedule.Range
			switch axis {
			case "movieRelease":
				target = &scope.MovieRelease
			case "seriesPremiere":
				target = &scope.SeriesPremiere
			case "seriesAiring":
				target = &scope.SeriesAiring
			default:
				return nil, fmt.Errorf("certification extension date scope for %q has unknown axis %q", id, axis)
			}
			var ranges []json.RawMessage
			if err := json.Unmarshal(rangesBlob, &ranges); err != nil || len(ranges) == 0 || len(ranges) > 4 {
				return nil, fmt.Errorf("certification extension date scope for %q has invalid %s ranges", id, axis)
			}
			for _, rangeBlob := range ranges {
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(rangeBlob, &fields); err != nil || len(fields) != 2 || fields["from"] == nil || fields["to"] == nil {
					return nil, fmt.Errorf("certification extension date scope for %q has malformed %s range", id, axis)
				}
				var r schedule.Range
				if err := json.Unmarshal(rangeBlob, &r); err != nil || r.From < 1900 || r.To > 2099 || r.From > r.To {
					return nil, fmt.Errorf("certification extension date scope for %q has invalid %s range", id, axis)
				}
				*target = append(*target, r)
			}
		}
		if !normalizedDateScope(&scope) {
			return nil, fmt.Errorf("certification extension date scope for %q is not canonical", id)
		}
		result[id] = scope
	}
	return result, nil
}

// CertificationCases projects the frozen manifest onto Runner's public Case
// seam. Every case starts with the two production structural bounds and the
// unsupported-id hard gate; narrower expectations can deepen individual cases
// without creating a parallel evaluator.
func CertificationCases() ([]Case, error) {
	return certificationCases(certificationFiles)
}

func certificationCases(files fs.FS) ([]Case, error) {
	metadata, err := loadCertificationCorpus(files)
	if err != nil {
		return nil, err
	}
	corpus := metadata.corpus
	fixture := metadata.fixture
	extension := metadata.extension
	operationByCase := make(map[string]string, len(fixture.Cases))
	keysByCase := make(map[string][]provision.Key, len(fixture.Cases))
	for _, c := range fixture.Cases {
		if len(c.Responses) > 0 {
			operationByCase[c.ID] = c.Responses[0].Operation
		}
		for _, response := range c.Responses {
			for _, candidate := range response.Candidates {
				key, keyErr := candidate.Key()
				if keyErr != nil {
					return nil, fmt.Errorf("certification fixture case %q has invalid candidate: %w", c.ID, keyErr)
				}
				keysByCase[c.ID] = append(keysByCase[c.ID], key)
			}
		}
	}
	dateScopes, err := strictDateScopes(extension.DateScopes)
	if err != nil {
		return nil, err
	}
	recoveryCases := make(map[string]bool, len(extension.RecoveryCases))
	for _, id := range extension.RecoveryCases {
		recoveryCases[id] = true
	}
	repairCases := make(map[string]bool, len(extension.RepairCases))
	for _, id := range extension.RepairCases {
		repairCases[id] = true
	}
	cases := make([]Case, 0, len(corpus.Cases)*6)
	for _, frozen := range corpus.Cases {
		if frozen.Description == "" {
			return nil, fmt.Errorf("certification case %q has a blank Intent", frozen.ID)
		}
		keys := keysByCase[frozen.FixtureCase]
		if explicit, ok := extension.ProposalKeys[frozen.ID]; ok {
			keys = explicit
		}
		base := Case{
			Name:                       frozen.ID,
			Intent:                     Intent{Description: frozen.Description, MustInclude: append([]string(nil), frozen.MustInclude...)},
			NoFabrication:              true,
			ExpectGroundedCompletion:   !frozen.AllowAbstention,
			ExpectedToolOperation:      operationByCase[frozen.FixtureCase],
			ExpectedPolicyCeiling:      extension.PolicyCeilings[frozen.ID],
			ExpectedProposalKeys:       append([]provision.Key(nil), keys...),
			ExpectedProposalAbstention: frozen.AllowAbstention,
			RecoveryExpected:           recoveryCases[frozen.ID],
			TrackRepairRecovery:        repairCases[frozen.ID],
		}
		if scope, ok := dateScopes[frozen.ID]; ok {
			base.ExpectedDateScope = cloneDateScope(&scope)
		}
		if expectation, ok := extension.ScheduleExpectations[frozen.ID]; ok {
			base.RequireScheduledPrograms = append([]string(nil), expectation.RequiredPrograms...)
			base.ForbidScheduledPrograms = append([]string(nil), expectation.ForbiddenPrograms...)
			base.RequireScheduledSequence = append([]string(nil), expectation.RequiredSequence...)
		}
		base.ExpectedProposalTerminal = extension.ProposalTerminals[frozen.ID]
		if base.ExpectedProposalTerminal != "" {
			base.ExpectedToolOperation = "none"
		}
		if frozen.AllowAbstention {
			base.ExpectedProposalKeys = nil
			base.ExpectedProposalAbstention = base.ExpectedProposalTerminal == ""
		}
		cases = append(cases, base)
		for _, variant := range frozen.Variants {
			if variant.ID == "" || variant.Description == "" {
				return nil, fmt.Errorf("certification case %q has a blank variant", frozen.ID)
			}
			variantCase := base
			variantCase.ExpectedDateScope = cloneDateScope(base.ExpectedDateScope)
			variantCase.ExpectedProposalKeys = append([]provision.Key(nil), base.ExpectedProposalKeys...)
			variantCase.RequireScheduledPrograms = append([]string(nil), base.RequireScheduledPrograms...)
			variantCase.ForbidScheduledPrograms = append([]string(nil), base.ForbidScheduledPrograms...)
			variantCase.RequireScheduledSequence = append([]string(nil), base.RequireScheduledSequence...)
			variantCase.Name = frozen.ID + "--" + variant.ID
			variantCase.Intent.Description = variant.Description
			if variant.MustInclude != nil {
				variantCase.Intent.MustInclude = append([]string(nil), variant.MustInclude...)
			} else {
				variantCase.Intent.MustInclude = append([]string(nil), base.Intent.MustInclude...)
			}
			cases = append(cases, variantCase)
		}
	}
	return withProductionStructuralBounds(cases), nil
}

// NewFrozenCertificationScheduleMaterializer binds only the digest-validated
// synthetic playable metadata to the public Runner schedule seam. It never
// consults a provider or constructs a second scheduler.
func NewFrozenCertificationScheduleMaterializer() (ScheduleMaterializer, error) {
	metadata, err := loadCertificationCorpus(certificationFiles)
	if err != nil {
		return nil, err
	}
	cases, err := certificationCases(certificationFiles)
	if err != nil {
		return nil, err
	}
	fixtureByID := make(map[string]certificationFixtureCase, len(metadata.fixture.Cases))
	for _, fixture := range metadata.fixture.Cases {
		fixtureByID[fixture.ID] = fixture
	}
	familyByName := make(map[string]string, len(cases))
	for _, family := range metadata.corpus.Cases {
		familyByName[family.ID] = family.FixtureCase
		for _, variant := range family.Variants {
			familyByName[family.ID+"--"+variant.ID] = family.FixtureCase
		}
	}
	titlesByCase := make(map[string]map[provision.Key]FixtureTitle)
	for _, c := range cases {
		if !requiresSchedule(c) {
			continue
		}
		fixtureID, ok := familyByName[c.Name]
		if !ok {
			return nil, fmt.Errorf("certification schedule case %q has no declared fixture binding", c.Name)
		}
		titles, ok := titlesByCase[fixtureID]
		if !ok {
			var titleErr error
			titles, titleErr = fixturePlayableTitles(fixtureByID[fixtureID])
			if titleErr != nil {
				return nil, fmt.Errorf("certification schedule fixture %q: %w", fixtureID, titleErr)
			}
			titlesByCase[fixtureID] = titles
		}
		if len(titles) == 0 {
			return nil, fmt.Errorf("certification schedule case %q has no playable metadata", c.Name)
		}
	}
	return frozenCertificationScheduleMaterializer{titlesByCase: titlesByCase, familyByName: familyByName}, nil
}

type frozenCertificationScheduleMaterializer struct {
	titlesByCase map[string]map[provision.Key]FixtureTitle
	familyByName map[string]string
}

func (m frozenCertificationScheduleMaterializer) Materialize(ctx context.Context, c Case, proposal suggest.Proposal) ([]MaterializedProgram, error) {
	fixtureID, ok := m.familyByName[c.Name]
	if !ok {
		return nil, fmt.Errorf("certification schedule case %q has no declared fixture binding", c.Name)
	}
	titles, ok := m.titlesByCase[fixtureID]
	if !ok {
		return nil, fmt.Errorf("certification schedule case %q has no playable metadata", c.Name)
	}
	return NewFixtureScheduleMaterializer(titles).Materialize(ctx, c, proposal)
}

// CertificationFamilySmokeCases returns the canonical base Intent from every
// frozen semantic family. It is the bounded live-model adapter smoke set, not a
// release certification or a source of training examples.
func CertificationFamilySmokeCases() ([]Case, error) {
	cases, err := CertificationCases()
	if err != nil {
		return nil, err
	}
	smoke := make([]Case, 0, len(cases)/6)
	for _, c := range cases {
		if !strings.Contains(c.Name, "--") {
			smoke = append(smoke, c)
		}
	}
	return smoke, nil
}

type embeddedCertificationGenerator struct {
	inner        *suggest.Suggester
	fixture      *embeddedCatalogFixture
	caseByIntent map[string]string
}

func (g *embeddedCertificationGenerator) Suggest(ctx context.Context, intent suggest.Intent) (suggest.Proposal, error) {
	caseID, ok := g.caseByIntent[certificationIntentKey(intent.Description, intent.MustInclude)]
	if !ok {
		return suggest.Proposal{}, fmt.Errorf("Intent is absent from the embedded certification corpus")
	}
	g.fixture.selectCase(caseID)
	return g.inner.Suggest(ctx, intent)
}

// NewEmbeddedCertificationGenerator wires the production Suggester to the
// digest-pinned synthetic catalog. The provider is the only live boundary, so
// every candidate model observes identical tool results.
func NewEmbeddedCertificationGenerator(provider llm.Provider) (Generator, Observer, error) {
	return newEmbeddedCertificationGenerator(certificationFiles, provider)
}

func newEmbeddedCertificationGenerator(files fs.FS, provider llm.Provider) (Generator, Observer, error) {
	metadata, err := loadCertificationCorpus(files)
	if err != nil {
		return nil, nil, err
	}
	corpus := metadata.corpus
	fixtureBlob, err := fs.ReadFile(files, corpus.Fixture.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("read certification fixture: %w", err)
	}
	var decoded certificationCatalogFixture
	if err := json.Unmarshal(fixtureBlob, &decoded); err != nil {
		return nil, nil, fmt.Errorf("decode certification fixture: %w", err)
	}
	fixture := newEmbeddedCatalogFixture(decoded)
	observed := &observedProvider{inner: provider}
	cat := catalog.New(embeddedLibraryCatalog{fixture}, embeddedTMDBCatalog{fixture}).WithPresence(fixture)
	suggester := suggest.New(observed, cat, fixture, 10).WithRatings(fixture)
	caseByIntent := make(map[string]string, len(corpus.Cases))
	bind := func(description string, mustInclude []string, fixtureCase string) error {
		key := certificationIntentKey(description, mustInclude)
		if _, duplicate := caseByIntent[key]; duplicate {
			return fmt.Errorf("certification corpus has duplicate Intent binding for %q", description)
		}
		caseByIntent[key] = fixtureCase
		return nil
	}
	for _, c := range corpus.Cases {
		if err := bind(c.Description, c.MustInclude, c.FixtureCase); err != nil {
			return nil, nil, err
		}
		for _, variant := range c.Variants {
			mustInclude := c.MustInclude
			if variant.MustInclude != nil {
				mustInclude = variant.MustInclude
			}
			if err := bind(variant.Description, mustInclude, c.FixtureCase); err != nil {
				return nil, nil, err
			}
		}
	}
	return &embeddedCertificationGenerator{inner: suggester, fixture: fixture, caseByIntent: caseByIntent}, observed, nil
}

func certificationIntentKey(description string, mustInclude []string) string {
	if len(mustInclude) == 0 {
		mustInclude = nil
	}
	encoded, err := json.Marshal(struct {
		Description string   `json:"description"`
		MustInclude []string `json:"mustInclude"`
	}{Description: description, MustInclude: mustInclude})
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

type embeddedCatalogFixture struct {
	operations   int
	latencyNanos int64
	mu           sync.RWMutex
	current      string
	fixtureID    string
	cases        map[string]certificationFixtureCase
	byID         map[int]catalog.Candidate
	// Optional observation at the actual catalog boundary, after the Suggester
	// has validated and projected its model-facing date interpretation.
	onDiscover func(catalog.DiscoveryQuery)
}

func newEmbeddedCatalogFixture(decoded certificationCatalogFixture) *embeddedCatalogFixture {
	f := &embeddedCatalogFixture{
		fixtureID: decoded.FixtureID,
		cases:     make(map[string]certificationFixtureCase, len(decoded.Cases)),
		byID:      make(map[int]catalog.Candidate),
	}
	for _, c := range decoded.Cases {
		f.cases[c.ID] = c
		for _, response := range c.Responses {
			for _, candidate := range response.Candidates {
				f.byID[candidate.TMDBID] = candidate
			}
		}
	}
	return f
}

func (f *embeddedCatalogFixture) selectCase(id string) {
	f.mu.Lock()
	f.current = id
	f.mu.Unlock()
}

func (f *embeddedCatalogFixture) response(operation string) ([]catalog.Candidate, error) {
	started := time.Now()
	defer func() {
		f.mu.Lock()
		f.operations++
		f.latencyNanos += time.Since(started).Nanoseconds()
		f.mu.Unlock()
	}()
	f.mu.RLock()
	c := f.cases[f.current]
	f.mu.RUnlock()
	for _, response := range c.Responses {
		if response.Operation != operation {
			continue
		}
		if response.Error != "" {
			return nil, fmt.Errorf("%s", response.Error)
		}
		return append([]catalog.Candidate(nil), response.Candidates...), nil
	}
	return nil, nil
}

func (f *embeddedCatalogFixture) beginMetrics() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.operations = 0
	f.latencyNanos = 0
}

func (f *embeddedCatalogFixture) metrics() (int, int64) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.operations, f.latencyNanos
}

func (f *embeddedCatalogFixture) currentCase() certificationFixtureCase {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.cases[f.current]
}

func (f *embeddedCatalogFixture) Exists(_ context.Context, _ provision.MediaType, tmdbID int) (bool, error) {
	_, ok := f.byID[tmdbID]
	return ok, nil
}

func (f *embeddedCatalogFixture) ContentRating(_ context.Context, _ provision.MediaType, tmdbID int) (string, error) {
	return f.byID[tmdbID].OfficialRating, nil
}

func (f *embeddedCatalogFixture) Present(_ context.Context, _ provision.MediaType, tmdbID, _ int) (catalog.Presence, bool, error) {
	candidate, ok := f.byID[tmdbID]
	if !ok || !candidate.InLibrary {
		return catalog.Presence{}, false, nil
	}
	return catalog.Presence{LibraryItemID: candidate.LibraryItemID, OfficialRating: candidate.OfficialRating, Genres: candidate.Genres}, true, nil
}

type embeddedLibraryCatalog struct{ fixture *embeddedCatalogFixture }

func (c embeddedLibraryCatalog) Search(context.Context, string, int) ([]library.SearchResult, error) {
	candidates, err := c.fixture.response("title")
	if err != nil {
		return nil, err
	}
	results := make([]library.SearchResult, 0, len(candidates))
	for _, candidate := range candidates {
		if !candidate.InLibrary {
			continue
		}
		mediaType := library.Movie
		if candidate.MediaType == provision.Series {
			mediaType = library.Series
		}
		results = append(results, library.SearchResult{
			LibraryItemID: candidate.LibraryItemID, Name: candidate.Name, Year: candidate.Year,
			MediaType: mediaType, TMDBID: candidate.TMDBID, TVDBID: candidate.TVDBID,
			Genres: candidate.Genres, Overview: candidate.Overview, OfficialRating: candidate.OfficialRating,
		})
	}
	return results, nil
}

type embeddedTMDBCatalog struct{ fixture *embeddedCatalogFixture }

func (c embeddedTMDBCatalog) Search(context.Context, string, int) ([]catalog.Candidate, error) {
	return c.fixture.response("title")
}

func (c embeddedTMDBCatalog) Discover(_ context.Context, query catalog.DiscoveryQuery, _ int) ([]catalog.Candidate, error) {
	if c.fixture.onDiscover != nil {
		c.fixture.onDiscover(query)
	}
	var operation string
	switch {
	case query.Network != "":
		operation = "network"
	case len(query.Cast) > 0 && len(query.Creators) > 0:
		operation = "people"
	case len(query.Cast) > 0:
		operation = "cast"
	case len(query.Creators) > 0:
		operation = "creator"
	case len(query.Keywords) > 0:
		operation = "keyword"
	default:
		operation = "genre"
	}
	candidates, err := c.fixture.response(operation)
	fixtureCase := c.fixture.currentCase()
	if err != nil || !fixtureCase.DateQuerySensitive {
		return candidates, err
	}
	filtered := candidates[:0]
	for _, candidate := range candidates {
		if (query.MediaType == "" || candidate.MediaType == query.MediaType) &&
			(query.YearFrom == 0 || candidate.Year >= query.YearFrom) && (query.YearTo == 0 || candidate.Year <= query.YearTo) {
			filtered = append(filtered, candidate)
		}
	}
	return filtered, nil
}
