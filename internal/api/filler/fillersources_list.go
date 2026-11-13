package filler

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/filler/source"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

func (s *Handlers) listFillerSources(ctx context.Context, _ *struct{}) (*fillerSourcesOutput, error) {
	if s.store == nil {
		return nil, huma.Error501NotImplemented("no store configured")
	}
	// Count by provenance. The `source` column is free text written by whatever discovered the
	// clip, so an unrecognized value is possible and must not vanish — see the Total field.
	//
	// ⚠ Counted in SQL: loading every column of every clip row to build a map of integers and a
	// total would dominate the cost of this page on a large catalog. The filter is the zero
	// filter, so the default exclusions (removed, held) apply.
	bySource, err := s.store.CountClipsBySource(ctx, clipstore.ClipFilter{})
	if err != nil {
		return nil, huma.Error500InternalServerError("count clips", err)
	}
	// The Incoming count must use the same lifecycle projection as the page it links to. Counting
	// every held row includes terminal composite parents; counting only ordinary held clips drops
	// active reels.
	incomingBySource, err := s.store.CountIncomingConveyorBySource(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("count incoming clips", err)
	}
	totalClips := 0
	for _, n := range bySource {
		totalClips += n
	}

	// Report the filesystem root this generation is actually operating against. Settings may
	// already show a different desired root while restart is pending; presenting that desired
	// path here would claim scan/fetch actions target a directory they do not yet use.
	dir := s.fillerLayout.ClipDir()

	// An unanswerable read is ON: the handle answers the setting's declared default, the same
	// as the syncer's own gate. Treating it as "off" would render the drop-folder switched
	// off on a page whose whole job is telling the operator why their catalog is empty.
	folderEnabled := settings.FillerSourceFolderEnabled.Get(s.liveSettings)
	providers, err := s.store.ListFillerProviders(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("list filler providers", err)
	}
	providerEnabled := make(map[string]bool, len(providers))
	for _, provider := range providers {
		providerEnabled[provider.Kind] = provider.Enabled
	}

	// The operator-registered sources — archive collections and YouTube playlists — as PEER rows
	// (V37). A read failure is NOT fatal: the two config-backed rows below are the answer to "why
	// is my catalog empty", and losing that whole page because the registry did not load would be
	// a poor trade.
	//
	// ⚠ The SEEDED singletons are skipped here — not the kinds. Migration 00029 materialises one
	// `folder` and one `library` row so the list is complete on a fresh install, but their live
	// state comes from configuration (the folder's path from `filler.dir`, its switch from
	// `filler.source.folder.enabled`), so emitting the stored copy too would list the drop-folder
	// TWICE with the stale one free to disagree with the setting.
	//
	// ⚠ **The seeded ROW is skipped, never the whole kind.** Many folders and libraries are
	// addable (§10), and skipping a kind would hide every one of them — the operator adds a
	// source, gets a 200, and never sees it again. The seeded folder is the one whose URI IS the
	// configured directory; a folder pointing anywhere else is a genuine peer. The seeded library
	// row carries a blank URI (nothing was ever configured into it) and is skipped on that basis,
	// with the provenance row below covering older clips.
	// ⚠ Split by whether the kind ROLLS UP (§10 V51c): ungrouped rows stay top-level peers, and
	// the provider kinds are collected per-provider so the group node can be emitted above its
	// own children in one pre-ordered pass.
	var registered []FillerSourceDTO
	byProvider := map[string][]FillerSourceDTO{}
	home := HomeGeography(s.liveSettings).Normalize()
	if srcs, srcErr := s.store.ListFillerSources(ctx); srcErr != nil {
		s.log.Warn("list filler sources", "err", srcErr)
	} else {
		for _, src := range srcs {
			// ⚠ A row with a blank URI is a seeded placeholder, never configured into, and is skipped
			// whatever its provider. The seeded blank-URI `youtube` row (migration 00034) has no URI
			// and nothing to fetch; the DERIVED group node (V51c) is what represents YouTube, so
			// skipping the stored row removes a duplicate instead of removing YouTube from the list.
			if !src.Configured() {
				continue
			}
			if src.Kind == string(source.FolderID) && src.URI == dir {
				continue // the config-backed drop-folder; rendered from configuration above
			}
			label := src.Label
			if label == "" {
				label = src.URI
			}
			eligible := src.ServesHome(home)
			row := FillerSourceDTO{
				ID:               src.ID,
				Kind:             src.Kind,
				Enabled:          src.Enabled,
				EffectiveEnabled: src.EffectiveEnabled(),
				ProviderEnabled:  src.ProviderEnabled,
				OnForPlace:       src.OnForPlace,
				Switchable:       true,
				Removable:        true,
				Target:           label,
				URI:              src.URI,
				PageURL:          s.sourcePageURL(src.Kind, src.URI),
				Detail:           s.sourceDetail(src.Kind, src.URI),
				Count:            bySource[src.ID],
				Incoming:         incomingBySource[src.ID],
				Configured:       true,
				// ⚠ SCANNED sources are refreshed by the sync, DOWNLOADED ones by ingest — and
				// both are reachable through this row's "Fetch now", so both report fetchable.
				// A scanned folder whose button did nothing would read as broken; the store's
				// `Fetchable()`/`Scannable()` pair is what keeps a folder out of a PULL plan,
				// which is the distinction that actually matters.
				Fetchable: canFetchRow(src, s.filler != nil) && eligible,
				// ⚠ Only a provider that implements Searcher can be searched in place. A "search"
				// box on a YouTube playlist would have nothing to query: yt-dlp enumerates a
				// playlist, it does not search YouTube, and offering the box would be a control
				// that returns nothing forever. A folder or library is not searchable for the same
				// reason.
				Searchable:         s.sourceSearchable(src.Kind),
				AutomaticDownloads: s.sourceAutomaticDownloads(src, eligible),
				LastCheck:          fillerSourceCheckSummaryDTO(src.LastCheckSummary),
			}
			// Exact source attribution (§10 V57), for downloaded and scanned sources alike. Older
			// kind-only provenance remains in the folder/legacy aggregate rather than being guessed
			// onto one of several registered rows.
			if !src.LastCheckedAt.IsZero() {
				row.LastCheckedAt = src.LastCheckedAt.UTC().Format(time.RFC3339)
			}
			// ⚠ Sent through UNCHANGED, including empty. Empty means UNKNOWN and the client
			// renders nothing — never a reassuring default. Substituting "public domain" here
			// would have Loomarr asserting a legal fact about ~92% of archive.org items that
			// declare no licence at all.
			row.License = src.License
			row.Country = src.Geography.Country
			// ⚠ The parent is DERIVED from the kind here, at the one place a row is built. Any
			// other arrangement — a stored column, a second pass that assigns parents — is a
			// place the parent could disagree with the kind.
			if parent, grouped := s.providerFor(src.Kind); grouped {
				row.ParentID = parent
				byProvider[src.Kind] = append(byProvider[src.Kind], row)
				continue
			}
			registered = append(registered, row)
		}
	}

	out := &fillerSourcesOutput{}
	out.Body.Total = totalClips
	out.Body.Sources = []FillerSourceDTO{
		{
			ID:   string(source.FolderID),
			Kind: string(source.FolderID),
			// The folder's switch is a SETTING, because the folder itself is derived from one.
			// The handle answers the declared default (on) when the reader cannot, so a
			// settings service that cannot answer does not silently stop the scan.
			Enabled:          folderEnabled,
			EffectiveEnabled: folderEnabled,
			ProviderEnabled:  true,
			Switchable:       true,
			Target:           OrPlaceholder(dir, "not configured"),
			Detail:           "watched directly — new files appear on the next pass",
			// filler-dir is what DirSource writes; the older tunarr-local value is counted
			// here too because those clips also live in the folder — the provenance string
			// changed with §9.1, the files did not.
			Count:      bySource["filler-dir"] + bySource["tunarr-local"],
			Incoming:   incomingBySource["filler-dir"] + incomingBySource["tunarr-local"],
			Configured: dir != "",
			Fetchable:  dir != "",
		},
	}
	// The media-server PROVENANCE row (§10). ⚠ Shown only when clips actually carry that source,
	// which is why it is appended conditionally rather than declared above.
	//
	// It describes where OLDER clips came from, and it is not a source doing work: `enabled:true`
	// says "these clips count", not "a scan is running". A scanned library is an operator-added
	// row appended below with its own target, switch and counts. Rendering this one unconditionally
	// alongside those would put two different things called "library" in one list, one of them
	// inert — which is the competing-descriptions problem this file's header exists to prevent.
	library := string(source.LibraryID)
	if n, incoming := bySource[library], incomingBySource[library]; n > 0 || incoming > 0 {
		out.Body.Sources = append(out.Body.Sources, FillerSourceDTO{
			ID:         library,
			Kind:       library,
			Enabled:    true,
			Switchable: false, // provenance, not a scan — there is no work to stop
			Target:     "media server (previously scanned)",
			Detail:     "where these clips originally came from",
			Count:      n,
			Incoming:   incoming,
			Configured: true,
		})
	}
	// Each registered source is a peer row appended here and counts its own clips; clips whose
	// provenance is the older `ingest` string are counted by the folder row they physically live
	// in.
	out.Body.Sources = append(out.Body.Sources, registered...)
	// The provider roll-up (§10 V51c), appended last and PRE-ORDERED: each group node immediately
	// followed by its own children.
	//
	// ⚠ **Flat and pre-ordered, not nested.** A nested `children: []` would generate a recursive
	// type through orval, which it handles badly, and the frontend has no tree primitive — a
	// twirl-down renders from exactly this shape by hiding rows whose parent is collapsed. It is
	// also purely ADDITIVE, so a client that knows nothing about `group`/`parentId` renders a
	// plain flat list rather than breaking.
	for _, g := range s.providerGroups() {
		out.Body.Sources = append(out.Body.Sources, providerNode(g.id, g.kind, g.label, g.detail, providerEnabled[g.kind], byProvider[g.kind]))
		out.Body.Sources = append(out.Body.Sources, byProvider[g.kind]...)
	}
	for i := range out.Body.Sources {
		s.projectSourceReadiness(&out.Body.Sources[i], home)
	}
	return out, nil
}

