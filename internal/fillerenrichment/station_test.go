package fillerenrichment_test

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/broadcaststation"
	"github.com/loomarr/loomarr/internal/fillerenrichment"
)

var cleveland = broadcaststation.Station{ID: "fcc:59441", Country: "US", City: "Cleveland", Region: "OH"}

func geographyOf(t *testing.T, signals fillerenrichment.Signals) fillerenrichment.State {
	t.Helper()
	for _, state := range fillerenrichment.AnalyzeDeterministic(signals) {
		if state.Axis == fillerenrichment.AxisGeography {
			return state
		}
	}
	t.Fatalf("no geography state for %q", signals.Title)
	return fillerenrichment.State{}
}

// #2059 decision 6: a station's own material is local to it; anything else it is credited with only
// says where the recording was made.
func TestAnalyzeDeterministic_StationMaterialIsLocal(t *testing.T) {
	at := time.Unix(1_800_000_000, 0)
	match := &broadcaststation.Match{Station: cleveland, CallSign: "WEWS", AirDate: "1991-03-10"}
	for _, title := range []string{"WEWS NewsChannel 5 News Update 03/10/1991", "Weather Forecast Update on WEWS 5",
		"WEWS 5 sign-off 1991", "WEWS 5 promo 1991", "WEWS-TV ident", "WEWS 5 Promo", "WEWS 5 News", "WEWS 5 Sign-Off"} {
		state := geographyOf(t, fillerenrichment.Signals{ClipHash: "c", Kind: "interstitial", Title: title, Station: match, ObservedAt: at})
		want := fillerenrichment.Geography{Scope: "local", Country: "US", Station: "fcc:59441", AirDate: "1991-03-10"}
		if state.Status != fillerenrichment.StatusComplete || state.Value.Geography != want ||
			state.Evidence.Kind != fillerenrichment.EvidenceTrustedMap {
			t.Errorf("%q: geography = %+v (%s), want local %+v from the station directory", title, state.Value.Geography, state.Evidence.Kind, want)
		}
	}
	// A station ID is station material whatever its title says.
	state := geographyOf(t, fillerenrichment.Signals{ClipHash: "c", Kind: "station_id", Title: "WEWS 5 1991", Station: match, ObservedAt: at})
	if state.Value.Geography.Scope != "local" {
		t.Errorf("a station ID's geography = %+v, want local", state.Value.Geography)
	}
}

func TestAnalyzeDeterministic_StationCreditedAdIsNationalProvenance(t *testing.T) {
	match := &broadcaststation.Match{Station: cleveland, CallSign: "WJW"}
	state := geographyOf(t, fillerenrichment.Signals{ClipHash: "c", Kind: "commercial",
		Title: "Brambly Lotion Commercial WJW 8 1993-12-22", Station: match, ObservedAt: time.Unix(1_800_000_000, 0)})
	if state.Value.Geography.Scope != "national" || state.Value.Geography.Station != "fcc:59441" ||
		state.Evidence.Kind != fillerenrichment.EvidenceInference || state.Evidence.Confidence >= 80 {
		t.Fatalf("a station-credited ad = %+v (%s, %d), want national provenance below a confident research verdict",
			state.Value.Geography, state.Evidence.Kind, state.Evidence.Confidence)
	}
	// Ad copy and network promos use the words without being the station's own segment.
	for _, title := range []string{
		"Brambly Newsweekly Commercial WJW 8",
		"Brambly All-Weather Tyres Commercial WJW 8",
		"Kestrel One network news tonight promo, recorded off WJW 8",
		"Brambly Bank - sign on today! WJW 8",
		"Sign off with a Brambly Cola, WJW 8",
		"Kestrel One Nightly News promo 1985, WJW 8", // a network newscast, recorded off the station
		"Kestrel One Evening News promo, WJW 8",
	} {
		state = geographyOf(t, fillerenrichment.Signals{ClipHash: "c", Kind: "commercial", Title: title, Station: match,
			ObservedAt: time.Unix(1_800_000_000, 0)})
		if state.Value.Geography.Scope != "national" {
			t.Errorf("%q = %+v, want national", title, state.Value.Geography)
		}
	}
	// On-screen text is not item text: an ad's frame saying "weather report" is still an ad.
	state = geographyOf(t, fillerenrichment.Signals{ClipHash: "c", Kind: "commercial", Title: "Brambly Lotion WJW 8",
		VisibleText: "WEATHER REPORT", Station: match, ObservedAt: time.Unix(1_800_000_000, 0)})
	if state.Value.Geography.Scope != "national" {
		t.Errorf("visible text made an ad local: %+v", state.Value.Geography)
	}
	// A source's own declaration outranks the provenance guess.
	state = geographyOf(t, fillerenrichment.Signals{ClipHash: "c", Kind: "commercial", SourceID: "archive:x",
		Source: fillerenrichment.Geography{Scope: "national", Country: "US"}, Title: "Brambly Lotion WJW 8",
		Station: match, ObservedAt: time.Unix(1_800_000_000, 0)})
	if state.Evidence.Kind != fillerenrichment.EvidenceSourceDefault {
		t.Fatalf("with a declaring source, evidence = %s, want the source default", state.Evidence.Kind)
	}
}

// Research said "national" for Cleveland news before the station directory existed; the local
// answer outranks that inference, so a re-run corrects it.
func TestStationLocalOutranksResearchNational(t *testing.T) {
	at := time.Unix(1_800_000_000, 0)
	research := fillerenrichment.State{ClipHash: "c", Axis: fillerenrichment.AxisGeography, Status: fillerenrichment.StatusComplete,
		Value: fillerenrichment.Value{Geography: fillerenrichment.Geography{Scope: "national", Country: "US"}},
		Evidence: fillerenrichment.Evidence{Kind: fillerenrichment.EvidenceInference, Reference: "context.report:x:citations:1",
			Confidence: 80, Producer: "context-model:openai", ProducerVersion: "filler-context-v2", ObservedAt: at}}
	local := geographyOf(t, fillerenrichment.Signals{ClipHash: "c", Kind: "interstitial", Title: "WEWS NewsChannel 5 News Update",
		Station: &broadcaststation.Match{Station: cleveland, CallSign: "WEWS"}, ObservedAt: at})
	got, changed, err := fillerenrichment.Apply(research, local)
	if err != nil || !changed || got.Value.Geography.Scope != "local" {
		t.Fatalf("Apply(research national, station local) = %+v, %v, %v; want local", got.Value.Geography, changed, err)
	}
}
