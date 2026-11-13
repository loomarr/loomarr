package filler

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/filler/acquisition"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/operator"

	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/filler/source"
	"github.com/loomarr/loomarr/internal/platform/settings"
)

// GET /v1/filler/sources — where a clip catalog comes from (§10, V28).
//
// ⚠ A READ-MODEL over two kinds of row. The persisted registry holds what an operator registered
// (an Archive.org collection, a playlist, an added folder or library): state a setting cannot
// hold, such as a fetch cursor, a licence and a last error. The seeded `folder` row and the
// `library` provenance row are DERIVED from configuration (`filler.dir`, the library scan), and
// the stored copy of a seeded row is skipped: a stored row and a setting describing the same
// folder would need a precedence rule — "the row says /data/filler, the setting says /srv/clips,
// which wins?" is a question with no good answer. Each row's live clip count comes from the
// catalog itself.

// FillerSourceLocationSource says where a source's location came from: the installation default,
// the source's own setting, or nowhere.
type FillerSourceLocationSource string

const (
	FillerSourceLocationInstallation FillerSourceLocationSource = "installation"
	FillerSourceLocationExplicit     FillerSourceLocationSource = "source"
	FillerSourceLocationMissing      FillerSourceLocationSource = "missing"
)

type FillerSourceReadiness string

const (
	FillerSourceReady         FillerSourceReadiness = "ready"
	FillerSourceOff           FillerSourceReadiness = "off"
	FillerSourceProviderOff   FillerSourceReadiness = "provider_off"
	FillerSourceNeedsLocation FillerSourceReadiness = "needs_location"
	FillerSourceNotConfigured FillerSourceReadiness = "not_configured"
	FillerSourceOutOfArea     FillerSourceReadiness = "out_of_area"
	FillerSourceUnavailable   FillerSourceReadiness = "unavailable"
)

type FillerSourceAction string

const (
	FillerSourceConfigure    FillerSourceAction = "configure"
	FillerSourceEnable       FillerSourceAction = "enable"
	FillerSourceSetLocation  FillerSourceAction = "set_location"
	FillerSourceEditLocation FillerSourceAction = "edit_location"
	FillerSourceDisable      FillerSourceAction = "disable"
	FillerSourceRemove       FillerSourceAction = "remove"
	FillerSourceFetch        FillerSourceAction = "fetch"
	FillerSourceSearch       FillerSourceAction = "search"
)

