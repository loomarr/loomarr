package station_test

import (
	"io"
	"log/slog"
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/station"
	"github.com/loomarr/loomarr/internal/testkit"
)

// A Philadelphia household (place 4560349) receives KYW Philadelphia (fcc:25453) and not
// Cleveland's WEWS (fcc:59441), #2059's misplaced station. The stations are the shipped seed under
// test.
func TestLocalStationsResolvesThePickedPlaceAgainstTheSeededDirectory(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	radius := 100
	local := &station.LocalResolver{Stations: station.NewLazy(st.ListBroadcastStations),
		RadiusKM: func() int { return radius }, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}

	near := local.Resolve("US", "4560349")
	if !near["fcc:25453"] || near["fcc:59441"] {
		t.Fatalf("Philadelphia at 100 km: KYW %v, WEWS %v; want KYW only", near["fcc:25453"], near["fcc:59441"])
	}
	if got := local.Resolve("CA", "4560349"); got != nil {
		t.Fatalf("a US place resolved for Canada: %d stations", len(got))
	}
	if got := local.Resolve("US", "no-such-place"); got != nil {
		t.Fatalf("an unknown place resolved %d stations", len(got))
	}
	// The radius is live: a tighter one reaches fewer stations, and its answer is cached apart.
	radius = 10
	if tight := local.Resolve("US", "4560349"); len(tight) >= len(near) {
		t.Fatalf("10 km reached %d stations, 100 km %d", len(tight), len(near))
	}

	household := filler.Geography{Country: "US", Place: "4560349"}.Resolve(local.Resolve)
	clevelandNews := filler.Clip{GeographicScope: filler.GeographicLocal, Country: "US", Station: "fcc:59441"}
	philadelphiaNews := filler.Clip{GeographicScope: filler.GeographicLocal, Country: "US", Station: "fcc:25453"}
	if filler.GeographicallyEligible(clevelandNews, household) || !filler.GeographicallyEligible(philadelphiaNews, household) {
		t.Fatal("want Philadelphia station news eligible in Philadelphia and Cleveland's not")
	}
}
