package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/taxonomy"
)

// The taxonomy API (§10 V45a): read the tag graph, serve it as a tagging vocabulary, and let an
// admin edit it. Every WRITE enters the store's atomic graph-edit seam: validation, the row change,
// closure rebuild, and clip-rollup rebuild commit together, so a request cannot leave two taxonomy
// generations visible.
//
// ⚠ This is where an unknown-slug is REJECTED on write (the layering note in schedule/policy.go): a
// FillerSelection's categories are opaque slugs to the domain, validated here against the live graph.

// TaxonDTO is one node of the taxonomy graph on the wire.
type TaxonDTO struct {
	Slug   string `json:"slug" doc:"Stable machine id — the token the tagger emits and clip_tags references"`
	Label  string `json:"label" doc:"Human display form"`
	Parent string `json:"parent,omitempty" doc:"Parent slug on the same axis; empty for an axis root"`
	Axis   string `json:"axis" enum:"product,format,seasonal,audience-cue" doc:"The independent dimension this taxon lives on"`
	// Synonyms/RetiredAliases are the resolve index: near-miss forms and former slugs that still
	// ground to this Slug, so the tagger is LLM-friendly and a rename never drops tagged clips.
	Synonyms       []string `json:"synonyms,omitempty"`
	RetiredAliases []string `json:"retiredAliases,omitempty" doc:"Former slugs that still resolve here after a rename (§10)"`
	AssertedClips  int      `json:"assertedClips" doc:"Playable catalog clips directly asserted with this taxon"`
	MatchedClips   int      `json:"matchedClips" doc:"Playable catalog clips matching this taxon, including descendant rollups"`
	StoredClips    int      `json:"storedClips" doc:"All stored clips directly assigned this taxon, including held, removed, and compilation records; blocks deletion"`
}

// CreateTaxonDTO is the create command. Usage belongs only to TaxonDTO's read model; separating
// command and query shapes prevents generated clients from asking an operator to submit counts the
// server computes.
type CreateTaxonDTO struct {
	Slug           string   `json:"slug" doc:"Stable machine id — immutable after creation"`
	Label          string   `json:"label"`
	Parent         string   `json:"parent,omitempty"`
	Axis           string   `json:"axis" enum:"product,format,seasonal,audience-cue"`
	Synonyms       []string `json:"synonyms,omitempty"`
	RetiredAliases []string `json:"retiredAliases,omitempty"`
}

// UpdateTaxonDTO omits the immutable slug: the path identifies the node and is authoritative.
// Keeping it out of the schema also means a generated client cannot accidentally imply rename.
type UpdateTaxonDTO struct {
	Label          string   `json:"label"`
	Parent         string   `json:"parent,omitempty"`
	Axis           string   `json:"axis" enum:"product,format,seasonal,audience-cue"`
	Synonyms       []string `json:"synonyms,omitempty"`
	RetiredAliases []string `json:"retiredAliases,omitempty"`
}

// TaxonomyImpactCommandDTO describes one prospective advanced vocabulary edit. A single command
// shape keeps create/update/delete impact behind one generated client operation; the server still
// validates the operation-specific requirements and the complete prospective graph.
type TaxonomyImpactCommandDTO struct {
	Operation      string   `json:"operation" enum:"create,update,delete"`
	Slug           string   `json:"slug" doc:"The new slug for create, or existing slug for update/delete"`
	Label          string   `json:"label,omitempty"`
	Parent         string   `json:"parent,omitempty"`
	Axis           string   `json:"axis,omitempty" enum:"product,format,seasonal,audience-cue,"`
	Synonyms       []string `json:"synonyms,omitempty"`
	RetiredAliases []string `json:"retiredAliases,omitempty"`
}

type TaxonomyImpactNodeDTO struct {
	Slug  string `json:"slug"`
	Label string `json:"label"`
}

type TaxonomyImpactChannelDTO struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Number int    `json:"number"`
}