// FillerSourceDTO is one row of the Sources tab.
type FillerSourceDTO struct {
	// ID addresses this row on PATCH/DELETE. The two DERIVED rows carry the stable literals
	// `folder` and `library`; a remote collection carries its registry id.
	//
	// ⚠ Giving the derived rows ids does NOT make them table rows — they remain a read-model
	// over configuration (see the file header). It gives the Sources tab ONE way to address a
	// source, so the UI does not have to know that flipping the folder writes a setting while
	// flipping a collection writes a column. That asymmetry is a storage fact, not something an
	// operator should have to hold.
	ID string `json:"id"`
	// Enabled is the row's on/off switch (V35). ⚠ Off means Loomarr stops scanning, searching
	// and downloading from this source — it does NOT remove clips already in the catalog.
	Enabled bool `json:"enabled"`
	// EffectiveEnabled is the server-owned acquisition decision after provider policy is applied.
	// Enabled remains this target's remembered switch so pausing a provider never looks like it
	// rewrote every child.
	EffectiveEnabled bool `json:"effectiveEnabled"`
	ProviderEnabled  bool `json:"providerEnabled"`
	// OnForPlace is true when Loomarr switched this curated source on because it covers a station
	// near the household's or a channel's place (#2059 decision 10). Switching it off keeps it off.
	OnForPlace bool `json:"onForPlace,omitempty"`
	// Switchable is false for a row with no work to stop — a switch there would dim a row and
	// change nothing.
	//
	// ⚠ The remaining case is the `library` PROVENANCE row, which says where older clips came
	// from. An operator-ADDED library is switchable like any other source: it is scanned (§10),
	// so there is real work to stop.
	Switchable bool `json:"switchable"`
	// Removable is true only for rows that can be forgotten — the registered remotes. The
	// derived rows describe configuration, which is changed in Settings, not deleted here.
	Removable bool `json:"removable"`
	// Kind is what sort of source this is (V37: one flat vocabulary, one row per source).
	//
	// The list is flat, with no container row: a collection carries `archive` and a playlist
	// carries `youtube` directly.
	//
	// ⚠ `packs` is deliberately absent: there is no pack index to read (no URL, no manifest, no
	// fetcher), and §10 forbids a row that dims and changes nothing.
	Kind string `json:"kind" doc:"The provider's id, as listed by GET /v1/filler/providers"`
	// PageURL is where the source lives in the provider's own service, for a link out. Absent
	// when the provider has no pages or the source no address.
	PageURL string `json:"pageUrl,omitempty" doc:"The source's own page at its provider; absent when it has none"`
	// Searchable marks a source whose catalog can be searched in place — the Sources tab's
	// per-row search expander. Only `archive` today; the mock draws the same restriction.
	Searchable bool `json:"searchable"`
	// Target is the thing itself — a path, a library name, a URL.
	Target string `json:"target"`
	// URI is the machine-facing source reference. It is deliberately separate from Target:
	// Target may be an operator label, while a collection search must send the actual Archive
	// identifier. Absent on derived provider/provenance rows that have no source to address.
	URI string `json:"uri,omitempty"`
	// Detail is operator-facing prose explaining how this source behaves, rendered verbatim.
	Detail string `json:"detail"`
	// Count is how many catalog clips came from this source, counted live.
	Count int `json:"count"`
	// Incoming is how many clips from this source remain on Incoming's preparation/decision
	// conveyor. Kept separate from Count because these clips cannot play yet; terminal composite
	// containers are excluded because they are retained evidence, not pending work.
	Incoming int `json:"incoming" doc:"Clips from this source still being prepared or awaiting a decision; NOT included in count"`
	// Configured is false for a source the install could use but has not set up. Rendered as
	// an invitation rather than hidden: "no drop-folder configured" is the answer to "why is
	// my catalog empty", and hiding the row leaves that question unanswered.
	Configured bool `json:"configured"`
	// Fetchable marks a source that `POST /v1/filler/sources/fetch` can refresh on demand.
	Fetchable bool `json:"fetchable"`
	// LastCheckedAt is absent when never checked. A successful empty check still advances it;
	// bringing in an item is a separate provenance fact that does not drive scheduling.
	LastCheckedAt string `json:"lastCheckedAt,omitempty" doc:"RFC3339; absent if never checked"`
	// LastCheck explains the bounded automatic-acquisition decisions without exposing yt-dlp
	// diagnostics or cursor mechanics. Absent until a check has observed at least one item.
	LastCheck *FillerSourceCheckSummaryDTO `json:"lastCheck,omitempty"`
	// AutomaticDownloads is the server-resolved source policy. Omitted on derived/provider rows
	// that cannot automatically download media themselves.
	AutomaticDownloads *SourceAutomaticDownloadsDTO `json:"automaticDownloads,omitempty"`
	// License is what the source DECLARED about its material — the mock's per-row licence chip.
	//
	// ⚠ **Absent means UNKNOWN, never "public domain".** ~92% of archive.org items declare no
	// licence at all (667 of 8362 measured in `classic_tv_commercials`, 2026-07-31), so absence is
	// the common case and carries no permission whatsoever. The UI must render nothing rather than
	// a reassuring default — a chip that said "public domain" by omission would be Loomarr
	// asserting a legal fact nobody checked.
	License string `json:"license,omitempty" doc:"Licence the source declared; absent means unknown, NOT public domain"`
	Country string `json:"country,omitempty" doc:"ISO 3166-1 alpha-2 country asserted for this source"`
	// EffectiveCountry is what this source uses now. It differs from Country when the row
	// follows Installation geography, which is the ordinary zero-config path.
	EffectiveCountry string                     `json:"effectiveCountry,omitempty" doc:"Country this source uses after Installation geography inheritance"`
	LocationSource   FillerSourceLocationSource `json:"locationSource" enum:"installation,source,missing" doc:"Where the effective location came from"`
	// Readiness and Actions are the server-owned answer to whether this row can do useful work.
	// The browser must not infer readiness from enabled/configured/fetchable flags that answer
	// different questions.
	Readiness FillerSourceReadiness `json:"readiness" enum:"ready,off,provider_off,needs_location,not_configured,out_of_area,unavailable"`
	Ready     bool                  `json:"ready"`
	Actions   []FillerSourceAction  `json:"actions" doc:"Closed actions currently valid for this source"`
	// Group marks a PROVIDER node — one service, with the targets an operator added beneath it
	// (§10 V51c). Three archive.org collections stop being three sibling rows and become one
	// Archive.org row that twirls down.
	//
	// ⚠ **A separate axis from `Kind`, deliberately** — the same call §10 V45 makes for
	// `IsComposite`. Adding `provider` to the kind enum would force every switch on kind to grow
	// a case, which is how a container leaks into a place that expects a source: a pull planner
	// or a fetch loop that handled the new value by falling through its `default`.
	Group bool `json:"group,omitempty"`
	// ParentID is the provider node this row sits under, or empty for a top-level row.
	//
	// ⚠ **Derived from `Kind` at read time — there is no `parent_id` column and no migration.**
	// The grouping being asked for IS already a column: every `archive` row belongs under
	// Archive.org, and there is no representable case where it belongs anywhere else. A stored
	// parent would be a second encoding of that fact, and second encodings make illegal states
	// representable (`kind='archive', parentId='provider:youtube'`).
	//
	// ⚠ **Never accepted on a request body.** `POST /v1/filler/sources` takes a kind and derives
	// the parent; accepting one would let a client assert a parent that contradicts the kind.
	ParentID string `json:"parentId,omitempty"`
}

