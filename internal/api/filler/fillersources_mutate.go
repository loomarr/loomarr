package filler

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/filler/acquisition"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/filler/source"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// addFillerSourceInput registers a source of any addable kind.
type addFillerSourceInput struct {
	Body struct {
		// ⚠ Required, and validated PER KIND below. The archive identifier rule — which rejects
		// anything containing a dot — would 400 a YouTube playlist URL on a message about
		// archive.org collections. One validator cannot serve two vocabularies.
		Kind    string `json:"kind" doc:"The provider's id, as listed by GET /v1/filler/providers, whose sources are added by typing an address"`
		URI     string `json:"uri" minLength:"1" doc:"An archive.org collection identifier/URL, a YouTube playlist or channel URL, or a folder path Loomarr can read (a watched folder, or a folder of the household library)"`
		Label   string `json:"label,omitempty" doc:"Operator-facing name; falls back to the identifier"`
		Country string `json:"country,omitempty" maxLength:"2" doc:"ISO 3166-1 alpha-2 source country"`
	}
}

// normalizeSourceURI validates a URI against its OWN provider's rules and returns the canonical
// identity to store, or "" if the provider cannot use it.
//
// ⚠ The providers disagree about what a valid URI even looks like: an archive identifier is a
// plain slug and is REJECTED if it contains a dot, while a YouTube playlist URL is nothing but
// dots and slashes. That is why validation is the provider's rather than one shared check: the
// archive rule applied to everything would make YouTube impossible to register.
//
// ⚠ The folder and the library are ADDABLE, many of each (§10). The seeded singleton rows
// materialised by migration 00029 are a separate, derived case.
func (s *Handlers) normalizeSourceURI(kind, raw string) string {
	addresser, ok := s.sourceProviders.Addresser(source.ID(kind))
	if !ok {
		return ""
	}
	return addresser.Address(raw)
}

type addFillerSourceOutput struct {
	Body RemoteSourceDTO
}

// addFillerSource registers a remote collection. Downloading is a separate, gated act.
func (s *Handlers) addFillerSource(ctx context.Context, in *addFillerSourceInput) (*addFillerSourceOutput, error) {
	if s.store == nil {
		return nil, huma.Error501NotImplemented("no store configured")
	}
	kind := strings.TrimSpace(in.Body.Kind)
	// ⚠ Every addable kind is accepted, `folder` and `library` included: refusing them would make
	// the UI's own "Add a source" dialog fail for two of the three kinds it offers.
	id := s.normalizeSourceURI(kind, in.Body.URI)
	if id == "" {
		// The message names the shape THIS provider accepts; a shared "invalid URI" would tell a
		// YouTube user about archive.org collections.
		problem := s.providerDescription(kind).BadAddress
		if problem.Title == "" {
			problem = source.Problem{Title: "That kind of source can't be added", Detail: "Choose one of the source services listed."}
		}
		return nil, httpcore.BadRequest(problem.Title, problem.Detail)
	}
	var resolvedTitle string
	if _, discoverable := s.sourceProviders.Finder(source.ID(kind)); discoverable {
		if s.filler == nil {
			return nil, huma.Error501NotImplemented("source verification is not available on this instance")
		}
		if err := s.requireFillerProviderEnabled(ctx, kind); err != nil {
			return nil, err
		}
		resolved, err := s.filler.ResolveSource(ctx, kind, id)
		if err != nil {
			if errors.Is(err, acquisition.ErrInvalidSourceReference) {
				return nil, httpcore.Unprocessable("That source could not be verified", err.Error())
			}
			return nil, huma.Error502BadGateway("verify source", err)
		}
		if resolved.Provider != source.ID(kind) {
			return nil, huma.Error502BadGateway("verify source", errors.New("provider returned the wrong source kind"))
		}
		// The provider's own address rule reads the canonical target, whichever of its identifier
		// and URL that rule takes.
		id = s.normalizeSourceURI(kind, resolved.CanonicalID)
		if id == "" {
			id = s.normalizeSourceURI(kind, resolved.CanonicalURL)
		}
		if id == "" {
			return nil, huma.Error502BadGateway("verify source", errors.New("provider returned an invalid canonical identifier"))
		}
		resolvedTitle = strings.TrimSpace(resolved.Title)
	}
	label := strings.TrimSpace(in.Body.Label)
	if label == "" {
		label = resolvedTitle
		if label == "" {
			label = id
		}
	}
	// ⚠ The row id is namespaced by kind: a YouTube URL and an archive slug share one table, and
	// an un-namespaced id lets one kind's row silently UPSERT over another's.
	rowID := kind + ":" + id
	// NewFillerSource, never a struct literal: `Enabled` is a bool, so a literal that omits it
	// registers the collection SWITCHED OFF (see the store).
	src := records.NewFillerSource(rowID, kind, id, label, time.Now().UTC())
	src.Geography = pod.Geography{Country: in.Body.Country}.Normalize()
	if err := src.Geography.Validate(); err != nil {
		return nil, httpcore.Unprocessable("Invalid source geography", err.Error())
	}
	if err := s.store.UpsertFillerSource(ctx, src); err != nil {
		return nil, huma.Error500InternalServerError("register filler source", err)
	}
	// Registration is also the explicit correction path for an existing source with the same
	// identity. Upsert deliberately preserves policy fields; this targeted write changes only the
	// geography the operator supplied on this request.
	if err := s.store.SetFillerSourceGeography(ctx, src.ID, src.Geography); err != nil {
		return nil, huma.Error500InternalServerError("set filler source geography", err)
	}
	return &addFillerSourceOutput{Body: RemoteSourceDTO{
		ID: src.ID, Label: src.Label, URI: src.URI, Enabled: src.Enabled,
		Country: src.Geography.Country,
	}}, nil
}

