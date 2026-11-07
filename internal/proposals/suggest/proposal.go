// Package suggest is the Suggester (design §8): it turns a channel intent into a
// grounded proposal (a lineup from the library + an acquisition list of missing
// titles). It owns the grounding loop — the LLM proposes names or candidates,
// but only real ids returned by Catalog operations survive before display,
// and nothing is auto-executed (§8 human-in-the-loop). Generation runs as a
// persisted job (§8 execution model); this package is the pure suggestion logic,
// driven by a worker (Phase 11e) and exposed via the API (Phase 11f).
//
// The value types it produces and consumes (Intent, Proposal, ProposalItem, Scores,
// DecisionTrace) live in package proposals, so the store and every consumer share them
// without importing the suggester.
//
// Source ownership follows the generation pipeline: suggester.go owns the public
// facade and model loop, prompt.go owns prompt messages, tools.go owns catalog
// tool execution, proposal_grounding.go owns the trust boundary that constructs a
// Proposal, and intent_policy.go owns deterministic interpretation of user intent.
package suggest

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/proposals/webreference"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/store"
)

// Intent is one suggester run's view of a request: the persisted proposals.Intent plus the
// evidence that run gathers while it works. The evidence is execution-only. It never
// enters the API, a persisted proposal or a job's intent JSON, because only the embedded
// proposals.Intent is ever serialised.
type Intent struct {
	proposals.Intent
	// ReferenceTitles are bounded, source-resolved title anchors for this one
	// Suggest execution.
	ReferenceTitles     []string
	ReferenceResolved   bool
	referenceEvidence   webreference.Evidence
	referenceKeys       map[provision.Key]bool
	referenceCandidates []catalog.Candidate
	// membershipKeys contains only Catalog identities grounded from explicit user or
	// public-reference title anchors. A model's collection-tool roster is a search
	// hypothesis: catalog identity verifies that title exists, not that it belongs
	// to the named set. It is execution-only evidence, never model data.
	membershipKeys    map[provision.Key]bool
	membershipSources *membershipSourceState
	// requiredTitleKeys retains the independently resolved identity for each
	// explicit include. The title is only a lookup label; the bound Key is the
	// authority used for preservation and same-name rejection.
	requiredTitleKeys map[string]provision.Key
	// exampleFill is the era and neighbour pool derived from resolved example
	// anchors when the request states no era (#1499). Nil when not applicable.
	exampleFill     *exampleFill
	curatedTitleKey provision.Key
	curatedTitleSet bool
	// networkDiscoveryComplete is invocation-local evidence that a network-style
	// request actually retrieved a nonempty network-qualified Catalog pool.
	networkDiscoveryComplete bool
}

// ErrChannelIntentUnavailable reports a channel that carries no readable original request: its
// job was purged before channels owned their intent, or it was made by hand. A refine or a
// re-curation has nothing to seed from, and must say so rather than guess.
var ErrChannelIntentUnavailable = errors.New("suggest: the channel's original request is unavailable")

// IntentOfChannel reads the approved intent a channel stores, ready to seed a refine or a
// re-curation. It returns ErrChannelIntentUnavailable when the channel has none.
func IntentOfChannel(ch store.Channel) (proposals.Intent, error) {
	if ch.IntentJSON == "" {
		return proposals.Intent{}, ErrChannelIntentUnavailable
	}
	var intent proposals.Intent
	if err := json.Unmarshal([]byte(ch.IntentJSON), &intent); err != nil {
		return proposals.Intent{}, fmt.Errorf("%w: %v", ErrChannelIntentUnavailable, err)
	}
	return intent.ChannelIntent(), nil
}

// fromCandidate builds a ProposalItem from a grounded catalog Candidate, carrying
// the rationale/confidence the model attached.
func fromCandidate(c catalog.Candidate, rationale string, confidence float64) proposals.ProposalItem {
	return proposals.ProposalItem{
		MediaType: c.MediaType, TMDBID: c.TMDBID, TVDBID: c.TVDBID,
		Name: c.Name, Year: c.Year, InLibrary: c.InLibrary, LibraryItemID: c.LibraryItemID,
		Rationale: rationale, Confidence: confidence, Source: string(c.Source),
		Genres: c.Genres, Overview: c.Overview, OfficialRating: c.OfficialRating,
		OriginalLanguage: c.OriginalLanguage,
		OriginCountries:  append([]string(nil), c.OriginCountries...),
		RuntimeMinutes:   c.RuntimeMinutes,
		VoteAverage:      c.VoteAverage,
		VoteCount:        c.VoteCount,
		Keywords:         append([]string(nil), c.Keywords...),
		Networks:         append([]string(nil), c.Networks...),
		Cast:             append([]string(nil), c.Cast...),
		Creators:         append([]string(nil), c.Creators...),
	}.WithLibrarySize(c.SizeBytes)
}