// FillerSourceCheckSummaryDTO is the closed, plain-language-ready result of one bounded source
// sweep. Fields stay explicit so generated clients and the UI cannot invent disposition strings.
type FillerSourceCheckSummaryDTO struct {
	Queued             int `json:"queued"`
	AlreadyKnown       int `json:"alreadyKnown"`
	TooShort           int `json:"tooShort"`
	TooLong            int `json:"tooLong"`
	Live               int `json:"live"`
	Upcoming           int `json:"upcoming"`
	Private            int `json:"private"`
	Unavailable        int `json:"unavailable"`
	MetadataIncomplete int `json:"metadataIncomplete"`
}

func fillerSourceCheckSummaryDTO(summary source.Outcomes) *FillerSourceCheckSummaryDTO {
	var total int
	for _, count := range summary {
		total += count
	}
	if total == 0 {
		return nil
	}
	return &FillerSourceCheckSummaryDTO{
		Queued:             summary[source.OutcomeQueued],
		AlreadyKnown:       summary[source.OutcomeAlreadyKnown],
		TooShort:           summary[source.OutcomeTooShort],
		TooLong:            summary[source.OutcomeTooLong],
		Live:               summary[source.OutcomeLive],
		Upcoming:           summary[source.OutcomeUpcoming],
		Private:            summary[source.OutcomePrivate],
		Unavailable:        summary[source.OutcomeUnavailable],
		MetadataIncomplete: summary[source.OutcomeMetadataIncomplete],
	}
}

// providerIDPrefix namespaces the derived group ids (`provider:archive`, `provider:youtube`) so
// they can never collide with an operator-produced source id, and so ONE prefix test guards every
// route that addresses a row.
const providerIDPrefix = "provider:"

// isProviderID reports whether an id addresses a derived group node rather than a real source.
func isProviderID(id string) bool { return strings.HasPrefix(id, providerIDPrefix) }

func (s *Handlers) fillerFetchEvery() time.Duration {
	const fallback = 6 * time.Hour
	if s.liveSettings == nil {
		return fallback
	}
	every := settings.FillerFetchEvery.Get(s.liveSettings)
	if every < 0 {
		return fallback
	}
	return every
}

func (s *Handlers) fillerFetchMaxPerCheck() int {
	if s.liveSettings == nil {
		return 10
	}
	if value := settings.FillerFetchMaxPerRun.Get(s.liveSettings); value > 0 {
		return value
	}
	return 10
}

func automaticDownloadInterval(every time.Duration) string {
	switch {
	case every%(24*time.Hour) == 0:
		days := int(every / (24 * time.Hour))
		if days == 1 {
			return "day"
		}
		return fmt.Sprintf("%d days", days)
	case every%time.Hour == 0:
		hours := int(every / time.Hour)
		if hours == 1 {
			return "hour"
		}
		return fmt.Sprintf("%d hours", hours)
	default:
		minutes := int(every / time.Minute)
		if minutes == 1 {
			return "minute"
		}
		return fmt.Sprintf("%d minutes", minutes)
	}
}

func automaticDownloadSummary(mode string, every time.Duration, maxPerCheck int) string {
	if mode == "never" || every <= 0 {
		return "Doesn’t download automatically. You can still look for clips yourself."
	}
	policy := fmt.Sprintf("every %s, up to %d clips each check", automaticDownloadInterval(every), maxPerCheck)
	if maxPerCheck == 1 {
		policy = fmt.Sprintf("every %s, up to 1 clip each check", automaticDownloadInterval(every))
	}
	if mode == "defaults" {
		return "Uses your defaults: " + policy + "."
	}
	return strings.ToUpper(policy[:1]) + policy[1:] + "."
}

func (s *Handlers) sourceAutomaticDownloads(src records.FillerSource, eligible bool) *SourceAutomaticDownloadsDTO {
	if !s.sourceProviders.Fetched(source.ID(src.Kind)) {
		return nil
	}
	every, pollable := src.FetchEvery(s.fillerFetchEvery())
	mode := "defaults"
	if !pollable && src.FetchEverySeconds != nil {
		mode = "never"
	} else if src.FetchEverySeconds != nil || src.FetchMaxPerRun != nil {
		mode = "custom"
	}
	maxPerCheck := src.MaxPerRun(s.fillerFetchMaxPerCheck())
	out := &SourceAutomaticDownloadsDTO{
		Mode: mode, EverySeconds: int(every.Seconds()), MaxPerCheck: maxPerCheck,
		Summary: automaticDownloadSummary(mode, every, maxPerCheck),
	}
	// The fetcher's own plan for this source, so the page shows the check the job will actually make.
	if next, ok := src.FetchPlan(eligible, 0, s.fillerFetchEvery()).NextAutomaticCheck(time.Now().UTC()); ok {
		out.NextCheckAt = next.UTC().Format(time.RFC3339)
	}
	return out
}

type providerGroup struct {
	kind, id, label, detail string
}

// providerGroups are the kinds that roll up, in the order their groups appear: the fetched
// providers, in registry order.
//
// ⚠ **A scanned provider (the folder, the library) deliberately does NOT group.** A twirl-down
// exists because ONE SERVICE offers many targets; two watched folders are two unrelated
// directories with no service in common, so a "Folders" container would be a row that dims and
// changes nothing — §10's own forbidden shape. It also leaves the config-backed projection in
// `listFillerSources` untouched.
func (s *Handlers) providerGroups() []providerGroup {
	var groups []providerGroup
	for _, p := range s.sourceProviders.All() {
		d := p.Describe()
		if !d.Fetched {
			continue
		}
		groups = append(groups, providerGroup{
			kind: string(p.ID()), id: providerIDPrefix + string(p.ID()), label: d.Label, detail: d.GroupDetail,
		})
	}
	return groups
}

