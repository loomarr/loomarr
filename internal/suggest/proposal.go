// Package suggest is the Suggester (design §8): it turns a channel intent into a
// grounded proposal (a lineup from the library + an acquisition list of missing
// titles). It owns the grounding loop — the LLM proposes candidates ONLY via the
// catalog tool (real ids), every proposal item is re-validated before display,
// and nothing is auto-executed (§8 human-in-the-loop). Generation runs as a
// persisted job (§8 execution model); this package is the pure suggestion logic,
// driven by a worker (Phase 11e) and exposed via the API (Phase 11f).
package suggest

import (
	"github.com/mantonx/loomarr/internal/catalog"
	"github.com/mantonx/loomarr/internal/provision"
)

// Intent is a channel request: an NL description plus optional constraints (§8).
type Intent struct {
	Description string   `json:"description"`                // "90s action movies"
	Era         string   `json:"era,omitempty"`              // e.g. "1990s"
	Tone        string   `json:"tone,omitempty"`             // e.g. "high-energy"
	RuntimeTgt  int      `json:"runtimeTargetMin,omitempty"` // total target minutes
	MustInclude []string `json:"mustInclude,omitempty"`      // titles/terms to include
	MustExclude []string `json:"mustExclude,omitempty"`      // titles/terms to exclude
	MaxAcquire  int      `json:"maxAcquisitions,omitempty"`  // cap on acquisitions (§8 quota)
}

// ProposalItem is one entry in a lineup or acquisition list (§8 output contract).
// Identity is always a real external id (the grounding guarantee); Name/Year are
// for display only. It mirrors a catalog.Candidate plus the LLM's rationale.
type ProposalItem struct {
	MediaType     provision.MediaType `json:"mediaType"`
	TMDBID        int                 `json:"tmdbId,omitempty"`
	TVDBID        int                 `json:"tvdbId,omitempty"`
	Name          string              `json:"name"`
	Year          int                 `json:"year,omitempty"`
	Seasons       []int               `json:"seasons,omitempty"` // series acquisitions
	InLibrary     bool                `json:"inLibrary"`
	LibraryItemID string              `json:"libraryItemId,omitempty"`
	Rationale     string              `json:"rationale,omitempty"` // why-it-fits (LLM)
	Confidence    float64             `json:"confidence,omitempty"`
}

// Key derives the provisioning key (§3), enforcing the grounding guarantee: a
// validated ProposalItem always has a usable id.
func (p ProposalItem) Key() (provision.Key, error) {
	return provision.Title{
		MediaType: p.MediaType, TMDBID: p.TMDBID, TVDBID: p.TVDBID,
		Name: p.Name, Year: p.Year, Seasons: p.Seasons,
	}.Key()
}

// fromCandidate builds a ProposalItem from a grounded catalog Candidate, carrying
// the rationale/confidence the model attached.
func fromCandidate(c catalog.Candidate, rationale string, confidence float64) ProposalItem {
	return ProposalItem{
		MediaType: c.MediaType, TMDBID: c.TMDBID, TVDBID: c.TVDBID,
		Name: c.Name, Year: c.Year, InLibrary: c.InLibrary, LibraryItemID: c.LibraryItemID,
		Rationale: rationale, Confidence: confidence,
	}
}

// Proposal is the suggester's output (§8): a lineup of library items, an
// acquisition list of missing titles, ranked alternates, deterministic scores,
// and a rationale. Approved acquisitions feed the provisioner; the approved
// lineup feeds the scheduler.
type Proposal struct {
	Intent       Intent         `json:"intent"`
	Lineup       []ProposalItem `json:"lineup"`       // in-library items, ordered
	Acquisitions []ProposalItem `json:"acquisitions"` // missing titles to acquire
	Alternates   []ProposalItem `json:"alternates"`   // ranked backups (§9 substitution)
	Scores       Scores         `json:"scores"`       // deterministic post-scoring
	Rationale    string         `json:"rationale,omitempty"`
}

// Scores is the deterministic post-scoring layered on the LLM output (§8) so
// ranking isn't pure vibes. Criteria are configurable; v1 ships three.
type Scores struct {
	ThemeFit          float64 `json:"themeFit"`          // how well items match the intent terms
	AvailabilityRatio float64 `json:"availabilityRatio"` // in-library / total (live-now readiness)
	EraBalance        float64 `json:"eraBalance"`        // spread across the target era/years
	Overall           float64 `json:"overall"`           // weighted composite
}