type TaxonomyImpactDTO struct {
	DirectStoredClips      int                        `json:"directStoredClips" doc:"Stored clips directly asserting the selected taxon"`
	DescendantStoredClips  int                        `json:"descendantStoredClips" doc:"Distinct stored clips asserting a descendant"`
	AffectedStoredClips    int                        `json:"affectedStoredClips" doc:"Distinct stored clips whose derived lineage may change"`
	AffectedPlayableClips  int                        `json:"affectedPlayableClips" doc:"Airable clips whose channel fit may change"`
	Descendants            []TaxonomyImpactNodeDTO    `json:"descendants"`
	SavedChannelSelections []TaxonomyImpactChannelDTO `json:"savedChannelSelections" doc:"Saved channel selections referencing the selected taxon or descendants"`
	ResolverTermsAdded     []string                   `json:"resolverTermsAdded" doc:"New classifier spellings that will resolve after the edit"`
	ResolverTermsRemoved   []string                   `json:"resolverTermsRemoved" doc:"Classifier spellings that will stop resolving after the edit"`
	DeleteBlocked          bool                       `json:"deleteBlocked" doc:"True when direct stored assertions must be retagged before deletion"`
}

// TaxonomyAxisCoverageDTO reports unique playable clips covered on one independent dimension.
// It is not the sum of per-taxon counts because a clip may assert several tags on the same axis.
type TaxonomyAxisCoverageDTO struct {
	Axis          string `json:"axis" enum:"product,format,seasonal,audience-cue"`
	TaggedClips   int    `json:"taggedClips" doc:"Playable clips with at least one direct assertion on this axis"`
	UntaggedClips int    `json:"untaggedClips" doc:"Playable clips without a direct assertion on this axis; absence may be valid for sparse cue axes"`
}

func taxonToDTO(t taxonomy.Taxon) TaxonDTO {
	return TaxonDTO{
		Slug: t.Slug, Label: t.Label, Parent: t.Parent, Axis: string(t.Axis),
		Synonyms: t.Synonyms, RetiredAliases: t.RetiredAliases,
	}
}

func createDTOToTaxon(d CreateTaxonDTO) taxonomy.Taxon {
	return taxonomy.Canonicalize(taxonomy.Taxon{
		Slug: d.Slug, Label: d.Label, Parent: d.Parent, Axis: taxonomy.Axis(d.Axis),
		Synonyms: d.Synonyms, RetiredAliases: d.RetiredAliases,
	})
}

func updateDTOToTaxon(slug string, d UpdateTaxonDTO) taxonomy.Taxon {
	return taxonomy.Canonicalize(taxonomy.Taxon{
		Slug: slug, Label: d.Label, Parent: d.Parent, Axis: taxonomy.Axis(d.Axis),
		Synonyms: d.Synonyms, RetiredAliases: d.RetiredAliases,
	})
}

// registerTaxonomy mounts /v1/taxonomy* and the tagging vocabulary (§10 V45a). Read is visible to any
// authenticated user (the catalog UI shows tags); every edit is admin (the tag vocabulary is an
// operator concern, like the rest of filler ingestion).
func (s *Server) registerTaxonomy(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "list-taxonomy", Method: http.MethodGet, Path: "/v1/taxonomy",
		Summary: "The clip tag vocabulary (the taxonomy graph)",
		Description: "The operator-editable tag graph the filler tagger grounds against and curation matches over (§10 V45a). " +
			"Ordered by axis then slug. Read-only, so any authenticated user may call it.",
		Tags: []string{"filler"},
	}, RoleMember), s.listTaxonomy)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "preview-taxonomy-edit", Method: http.MethodPost, Path: "/v1/taxonomy/impact",
		Summary: "Preview a vocabulary edit", Description: "Admin only. Validates the prospective graph and reports stored clips, descendants, saved channel selections, and classifier terms affected without mutating anything.",
		Tags: []string{"filler"},
	}, RoleAdmin), s.previewTaxonomyEdit)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "create-taxon", Method: http.MethodPost, Path: "/v1/taxonomy",
		Summary: "Add a taxon", Description: "Admin only. Updates the graph and catalog tags atomically (§10 V45a).",
		Tags: []string{"filler"},
	}, RoleAdmin), s.createTaxon)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "update-taxon", Method: http.MethodPut, Path: "/v1/taxonomy/{slug}",
		Summary: "Edit a taxon", Description: "Admin only. Updates the graph and catalog tags atomically (§10 V45a).",
		Tags: []string{"filler"},
	}, RoleAdmin), s.updateTaxon)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "delete-taxon", Method: http.MethodDelete, Path: "/v1/taxonomy/{slug}",
		Summary: "Remove a taxon", Description: "Admin only. Children are reparented and catalog tags update atomically (§10 V45a).",
		Tags: []string{"filler"},
	}, RoleAdmin), s.deleteTaxon)
}

