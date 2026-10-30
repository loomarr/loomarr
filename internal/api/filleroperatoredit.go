package api

import (
	"context"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/fillerenrichment"
	"github.com/loomarr/loomarr/internal/taxonomy"
)

// The halves an operator's clip edit shares between the single-clip PATCH and the bulk bar, so the
// two surfaces cannot disagree about what grounding a tag or recording an edit means (§10 V45a).

// groundOperatorTags grounds an operator's tag set against the LIVE taxonomy. An unknown slug 422s
// before any tag is written — never silently persisted, never silently dropped. (The bulk bar
// grounds before touching any clip; the PATCH has already applied a rename by this point.)
func (s *Server) groundOperatorTags(ctx context.Context, raw []string) ([]string, *taxonomy.Forest, error) {
	taxa, err := s.store.ListTaxa(ctx)
	if err != nil {
		return nil, nil, err
	}
	forest := taxonomy.New(taxa)
	leaves, unknown, ok := forest.Ground(raw)
	if !ok {
		return nil, nil, errUnprocessable("Unknown tag",
			fmt.Sprintf("The tag %q is not in the classification vocabulary. Add it under Filler → Advanced → Classification vocabulary first, or choose an existing tag.", unknown))
	}
	return leaves, forest, nil
}

// operatorEvidence records one clip's operator edit as enrichment evidence, axis by axis.
type operatorEvidence struct {
	s         *Server
	clipHash  string
	reference string
	at        time.Time
}

func (e operatorEvidence) record(ctx context.Context, axis fillerenrichment.Axis, value fillerenrichment.Value) error {
	return e.apply(ctx, axis, value, "")
}

// recordTags records the grounded tag set on every taxonomy axis, so an axis the edit cleared reads
// as observed-empty rather than keeping an older machine guess.
func (e operatorEvidence) recordTags(ctx context.Context, forest *taxonomy.Forest, leaves []string) error {
	byAxis := make(map[fillerenrichment.Axis][]string)
	for _, leaf := range leaves {
		if taxon, ok := forest.Get(leaf); ok {
			axis := fillerenrichment.Axis(taxon.Axis)
			byAxis[axis] = append(byAxis[axis], leaf)
		}
	}
	for _, axis := range fillerenrichment.TaxonomyAxes() {
		if err := e.apply(ctx, axis, fillerenrichment.Value{Tags: byAxis[axis]}, fillerenrichment.OperatorTaxonomyVersion); err != nil {
			return err
		}
	}
	return nil
}

func (e operatorEvidence) apply(ctx context.Context, axis fillerenrichment.Axis, value fillerenrichment.Value, taxonomyVersion string) error {
	state := fillerenrichment.Operator(e.clipHash, axis, value, e.reference, taxonomyVersion, e.at)
	_, _, err := e.s.store.ApplyFillerEnrichment(ctx, state, e.at)
	return err
}