type setFillerSourceEnabledInput struct {
	ID   string `path:"id"`
	Body struct {
		Enabled bool `json:"enabled"`
		// AutomaticDownloads is omitted when this PATCH changes only the source switch or area.
		// The explicit mode avoids overloading JSON null with both "leave unchanged" and "reset".
		AutomaticDownloads *SourceAutomaticDownloadsInput `json:"automaticDownloads,omitempty"`
		Geography          *SourceGeographyDTO            `json:"geography,omitempty" doc:"Complete source coverage replacement; country-only means country-wide"`
	}
}

type SourceAutomaticDownloadsInput struct {
	Mode         string `json:"mode" enum:"defaults,custom,never"`
	EverySeconds int    `json:"everySeconds,omitempty" minimum:"60" maximum:"604800"`
	MaxPerCheck  int    `json:"maxPerCheck,omitempty" minimum:"1" maximum:"1000"`
}

type SourceAutomaticDownloadsDTO struct {
	Mode         string `json:"mode" enum:"defaults,custom,never"`
	EverySeconds int    `json:"everySeconds" minimum:"0" maximum:"604800"`
	MaxPerCheck  int    `json:"maxPerCheck" minimum:"1" maximum:"1000"`
	Summary      string `json:"summary"`
	NextCheckAt  string `json:"nextCheckAt,omitempty" doc:"RFC3339; absent while automatic checks are off"`
}

type SourceGeographyDTO struct {
	Country string `json:"country,omitempty" maxLength:"2"`
}

type setFillerSourceEnabledOutput struct {
	Body struct {
		ID                 string                       `json:"id"`
		Enabled            bool                         `json:"enabled"`
		AutomaticDownloads *SourceAutomaticDownloadsDTO `json:"automaticDownloads,omitempty"`
		Geography          *SourceGeographyDTO          `json:"geography,omitempty"`
	}
}

type setFillerProviderEnabledInput struct {
	Kind string `path:"kind" doc:"A fetched provider's id, as listed by GET /v1/filler/providers"`
	Body struct {
		Enabled bool `json:"enabled"`
	}
}

type setFillerProviderEnabledOutput struct {
	Body struct {
		Kind    string `json:"kind"`
		Enabled bool   `json:"enabled"`
	}
}

