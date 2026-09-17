//go:build eval

package eval

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/reference"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
)

//go:embed testdata/query-pilot-v1.json testdata/query-pilot-catalog-v1.json testdata/query-pilot-sources-v1.json testdata/query-expansion-v1.json testdata/query-expansion-catalog-v1.json testdata/query-expansion-sources-v1.json testdata/query-expansion-v2.json testdata/query-expansion-catalog-v2.json testdata/query-expansion-sources-v2.json testdata/query-expansion-v3.json testdata/query-expansion-catalog-v3.json testdata/query-expansion-v4.json testdata/query-expansion-catalog-v4.json testdata/query-expansion-v5.json testdata/query-expansion-catalog-v5.json testdata/query-expansion-v6.json testdata/query-mood-review-packet-v1.json testdata/query-mood-review-map-v1.json testdata/query-mood-review-submission-qwen-v1.json testdata/query-mood-review-submission-gemini-v1.json testdata/query-mood-review-submission-gemma-v1.json testdata/query-mood-review-authority-v1.json testdata/query-mood-review-openrouter-snapshot-v1.json
var queryPilotFiles embed.FS

// QueryPilotCorpus is exposed development evidence, never a release holdout.
type QueryPilotCorpus struct {
	SchemaVersion         int                    `json:"schemaVersion"`
	Version               string                 `json:"version"`
	Split                 string                 `json:"split"`
	PromptVersion         string                 `json:"promptVersion"`
	ToolSchemaVersion     string                 `json:"toolSchemaVersion"`
	SourceVersion         string                 `json:"sourceVersion"`
	CatalogFixture        CertificationFixture   `json:"catalogFixture"`
	SourcesFixture        CertificationFixture   `json:"sourcesFixture"`
	ExtraCatalogFixture   CertificationFixture   `json:"extraCatalogFixture"`
	MoodReviewPacket      CertificationFixture   `json:"moodReviewPacket,omitempty"`
	MoodReviewPrivateMap  CertificationFixture   `json:"moodReviewPrivateMap,omitempty"`
	MoodReviewSubmissions []CertificationFixture `json:"moodReviewSubmissions,omitempty"`
	MoodReviewAuthority   CertificationFixture   `json:"moodReviewAuthority,omitempty"`
	MoodReviewSnapshots   []CertificationFixture `json:"moodReviewSnapshots,omitempty"`
	SourceReviews         []QuerySourceReview    `json:"sourceReviews,omitempty"`
	Cases                 []QueryPilotCase       `json:"cases"`
}

type QueryPilotCase struct {
	ID                    string                  `json:"id"`
	Group                 string                  `json:"group"`
	Category              string                  `json:"category"`
	Type                  string                  `json:"type"`
	FixtureCase           string                  `json:"fixtureCase"`
	Description           string                  `json:"description"`
	RefineText            string                  `json:"refineText,omitempty"`
	CurrentLineup         []suggest.LineupContext `json:"currentLineup,omitempty"`
	MustInclude           []string                `json:"mustInclude,omitempty"`
	MustExclude           []string                `json:"mustExclude,omitempty"`
	MinGrounded           int                     `json:"minGrounded"`
	MinLineup             int                     `json:"minLineup,omitempty"`
	MinAcquisitions       int                     `json:"minAcquisitions,omitempty"`
	MinMovies             int                     `json:"minMovies,omitempty"`
	AllowedMediaTypes     []provision.MediaType   `json:"allowedMediaTypes,omitempty"`
	ExpectedToolOperation string                  `json:"expectedToolOperation,omitempty"`
	ExpectCeiling         string                  `json:"expectCeiling,omitempty"`
	ForbidRatingsAbove    string                  `json:"forbidRatingsAbove,omitempty"`
	AudienceAuthority     string                  `json:"audienceAuthority,omitempty"`
	ExpectAbstention      bool                    `json:"expectAbstention,omitempty"`
	AcceptableKeys        []provision.Key         `json:"acceptableKeys"`
	RequireKeys           []provision.Key         `json:"requireKeys,omitempty"`
	ForbidKeys            []provision.Key         `json:"forbidKeys,omitempty"`
	NoDates               bool                    `json:"noDates,omitempty"`
	Dates                 *schedule.DateScope     `json:"dates,omitempty"`
	SubjectiveReview      *QuerySubjectiveReview  `json:"subjectiveReview,omitempty"`
}

