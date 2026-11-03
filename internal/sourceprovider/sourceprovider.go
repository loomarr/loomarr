// Package sourceprovider is the contract every filler source provider implements: a place
// filler comes from, such as YouTube or archive.org, or a local folder or library.
//
// A provider owns everything particular to it: how an operator's address is read, how a listing
// is walked and resumed, which of its items are usable from their metadata, how it suggests and
// previews sources, and how it searches beyond its registered sources. Filler owns the rules every
// provider shares — budgets, leases, selection, queueing — and asks the provider for the rest, so
// adding a provider is one package plus one registration. Download is the provider's too, through
// clipfetch.Downloader, because it shares clipfetch's staging and storage plumbing.
//
// The package imports nothing from Loomarr, so filler, fillerstore, api and the provider packages
// can all name it without importing each other.
package sourceprovider

import "context"

// ID names a provider. It is stable: it is persisted on every source row and published on the
// wire, so a provider never changes its ID.
type ID string

// Provider is one place filler comes from. Its capabilities are the optional interfaces below it
// implements (Lister, Searcher, Finder, OpenSearcher); a caller asks with a type assertion.
type Provider interface {
	ID() ID
	Describe() Description
}

// Description is what the UI shows for a provider and which of its paths apply.
type Description struct {
	Label string
	// AddressHint is the example an operator sees in the source-address field.
	AddressHint string
	// Fetched is false for a provider whose sources are scanned where they are (a folder, the
	// media library) rather than downloaded.
	Fetched bool
}

// Source is the part of a registered source a provider reads.
type Source struct {
	ID  string
	URI string
	// Checkpoint is where the provider's previous check of this source stopped.
	Checkpoint Checkpoint
}

// Checkpoint resumes a bounded newest-first listing by stable item identity. Cursor is the last
// item examined in the in-progress sweep; PendingWatermark is the newest item seen when that sweep
// began; Watermark is the newest item committed by the prior completed sweep. A provider that does
// not resume leaves it empty.
type Checkpoint struct {
	Cursor           string
	Watermark        string
	PendingWatermark string
}

// Item is one item a source offers: an id stable enough to dedupe on, and the URL to hand the
// ingest path.
type Item struct {
	ID      string
	URL     string
	Title   string
	License string
	// ObservedYear is provider metadata used only to rank acquisition. It is not grounded clip era.
	ObservedYear  int
	PublishedAt   string
	DurationMS    int
	DurationKnown bool
	Height        int
	Availability  string
	LiveStatus    string
	// Gap is the channel coverage gap the pass selected this item for (EraGapKey, #749). Set only
	// on the way to ingest, where it is recorded on the downloaded artifact.
	Gap string
}

// Outcome is a durable, user-explainable result of considering one listed item. The set is closed
// and matches migration 00117; extractor-specific errors belong in diagnostics.
type Outcome string

const (
	OutcomeQueued             Outcome = "queued"
	OutcomeAlreadyKnown       Outcome = "already_known"
	OutcomeTooShort           Outcome = "too_short"
	OutcomeTooLong            Outcome = "too_long"
	OutcomeLive               Outcome = "live"
	OutcomeUpcoming           Outcome = "upcoming"
	OutcomePrivate            Outcome = "private"
	OutcomeUnavailable        Outcome = "unavailable"
	OutcomeMetadataIncomplete Outcome = "metadata_incomplete"
)

// Outcomes counts items by outcome, for the current or most recently completed sweep.
type Outcomes map[Outcome]int

// Searcher searches inside one registered source, such as a station's call sign within an
// archive.org collection.
type Searcher interface {
	Search(ctx context.Context, source Source, query string, limit int) ([]Item, error)
}

// OpenSearcher searches the whole provider, beyond its registered sources.
type OpenSearcher interface {
	OpenSearch(ctx context.Context, query string, limit int) ([]Item, error)
}

// Finder helps an operator choose a source: it suggests sources for a query and resolves an
// address the operator typed.
type Finder interface {
	Suggest(ctx context.Context, query string, limit int) ([]Suggestion, error)
	Resolve(ctx context.Context, input string) (Suggestion, error)
}

// Suggestion is a transient provider result or an exact resolved target. It is not a registered
// source and grants no persistence, download, or background-work authority.
type Suggestion struct {
	Provider     ID
	TargetType   string
	CanonicalID  string
	CanonicalURL string
	Title        string
	Description  string
	ItemCount    int
	PreviewItems []PreviewItem
}

// PreviewItem is a provider-native example returned while resolving a source. It is transient
// evidence for a person choosing a source, never an acquired or approved clip.
type PreviewItem struct {
	Title      string
	URL        string
	DurationMS int64
}