func (s *Handlers) projectSourceReadiness(row *FillerSourceDTO, home pod.Geography) {
	fetched := s.sourceProviders.Fetched(source.ID(row.Kind))
	row.Actions = []FillerSourceAction{}
	if row.Group {
		// Provider rows summarize children visually but do not perform work themselves.
		row.LocationSource = FillerSourceLocationMissing
		if !row.Enabled {
			row.Readiness = FillerSourceOff
			row.Actions = []FillerSourceAction{FillerSourceEnable}
		} else if !row.Configured {
			row.Readiness = FillerSourceNotConfigured
			row.Actions = []FillerSourceAction{FillerSourceConfigure, FillerSourceDisable}
		} else {
			row.Readiness = FillerSourceReady
			row.Ready = true
			row.Actions = []FillerSourceAction{FillerSourceConfigure, FillerSourceDisable}
		}
		return
	}

	home = home.Normalize()
	effective := pod.Geography{Country: row.Country}.Normalize()
	if effective.Country == "" {
		effective = pod.Geography{Country: home.Country} // a source inherits the country, never a place
		if effective.Country == "" {
			row.LocationSource = FillerSourceLocationMissing
		} else {
			row.LocationSource = FillerSourceLocationInstallation
		}
	} else {
		row.LocationSource = FillerSourceLocationExplicit
	}
	row.EffectiveCountry = effective.Country

	switch {
	case !row.Configured:
		row.Readiness = FillerSourceNotConfigured
		row.Actions = []FillerSourceAction{FillerSourceConfigure}
	case fetched && !row.ProviderEnabled:
		row.Readiness = FillerSourceProviderOff
		row.Actions = []FillerSourceAction{FillerSourceEditLocation}
		if row.Removable {
			row.Actions = append(row.Actions, FillerSourceRemove)
		}
	case !row.Enabled:
		row.Readiness = FillerSourceOff
		row.Actions = []FillerSourceAction{FillerSourceEnable}
		if fetched {
			row.Actions = append(row.Actions, FillerSourceEditLocation)
		}
		if row.Removable {
			row.Actions = append(row.Actions, FillerSourceRemove)
		}
	case home.Country == "":
		row.Readiness = FillerSourceNeedsLocation
		row.Actions = []FillerSourceAction{FillerSourceSetLocation}
	case row.Country != "" && !pod.SourceGeographicallyEligible(effective, home):
		row.Readiness = FillerSourceOutOfArea
		row.Actions = []FillerSourceAction{FillerSourceEditLocation, FillerSourceDisable, FillerSourceRemove}
	case fetched && !row.Fetchable:
		row.Readiness = FillerSourceUnavailable
		row.Actions = []FillerSourceAction{FillerSourceEditLocation, FillerSourceDisable, FillerSourceRemove}
	default:
		row.Readiness = FillerSourceReady
		row.Ready = true
		if row.Fetchable {
			row.Actions = append(row.Actions, FillerSourceFetch)
		}
		if row.Searchable {
			row.Actions = append(row.Actions, FillerSourceSearch)
		}
		if row.Switchable {
			row.Actions = append(row.Actions, FillerSourceDisable)
		}
		if row.Removable {
			row.Actions = append(row.Actions, FillerSourceRemove)
		}
		if fetched {
			row.Actions = append(row.Actions, FillerSourceEditLocation)
		}
	}
}

