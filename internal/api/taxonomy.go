package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/taxonomy"
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
	err := s.store.ApplyTaxonomyEdit(ctx, edit, time.Now())
	switch {
	case errors.Is(err, taxonomy.ErrInvalidForest):
		return errUnprocessable("Invalid taxonomy", err.Error())
	case errors.Is(err, store.ErrTaxonConflict):
		return errConflict("Taxonomy changed", err.Error())
	default:
		return err
	}
}