// ⚠ Each registered collection is a PEER row with its own `kind`, `enabled` and `lastCheckedAt`;
// there is no container row nesting them. The list still has to say "you could set up a library
// but have not", which a table of things-that-exist cannot express: `configured` is a per-row
// fact, and `folder`/`library` are singleton rows materialised even when unset (migration
// 00029). Dropping that would delete the answer to "why is my catalog empty?".

// RemoteSourceDTO is one registered remote source (§10, V33).
type RemoteSourceDTO struct {
	ID string `json:"id"`
	// Enabled is this collection's own switch, stored as a column on its row (V35).
	Enabled bool `json:"enabled"`
	// Label falls back to the URI when a source has none, so a row is never blank.
	Label   string `json:"label"`
	URI     string `json:"uri"`
	Country string `json:"country,omitempty"`
	// LastFetchedAt is absent when never fetched — rendered as "never" rather than as an
	// epoch date nobody meant.
	LastFetchedAt string `json:"lastFetchedAt,omitempty" doc:"RFC3339; absent if never fetched"`
}

// FillerSourceSuggestionDTO is a transient provider result. It deliberately has no source id or
// enabled state: those exist only after the separate registration command succeeds.
type FillerSourceSuggestionDTO struct {
	Provider     string                       `json:"provider" doc:"The provider's id, as listed by GET /v1/filler/providers"`
	TargetType   string                       `json:"targetType" enum:"collection,channel,playlist"`
	CanonicalID  string                       `json:"canonicalId"`
	CanonicalURL string                       `json:"canonicalUrl"`
	Title        string                       `json:"title"`
	Description  string                       `json:"description,omitempty"`
	ItemCount    int                          `json:"itemCount,omitempty"`
	AlreadyAdded bool                         `json:"alreadyAdded"`
	PreviewItems []FillerSourcePreviewItemDTO `json:"previewItems,omitempty" maxItems:"3"`
}

// FillerSourcePreviewItemDTO is a transient provider item used to inspect a source before adding it.
type FillerSourcePreviewItemDTO struct {
	Title      string `json:"title"`
	URL        string `json:"url"`
	DurationMS int64  `json:"durationMs,omitempty"`
	// EmbedURL plays the item in the provider's own player; absent when it cannot be embedded.
	EmbedURL string `json:"embedUrl,omitempty" doc:"Player that plays the item in place; absent when the provider does not allow it"`
}

// FillerProviderDescriptionDTO is one registered source provider and what it can do.
type FillerProviderDescriptionDTO struct {
	ID          string `json:"id" doc:"The provider's persisted identity, as a source row's kind"`
	Label       string `json:"label" doc:"Operator-facing name"`
	AddressHint string `json:"addressHint,omitempty" doc:"An example of the address a source of this provider takes"`
	Fetched     bool   `json:"fetched" doc:"Sources are downloaded on a schedule; false means they are scanned where they are"`
	Listable    bool   `json:"listable" doc:"Filler checks the provider's sources on a schedule"`
	Searchable  bool   `json:"searchable" doc:"A registered source can be searched in place"`
	// Discoverable providers can suggest sources for a query and verify an address.
	Discoverable   bool `json:"discoverable" doc:"Sources can be suggested for a query and an address verified before it is added"`
	OpenSearchable bool `json:"openSearchable" doc:"The provider can be searched beyond its registered sources"`
	Addable        bool `json:"addable" doc:"An operator can register a source of this provider by typing its address"`
	// The words the source page shows for the provider, so no client carries a copy per provider.
	Blurb           string `json:"blurb,omitempty" doc:"The sentence under the provider's heading on the source page"`
	SourceNoun      string `json:"sourceNoun,omitempty" doc:"What one registered source is, as the source drawer's subtitle"`
	AddressLabel    string `json:"addressLabel,omitempty" doc:"Names the field a source's address is typed into when the provider has no finder"`
	PreviewHeading  string `json:"previewHeading,omitempty" doc:"Introduces the examples shown from a source"`
	FindLabel       string `json:"findLabel,omitempty" doc:"Names the source finder's field; absent unless discoverable"`
	FindPlaceholder string `json:"findPlaceholder,omitempty" doc:"The source finder's placeholder"`
	FindAdd         string `json:"findAdd,omitempty" doc:"The button that registers the source the finder chose"`
	FindNoMatches   string `json:"findNoMatches,omitempty" doc:"What a search with no results tells the operator to do next"`
	FindMinQuery    int    `json:"findMinQuery,omitempty" doc:"The fewest characters a search runs with"`
}

type fillerProvidersOutput struct {
	Body struct {
		Providers []FillerProviderDescriptionDTO `json:"providers"`
	}
}