// QuerySubjectiveReview records exposed subjective development evidence
// separately from deterministic grounding and policy gates.
type QuerySubjectiveReview struct {
	Version         string          `json:"version"`
	Status          string          `json:"status"`
	Rubric          string          `json:"rubric"`
	PositiveKeys    []provision.Key `json:"positiveKeys"`
	NegativeKeys    []provision.Key `json:"negativeKeys"`
	AuthoritySHA256 string          `json:"authoritySha256,omitempty"`
	Rules           []QueryMoodRule `json:"rules,omitempty"`
}

type QueryMoodRule struct {
	Axis string `json:"axis"`
	Min  *int   `json:"min,omitempty"`
	Max  *int   `json:"max,omitempty"`
}

// LoadEmbeddedQueryPilotCorpus validates identity and facts before any provider
// can be constructed by a caller. The manifest is authored, not a paraphrase loop.
func LoadEmbeddedQueryPilotCorpus() (QueryPilotCorpus, error) {
	return loadQueryDevelopmentCorpus("testdata/query-pilot-v1.json")
}

func loadQueryDevelopmentCorpus(path string) (QueryPilotCorpus, error) {
	blob, err := queryPilotFiles.ReadFile(path)
	if err != nil {
		return QueryPilotCorpus{}, err
	}
	var corpus QueryPilotCorpus
	decoder := json.NewDecoder(bytes.NewReader(blob))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&corpus); err != nil {
		return QueryPilotCorpus{}, fmt.Errorf("query pilot manifest: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return QueryPilotCorpus{}, fmt.Errorf("query pilot manifest has trailing data")
	}
	if corpus.SchemaVersion != 1 || corpus.Version == "" || corpus.Split != "development" || len(corpus.Cases) == 0 {
		return QueryPilotCorpus{}, fmt.Errorf("query pilot development identity is incomplete")
	}
	if corpus.PromptVersion != suggest.PlannerPromptVersion || corpus.ToolSchemaVersion != suggest.PlannerToolSchemaVersion || corpus.SourceVersion != suggest.PlannerSourceVersion {
		return QueryPilotCorpus{}, fmt.Errorf("query pilot differs from the production planner contract")
	}
	for _, binding := range []CertificationFixture{corpus.CatalogFixture, corpus.SourcesFixture, corpus.ExtraCatalogFixture} {
		facts, err := readQueryPilotFacts(binding.Path)
		if err != nil {
			return QueryPilotCorpus{}, err
		}
		digest := sha256.Sum256(facts)
		if hex.EncodeToString(digest[:]) != binding.SHA256 {
			return QueryPilotCorpus{}, fmt.Errorf("query pilot fixture digest mismatch: %s", binding.Path)
		}
	}
	factsBlob, err := releaseGateFiles.ReadFile(corpus.CatalogFixture.Path)
	if err != nil {
		return QueryPilotCorpus{}, err
	}
	var facts certificationCatalogFixture
	if err := json.Unmarshal(factsBlob, &facts); err != nil {
		return QueryPilotCorpus{}, err
	}
	extraBlob, err := readQueryPilotFacts(corpus.ExtraCatalogFixture.Path)
	if err != nil {
		return QueryPilotCorpus{}, err
	}
	var extra certificationCatalogFixture
	if err := json.Unmarshal(extraBlob, &extra); err != nil {
		return QueryPilotCorpus{}, err
	}
	facts.Cases = append(facts.Cases, extra.Cases...)
	keysByFixture := make(map[string]map[provision.Key]bool)
	for _, fixture := range facts.Cases {
		keys, err := fixtureCandidateKeys(fixture)
		if err != nil {
			return QueryPilotCorpus{}, err
		}
		keysByFixture[fixture.ID] = keys
	}
	moodAuthority, err := loadQueryMoodReviewAuthority(corpus)
	if err != nil {
		return QueryPilotCorpus{}, err
	}
	seen := make(map[string]bool)
	descriptions := make(map[string]bool)
	for _, c := range corpus.Cases {
		if c.ID == "" || c.Group == "" || c.Category == "" || c.Description == "" || seen[c.ID] || descriptions[c.Description] {
			return QueryPilotCorpus{}, fmt.Errorf("query pilot has missing or duplicate case identity: %q", c.ID)
		}
		seen[c.ID], descriptions[c.Description] = true, true
		if c.Type != "minimum" && c.Type != "invariance" && c.Type != "directional" {
			return QueryPilotCorpus{}, fmt.Errorf("query pilot case %q has unsupported test type", c.ID)
		}
		if c.ExpectAbstention {
			if c.MinGrounded != 0 || c.MinLineup != 0 || c.MinAcquisitions != 0 || c.MinMovies != 0 || len(c.AcceptableKeys) != 0 || len(c.RequireKeys) != 0 {
				return QueryPilotCorpus{}, fmt.Errorf("query pilot case %q has nonempty expectations for an abstention", c.ID)
			}
		} else if c.MinGrounded < 1 || c.MinGrounded > len(c.AcceptableKeys) || c.MinLineup < 0 || c.MinAcquisitions < 0 || c.MinLineup+c.MinAcquisitions > len(c.AcceptableKeys) {
			return QueryPilotCorpus{}, fmt.Errorf("query pilot case %q has impossible breadth or ownership expectations", c.ID)
		}
		if c.NoDates == (c.Dates != nil) || (c.Dates != nil && !normalizedDateScope(c.Dates)) {
			return QueryPilotCorpus{}, fmt.Errorf("query pilot case %q must declare none or canonical date axes", c.ID)
		}
		if c.ExpectedToolOperation != "" && !slices.Contains([]string{"title", "genre", "keyword", "network", "cast", "creator", "people"}, c.ExpectedToolOperation) {
			return QueryPilotCorpus{}, fmt.Errorf("query pilot case %q has unsupported tool operation", c.ID)
		}
		seenMediaTypes := make(map[provision.MediaType]bool)
		for _, mediaType := range c.AllowedMediaTypes {
			if !mediaType.Valid() || seenMediaTypes[mediaType] {
				return QueryPilotCorpus{}, fmt.Errorf("query pilot case %q has invalid or duplicate allowed media type", c.ID)
			}
			seenMediaTypes[mediaType] = true
		}
		for _, ceiling := range []string{c.ExpectCeiling, c.ForbidRatingsAbove} {
			if ceiling != "" && schedule.NormalizeRating(ceiling) == "" {
				return QueryPilotCorpus{}, fmt.Errorf("query pilot case %q has an unsupported rating ceiling", c.ID)
			}
		}
		hasAudienceCeiling := c.ExpectCeiling != "" || c.ForbidRatingsAbove != ""
		if hasAudienceCeiling != (c.AudienceAuthority != "") || (c.AudienceAuthority != "" && !slices.Contains([]string{"synthetic", "US-MPA"}, c.AudienceAuthority)) {
			return QueryPilotCorpus{}, fmt.Errorf("query pilot case %q has an absent or unsupported audience authority", c.ID)
		}
		seenCurrent := make(map[string]bool)
		for _, item := range c.CurrentLineup {
			if _, _, _, ok := provision.ParseKey(provision.Key(item.Key)); !ok || seenCurrent[item.Key] {
				return QueryPilotCorpus{}, fmt.Errorf("query pilot case %q has invalid or duplicate current-lineup identity", c.ID)
			}
			seenCurrent[item.Key] = true
		}
		known := keysByFixture[c.FixtureCase]
		if known == nil {
			return QueryPilotCorpus{}, fmt.Errorf("query pilot case %q has no source fixture", c.ID)
		}
		allowed := make(map[provision.Key]bool)
		for _, key := range c.AcceptableKeys {
			if !known[key] || allowed[key] {
				return QueryPilotCorpus{}, fmt.Errorf("query pilot case %q has unknown or duplicate acceptable key %q", c.ID, key)
			}
			allowed[key] = true
		}
		for _, key := range c.RequireKeys {
			if !allowed[key] {
				return QueryPilotCorpus{}, fmt.Errorf("query pilot case %q requires an unacceptable key", c.ID)
			}
		}
		for _, key := range c.ForbidKeys {
			if !known[key] || allowed[key] {
				return QueryPilotCorpus{}, fmt.Errorf("query pilot case %q has absent or conflicting negative evidence", c.ID)
			}
		}
		if review := c.SubjectiveReview; review != nil {
			if review.Version != MoodReviewRubricVersion || strings.TrimSpace(review.Rubric) == "" || len(review.PositiveKeys) == 0 || len(review.NegativeKeys) == 0 {
				return QueryPilotCorpus{}, fmt.Errorf("query pilot case %q has incomplete subjective review protocol", c.ID)
			}
			for _, key := range review.PositiveKeys {
				if !allowed[key] {
					return QueryPilotCorpus{}, fmt.Errorf("query pilot case %q has a subjective positive outside its acceptable set", c.ID)
				}
			}
			for _, key := range review.NegativeKeys {
				if !slices.Contains(c.ForbidKeys, key) {
					return QueryPilotCorpus{}, fmt.Errorf("query pilot case %q has a subjective negative outside its forbidden set", c.ID)
				}
			}
			switch review.Status {
			case "rubric-authored-development":
				if review.AuthoritySHA256 != "" || len(review.Rules) != 0 {
					return QueryPilotCorpus{}, fmt.Errorf("query pilot case %q attaches authority to an authored-only rubric", c.ID)
				}
			case MoodReviewStatusModelAttested:
				if moodAuthority == nil || review.AuthoritySHA256 != corpus.MoodReviewAuthority.SHA256 {
					return QueryPilotCorpus{}, fmt.Errorf("query pilot case %q is not bound to the corpus mood authority", c.ID)
				}
				if err := validateQueryMoodProjection(*review, *moodAuthority); err != nil {
					return QueryPilotCorpus{}, fmt.Errorf("query pilot case %q: %w", c.ID, err)
				}
			default:
				return QueryPilotCorpus{}, fmt.Errorf("query pilot case %q has unsupported subjective review status", c.ID)
			}
		}
	}
	return corpus, nil
}

