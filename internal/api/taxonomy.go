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
// admin edit it. Every WRITE reindexes — rebuild the closure from the edited graph, then recompute
// every clip's rollups — so a graph edit and the derived clip_tags never drift. The reindex is the
// set-based store primitive (RebuildClosure + RebuildRollups), NOT a per-clip loop.
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
}

func taxonToDTO(t taxonomy.Taxon) TaxonDTO {
	return TaxonDTO{
		Slug: t.Slug, Label: t.Label, Parent: t.Parent, Axis: string(t.Axis),
		Synonyms: t.Synonyms, RetiredAliases: t.RetiredAliases,
	}
}

func dtoToTaxon(d TaxonDTO) taxonomy.Taxon {
	return taxonomy.Taxon{
		Slug: d.Slug, Label: d.Label, Parent: d.Parent, Axis: taxonomy.Axis(d.Axis),
		Synonyms: d.Synonyms, RetiredAliases: d.RetiredAliases,
	}
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
		Summary: "Add a taxon", Description: "Admin only. Reindexes the catalog's rolled-up tags (§10 V45a).",
		Tags: []string{"filler"},
	}, RoleAdmin), s.createTaxon)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "update-taxon", Method: http.MethodPut, Path: "/v1/taxonomy/{slug}",
		Summary: "Edit a taxon", Description: "Admin only. Reindexes the catalog's rolled-up tags (§10 V45a).",
		Tags: []string{"filler"},
	}, RoleAdmin), s.updateTaxon)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "delete-taxon", Method: http.MethodDelete, Path: "/v1/taxonomy/{slug}",
		Summary: "Remove a taxon", Description: "Admin only. Children are reparented to the grandparent; reindexes (§10 V45a).",
		Tags: []string{"filler"},
	}, RoleAdmin), s.deleteTaxon)
}

type listTaxonomyInput struct{}
type listTaxonomyOutput struct {
	Body struct {
		Taxa []TaxonDTO `json:"taxa"`
	}
}

func (s *Server) listTaxonomy(ctx context.Context, _ *listTaxonomyInput) (*listTaxonomyOutput, error) {
	taxa, err := s.store.ListTaxa(ctx)
	if err != nil {
		return nil, err
	}
	out := &listTaxonomyOutput{}
	out.Body.Taxa = make([]TaxonDTO, 0, len(taxa))
	for _, t := range taxa {
		out.Body.Taxa = append(out.Body.Taxa, taxonToDTO(t))
	}
	return out, nil
}

type createTaxonInput struct {
	Body TaxonDTO
}

func (s *Server) createTaxon(ctx context.Context, in *createTaxonInput) (*struct{ Body TaxonDTO }, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if in.Body.Slug == "" || in.Body.Label == "" || in.Body.Axis == "" {
		return nil, errUnprocessable("Incomplete taxon", "A taxon needs a slug, a label, and an axis.")
	}
	// ⚠ A non-empty parent must EXIST, or the new taxon is orphaned (its rollups would be a dangling
	// reference the graph walk stops at — see taxonomy.Forest.Ancestors). Validate against the live
	// graph before writing, the same resolve-or-reject gate the clip PATCH uses for tags.
	if in.Body.Parent != "" {
		taxa, err := s.store.ListTaxa(ctx)
		if err != nil {
			return nil, err
		}
		if _, ok := taxonomy.New(taxa).Get(in.Body.Parent); !ok {
			return nil, errUnprocessable("Unknown parent", "The parent taxon does not exist. Create it first, or leave the parent empty for a top-level taxon.")
		}
	}
	if err := s.editTaxonAndReindex(ctx, func(now time.Time) error {
		return s.store.UpsertTaxon(ctx, dtoToTaxon(in.Body), now)
	}); err != nil {
		return nil, err
	}
	return &struct{ Body TaxonDTO }{Body: in.Body}, nil
}

type updateTaxonInput struct {
	Slug string `path:"slug"`
	Body TaxonDTO
}

func (s *Server) updateTaxon(ctx context.Context, in *updateTaxonInput) (*struct{ Body TaxonDTO }, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	// The path slug is authoritative: the body's slug is ignored so a client cannot rename via PUT
	// (a rename is a delete + create, which would need the retired-alias handling). Keep them aligned.
	in.Body.Slug = in.Slug
	if in.Body.Label == "" || in.Body.Axis == "" {
		return nil, errUnprocessable("Incomplete taxon", "A taxon needs a label and an axis.")
	}
	if err := s.editTaxonAndReindex(ctx, func(now time.Time) error {
		return s.store.UpsertTaxon(ctx, dtoToTaxon(in.Body), now)
	}); err != nil {
		return nil, err
	}
	return &struct{ Body TaxonDTO }{Body: in.Body}, nil
}

type deleteTaxonInput struct {
	Slug string `path:"slug"`
}

func (s *Server) deleteTaxon(ctx context.Context, in *deleteTaxonInput) (*struct{}, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	err := s.editTaxonAndReindex(ctx, func(_ time.Time) error {
		return s.store.DeleteTaxon(ctx, in.Slug)
	})
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Taxon not found", "That tag doesn't exist — it may have already been removed.")
	}
	if err != nil {
		return nil, err
	}
	return &struct{}{}, nil
}

// editTaxonAndReindex applies a graph edit then REINDEXES — rebuild the closure from the edited
// graph, then recompute every clip's rollups (§10 V45a). Synchronous here so an operator's edit is
// reflected immediately; the scheduled reindex job is the eventual-convergence backstop for a rebuild
// that fails transiently. The reindex is the set-based store primitive, never a per-clip loop.
//
// ⚠ Order matters: RebuildClosure reads the edited `taxa`, RebuildRollups reads the fresh closure.
func (s *Server) editTaxonAndReindex(ctx context.Context, edit func(now time.Time) error) error {
	now := time.Now()
	if err := edit(now); err != nil {
		return err
	}
	taxa, err := s.store.ListTaxa(ctx)
	if err != nil {
		return err
	}
	if err := s.store.RebuildClosure(ctx, taxonomy.New(taxa), now); err != nil {
		return err
	}
	return s.store.RebuildRollups(ctx)
}