func (s *Handlers) setFillerProviderEnabled(ctx context.Context, in *setFillerProviderEnabledInput) (*setFillerProviderEnabledOutput, error) {
	if s.store == nil {
		return nil, huma.Error501NotImplemented("no store configured")
	}
	if err := s.store.SetFillerProviderEnabled(ctx, in.Kind, in.Body.Enabled); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpcore.NotFound("Provider not found", "That filler provider isn't available on this install.")
		}
		return nil, huma.Error500InternalServerError("set filler provider enabled", err)
	}
	out := &setFillerProviderEnabledOutput{}
	out.Body.Kind, out.Body.Enabled = in.Kind, in.Body.Enabled
	return out, nil
}

// setFillerSourceEnabled flips a source's switch.
//
// ⚠ The dispatch below is the one place the storage asymmetry surfaces, and it is deliberately
// here rather than in the client: `folder` is derived from configuration so its switch is a
// SETTING, while a collection is a row so its switch is a COLUMN. A UI that had to know which
// is which would encode a persistence decision in a button handler.
//
// A registered source's switch, coverage and tuning are one store command (ChangeFillerSource), so
// they land together. The folder's switch is a setting in another subsystem and cannot join that
// statement: its coverage is written after the setting is accepted, which is why a pinned switch
// refuses the whole PATCH before anything is saved.
func (s *Handlers) setFillerSourceEnabled(ctx context.Context, in *setFillerSourceEnabledInput) (*setFillerSourceEnabledOutput, error) {
	// ⚠ A derived GROUP node carries no state, so there is nothing here to write (§10 V51c). One
	// prefix test rather than a case per provider: the ids are namespaced precisely so a single
	// guard covers every group that exists now and every one added later.
	if isProviderID(in.ID) {
		return nil, httpcore.Conflict("That source has no switch",
			"This row groups the sources you've added — switch them off individually.")
	}
	var every, maxPerRun *int
	if policy := in.Body.AutomaticDownloads; policy != nil {
		switch policy.Mode {
		case "defaults":
			// nil/nil clears both durable overrides.
		case "never":
			never := 0
			every = &never
		case "custom":
			if policy.EverySeconds < 60 || policy.MaxPerCheck < 1 {
				return nil, httpcore.Unprocessable("Invalid automatic download policy",
					"Choose an interval of at least one minute and at least one clip per check.")
			}
			everyValue, maxValue := policy.EverySeconds, policy.MaxPerCheck
			every, maxPerRun = &everyValue, &maxValue
		default:
			return nil, httpcore.Unprocessable("Invalid automatic download policy",
				"Choose defaults, custom, or never.")
		}
	}

	out := &setFillerSourceEnabledOutput{}
	out.Body.ID, out.Body.Enabled = in.ID, in.Body.Enabled
	out.Body.Geography = in.Body.Geography
	// ⚠ Every refusal is decided before the first write, so a refused PATCH saves nothing. The
	// coverage is validated here and written by the branch below that owns the first refusable
	// write: the folder's switch is a setting the environment can pin, which only the write
	// itself can tell us, so there the coverage goes after it.
	var geography pod.Geography
	if in.Body.Geography != nil {
		if s.store == nil {
			return nil, huma.Error501NotImplemented("no store configured")
		}
		geography = pod.Geography{Country: in.Body.Geography.Country}.Normalize()
		if err := geography.Validate(); err != nil {
			return nil, httpcore.Unprocessable("Invalid source geography", err.Error())
		}
	}

	switch in.ID {
	case string(source.FolderID):
		if s.patchSettings == nil {
			return nil, huma.Error501NotImplemented("settings are not available on this instance")
		}
		// Through Patch rather than a bespoke setter: it validates, hot-applies and records
		// WHO changed it, so flipping this switch is audited exactly like editing the same key
		// on the Settings page. A private write path would be the one setting with no history.
		statuses := s.patchSettings(ctx,
			map[string]string{settings.FillerSourceFolderEnabled.Key(): strconv.FormatBool(in.Body.Enabled)},
			httpcore.AuditActor(ctx))
		for _, status := range statuses {
			if status == "pinned" {
				return nil, httpcore.Conflict("That switch is set by the environment",
					"FILLER_SOURCE_FOLDER_ENABLED is pinned in this deployment's configuration, so it can't be changed here.")
			}
			if status != "saved" {
				return nil, huma.Error500InternalServerError("save the drop-folder switch: "+status, nil)
			}
		}
		if in.Body.Geography != nil {
			if err := s.setSourceGeography(ctx, in.ID, geography); err != nil {
				return nil, err
			}
		}
		return out, nil
	// Every other id is a registered source, `library` included: it is scanned like any other,
	// so it has a real switch. A group node never reaches here — the prefix guard above covers
	// every id that is one (§10 V51c).
	default:
		if s.store == nil {
			return nil, huma.Error501NotImplemented("no store configured")
		}
		// One command, one statement: the switch, the coverage and the tuning land together, so a
		// refused or failed PATCH saves none of them. A switch- or geography-only PATCH names no
		// policy and so must not reset automatic-download tuning.
		change := records.FillerSourceChange{Enabled: in.Body.Enabled}
		if in.Body.Geography != nil {
			change.Geography = &geography
		}
		if in.Body.AutomaticDownloads != nil {
			change.FetchPolicy = &records.FillerFetchPolicy{EverySeconds: every, MaxPerRun: maxPerRun}
		}
		if err := s.store.ChangeFillerSource(ctx, in.ID, change); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpcore.NotFound("Source not found", "That source isn't registered — it may have been removed.")
			}
			return nil, huma.Error500InternalServerError("change filler source", err)
		}
		if in.Body.AutomaticDownloads == nil {
			return out, nil
		}
		effectiveEvery := s.fillerFetchEvery()
		effectiveMax := s.fillerFetchMaxPerCheck()
		if every != nil {
			effectiveEvery = time.Duration(*every) * time.Second
		}
		if maxPerRun != nil {
			effectiveMax = *maxPerRun
		}
		out.Body.AutomaticDownloads = &SourceAutomaticDownloadsDTO{
			Mode: in.Body.AutomaticDownloads.Mode, EverySeconds: int(effectiveEvery.Seconds()), MaxPerCheck: effectiveMax,
			Summary: automaticDownloadSummary(in.Body.AutomaticDownloads.Mode, effectiveEvery, effectiveMax),
		}
		return out, nil
	}
}