type listTaxonomyInput struct{}
type listTaxonomyOutput struct {
	Body struct {
		Taxa              []TaxonDTO                `json:"taxa"`
		TotalClips        int                       `json:"totalClips"`
		TaggedClips       int                       `json:"taggedClips"`
		UnclassifiedClips int                       `json:"unclassifiedClips"`
		AxisCoverage      []TaxonomyAxisCoverageDTO `json:"axisCoverage"`
	}
}

type previewTaxonomyEditInput struct{ Body TaxonomyImpactCommandDTO }
type previewTaxonomyEditOutput struct{ Body TaxonomyImpactDTO }

func (s *Server) previewTaxonomyEdit(ctx context.Context, in *previewTaxonomyEditInput) (*previewTaxonomyEditOutput, error) {
	edit, err := impactCommandToEdit(in.Body)
	if err != nil {
		return nil, err
	}
	var impact TaxonomyImpact
	if s.taxonomy != nil {
		impact, err = s.taxonomy.Preview(ctx, edit)
	} else if s.store != nil {
		impact.Store, err = s.store.PreviewTaxonomyEdit(ctx, edit)
	} else {
		return nil, huma.Error501NotImplemented("no taxonomy editor configured")
	}
	if err != nil {
		return nil, mapTaxonomyEditError(err)
	}
	out := &previewTaxonomyEditOutput{}
	out.Body = taxonomyImpactToDTO(impact, edit.Delete)
	return out, nil
}

func impactCommandToEdit(command TaxonomyImpactCommandDTO) (store.TaxonomyEdit, error) {
	taxon := taxonomy.Canonicalize(taxonomy.Taxon{
		Slug: command.Slug, Label: command.Label, Parent: command.Parent, Axis: taxonomy.Axis(command.Axis),
		Synonyms: command.Synonyms, RetiredAliases: command.RetiredAliases,
	})
	switch command.Operation {
	case "create":
		return store.TaxonomyEdit{Create: true, Taxon: taxon}, nil
	case "update":
		return store.TaxonomyEdit{Slug: taxon.Slug, Taxon: taxon}, nil
	case "delete":
		return store.TaxonomyEdit{Delete: true, Slug: taxon.Slug}, nil
	default:
		return store.TaxonomyEdit{}, errUnprocessable("Unknown taxonomy operation", "Choose create, update, or delete.")
	}
}

func taxonomyImpactToDTO(impact TaxonomyImpact, deleting bool) TaxonomyImpactDTO {
	out := TaxonomyImpactDTO{
		DirectStoredClips:      impact.Store.DirectStoredClips,
		DescendantStoredClips:  impact.Store.DescendantStoredClips,
		AffectedStoredClips:    impact.Store.AffectedStoredClips,
		AffectedPlayableClips:  len(impact.Store.PlayableClipHashes),
		ResolverTermsAdded:     impact.Store.ResolverTermsAdded,
		ResolverTermsRemoved:   impact.Store.ResolverTermsRemoved,
		DeleteBlocked:          deleting && impact.Store.DirectStoredClips > 0,
		Descendants:            make([]TaxonomyImpactNodeDTO, 0, len(impact.Store.Descendants)),
		SavedChannelSelections: make([]TaxonomyImpactChannelDTO, 0, len(impact.Channels)),
	}
	for _, descendant := range impact.Store.Descendants {
		out.Descendants = append(out.Descendants, TaxonomyImpactNodeDTO{Slug: descendant.Slug, Label: descendant.Label})
	}
	for _, channel := range impact.Channels {
		out.SavedChannelSelections = append(out.SavedChannelSelections, TaxonomyImpactChannelDTO{
			ID: channel.ID, Name: channel.Name, Number: channel.Number,
		})
	}
	return out
}

