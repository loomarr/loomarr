// Package proposal holds the value types of a channel proposal: the request that
// asked for a channel (Intent), the grounded lineup the suggester answered with
// (Proposal, ProposalItem) and the evidence that travels with it (Scores,
// DecisionTrace).
//
// It is a leaf. The suggester builds these values, the store persists and decodes them,
// and the binder, API and re-curation read them; none of those packages needs to import the
// others to share a type. A proposal's JSON is decoded in one place, the store boundary,
// and everything above it passes the typed value.
package proposal

import (
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/storagegovernor"
)

// EditorialRole records the grounded origin of a selected pick in one proposal run.
// A missing role is unknown; ownership never means favorite or explicit preference.
type EditorialRole string

const (
	EditorialCore      EditorialRole = "core"
	EditorialAdjacent  EditorialRole = "adjacent"
	EditorialDiscovery EditorialRole = "discovery"
)

// ProposalItem is one entry in a lineup or acquisition list (§8 output contract).
// Identity is always a real external id (the grounding guarantee); Name/Year are
// for display only. It mirrors a catalog.Candidate plus the LLM's rationale.
type ProposalItem struct {
	EditorialRole EditorialRole       `json:"editorialRole,omitempty" enum:"core,adjacent,discovery"`
	MediaType     provision.MediaType `json:"mediaType"`
	TMDBID        int                 `json:"tmdbId,omitempty"`
	TVDBID        int                 `json:"tvdbId,omitempty"`
	Name          string              `json:"name"`
	Year          int                 `json:"year,omitempty"`
	Seasons       []int               `json:"seasons,omitempty"` // series ACQUISITION (what to download)
	// SeasonMin/SeasonMax: optional AIRING season window for a series pick (which
	// seasons play on the channel), distinct from Seasons (what to acquire). Set by
	// the grounded suggester when the intent implies an era ("classic" → 1–10);
	// carried onto LineupEntry.SeasonMin/Max and enforced at series expansion (§9).
	// 0 = unbounded on that end. Validated + clamped by the suggester before it lands.
	SeasonMin        int                       `json:"seasonMin,omitempty"`
	SeasonMax        int                       `json:"seasonMax,omitempty"`
	EpisodeSelection schedule.EpisodeSelection `json:"episodeSelection,omitempty"`
	InLibrary        bool                      `json:"inLibrary"`
	LibraryItemID    string                    `json:"libraryItemId,omitempty"`
	Rationale        string                    `json:"rationale,omitempty"` // why-it-fits (LLM)
	Confidence       float64                   `json:"confidence,omitempty"`
	// Source records which corpus surfaced this pick, carried from the grounded Candidate
	// (§8.3). It is PROVENANCE, never identity — Key() ignores it, exactly as it ignores
	// Genres/Overview/OfficialRating.
	//
	// The re-curation quality bar reads it: an adjacency pick arrives with a consensus the
	// model didn't compute and can't see, so a bare 0 confidence from a model that declined
	// to score a title it was handed must not be read as "the model judged this poorly".
	Source string `json:"source,omitempty"`
	// AdjacentVotes is how many of the channel's own titles independently recommended this
	// pick (§8.3) — 0 for anything that did not come from the adjacency corpus.
	//
	// It rides to the approval surface because it is the REASON: "recommended by 5 of your
	// films" is a claim an operator can check, where an LLM rationale can only be read. The
	// consensus is also what the unscored-pick floor trusts, so showing it makes that
	// decision legible rather than magic.
	AdjacentVotes int `json:"adjacentVotes,omitempty"`
	// Genres + Overview carry from the grounded Candidate so deterministic theme
	// scoring measures real metadata, not the title string (§8). Display/scoring
	// only — never identity (Key ignores them).
	Genres           []string `json:"genres,omitempty"`
	Overview         string   `json:"overview,omitempty"`
	OriginalLanguage string   `json:"originalLanguage,omitempty"`
	OriginCountries  []string `json:"originCountries,omitempty"`
	RuntimeMinutes   int      `json:"runtimeMinutes,omitempty"`
	VoteAverage      float64  `json:"voteAverage,omitempty"`
	VoteCount        int      `json:"voteCount,omitempty"`
	Keywords         []string `json:"keywords,omitempty"`
	Networks         []string `json:"networks,omitempty"`
	Cast             []string `json:"cast,omitempty"`
	Creators         []string `json:"creators,omitempty"`
	// OfficialRating carries from the grounded Candidate for ChannelPolicy audience
	// enforcement (programming-design §4): it's stamped onto the channel's lineup
	// entry at create time so enforcement filters without a library hit. Display/
	// enforcement only — never identity.
	OfficialRating string `json:"officialRating,omitempty"`
	// SizeBytes + SizeConfidence state what an owned pick occupies, for the approval
	// consequence summary (#1817). Read at proposal creation from the library row the
	// grounding already fetched, with no lookup of its own; an approver's addition takes it
	// from the presence check the gate already runs. Both are ABSENT when unavailable
	// (an acquisition, or a series with no source of its own), never 0. Display only.
	SizeBytes      int64                          `json:"sizeBytes,omitempty" doc:"Owned title's library file size in bytes; absent when unavailable, never 0"`
	SizeConfidence storagegovernor.SizeConfidence `json:"sizeConfidence,omitempty"`
}