func readQueryPilotFacts(path string) ([]byte, error) {
	if blob, err := queryPilotFiles.ReadFile(path); err == nil {
		return blob, nil
	}
	return releaseGateFiles.ReadFile(path)
}

func loadQueryMoodReviewAuthority(corpus QueryPilotCorpus) (*MoodReviewAuthority, error) {
	bindings := []CertificationFixture{corpus.MoodReviewPacket, corpus.MoodReviewPrivateMap, corpus.MoodReviewAuthority}
	configured := false
	for _, binding := range bindings {
		configured = configured || binding.Path != "" || binding.SHA256 != ""
	}
	configured = configured || len(corpus.MoodReviewSubmissions) != 0 || len(corpus.MoodReviewSnapshots) != 0
	if !configured {
		return nil, nil
	}
	if len(corpus.MoodReviewSubmissions) < 2 || len(corpus.MoodReviewSubmissions) > 3 {
		return nil, fmt.Errorf("query mood authority requires two submissions and at most one adjudicator")
	}
	packet, err := readBoundQueryMoodArtifact(corpus.MoodReviewPacket)
	if err != nil {
		return nil, err
	}
	privateMap, err := readBoundQueryMoodArtifact(corpus.MoodReviewPrivateMap)
	if err != nil {
		return nil, err
	}
	snapshotBlobs := make(map[string][]byte)
	for _, binding := range corpus.MoodReviewSnapshots {
		blob, err := readBoundQueryMoodArtifact(binding)
		if err != nil {
			return nil, err
		}
		snapshotBlobs[binding.SHA256] = blob
	}
	submissionBlobs := make([][]byte, 0, len(corpus.MoodReviewSubmissions))
	for _, binding := range corpus.MoodReviewSubmissions {
		blob, err := readBoundQueryMoodArtifact(binding)
		if err != nil {
			return nil, err
		}
		var submission MoodReviewSubmission
		if err := decodeMoodReviewJSON(blob, &submission); err != nil {
			return nil, fmt.Errorf("decode query mood submission: %w", err)
		}
		if submission.Reviewer.SnapshotSHA256 != "" {
			snapshotBlob := snapshotBlobs[submission.Reviewer.SnapshotSHA256]
			if snapshotBlob == nil {
				return nil, fmt.Errorf("query mood submission lacks its bound provider snapshot")
			}
			var snapshot fillerbakeoff.OpenRouterSnapshot
			if err := decodeMoodReviewJSON(snapshotBlob, &snapshot); err != nil {
				return nil, fmt.Errorf("decode query mood provider snapshot: %w", err)
			}
			_, capabilityDigest, err := fillerbakeoff.OpenRouterAssessorIdentity(snapshot, submission.Reviewer.Model, submission.Reviewer.Route, submission.Reviewer.RouteSlug, "disabled")
			if err != nil || capabilityDigest != submission.Reviewer.IdentitySHA256 {
				return nil, fmt.Errorf("query mood submission does not match its provider route snapshot")
			}
		}
		submissionBlobs = append(submissionBlobs, blob)
	}
	locked, err := CompileMoodReviewAuthority(packet, privateMap, submissionBlobs...)
	if err != nil {
		return nil, fmt.Errorf("compile query mood authority: %w", err)
	}
	authorityBlob, err := readBoundQueryMoodArtifact(corpus.MoodReviewAuthority)
	if err != nil {
		return nil, err
	}
	var stored MoodReviewAuthority
	if err := decodeMoodReviewJSON(authorityBlob, &stored); err != nil {
		return nil, fmt.Errorf("decode query mood authority: %w", err)
	}
	if !reflect.DeepEqual(stored, locked) || stored.Status != MoodReviewStatusModelAttested || !slices.Contains([]string{MoodReviewCompletenessComplete, MoodReviewCompletenessPartial}, stored.Completeness) {
		return nil, fmt.Errorf("query mood authority does not match its locked packet and submissions")
	}
	return &stored, nil
}

