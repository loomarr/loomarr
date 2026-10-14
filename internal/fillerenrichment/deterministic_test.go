package fillerenrichment_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerenrichment"
)

// These tests pin the trusted brand dictionary in deterministic.go, whose entries are real brands
// (HP Sauce, Tootsie Pop), against the sidecars in testdata. A dictionary phrase is the only text
// that fires a trusted product mapping, so this file keeps the real names on purpose (#1592) and
// the demolibrary guard lists it in realDataGo. Every other fillerenrichment test uses invented
// brands.

type sidecarFixture struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	UploadDate  string `json:"upload_date"`
	Loomarr     struct {
		OriginalName string `json:"originalName"`
		SourceID     string `json:"sourceId"`
	} `json:"loomarr"`
}

func readSignals(t *testing.T, name, hash string) fillerenrichment.Signals {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var fixture sidecarFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	return fillerenrichment.Signals{ClipHash: hash, Kind: "commercial", Title: fixture.Title,
		Description: fixture.Description, OriginalName: fixture.Loomarr.OriginalName,
		UploadDate: fixture.UploadDate, SourceID: fixture.Loomarr.SourceID, ObservedAt: time.Unix(100, 0).UTC()}
}

func TestAnalyzeDeterministic_TootsieUsesItemTextButNotUploadDate(t *testing.T) {
	got := statesByAxis(fillerenrichment.AnalyzeDeterministic(readSignals(t, "tootsie-pop.info.json", "tootsie")))
	if got[fillerenrichment.AxisKind].Value.Text != "commercial" {
		t.Fatalf("kind = %+v", got[fillerenrichment.AxisKind])
	}
	if got[fillerenrichment.AxisBrand].Value.Text != "Tootsie Pop" {
		t.Fatalf("brand = %+v", got[fillerenrichment.AxisBrand])
	}
	if tags := got[fillerenrichment.AxisProduct].Value.Tags; len(tags) != 1 || tags[0] != "candy" {
		t.Fatalf("product = %+v", got[fillerenrichment.AxisProduct])
	}
	if got[fillerenrichment.AxisEra].Value.Year != 0 || got[fillerenrichment.AxisEra].Status != fillerenrichment.StatusComplete {
		t.Fatalf("upload date became era or deterministic check was not recorded: %+v", got[fillerenrichment.AxisEra])
	}
	// The fixture's source declares no country: the pass records an empty withdrawal, never a
	// completed location, so later passes still look for one.
	if geo, exists := got[fillerenrichment.AxisGeography]; exists &&
		(geo.Status == fillerenrichment.StatusComplete || geo.Value.Geography != (fillerenrichment.Geography{})) {
		t.Fatalf("unknown geography was marked complete: %+v", geo)
	}
}

func TestAnalyzeDeterministic_HPKeepsBroadcastYearAndTrustedNetwork(t *testing.T) {
	got := statesByAxis(fillerenrichment.AnalyzeDeterministic(readSignals(t, "hp-sauce.info.json", "hp")))
	if got[fillerenrichment.AxisEra].Value.Year != 1999 || got[fillerenrichment.AxisBrand].Value.Text != "HP Sauce" {
		t.Fatalf("HP facts = era %+v, brand %+v", got[fillerenrichment.AxisEra], got[fillerenrichment.AxisBrand])
	}
	product := got[fillerenrichment.AxisProduct]
	if len(product.Value.Tags) != 1 || product.Value.Tags[0] != "condiments" {
		t.Fatalf("product = %+v", product)
	}
	geo := got[fillerenrichment.AxisGeography]
	if geo.Value.Geography.Country != "GB" || geo.Value.Geography.Network != "Five" || geo.Evidence.Kind != fillerenrichment.EvidenceTrustedMap {
		t.Fatalf("geography = %+v", geo)
	}
}