func (s *Handlers) listFillerProviders(context.Context, *struct{}) (*fillerProvidersOutput, error) {
	out := &fillerProvidersOutput{}
	out.Body.Providers = []FillerProviderDescriptionDTO{}
	for _, p := range s.sourceProviders.All() {
		d := p.Describe()
		c := source.CapabilitiesOf(p)
		_, addable := p.(source.Addresser)
		out.Body.Providers = append(out.Body.Providers, FillerProviderDescriptionDTO{
			ID: string(p.ID()), Label: d.Label, AddressHint: d.AddressHint, Fetched: d.Fetched,
			Listable: c.Listable, Searchable: c.Searchable, Discoverable: c.Discoverable,
			OpenSearchable: c.OpenSearchable, Addable: addable,
			Blurb: d.Blurb, SourceNoun: d.SourceNoun, AddressLabel: d.AddressLabel, PreviewHeading: d.PreviewHeading,
			FindLabel: d.Find.Label, FindPlaceholder: d.Find.Placeholder, FindAdd: d.Find.Add,
			FindNoMatches: d.Find.NoMatches, FindMinQuery: d.Find.MinQuery,
		})
	}
	return out, nil
}

type fillerSourceSuggestionsInput struct {
	Kind  string `path:"kind" doc:"A discoverable provider's id, as listed by GET /v1/filler/providers"`
	Query string `query:"q" minLength:"2" maxLength:"120" required:"true"`
	Limit int    `query:"limit" minimum:"1" maximum:"8" default:"8"`
}

type fillerSourceSuggestionsOutput struct {
	Body struct {
		Suggestions []FillerSourceSuggestionDTO `json:"suggestions"`
		Exact       bool                        `json:"exact" doc:"The query was an address, not words: suggestions holds the one source it names, already checked"`
	}
}

type resolveFillerSourceInput struct {
	Kind string `path:"kind" doc:"A discoverable provider's id, as listed by GET /v1/filler/providers"`
	Body struct {
		Input string `json:"input" minLength:"1" maxLength:"500"`
	}
}

type resolveFillerSourceOutput struct{ Body FillerSourceSuggestionDTO }

type fillerSourcesOutput struct {
	Body struct {
		Sources []FillerSourceDTO `json:"sources"`
		// Total is the catalog size — the mock's "N sources · M clips" line. Sent rather than
		// summed client-side because a clip whose `source` matches no known row still counts
		// toward the catalog, and a client summing the rows would under-report.
		Total int `json:"total"`
	}
}

type queueFillerSourceItemInput struct {
	ID   string `path:"id" minLength:"1" maxLength:"256"`
	Body struct {
		RemoteID string `json:"remoteId" minLength:"1" maxLength:"512" doc:"Provider-stable item identity returned by discovery"`
		URL      string `json:"url" minLength:"1" maxLength:"2048" format:"uri" doc:"Canonical item URL returned by discovery"`
	}
}

type queueFillerSourceItemOutput struct {
	Body struct {
		JobID string `json:"jobId" doc:"Durable acquisition id; read /v1/filler/acquisitions/{jobId}"`
	}
}

