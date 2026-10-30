package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerenrichment"
	"github.com/loomarr/loomarr/internal/taxonomy"
)

// The halves an operator's clip edit shares between the single-clip PATCH and the bulk bar, so the
// two surfaces cannot disagree about what grounding a tag or recording an edit means (§10 V45a).

// groundOperatorTags grounds an operator's tag set against the LIVE taxonomy. An unknown slug 422s
// before the edit writes anything — never silently persisted, never silently dropped.
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

// resolveOperatorGeography turns an operator's geography edit into the grounded value to store,
// refusing it before the edit writes anything. Every station resolves against the directory
// (#2059 decision 5): a local clip airs near it, and a national one keeps it as provenance.
func (s *Server) resolveOperatorGeography(ctx context.Context, g ClipGeographyDTO) (fillerenrichment.Geography, error) {
	scope := filler.GeographicScope(g.Scope)
	geo := filler.Geography{Country: g.Country}.Normalize()
	if err := geo.Validate(); err != nil {
		return fillerenrichment.Geography{}, errUnprocessable("Invalid geography", err.Error())
	}
	airDate := strings.TrimSpace(g.AirDate)
	if airDate != "" {
		if _, err := time.Parse(time.DateOnly, airDate); err != nil {
			return fillerenrichment.Geography{}, errUnprocessable("Invalid air date", "Air date must use YYYY-MM-DD.")
		}
	}
	station := ""
	if typed := strings.TrimSpace(g.Station); typed != "" && scope != filler.GeographicUnknown {
		dir, err := s.stationDirectory(ctx)
		if err != nil {
			return fillerenrichment.Geography{}, huma.Error500InternalServerError("load broadcast stations", err)
		}
		found, ok := dir.Resolve(typed, airDate)
		if !ok {
			return fillerenrichment.Geography{}, errUnprocessable("Unknown station", "That call sign doesn't match exactly one TV station on that air date. Check the call sign and date.")
		}
		if geo.Country != "" && geo.Country != found.Country {
			return fillerenrichment.Geography{}, errUnprocessable("Invalid geography", "That station is in "+found.Country+", not "+geo.Country+".")
		}
		station, geo.Country = found.ID, found.Country
	}
	switch scope {
	case filler.GeographicUnknown:
		geo = filler.Geography{}
	case filler.GeographicNational:
		if geo.Country == "" {
			return fillerenrichment.Geography{}, errUnprocessable("Invalid geography", "National clips require a country.")
		}
	case filler.GeographicLocal:
		if station == "" {
			return fillerenrichment.Geography{}, errUnprocessable("Invalid geography", "Local clips require the broadcast station they belong to.")
		}
	default:
		return fillerenrichment.Geography{}, errUnprocessable("Invalid geography", "Geographic scope must be unknown, national, or local.")
	}
	return fillerenrichment.Geography{
		Scope: string(scope), Country: geo.Country, Network: strings.TrimSpace(g.Network), Station: station, AirDate: airDate,
	}, nil
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
