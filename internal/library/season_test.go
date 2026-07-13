package library

import "testing"

func TestParseSeasonPrecision(t *testing.T) {
	// Empty defaults to series (§6/§15 default).
	if p, err := ParseSeasonPrecision(""); err != nil || p != PrecisionSeries {
		t.Errorf(`ParseSeasonPrecision("") = %q,%v want series,nil`, p, err)
	}
	if _, err := ParseSeasonPrecision("seasons"); err != nil {
		t.Errorf("seasons should parse: %v", err)
	}
	if _, err := ParseSeasonPrecision("episodes"); err == nil {
		t.Error("unknown precision should error")
	}
}

func TestSeriesPresent(t *testing.T) {
	// series mode: show present => available, no season check.
	if present, need := PrecisionSeries.SeriesPresent(true); !present || need {
		t.Errorf("series mode, show present: got present=%v need=%v want true,false", present, need)
	}
	// series mode: show absent => not present.
	if present, _ := PrecisionSeries.SeriesPresent(false); present {
		t.Error("series mode, show absent should not be present")
	}
	// seasons mode: show present but seasons must still be verified.
	if present, need := PrecisionSeasons.SeriesPresent(true); present || !need {
		t.Errorf("seasons mode, show present: got present=%v need=%v want false,true", present, need)
	}
}
