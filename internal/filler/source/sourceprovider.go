// Package source is the contract every filler source provider implements: a place
// filler comes from, such as YouTube or archive.org, or a local folder or library.
//
// A provider owns everything particular to it: how an operator's address is read, how a listing
// is walked and resumed, which of its items are usable from their metadata, how it suggests and
// previews sources, and how it searches beyond its registered sources. Filler owns the rules every
// provider shares — budgets, leases, selection, queueing — and asks the provider for the rest, so
// adding a provider is one package plus one registration. Download is the provider's too, through
// clipfetch.Downloader, because it shares clipfetch's staging and storage plumbing; clipfetch.
// Downloaders maps a provider's ID to its downloader, and the app registers the two together.
// The downloader's types are clipfetch's, which imports filler, so it is not held here.
//
// The package imports nothing from Loomarr, so filler, fillerstore, api and the provider packages
// can all name it without importing each other.
package source

import (
	"context"
	"time"
)

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
	// GroupDetail is the sentence under the provider's row in the source list, which rolls up the
	// sources registered under it. Empty for a provider whose sources are not grouped.
	GroupDetail string
	// SourceDetail is the sentence under each of its registered sources. Empty shows the source's
	// own address instead.
	SourceDetail string
	// SearchDown and ResolveDown are what an operator reads when the provider's service cannot be
	// reached while searching for a source or checking one. Empty falls back to a generic message.
	SearchDown, ResolveDown string
	// BadAddress is what an operator reads when the address they typed is not one this provider
	// can use. It names the shape the provider accepts.
	BadAddress Problem
	// Blurb is the sentence under the provider's heading on the source page, saying what it offers.
	Blurb string
	// SourceNoun names what one registered source of this provider is, as the drawer's subtitle:
	// "Archive.org collection", "Folder on this server".
	SourceNoun string
	// AddressLabel names the field an operator types a source's address into when the provider
	// takes sources by hand and has no finder: "Folder path".
	AddressLabel string
	// PreviewHeading introduces the examples shown from a source. Empty for a provider with no
	// previews.
	PreviewHeading string
	// Find is the copy of the finder a Finder provider offers. Zero for any other.
	Find FindCopy
}

// FindCopy is the words around a provider's source finder.
type FindCopy struct {
	// Label names the finder's field: "Find a YouTube channel or playlist".
	Label string
	// Placeholder shows the two ways in, a search and an address.
	Placeholder string
	// Add is the button that registers the chosen source.
	Add string
	// NoMatches is what a search with no results tells the operator to do next.
	NoMatches string
	// MinQuery is the fewest characters a search is worth running with.
	MinQuery int
}

// Problem is a refusal's headline and the way out of it.
type Problem struct {
	Title, Detail string
}

// Addresser reads the address an operator typed when registering a source. A provider without one
// cannot have sources registered by hand.
type Addresser interface {
	// Address returns the canonical address to store, or "" when the provider cannot use raw. A
	// provider validates by its own grammar: an archive.org identifier and a YouTube URL share no
	// rule.
	Address(raw string) string
}

// Linker gives the places an operator can look at a provider's material in the provider's own
// service. A provider without one has nothing to link to.
type Linker interface {
	// SourceURL is the public page of a registered source, from the address stored for it, or ""
	// when it has none.
	SourceURL(address string) string
	// EmbedURL is a player that plays an item the provider listed, from the item's page URL, or ""
	// when the provider does not allow the item to be embedded.
	EmbedURL(itemURL string) string
}

// Recognizer tells an address from words to search for, so a finder given either does the right
// thing with it. It is stricter than Addresser: a bare word is a valid address and also a search.
type Recognizer interface {
	// Recognizes reports whether input is an address of one of this provider's sources.
	Recognizes(input string) bool
}

// Capabilities are the optional interfaces a provider implements, as the UI needs to know them.
type Capabilities struct {
	// Listable providers are fetched: filler checks their sources on a schedule.
	Listable bool
	// Searchable providers search inside a registered source.
	Searchable bool
	// Discoverable providers suggest sources for a query and verify an address before it is
	// registered.
	Discoverable bool
	// OpenSearchable providers search beyond their registered sources.
	OpenSearchable bool
}

// CapabilitiesOf reports which optional interfaces p implements.
func CapabilitiesOf(p Provider) Capabilities {
	_, listable := p.(Lister)
	_, searchable := p.(Searcher)
	_, discoverable := p.(Finder)
	_, openSearchable := p.(OpenSearcher)
	return Capabilities{Listable: listable, Searchable: searchable, Discoverable: discoverable, OpenSearchable: openSearchable}
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
	// Station is the call sign whose search inside the source found this item. A provider sets it
	// on the hits of the station searches it ran for a ListRequest.
	Station string
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

// Lister reads what one registered source currently offers, without downloading media. A provider
// that implements it is fetched: filler checks its sources on a schedule and asks it for each
// listing.
//
// The provider owns how the listing is read: how much of the source it walks, where it resumes
// from, which of its items its own metadata rules out, and which of the rest are worth offering
// first. The caller owns what is shared: the budget, the lease on the source, the rejections every
// provider is held to, the dedupe against what is already known, and the selection.
type Lister interface {
	List(ctx context.Context, source Source, req ListRequest) (Listing, error)
}

// ListRequest is one check's terms, set by the caller.
type ListRequest struct {
	// Take is the most items the caller will select from this listing.
	Take int
	// List is Take before the pool's shortfall cut it. A provider that over-lists sizes its read
	// from List, because a short read of a source whose newest items are all known would report
	// nothing new while the pool stays short.
	List int
	// Now is the check's time, for a provider that rotates its work by the hour.
	Now time.Time
	// Stations are the call signs a provider that can search inside a source looks for. Empty
	// when the pass is not looking for any.
	Stations []string
	// MinDuration and MaxDuration are the clip envelope a provider whose listing carries an honest
	// duration screens against. Zero is unbounded.
	MinDuration, MaxDuration time.Duration
	// Reject is the rejection every provider is held to. It is asked first of each item the
	// provider reads, and reports the outcome that rules the item out. A provider counts the
	// outcome in Listing.Outcomes and drops the item.
	Reject func(Item) (Outcome, bool)
	// Known reports whether the caller already has the item: acquired, queued or declined. A
	// provider that resumes a listing counts it against Take only when it is not known, so a page
	// of known items does not use up the check. A provider that does not resume need not ask: the
	// caller's selection drops a known item.
	Known func(Item) bool
}

// Listing is what one check of a source found.
type Listing struct {
	// Items are the items to offer the caller's selection, in the provider's order. They have been
	// through Reject and the provider's own screens.
	Items []Item
	// Checkpoint is where this check stopped, to be stored once the check completes. A provider
	// that does not resume returns the one it was given.
	Checkpoint Checkpoint
	// ReplaceOutcomes begins a new sweep: the caller replaces the source's outcome summary with
	// this check's. Otherwise the outcomes extend the summary an in-progress cursor retained.
	ReplaceOutcomes bool
	// Outcomes counts the items the provider read and did not offer, by why. A known item is
	// counted OutcomeAlreadyKnown.
	Outcomes Outcomes
}

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
// address the operator typed. It returns only targets the provider can register as a source, such
// as a channel or playlist, so a caller need not check the target's type.
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