// WithLibrarySize sets an owned pick's exact library file size, or clears both size fields
// when the pick is not owned or the library stated no size. Every path that sets or
// clears InLibrary calls it (grounding, the approval addition resolver and the gate in
// ResolveApprovalEdit), so a client-supplied size never reaches the approval surface.
func (p ProposalItem) WithLibrarySize(sizeBytes int64) ProposalItem {
	p.SizeBytes, p.SizeConfidence = 0, ""
	if p.InLibrary && sizeBytes > 0 {
		p.SizeBytes, p.SizeConfidence = sizeBytes, storagegovernor.SizeExact
	}
	return p
}

// Key derives the provisioning key (§3), enforcing the grounding guarantee: a
// validated ProposalItem always has a usable id.
func (p ProposalItem) Key() (provision.Key, error) {
	return provision.Title{
		MediaType: p.MediaType, TMDBID: p.TMDBID, TVDBID: p.TVDBID,
		Name: p.Name, Year: p.Year, Seasons: p.Seasons,
	}.Key()
}

// Proposal is the suggester's output (§8): a lineup of library items, an
// acquisition list of missing titles, ranked alternates, deterministic scores,
// and a rationale. Approved acquisitions feed the provisioner; the approved
// lineup feeds the scheduler.
type Proposal struct {
	Intent Intent `json:"intent"`
	// ChannelName is the LLM's proposed channel name (§8) — a real title derived from the
	// intent + grounded picks (e.g. "Springfield Classics"), not the raw prompt. Empty ⇒
	// the API falls back to a truncated intent description.
	ChannelName  string         `json:"channelName,omitempty"`
	Lineup       []ProposalItem `json:"lineup"`       // in-library items, ordered
	Acquisitions []ProposalItem `json:"acquisitions"` // missing titles to acquire
	Alternates   []ProposalItem `json:"alternates"`   // ranked backups (§9 substitution)
	Scores       Scores         `json:"scores"`       // deterministic post-scoring
	Rationale    string         `json:"rationale,omitempty"`
	// Policy is the grounded ChannelPolicy the suggester extracted (programming-
	// design §8): scope/audience/ordering/seasonal, validated + clamped (off-ladder
	// ceilings dropped unless explicit child safety supplies its deterministic bound;
	// era bounded). It rides the proposal into channel-create,
	// where it lands on the channel row. Empty = the channel uses built-in defaults.
	Policy schedule.ChannelPolicy `json:"policy,omitempty"`
	// Retired are lineup keys the auto-curate turnstile decided to rotate OUT to make room
	// for this proposal's incoming titles (§8.2a). Written by `recurate`, applied by the
	// binder — never by the suggester, and never by a human approval.
	//
	// ⚠ It rides the PROPOSAL rather than being applied directly to the channel, and that is
	// the whole point of the field. `recurate` used to trim `ch.Lineup` and call UpsertChannel
	// itself, which made it a second lineup writer racing the binder's additive union — the
	// two were ordered against each other by a code comment, and an additive union that ran
	// first would put every retired title straight back. One writer (the binder), one
	// primitive (schedule.ApplyLineup), and the retirement is now an INPUT to it.
	Retired []provision.Key `json:"retired,omitempty"`
	// Refused are picks the model grounded that this proposal's OWN extracted policy cannot
	// air (§4). They are moved out of Lineup/Acquisitions and kept here with a reason, so the
	// approval card shows what will not play instead of offering it.
	//
	// ⚠ Kept rather than deleted, deliberately. The operator's fix is usually to raise the
	// ceiling, not to lose the title — and a pick that silently vanished between the model's
	// answer and the approval screen is indistinguishable from one the model never made.
	Refused []RefusedPick `json:"refused,omitempty"`
	// SeasonNotKept names the holidays this request made the channel exclusive to that the
	// household's country doesn't keep (#2030 decision 38). They were left out of the seasonal
	// policy, since their season never comes; the approval card says so.
	SeasonNotKept *SeasonNotKept `json:"seasonNotKept,omitempty"`
	// Trace is immutable evidence from the original grounded proposal run. It is
	// never updated by approval edits or later scheduler/channel decisions.
	Trace DecisionTrace `json:"trace"`
	// FromIdea is the library channel idea this proposal was built from (#1720), e.g.
	// "genre:comedy": its lineup came from the library, not the model. Set by the server only;
	// no request body carries a Proposal, so a client can't claim one.
	FromIdea string `json:"fromIdea,omitempty" doc:"The library channel idea this request came from, if any"`
	// Manual is true when a person picked every title by hand with the LLM off (#1817, G2) —
	// set by the server only, from the manual-channel submit path, never from a client body.
	// ProposalReview reads it to show the maintainer-approved copy ("You picked every title in
	// this channel yourself — there's nothing to check.") instead of the stale-draft fallback,
	// which was written for a proposal the model DID produce but couldn't be rechecked.
	Manual bool `json:"manual,omitempty" doc:"Every title was picked by hand with the LLM off; nothing here was generated"`
}

// RefusedPick is one grounded pick the extracted policy will not air, with the exclusion
// vocabulary §4 already uses (`over_ceiling`) so the approval card and the channel's exclusion
// report say the same words about the same event.
type RefusedPick struct {
	Item   ProposalItem `json:"item"`
	Reason string       `json:"reason"` // "over_ceiling"
}

// SeasonNotKept is the holidays, by label, that a channel was asked to be exclusive to but that
// Country (the calendar's ISO 3166-1 code) doesn't keep.
type SeasonNotKept struct {
	Country  string   `json:"country" doc:"ISO 3166-1 alpha-2 code of the calendar the holidays were read against"`
	Holidays []string `json:"holidays" doc:"Labels of the holidays that country doesn't keep"`
}