// registerFillerSources mounts the sources read-model. Admin-only: it names filesystem paths
// and library targets, which is infrastructure detail a member has no business reading.
func (s *Handlers) registerFillerSources(api huma.API) {
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "list-filler-sources", Method: http.MethodGet, Path: "/v1/filler/sources",
		Summary: "Where the clip catalog comes from",
		Description: "Admin only. Derived from the configured drop-folder and library scan plus live " +
			"per-source clip counts — there is no sources table (§10). A source the install could use " +
			"but has not configured is returned with configured:false rather than omitted.",
		Tags: []string{"filler"},
	}, httpcore.RoleAdmin), s.listFillerSources)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "fetch-filler-source", Method: http.MethodPost, Path: "/v1/filler/sources/fetch",
		Summary: "Look for new clips in one source",
		Description: "Admin only (§10 V56). Runs one ordinary bounded acquisition pass for the selected " +
			"source, then scans configured local sources. It retains provider/source enablement, geography, " +
			"deduplication and safety limits; only timing is bypassed.",
		Tags: []string{"filler"},
	}, httpcore.RoleAdmin), s.fetchFillerSource)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "queue-filler-source-item", Method: http.MethodPost, Path: "/v1/filler/sources/{id}/items",
		Summary: "Add one clip found inside a source",
		Description: "Admin only (§10). Queues one exact discovered item under its registered parent source. " +
			"The server derives provider policy from the source; this never registers the item as another source.",
		Tags: []string{"filler"},
	}, httpcore.RoleAdmin), s.queueFillerSourceItem)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "add-filler-source", Method: http.MethodPost, Path: "/v1/filler/sources",
		Summary: "Add a remote filler source",
		Description: "Admin only (§10 V35/V38b/V67). Registers a verified Archive.org collection or YouTube channel/playlist for bounded scheduled " +
			"acquisition. Registration itself only records the source; enabled remote sources are downloaded " +
			"on their configured schedule, while composed multi-source pulls retain their approval gate.",
		Tags: []string{"filler"},
	}, httpcore.RoleAdmin), s.addFillerSource)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "list-filler-providers", Method: http.MethodGet, Path: "/v1/filler/providers",
		Summary: "List the filler source providers",
		Description: "Admin only. Every provider a filler source can name, in the order the source list shows " +
			"them, with what each can do. The registry is the source of truth: a provider added to the " +
			"server appears here with no client change.",
		Tags: []string{"filler"},
	}, httpcore.RoleAdmin), s.listFillerProviders)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "set-filler-provider-enabled", Method: http.MethodPatch, Path: "/v1/filler/providers/{kind}",
		Summary: "Pause or resume a filler provider",
		Description: "Admin only (§10 V51c). Changes provider policy without rewriting child source " +
			"switches or removing clips already in the catalog.",
		Tags: []string{"filler"},
	}, httpcore.RoleAdmin), s.setFillerProviderEnabled)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "suggest-filler-sources", Method: http.MethodGet, Path: "/v1/filler/providers/{kind}/suggestions",
		Summary: "Find a filler source",
		Description: "Admin only (§10 V67). Searches one provider for source candidates without " +
			"registering a source, downloading media, or granting background-work authority.",
		Tags: []string{"filler"},
	}, httpcore.RoleAdmin), s.suggestFillerSources)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "resolve-filler-source", Method: http.MethodPost, Path: "/v1/filler/providers/{kind}/resolve",
		Summary: "Check a filler source",
		Description: "Admin only (§10 V67). Validates one exact provider URL or identifier and returns " +
			"its canonical target without registering or downloading it.",
		Tags: []string{"filler"},
	}, httpcore.RoleAdmin), s.resolveFillerSource)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "set-filler-source-enabled", Method: http.MethodPatch, Path: "/v1/filler/sources/{id}",
		Summary: "Control source acquisition and fetch cadence",
		Description: "Admin only (§10 V35, extended V38c/V57). Off means Loomarr stops scanning, searching and " +
			"downloading from this source. ⚠ It does NOT remove clips already in the catalog, and it is not " +
			"a delete: the source keeps its licence and fetch history, so switching it back on resumes rather " +
			"than restarts. Source configuration grants no catalog-admission authority. `folder` writes the " +
			"drop-folder setting; any other id writes that source's own row. `automaticDownloads` is optional " +
			"and uses an explicit defaults, custom, or never mode, so changing the switch or area cannot " +
			"accidentally reset the policy. Library sources became switchable in " +
			"V38c, when §10 restored them to being scanned.",
		Tags: []string{"filler"},
	}, httpcore.RoleAdmin), s.setFillerSourceEnabled)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "delete-filler-source", Method: http.MethodDelete, Path: "/v1/filler/sources/{id}",
		Summary: "Forget a registered remote source",
		Description: "Admin only (§10 V35). ⚠ Clips it already brought in are NOT deleted — they are real " +
			"files, already tagged and possibly pinned into a channel, and forgetting where something came " +
			"from is not a reason to throw it away. Only the registered remotes can be deleted; the derived " +
			"folder and library rows describe configuration, which is changed in Settings.",
		Tags: []string{"filler"},
	}, httpcore.RoleAdmin), s.deleteFillerSource)
}

func (s *Handlers) suggestFillerSources(ctx context.Context, in *fillerSourceSuggestionsInput) (*fillerSourceSuggestionsOutput, error) {
	if s.filler == nil || s.store == nil {
		return nil, huma.Error501NotImplemented("filler source discovery is not available on this instance")
	}
	if err := s.requireFillerProviderEnabled(ctx, in.Kind); err != nil {
		return nil, err
	}
	query := strings.TrimSpace(in.Query)
	if minimum := s.providerDescription(in.Kind).Find.MinQuery; len([]rune(query)) < minimum {
		return nil, httpcore.Unprocessable("That search is too short", fmt.Sprintf("Type at least %d characters.", minimum))
	}
	// An address is checked rather than searched for: it names one source, and a search for it
	// would return whatever else happens to match.
	if recognizer, ok := s.sourceProviders.Recognizer(source.ID(in.Kind)); ok && recognizer.Recognizes(query) {
		resolve := &resolveFillerSourceInput{Kind: in.Kind}
		resolve.Body.Input = query
		resolved, err := s.resolveFillerSource(ctx, resolve)
		if err != nil {
			return nil, err
		}
		out := &fillerSourceSuggestionsOutput{}
		out.Body.Exact = true
		out.Body.Suggestions = []FillerSourceSuggestionDTO{resolved.Body}
		return out, nil
	}
	limit := in.Limit
	if limit <= 0 || limit > 8 {
		limit = 8
	}
	results, err := s.filler.SuggestSources(ctx, in.Kind, query, limit)
	if err != nil {
		if errors.Is(err, acquisition.ErrInvalidSourceReference) {
			return nil, httpcore.Unprocessable("That search could not be used", err.Error())
		}
		if down := s.providerDescription(in.Kind).SearchDown; down != "" && errors.Is(err, acquisition.ErrSourceProvider) {
			return nil, huma.Error502BadGateway(down, err)
		}
		return nil, huma.Error502BadGateway("find filler sources", err)
	}
	registered, err := s.registeredFillerSourceTargets(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("list registered filler sources", err)
	}
	out := &fillerSourceSuggestionsOutput{}
	for _, result := range results {
		out.Body.Suggestions = append(out.Body.Suggestions, s.sourceSuggestionDTO(result, registered))
	}
	return out, nil
}