func readBoundQueryMoodArtifact(binding CertificationFixture) ([]byte, error) {
	if binding.Path == "" || !moodReviewSHA(binding.SHA256) {
		return nil, fmt.Errorf("query mood artifact binding is incomplete")
	}
	blob, err := queryPilotFiles.ReadFile(binding.Path)
	if err != nil {
		return nil, err
	}
	if moodReviewSHA256(blob) != binding.SHA256 {
		return nil, fmt.Errorf("query mood artifact digest mismatch: %s", binding.Path)
	}
	return blob, nil
}

func validateQueryMoodProjection(review QuerySubjectiveReview, authority MoodReviewAuthority) error {
	if len(review.Rules) == 0 || len(review.Rules) > len(moodAxisNames) {
		return fmt.Errorf("subjective projection has no bounded ordinal rules")
	}
	seenAxes := make(map[string]bool)
	for _, rule := range review.Rules {
		if !slices.Contains(moodAxisNames, rule.Axis) || seenAxes[rule.Axis] || (rule.Min == nil && rule.Max == nil) || (rule.Min != nil && (*rule.Min < 0 || *rule.Min > 3)) || (rule.Max != nil && (*rule.Max < 0 || *rule.Max > 3)) || (rule.Min != nil && rule.Max != nil && *rule.Min > *rule.Max) {
			return fmt.Errorf("subjective projection has an invalid rule for %q", rule.Axis)
		}
		seenAxes[rule.Axis] = true
	}
	decisions := make(map[provision.Key]MoodReviewDecision)
	for _, decision := range authority.Decisions {
		decisions[decision.Key] = decision
	}
	for _, key := range review.PositiveKeys {
		decision, ok := decisions[key]
		if !ok {
			return fmt.Errorf("subjective positive %q lacks an authority decision", key)
		}
		matches, err := queryMoodDecisionMatches(decision, review.Rules)
		if err != nil {
			return err
		}
		if !matches {
			return fmt.Errorf("subjective positive %q fails its authority rules", key)
		}
	}
	for _, key := range review.NegativeKeys {
		decision, ok := decisions[key]
		if !ok {
			return fmt.Errorf("subjective negative %q lacks an authority decision", key)
		}
		matches, err := queryMoodDecisionMatches(decision, review.Rules)
		if err != nil {
			return err
		}
		if matches {
			return fmt.Errorf("subjective negative %q satisfies every authority rule", key)
		}
	}
	return nil
}