// providerFor maps a source kind to its group id, and reports whether that kind rolls up at all.
func (s *Handlers) providerFor(kind string) (string, bool) {
	for _, g := range s.providerGroups() {
		if g.kind == kind {
			return g.id, true
		}
	}
	return "", false
}

// providerNode builds one derived group row from the children beneath it.
//
// ⚠ **Emitted even with ZERO children**, which is what keeps the top-level row count stable when
// an operator deletes their last collection: the service does not vanish from the page, it becomes
// an empty state inviting them to add another. It is also the answer to "where did Archive.org
// go?" — the same reasoning that keeps the unconfigured drop-folder row visible.
func providerNode(id, kind, label, detail string, enabled bool, children []FillerSourceDTO) FillerSourceDTO {
	node := FillerSourceDTO{
		ID: id, Kind: kind, Group: true,
		Target: label, Detail: detail,
		Enabled: enabled, EffectiveEnabled: enabled, ProviderEnabled: enabled,
		Switchable: true,
		// Not removable: there is no registration to forget. Deleting Archive.org would have to
		// mean deleting every collection under it, which is a bulk act an operator should perform
		// deliberately, one row at a time, seeing what goes.
		Removable: false,
		// Nothing to fetch or search AT the provider: a group has no URI. Its children each carry
		// their own "Fetch now" and, for archive, their own search box.
		Fetchable:  false,
		Searchable: false,
		// ⚠ **Configured tracks whether anything is BEHIND it**, which is the distinction
		// `Configured()` was extracted for: a provider with no children is an invitation, not a
		// fault, and the tab renders those differently.
		Configured: len(children) > 0,
	}
	// The clip count is the honest SUM of what the children claim. ⚠ It is not an estimate and
	// never invents attribution: `sync.go` writes `filler-dir` for everything the folder scan
	// finds and nothing records which SOURCE a downloaded clip came from, so most children
	// legitimately report 0 — and the group reports 0 too, rather than a plausible-looking total.
	for _, c := range children {
		node.Count += c.Count
		node.Incoming += c.Incoming
		// LastCheckedAt is a read-only MAX over the children, computed HERE so no column can
		// disagree with it. Absent when no child has ever been checked, so the row renders "never"
		// rather than an epoch date nobody meant.
		//
		// ⚠ Compared as STRINGS, which is correct only because every one of them was formatted
		// by the same `.UTC().Format(time.RFC3339)` two hundred lines up: fixed width, fixed
		// offset, so lexical order is chronological order. A local-time or variable-offset
		// timestamp would break that silently, which is why the formatting has one home.
		if c.LastCheckedAt > node.LastCheckedAt {
			node.LastCheckedAt = c.LastCheckedAt
		}
	}
	return node
}

// sourceSearchable reports whether a source of this kind can be searched in place.
func (s *Handlers) sourceSearchable(kind string) bool {
	_, ok := s.sourceProviders.Searcher(source.ID(kind))
	return ok && s.filler != nil
}

// sourcePageURL is the source's page at its provider, or "" when the provider has none.
func (s *Handlers) sourcePageURL(kind, uri string) string {
	if linker, ok := s.sourceProviders.Linker(source.ID(kind)); ok {
		return linker.SourceURL(uri)
	}
	return ""
}

// sourceDetail is the operator-facing sentence under a registered source's name. Keep it true
// across every readiness and policy state; the projected status and source workspace own whether
// automatic downloads can currently run.
func (s *Handlers) sourceDetail(kind, uri string) string {
	if p, ok := s.sourceProviders.Get(source.ID(kind)); ok && p.Describe().SourceDetail != "" {
		return p.Describe().SourceDetail
	}
	return uri
}