func (s *Handlers) resolveFillerSource(ctx context.Context, in *resolveFillerSourceInput) (*resolveFillerSourceOutput, error) {
	if s.filler == nil || s.store == nil {
		return nil, huma.Error501NotImplemented("filler source discovery is not available on this instance")
	}
	if err := s.requireFillerProviderEnabled(ctx, in.Kind); err != nil {
		return nil, err
	}
	result, err := s.filler.ResolveSource(ctx, in.Kind, strings.TrimSpace(in.Body.Input))
	if err != nil {
		if errors.Is(err, acquisition.ErrInvalidSourceReference) {
			return nil, httpcore.Unprocessable("That source could not be verified", err.Error())
		}
		if down := s.providerDescription(in.Kind).ResolveDown; down != "" && errors.Is(err, acquisition.ErrSourceProvider) {
			return nil, huma.Error502BadGateway(down, err)
		}
		return nil, huma.Error502BadGateway("check filler source", err)
	}
	registered, err := s.registeredFillerSourceTargets(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("list registered filler sources", err)
	}
	return &resolveFillerSourceOutput{Body: s.sourceSuggestionDTO(result, registered)}, nil
}

// providerDescription is the registered provider's description; the zero value for a kind nothing
// is registered under.
func (s *Handlers) providerDescription(kind string) source.Description {
	if p, ok := s.sourceProviders.Get(source.ID(kind)); ok {
		return p.Describe()
	}
	return source.Description{}
}

// discoverableLabels names the providers an operator can search or verify a source with, for
// "Choose …" refusals.
func (s *Handlers) discoverableLabels() string {
	var labels []string
	for _, p := range s.sourceProviders.All() {
		if _, ok := p.(source.Finder); ok {
			labels = append(labels, p.Describe().Label)
		}
	}
	return strings.Join(labels, " or ")
}

func (s *Handlers) requireFillerProviderEnabled(ctx context.Context, kind string) error {
	providers, err := s.store.ListFillerProviders(ctx)
	if err != nil {
		return huma.Error500InternalServerError("read filler provider policy", err)
	}
	for _, provider := range providers {
		if provider.Kind != kind {
			continue
		}
		if !provider.Enabled {
			return httpcore.Conflict("That source service is paused", "Turn it back on before searching or checking a source.")
		}
		return nil
	}
	return httpcore.Unprocessable("Unknown filler source service", "Choose "+s.discoverableLabels()+".")
}

func (s *Handlers) queueFillerSourceItem(ctx context.Context, in *queueFillerSourceItemInput) (*queueFillerSourceItemOutput, error) {
	if s.filler == nil {
		return nil, httpcore.NotImplemented("Filler isn't set up", "Enable filler before adding clips from a source.")
	}
	sources, err := s.store.ListFillerSources(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("read filler source", err)
	}
	var selected *records.FillerSource
	for i := range sources {
		if sources[i].ID == in.ID {
			selected = &sources[i]
			break
		}
	}
	if selected == nil {
		return nil, huma.Error404NotFound("Filler source not found")
	}
	if !s.sourceProviders.Fetched(source.ID(selected.Kind)) {
		return nil, httpcore.Unprocessable("That source has no online catalog", "Choose an "+s.discoverableLabels()+" source.")
	}
	if !selected.Enabled {
		return nil, httpcore.Conflict("That source is paused", "Turn this source back on before adding a clip from it.")
	}
	if !selected.ProviderEnabled {
		return nil, httpcore.Conflict("That source service is paused", "Turn it back on before adding a clip from it.")
	}

	jobID, err := s.filler.IngestSourceItems(ctx, selected.ID, selected.Kind, []source.Item{{
		ID: in.Body.RemoteID, URL: in.Body.URL,
	}})
	if errors.Is(err, acquisition.ErrProviderPaused) {
		return nil, httpcore.Conflict("That source service is paused", "Turn it back on before adding a clip from it.")
	}
	if errors.Is(err, operator.ErrIngestUnavailable) {
		return nil, httpcore.Conflict("Downloads aren't available here",
			"This install can't run the download tooling. The official image ships it; a custom build may be missing it, or INGEST_YTDLP_PATH may point somewhere wrong.")
	}
	if err != nil {
		return nil, err
	}
	out := &queueFillerSourceItemOutput{}
	out.Body.JobID = jobID
	return out, nil
}

func (s *Handlers) registeredFillerSourceTargets(ctx context.Context) (map[string]bool, error) {
	sources, err := s.store.ListFillerSources(ctx)
	if err != nil {
		return nil, err
	}
	registered := make(map[string]bool, len(sources)*2)
	for _, source := range sources {
		registered[source.Kind+":"+source.URI] = true
	}
	return registered, nil
}