func queryMoodDecisionMatches(decision MoodReviewDecision, rules []QueryMoodRule) (bool, error) {
	for _, rule := range rules {
		if slices.Contains(decision.UncertainAxes, rule.Axis) {
			return false, fmt.Errorf("subjective projection depends on uncertain axis %q for %q", rule.Axis, decision.Key)
		}
		value, ok := moodScoreForAxis(decision.Scores, rule.Axis)
		if !ok {
			return false, fmt.Errorf("subjective projection uses unknown axis %q", rule.Axis)
		}
		if (rule.Min != nil && value < *rule.Min) || (rule.Max != nil && value > *rule.Max) {
			return false, nil
		}
	}
	return true, nil
}

func moodScoreForAxis(scores MoodAxisScores, axis string) (int, bool) {
	switch axis {
	case "valence":
		return scores.Valence, true
	case "arousal":
		return scores.Arousal, true
	case "threatFear":
		return scores.ThreatFear, true
	case "comedicWarmth":
		return scores.ComedicWarmth, true
	case "attentionalDemand":
		return scores.AttentionalDemand, true
	default:
		return 0, false
	}
}

// NewEmbeddedQueryPilotGenerator reuses the production Suggester, observed
// provider and shared Catalog adapters. Only the caller's provider can vary.
func NewEmbeddedQueryPilotGenerator(provider llm.Provider) (Generator, Observer, error) {
	corpus, err := LoadEmbeddedQueryPilotCorpus()
	if err != nil {
		return nil, nil, err
	}
	return newQueryDevelopmentGenerator(corpus, provider)
}