// setSourceGeography writes a validated coverage and names an unknown source the way every
// source route does.
func (s *Handlers) setSourceGeography(ctx context.Context, id string, geography pod.Geography) error {
	if err := s.store.SetFillerSourceGeography(ctx, id, geography); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return httpcore.NotFound("Source not found", "That source isn't registered — it may have been removed.")
		}
		return huma.Error500InternalServerError("set filler source geography", err)
	}
	return nil
}

type deleteFillerSourceInput struct {
	ID string `path:"id"`
}

// deleteFillerSource forgets a registered remote. Its clips stay.
func (s *Handlers) deleteFillerSource(ctx context.Context, in *deleteFillerSourceInput) (*struct{}, error) {
	if s.store == nil {
		return nil, huma.Error501NotImplemented("no store configured")
	}
	// ⚠ A derived GROUP node is not a registration either (§10 V51c). Deleting Archive.org would
	// have to mean deleting every collection beneath it — a bulk act an operator should perform
	// deliberately, one row at a time, seeing exactly what goes.
	if isProviderID(in.ID) {
		return nil, httpcore.Conflict("That source can't be removed here",
			"This row groups the sources you've added — remove them individually.")
	}
	switch in.ID {
	case string(source.FolderID), string(source.LibraryID):
		// ⚠ The config-backed SINGLETONS are not registrations, so there is nothing to
		// un-register. Deleting one would have to mean "unset filler.dir", which belongs in
		// Settings where the consequence is legible — and since migration 00029 materialises
		// these rows, a delete here would be undone by the next migrate anyway.
		return nil, httpcore.Conflict("That source can't be removed here",
			"This row describes how Loomarr is configured. Change it in Settings instead.")
	}
	if err := s.store.DeleteFillerSource(ctx, in.ID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpcore.NotFound("Source not found", "That source isn't registered — it may already have been removed.")
		}
		return nil, huma.Error500InternalServerError("delete filler source", err)
	}
	return nil, nil
}
