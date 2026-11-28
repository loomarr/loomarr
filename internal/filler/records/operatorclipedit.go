package records

import (
	"context"
	"fmt"
	"time"

	catalogclips "github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/enrichment"
	"github.com/loomarr/loomarr/internal/filler/taxonomy"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// OperatorClipEdit is one operator change to a clip. A nil field is left as it is. The caller has
// grounded the tags and geography.
type OperatorClipEdit struct {
	Hash      string
	At        time.Time
	Reference string

	Name      *string
	Era       *int
	Audience  *string
	Role      *string
	Brand     *string
	Geography *enrichment.Geography
	Tags      *OperatorTags
}

// OperatorTags is the clip's whole new set of leaf tags and the taxonomy that places them.
type OperatorTags struct {
	Leaves []string
	Forest *taxonomy.Forest
}

// ApplyOperatorClipEdit writes the clip's columns, tags and operator evidence in one transaction, so
// an edit that fails part way changes nothing.
func (s *sqlStore) ApplyOperatorClipEdit(ctx context.Context, e OperatorClipEdit) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("apply operator clip edit: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// Tags go first: they take the taxonomy lock, which precedes any clip row lock.
	if e.Tags != nil {
		if err := s.db.Clips(tx).ReplaceLeafTags(ctx, e.Hash, e.Tags.Leaves); err != nil {
			return err
		}
	}
	if err := s.requireEnrichmentClipTx(ctx, tx, e.Hash); err != nil {
		return err
	}
	clips := s.db.Clips(tx)
	if e.Name != nil {
		if err := clips.Rename(ctx, e.Hash, *e.Name, catalogclips.NameFromCurated, e.At); err != nil {
			return err
		}
	}
	if e.Era != nil || e.Audience != nil {
		// Both columns are written together, so the one the edit does not name keeps its stored value.
		var era int
		var audience string
		if err := tx.QueryRowContext(ctx, s.ph(`SELECT era, audience FROM clips WHERE hash = ?`), e.Hash).Scan(&era, &audience); err != nil {
			return fmt.Errorf("apply operator clip edit: read classification: %w", err)
		}
		if e.Era != nil {
			era = *e.Era
		}
		if e.Audience != nil {
			audience = *e.Audience
		}
		if err := clips.SetClassification(ctx, e.Hash, era, audience, e.At); err != nil {
			return err
		}
	}
	if e.Role != nil {
		if err := clips.SetRole(ctx, e.Hash, *e.Role, e.At); err != nil {
			return err
		}
	}
	if g := e.Geography; g != nil {
		if err := clips.SetOperatorGeography(ctx, e.Hash, clipstore.ClipGeography{
			Scope: g.Scope, Country: g.Country, Network: g.Network, Station: g.Station, AirDate: g.AirDate,
		}, e.At); err != nil {
			return err
		}
	}
	if e.Brand != nil {
		if err := clips.SetBrand(ctx, e.Hash, *e.Brand, e.At); err != nil {
			return err
		}
	}
	for _, state := range e.evidence() {
		if err := state.Validate(); err != nil {
			return err
		}
		if _, _, err := s.applyFillerEnrichmentTx(ctx, tx, state, e.At); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("apply operator clip edit: commit: %w", err)
	}
	return nil
}

// evidence is the edit as operator evidence, one state per axis it decides. A tag edit decides every
// taxonomy axis, so an axis it cleared reads as observed-empty rather than keeping an older guess.
func (e OperatorClipEdit) evidence() []enrichment.State {
	var states []enrichment.State
	add := func(axis enrichment.Axis, value enrichment.Value, taxonomyVersion string) {
		states = append(states, enrichment.Operator(e.Hash, axis, value, e.Reference, taxonomyVersion, e.At))
	}
	if e.Era != nil {
		add(enrichment.AxisEra, enrichment.Value{Year: *e.Era}, "")
	}
	if e.Audience != nil {
		add(enrichment.AxisAudience, enrichment.Value{Text: *e.Audience}, "")
	}
	if e.Role != nil {
		add(enrichment.AxisRole, enrichment.Value{Text: *e.Role}, "")
	}
	if e.Brand != nil {
		add(enrichment.AxisBrand, enrichment.Value{Text: *e.Brand}, "")
	}
	if e.Geography != nil {
		add(enrichment.AxisGeography, enrichment.Value{Geography: *e.Geography}, "")
	}
	if e.Tags != nil {
		byAxis := make(map[enrichment.Axis][]string)
		for _, leaf := range e.Tags.Leaves {
			if taxon, ok := e.Tags.Forest.Get(leaf); ok {
				axis := enrichment.Axis(taxon.Axis)
				byAxis[axis] = append(byAxis[axis], leaf)
			}
		}
		for _, axis := range enrichment.TaxonomyAxes() {
			add(axis, enrichment.Value{Tags: byAxis[axis]}, enrichment.OperatorTaxonomyVersion)
		}
	}
	return states
}