func newQueryDevelopmentGenerator(corpus QueryPilotCorpus, provider llm.Provider) (Generator, Observer, error) {
	var decoded certificationCatalogFixture
	for _, binding := range []CertificationFixture{corpus.CatalogFixture, corpus.ExtraCatalogFixture} {
		blob, err := readQueryPilotFacts(binding.Path)
		if err != nil {
			return nil, nil, err
		}
		var facts certificationCatalogFixture
		if err := json.Unmarshal(blob, &facts); err != nil {
			return nil, nil, err
		}
		decoded.Cases = append(decoded.Cases, facts.Cases...)
	}
	decoded.SchemaVersion, decoded.FixtureID = 1, corpus.Version
	fixture := newEmbeddedCatalogFixture(decoded)
	observed := &observedProvider{inner: provider}
	cat := catalog.New(embeddedLibraryCatalog{fixture}, embeddedTMDBCatalog{fixture}).WithPresence(fixture)
	sourceBlob, err := readQueryPilotFacts(corpus.SourcesFixture.Path)
	if err != nil {
		return nil, nil, err
	}
	references := &testkit.ReferenceResolver{ByURL: make(map[string]reference.Evidence)}
	if err := json.Unmarshal(sourceBlob, &references.ByLabel); err != nil {
		return nil, nil, err
	}
	// The immutable pilot predates stable acronym-subject extraction and stores
	// its synthetic source under the qualified label. Keep those bytes runnable
	// while production discovery uses the canonical acronym.
	if legacy, found := references.ByLabel["MCU Phase One"]; found {
		if _, current := references.ByLabel["MCU"]; !current {
			references.ByLabel["MCU"] = legacy
		}
	}
	bindings := make(map[string]string, len(corpus.Cases))
	for _, c := range corpus.Cases {
		bindings[c.Description] = c.FixtureCase
	}
	inner := suggest.New(observed, cat, fixture, 10).WithRatings(fixture).WithReferences(references)
	generator := &releaseGateGenerator{inner: inner, fixture: fixture, caseByIntent: bindings}
	return generator, &releaseGateObserver{provider: observed, fixture: fixture, generator: generator}, nil
}