func (s *Handlers) sourceSuggestionDTO(source source.Suggestion, registered map[string]bool) FillerSourceSuggestionDTO {
	provider := string(source.Provider)
	previewItems := make([]FillerSourcePreviewItemDTO, 0, len(source.PreviewItems))
	for _, item := range source.PreviewItems {
		previewItems = append(previewItems, FillerSourcePreviewItemDTO{
			Title: item.Title, URL: item.URL, DurationMS: item.DurationMS, EmbedURL: httpcore.EmbedURL(s.sourceProviders, provider, item.URL),
		})
	}
	return FillerSourceSuggestionDTO{
		Provider: provider, TargetType: source.TargetType, CanonicalID: source.CanonicalID,
		CanonicalURL: source.CanonicalURL, Title: source.Title, Description: source.Description,
		ItemCount: source.ItemCount, PreviewItems: previewItems,
		AlreadyAdded: registered[provider+":"+source.CanonicalID] || registered[provider+":"+source.CanonicalURL],
	}
}

// canFetchRow reports whether this row's "Fetch now" has anything to do.
//
// ⚠ It is a fact about the ROW, not only the install: "does the ingest route exist" alone would
// offer a Fetch-now button on a row with nothing to enumerate. The seeded YouTube source ships
// with an empty uri (§10 forbids Loomarr recommending a playlist, so the operator brings one).
// A control that cannot work is worse than no control.
//
// The two kinds fail for different reasons and are answered separately:
//   - SCANNED (folder, library) read local/LAN storage, so they need no ingest tooling at all —
//     "Fetch now" runs the same sync the scheduler does.
//   - DOWNLOADED (archive, youtube) need BOTH a target to fetch (the store's `Fetchable()`,
//     which is false on an empty uri) and the ingest route to hand it to.
func canFetchRow(src records.FillerSource, ingestAvailable bool) bool {
	if src.Scannable() {
		return true
	}
	return src.Fetchable() && ingestAvailable
}

type fetchFillerSourceOutput struct {
	Body struct {
		SourceID      string                       `json:"sourceId" doc:"Selected registered source"`
		SourcesPolled int                          `json:"sourcesPolled" doc:"Remote sources actually inspected by this pass"`
		Queued        int                          `json:"queued" doc:"New remote items queued for acquisition"`
		Skipped       int                          `json:"skipped" doc:"Remote items already known to the catalog or acquisition history"`
		MaxPerCheck   int                          `json:"maxPerCheck" doc:"Effective clip limit for this selected source"`
		StoppedBy     string                       `json:"stoppedBy,omitempty" enum:"catalog,provider" doc:"Capacity ceiling that stopped the pass"`
		Outcomes      *FillerSourceCheckSummaryDTO `json:"outcomes,omitempty"`
		Total         int                          `json:"total"`
		Added         int                          `json:"added"`
		Updated       int                          `json:"updated"`
		Pruned        int                          `json:"pruned"`
	}
}

type fetchFillerSourceInput struct {
	ID string `query:"id" minLength:"1" doc:"Registered source id to check"`
}

func (s *Handlers) fetchFillerSource(ctx context.Context, in *fetchFillerSourceInput) (*fetchFillerSourceOutput, error) {
	if s.filler == nil {
		return nil, huma.Error501NotImplemented("filler sync is not available on this instance")
	}
	if strings.TrimSpace(in.ID) == "" {
		return nil, httpcore.Unprocessable("Choose a source", "Look for new clips from one source at a time.")
	}
	fetchResult, err := s.filler.Fetch(ctx, in.ID)
	if err != nil {
		if errors.Is(err, operator.ErrIngestUnavailable) {
			return nil, httpcore.Conflict("Downloading isn't available on this install",
				"This build can't run the download tooling. Local folders can still be scanned.")
		}
		if errors.Is(err, acquisition.ErrSourceCheckInProgress) {
			return nil, httpcore.Conflict("Already looking for clips", "This source is already being checked.")
		}
		if errors.Is(err, cliplibrary.ErrSourceDisabled) {
			return nil, SourceDisabledError()
		}
		if errors.Is(err, acquisition.ErrFetchSourceNotFound) {
			return nil, httpcore.NotFound("Source not found", "That source isn't registered — it may have been removed.")
		}
		return nil, huma.Error502BadGateway("fetch remote filler", err)
	}
	total, added, updated, pruned, err := s.filler.Sync(ctx)
	// Same reason as the sync route: a source the operator switched off is an answer, not a
	// gateway failure. This is the "Fetch now" on the very row carrying the switch.
	if errors.Is(err, cliplibrary.ErrSourceDisabled) {
		return nil, SourceDisabledError()
	}
	if err != nil {
		return nil, huma.Error502BadGateway("sync filler catalog", err)
	}
	out := &fetchFillerSourceOutput{}
	out.Body.SourceID = in.ID
	out.Body.SourcesPolled = fetchResult.SourcesPolled
	out.Body.Queued = fetchResult.Queued
	out.Body.Skipped = fetchResult.Skipped
	out.Body.MaxPerCheck = fetchResult.MaxPerCheck
	out.Body.StoppedBy = fetchResult.StoppedBy
	out.Body.Outcomes = fillerSourceCheckSummaryDTO(fetchResult.Outcomes)
	out.Body.Total, out.Body.Added, out.Body.Updated, out.Body.Pruned = total, added, updated, pruned
	return out, nil
}
