package schedule

import (
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/holidayvocab"
	"github.com/loomarr/loomarr/internal/taxonomy"
)

// #2030: a seasonal clip airs only inside its holiday's window, so a seasonal tag with no window
// would never air, and a window with no tag would open with nothing to match it. Programming and
// filler share one vocabulary: every holiday identity is dated in some country and has a seasonal
// taxon, and every seasonal taxon is a holiday (or the generic `holiday` group, which spans the
// winter ones).
func TestHolidayVocabularyCalendarAndSeasonalTaxaAgree(t *testing.T) {
	vocab := holidayvocab.Default()
	var ids []string
	for _, d := range vocab.Definitions() {
		ids = append(ids, d.ID)
	}
	var dated []string
	for _, country := range vocab.Countries() {
		for _, o := range vocab.Calendar(country) {
			if !slices.Contains(dated, o.ID) {
				dated = append(dated, o.ID)
			}
		}
	}
	var seasonal []string
	for _, tx := range taxonomy.SeedForest() {
		if tx.Axis == taxonomy.AxisSeasonal && tx.Slug != GenericHolidayTag {
			seasonal = append(seasonal, tx.Slug)
		}
	}
	slices.Sort(ids)
	slices.Sort(dated)
	slices.Sort(seasonal)
	if !slices.Equal(ids, dated) {
		t.Errorf("holiday identities %v and dated holidays %v differ", ids, dated)
	}
	if !slices.Equal(ids, seasonal) {
		t.Errorf("holiday identities %v and seasonal taxa %v differ", ids, seasonal)
	}
}
