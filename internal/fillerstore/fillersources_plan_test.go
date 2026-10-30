package fillerstore

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

func TestFillerSource_ServesHomeNeedsAConfiguredCountry(t *testing.T) {
	unclassified := FillerSource{Kind: "archive"}
	british := FillerSource{Kind: "archive", Geography: filler.Geography{Country: "GB"}}
	if unclassified.ServesHome(filler.Geography{}) || !unclassified.GeographicallyEligible(filler.Geography{}) {
		t.Fatal("with no installation country, nothing is served even though the geography gate admits all")
	}
	if !unclassified.ServesHome(filler.Geography{Country: "us"}) {
		t.Fatal("a source with no country follows the installation's")
	}
	if british.ServesHome(filler.Geography{Country: "US"}) {
		t.Fatal("a British source served a US installation")
	}
}

func TestFillerSource_FetchPlanResolvesNeverAndEnablement(t *testing.T) {
	never := 0
	src := FillerSource{ID: "s", Kind: "archive", URI: "prelinger", Enabled: true, ProviderEnabled: true, FetchEverySeconds: &never}
	plan := src.FetchPlan(true, 2, time.Hour)
	if !plan.NeverFetch || plan.Every != 0 || !plan.Enabled || plan.Region != 2 || plan.ID != "s" {
		t.Fatalf("an opted-out source = %+v, want NeverFetch with no interval", plan)
	}
	src.FetchEverySeconds = nil
	if plan := src.FetchPlan(true, 0, time.Hour); plan.NeverFetch || plan.Every != time.Hour {
		t.Fatalf("an inheriting source = %+v, want the global interval", plan)
	}
	if src.FetchPlan(false, 0, time.Hour).Enabled {
		t.Fatal("a source outside the installation's geography was enabled")
	}
	src.ProviderEnabled = false
	if src.FetchPlan(true, 0, time.Hour).Enabled {
		t.Fatal("a source whose provider is switched off was enabled")
	}
}