func QueryPilotCases() ([]Case, error) {
	corpus, err := LoadEmbeddedQueryPilotCorpus()
	if err != nil {
		return nil, err
	}
	return queryDevelopmentCases(corpus), nil
}

func queryDevelopmentCases(corpus QueryPilotCorpus) []Case {
	cases := make([]Case, 0, len(corpus.Cases))
	for _, authored := range corpus.Cases {
		dates := cloneDateScope(authored.Dates)
		if authored.NoDates {
			dates = &schedule.DateScope{}
		}
		if authored.ExpectAbstention {
			// An explicit empty result has no Proposal on which to observe the
			// canonical policy. The authored request still retains its date and
			// audience constraints for production parsing and filtering.
			dates = nil
		}
		cases = append(cases, Case{
			Name: authored.ID, Intent: Intent{Description: authored.Description, RefineText: authored.RefineText, CurrentLineup: authored.CurrentLineup, MustInclude: authored.MustInclude, MustExclude: authored.MustExclude},
			NoFabrication: true, RequireUniqueKeys: true, OnlyAcceptableKeys: true, ExpectGroundedCompletion: !authored.ExpectAbstention,
			MinGrounded: authored.MinGrounded, MinLineup: authored.MinLineup, MinAcquisitions: authored.MinAcquisitions,
			MinMovies: authored.MinMovies, AllowedMediaTypes: authored.AllowedMediaTypes, ExpectedToolOperation: authored.ExpectedToolOperation,
			ExpectCeiling: authored.ExpectCeiling, ForbidRatingsAbove: authored.ForbidRatingsAbove,
			AcceptableKeys: authored.AcceptableKeys, MinAcceptableKeys: authored.MinGrounded,
			RequireKeys: authored.RequireKeys, ForbidKeys: authored.ForbidKeys, ExpectedDateScope: dates,
			ExpectedProposalAbstention: authored.ExpectAbstention,
		})
	}
	return withProductionStructuralBounds(cases)
}

// QueryPilotRunnerConfig makes development status explicit and retains strict
// policy/schema/completion assessment without modifying any certification bar.
func QueryPilotRunnerConfig(config RunnerConfig) (RunnerConfig, error) {
	return queryDevelopmentRunnerConfig("testdata/query-pilot-v1.json", "query-pilot-scorer-v1", config)
}

func queryDevelopmentRunnerConfig(path, scorer string, config RunnerConfig) (RunnerConfig, error) {
	corpus, err := loadQueryDevelopmentCorpus(path)
	if err != nil {
		return RunnerConfig{}, err
	}
	config.DevelopmentCorpus = true
	manifest, err := queryPilotFiles.ReadFile(path)
	if err != nil {
		return RunnerConfig{}, err
	}
	manifestDigest := sha256.Sum256(manifest)
	config.Contract = &CertificationContract{
		CorpusManifestSHA256:             hex.EncodeToString(manifestDigest[:]),
		SourcesFixtureSHA256:             corpus.SourcesFixture.SHA256,
		SupplementalCatalogFixtureSHA256: corpus.ExtraCatalogFixture.SHA256,
		SourceVersion:                    corpus.SourceVersion,
		CorpusVersion:                    corpus.Version, CatalogFixtureSHA256: corpus.CatalogFixture.SHA256,
		PromptVersion: corpus.PromptVersion, ToolSchemaVersion: corpus.ToolSchemaVersion, ScorerVersion: scorer,
		HardMetrics:    []string{"grounding", "correct_tool_operation", "required_anchors", "forbidden_matches", "distinct_breadth", "ownership"},
		QualityMetrics: []string{"policy_accuracy"},
		Thresholds: CertificationThresholds{
			MinGroundedCompletionRate: 1, MinCorrectToolOperationRate: 1,
			MinSchemaValidityRate: 1, MinPolicyAccuracyRate: 1,
			MaxP95ToolCalls: suggest.ProductionBounds().MaxToolCalls,
		},
	}
	return config, nil
}
