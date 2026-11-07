package proposals

// MinDescriptionLen is the shortest channel description a request may carry, counted after
// trimming. Intent.Description's minLength tag declares the same number to the spec.
const MinDescriptionLen = 3

// Intent is a channel request: an NL description plus optional constraints (§8).
// It is the persisted request a proposal, a job and a channel store; the evidence one
// suggester run gathers while serving it lives in the suggester, never here.
type Intent struct {
	// The bounds below are declared for the spec, so a client form reads them from the generated
	// constants instead of keeping its own copy. Huma enforces them on every request body that
	// carries an Intent (submit and revise); the handlers also refuse a description that is
	// shorter than MinDescriptionLen once trimmed.
	Description string   `json:"description" minLength:"3"`                           // "90s action movies"
	Era         string   `json:"era,omitempty"`                                       // e.g. "1990s"
	Tone        string   `json:"tone,omitempty"`                                      // e.g. "high-energy"
	RuntimeTgt  int      `json:"runtimeTargetMin,omitempty" minimum:"0"`              // total target minutes; 0 ⇒ none
	MustInclude []string `json:"mustInclude,omitempty"`                               // titles/terms to include
	MustExclude []string `json:"mustExclude,omitempty"`                               // titles/terms to exclude
	MaxAcquire  int      `json:"maxAcquisitions,omitempty" minimum:"0" maximum:"200"` // cap on acquisitions (§8 quota); 0 ⇒ the quota alone
	// ChannelName is a name the requester fixed (the manual builder's typed name when it hands
	// over to AI, #1914). When set, the proposal keeps it instead of the model's; empty ⇒ the
	// model names the channel.
	ChannelName string `json:"channelName,omitempty"`
	// Refine inputs (§7 refine): a free-text change ("add more Schwarzenegger, drop the
	// slow ones") plus the channel's CURRENT lineup as context. The prompt renders these
	// so the model reasons from what's already on the channel and returns a revised
	// lineup. Context only — new picks are still grounded through Catalog operations, so
	// refine can't invent titles. Empty on a fresh (non-refine) suggestion.
	RefineText    string          `json:"refineText,omitempty"`
	CurrentLineup []LineupContext `json:"currentLineup,omitempty"`
	// Adjacent are pre-seeded candidates from the recommendation graph walked over this
	// channel's own lineup (programming-design §8.3) — the deterministic second corpus,
	// merged with whatever the model finds through the Catalog.
	//
	// They are OFFERED, never placed: the model still chooses, and an offered title it
	// ignores is simply not picked. Grounding is unweakened because these are real
	// catalog candidates with real ids that went through the same presence backfill a
	// tool result does — Suggest seeds them into `surfaced` before generation, so
	// buildProposal's "every pick traces to a candidate the catalog returned" invariant
	// holds verbatim. Merging AFTER generation instead would append picks the chokepoint
	// never checked, which is the one thing grounding exists to prevent.
	//
	// Empty on a fresh suggestion (no lineup to walk from) and on any install without a
	// TMDB corpus wired.
	Adjacent []AdjacentContext `json:"adjacent,omitempty"`
	// DiscoveryScopeID is request context for channel-specific explicit feedback during
	// re-curation. It never enters the API or persisted intent JSON.
	DiscoveryScopeID string `json:"-"`
}

// ChannelIntent is what a channel was asked to be: the request without the inputs that belong to
// one run (the refine note, the lineup it started from, adjacency offers) and without a name the
// requester fixed, which belongs to the request that created the channel. It is what a channel
// stores as its approved intent and what a refine or re-curation is seeded from.
func (i Intent) ChannelIntent() Intent {
	i.RefineText = ""
	i.CurrentLineup = nil
	i.Adjacent = nil
	i.ChannelName = ""
	return i
}

// LineupContext is a lightweight "what's on this channel now" entry fed to the refiner —
// title/year for the model to reason about, plus the real key so a kept item can be
// matched back exactly. Display/context only; not an identity the grounding gate checks.
type LineupContext struct {
	Name string `json:"name"`
	Year int    `json:"year,omitempty"`
	Key  string `json:"key,omitempty"` // provisioning key, e.g. "movie:tmdb:603"
}

// AdjacentContext is one pre-seeded adjacency candidate (§8.3): a title the channel's own
// lineup points at through TMDB's recommendation graph.
//
// Votes is how many of the channel's titles independently recommended it — the consensus
// that makes this a signal rather than noise, and the reason the candidate can explain
// itself ("recommended by 5 of your films") where an LLM pick can only paraphrase. It is
// rendered in the prompt so the model can weigh a strong consensus differently from a
// marginal one, and it is what the approval card shows.
type AdjacentContext struct {
	Name  string `json:"name"`
	Year  int    `json:"year,omitempty"`
	Key   string `json:"key,omitempty"`
	Votes int    `json:"votes,omitempty"`
}
