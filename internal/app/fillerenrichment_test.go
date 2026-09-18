package app

import (
	"testing"
	"testing/fstest"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerenrichment"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestDeterministicFillerEnrichmentProjectsPinnedExamplesWithoutAProvider(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	at := time.Unix(1_700_000_000, 0).UTC()
	for _, clip := range []store.Clip{
		{Clip: filler.Clip{Hash: "hp", Path: "hp.mp4", Name: "HP Sauce Advert", Kind: filler.Commercial}, UpdatedAt: at, CreatedAt: at},
		{Clip: filler.Clip{Hash: "tootsie", Path: "tootsie.mp4", Name: "Tootsie Pop Classic Commercial", Kind: filler.Commercial}, UpdatedAt: at, CreatedAt: at.Add(time.Second)},
	} {
		if err := st.UpsertClip(t.Context(), clip); err != nil {
			t.Fatal(err)
		}
	}
	files := fstest.MapFS{
		"hp.info.json":      {Data: []byte(`{"title":"HP Sauce Advert","description":"An advert for HP Sauce broadcast 1999 on Five.","upload_date":"20250107","loomarr":{"originalName":"hp-sauce-advert.mp4","sourceId":"classic"}}`)},
		"tootsie.info.json": {Data: []byte(`{"title":"Tootsie Pop Classic Commercial","description":"The classic Tootsie Pop commercial. There are 3 versions.","upload_date":"20240215","loomarr":{"originalName":"tootsie-pop.mp4","sourceId":"classic"}}`)},
	}
	runner := fillerenrichment.NewRunner(
		fillerEnrichmentRepository{st: st}, fillerEnrichmentSignals{store: st, files: files}.Load,
		func() int { return 10 }, func() time.Time { return at.Add(time.Minute) },
	)
	result, err := runner.Run(t.Context())
	if err != nil || result.Considered != 2 || result.Updated != 2*len(fillerenrichment.DeterministicAxes) {
		t.Fatalf("Run() = %+v, err %v", result, err)
	}
	hp, err := st.GetClip(t.Context(), "hp")
	if err != nil {
		t.Fatal(err)
	}
	if hp.Era != 1999 || hp.Brand != "HP Sauce" || hp.Category != "condiments" || hp.Country != "GB" || hp.Network != "Five" {
		t.Fatalf("HP projection = %+v", hp)
	}
	tootsie, err := st.GetClip(t.Context(), "tootsie")
	if err != nil {
		t.Fatal(err)
	}
	if tootsie.Era != 0 || tootsie.Brand != "Tootsie Pop" || tootsie.Category != "candy" {
		t.Fatalf("Tootsie projection = %+v", tootsie)
	}
	candidates, err := st.ListFillerEnrichmentCandidates(t.Context(), fillerenrichment.DeterministicProducer,
		fillerenrichment.DeterministicProducerVersion, fillerenrichment.ControlledTaxonomyVersion, 10)
	if err != nil || len(candidates) != 0 {
		t.Fatalf("completed clips remained candidates: %+v, err %v", candidates, err)
	}
}
