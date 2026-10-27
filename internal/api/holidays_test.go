package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/schedule"
)

type holidaysWire struct {
	Country         string `json:"country"`
	CalendarCountry string `json:"calendarCountry"`
	Holidays        []struct {
		ID      string   `json:"id"`
		Members []string `json:"members"`
		Kept    []struct {
			Countries []string `json:"countries"`
		} `json:"kept"`
		Season *struct{ Start, End, Day string } `json:"season"`
	} `json:"holidays"`
}

func getHolidays(t *testing.T, harness *channelsHarness, country string) holidaysWire {
	t.Helper()
	harness.Channels.calendar = schedule.Calendar{Country: country}
	resp := do(t, harness.Server, http.MethodGet, "/v1/holidays", memberToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("holidays → %d, want 200", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	// The group has no words or dates, and every one of those lists is still a list on the wire.
	if bytes.Contains(raw, []byte("null")) {
		t.Fatalf("holidays body carries a null: %s", raw)
	}
	var out holidaysWire
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// The holidays endpoint serves every definition with this year's season in the household's country.
func TestHolidays_ServeDefinitionsWithTheHouseholdCountrysSeason(t *testing.T) {
	harness := newChannelsHarness(t)
	year := time.Now().UTC().Year()

	ca := getHolidays(t, harness, "CA")
	if ca.Country != "CA" || ca.CalendarCountry != "CA" {
		t.Fatalf("country = %q / %q, want CA / CA", ca.Country, ca.CalendarCountry)
	}
	seasons := map[string]string{}
	var group []string
	for _, h := range ca.Holidays {
		if h.Season != nil {
			seasons[h.ID] = h.Season.Day
		}
		if h.ID == "holiday" {
			group = h.Members
		}
	}
	if want := fmt.Sprintf("%d-12-25", year); seasons["christmas"] != want {
		t.Errorf("christmas day = %q, want %q", seasons["christmas"], want)
	}
	if want := fmt.Sprintf("%d-07-01", year); seasons["canada-day"] != want {
		t.Errorf("canada-day day = %q, want %q", seasons["canada-day"], want)
	}
	if _, ok := seasons["independence-day"]; ok {
		t.Error("CA has an Independence Day season")
	}
	if len(group) == 0 {
		t.Error("the group holiday lists no members")
	}

	de := getHolidays(t, harness, "DE")
	if de.Country != "DE" || de.CalendarCountry != "US" {
		t.Fatalf("an unlisted country = %q / %q, want DE / US", de.Country, de.CalendarCountry)
	}
	for _, h := range de.Holidays {
		if h.ID == "independence-day" && h.Season == nil {
			t.Error("an unlisted country does not read the US calendar")
		}
	}
}