func (s *Server) listTaxonomy(ctx context.Context, _ *listTaxonomyInput) (*listTaxonomyOutput, error) {
	taxa, err := s.store.ListTaxa(ctx)
	if err != nil {
		return nil, err
	}
	out := &listTaxonomyOutput{}
	usage, err := s.store.TaxonomyUsage(ctx)
	if err != nil {
		return nil, err
	}
	out.Body.TotalClips = usage.TotalClips
	out.Body.TaggedClips = usage.TaggedClips
	out.Body.UnclassifiedClips = usage.TotalClips - usage.TaggedClips
	for _, axis := range []taxonomy.Axis{
		taxonomy.AxisProduct,
		taxonomy.AxisFormat,
		taxonomy.AxisSeasonal,
		taxonomy.AxisAudienceCue,
	} {
		covered := usage.ByAxis[axis]
		out.Body.AxisCoverage = append(out.Body.AxisCoverage, TaxonomyAxisCoverageDTO{
			Axis: string(axis), TaggedClips: covered, UntaggedClips: usage.TotalClips - covered,
		})
	}
	out.Body.Taxa = make([]TaxonDTO, 0, len(taxa))
	for _, t := range taxa {
		dto := taxonToDTO(t)
		dto.AssertedClips = usage.ByTaxon[t.Slug].Asserted
		dto.MatchedClips = usage.ByTaxon[t.Slug].Matched
		dto.StoredClips = usage.ByTaxon[t.Slug].Stored
		out.Body.Taxa = append(out.Body.Taxa, dto)
	}
	return out, nil
}

type createTaxonInput struct {
	Body CreateTaxonDTO
}

func (s *Server) createTaxon(ctx context.Context, in *createTaxonInput) (*struct{ Body TaxonDTO }, error) {
	if in.Body.Slug == "" || in.Body.Label == "" || in.Body.Axis == "" {
		return nil, errUnprocessable("Incomplete taxon", "A taxon needs a slug, a label, and an axis.")
	}
	taxon := createDTOToTaxon(in.Body)
	if err := s.applyTaxonomyEdit(ctx, store.TaxonomyEdit{Create: true, Taxon: taxon}); err != nil {
		return nil, err
	}
	return &struct{ Body TaxonDTO }{Body: taxonToDTO(taxon)}, nil
}

type updateTaxonInput struct {
	Slug string `path:"slug"`
	Body UpdateTaxonDTO
}

func (s *Server) updateTaxon(ctx context.Context, in *updateTaxonInput) (*struct{ Body TaxonDTO }, error) {
	if in.Body.Label == "" || in.Body.Axis == "" {
		return nil, errUnprocessable("Incomplete taxon", "A taxon needs a label and an axis.")
	}
	taxon := updateDTOToTaxon(in.Slug, in.Body)
	if err := s.applyTaxonomyEdit(ctx, store.TaxonomyEdit{Slug: in.Slug, Taxon: taxon}); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, errNotFound("Taxon not found", "That tag doesn't exist — it may have already been removed.")
		}
		return nil, err
	}
	return &struct{ Body TaxonDTO }{Body: taxonToDTO(taxon)}, nil
}

type deleteTaxonInput struct {
	Slug string `path:"slug"`
}

func (s *Server) deleteTaxon(ctx context.Context, in *deleteTaxonInput) (*struct{}, error) {
	err := s.applyTaxonomyEdit(ctx, store.TaxonomyEdit{Delete: true, Slug: in.Slug})
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Taxon not found", "That tag doesn't exist — it may have already been removed.")
	}
	if err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

func (s *Server) applyTaxonomyEdit(ctx context.Context, edit store.TaxonomyEdit) error {
	var err error
	if s.taxonomy != nil {
		_, err = s.taxonomy.Apply(ctx, edit)
	} else {
		err = s.store.ApplyTaxonomyEdit(ctx, edit, time.Now())
	}
	return mapTaxonomyEditError(err)
}

func mapTaxonomyEditError(err error) error {
	switch {
	case errors.Is(err, taxonomy.ErrInvalidForest):
		return errUnprocessable("Invalid taxonomy", err.Error())
	case errors.Is(err, store.ErrTaxonConflict):
		return errConflict("Taxonomy changed", err.Error())
	default:
		return err
	}
}
