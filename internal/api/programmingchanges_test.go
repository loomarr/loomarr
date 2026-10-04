package api_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
)

const changesPath = "/v1/channels/c1/programming/changes"

type changesBody struct {
	FromMs, ToMs    int64
	Compared, Count int
	Truncated       bool
	Changes         []changeRow `json:"changes"`
}

type changeRow struct {
	StartMs, EndMs int64
	Before, After  *struct {
		Kind, Title, Key string
		StartMs, StopMs  int64
		Rule             struct {
			ID, Label string
			Matched   bool
		}
	}
}

func decodeChanges(t *testing.T, resp *http.Response) changesBody {
	t.Helper()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("changes → %d, want 200", resp.StatusCode)
	}
	var out changesBody
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

var changesFrom = time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)

func forecastAiring(title string, start time.Time, rule schedule.ActiveRuleAttribution) *channels.ForecastAiring {
	return &channels.ForecastAiring{
		Kind: schedule.SlotProgram, Key: provision.Key("movie:tmdb:" + title), Title: title,
		Start: start, Stop: start.Add(time.Hour), Rule: rule,
	}
}

// The handler hands the engine the same lowered draft programming/preview would, plus the
// requested span, and renders before → after with each side's winning rule.
func TestProgrammingChanges_PassesTheDraftAndRendersBeforeAfter(t *testing.T) {
	harness := newChannelsHarness(t)
	srv, chSvc := harness.Server, harness.Channels
	mkChannel(t, srv, "c1", "Movies", 5)
	sun := schedule.ActiveRuleAttribution{ID: "sun", Label: "Sunday horror", Priority: 10, Matched: true}
	base := schedule.ActiveRuleAttribution{Label: "Base policy"}
	at := changesFrom.Add(6 * time.Hour)
	chSvc.diff = channels.ScheduleDiff{
		From: changesFrom, To: changesFrom.Add(24 * time.Hour), Compared: 24,
		Changes: []channels.SlotChange{{
			Start: at, End: at.Add(time.Hour),
			Before: forecastAiring("Alpha", at, base), After: forecastAiring("Fright", at, sun),
		}, {
			// Nothing airable on the draft side: `after` is absent, not an empty object.
			Start: at.Add(time.Hour), End: at.Add(2 * time.Hour), Before: forecastAiring("Bravo", at.Add(time.Hour), base),
		}},
	}

	body := `{"lineup":[{"key":"movie:tmdb:1","name":"Alpha"}],"policy":{"ordering":"shuffle"}}`
	out := decodeChanges(t, do(t, srv, http.MethodPost, changesPath+"?from=2026-07-26T12:00:00Z&horizonHours=24", adminToken, body))

	if chSvc.draftPolicy == nil || chSvc.draftPolicy.Ordering != "shuffle" {
		t.Errorf("draft policy not passed through: %+v", chSvc.draftPolicy)
	}
	if len(chSvc.draftLineup) != 1 || chSvc.draftLineup[0].Key != "movie:tmdb:1" {
		t.Errorf("draft lineup not passed through: %+v", chSvc.draftLineup)
	}
	if !chSvc.diffFrom.Equal(changesFrom) || chSvc.diffHorizon != 24*time.Hour {
		t.Errorf("span = %s + %s, want %s + 24h", chSvc.diffFrom, chSvc.diffHorizon, changesFrom)
	}
	if out.FromMs != changesFrom.UnixMilli() || out.ToMs != changesFrom.Add(24*time.Hour).UnixMilli() {
		t.Errorf("span echoed = [%d, %d)", out.FromMs, out.ToMs)
	}
	if out.Compared != 24 || out.Count != 2 || out.Truncated || len(out.Changes) != 2 {
		t.Fatalf("summary = compared %d count %d truncated %v rows %d", out.Compared, out.Count, out.Truncated, len(out.Changes))
	}
	first := out.Changes[0]
	if first.StartMs != at.UnixMilli() || first.Before == nil || first.After == nil ||
		first.Before.Title != "Alpha" || first.After.Title != "Fright" || first.After.Kind != "program" {
		t.Fatalf("row 0 = %+v", first)
	}
	if first.Before.Rule.Matched || first.After.Rule.ID != "sun" || !first.After.Rule.Matched {
		t.Errorf("rule attribution = before %+v after %+v", first.Before.Rule, first.After.Rule)
	}
	if out.Changes[1].After != nil {
		t.Errorf("row 1 after = %+v, want absent", out.Changes[1].After)
	}
}

// Omitting from and horizonHours hands the engine zero values: "now" and "the schedule window"
// are the engine's to resolve, read once for both sides.
func TestProgrammingChanges_DefaultsAreTheEngines(t *testing.T) {
	harness := newChannelsHarness(t)
	srv, chSvc := harness.Server, harness.Channels
	mkChannel(t, srv, "c1", "Movies", 5)
	chSvc.diff = channels.ScheduleDiff{From: changesFrom, To: changesFrom.Add(24 * time.Hour)}

	out := decodeChanges(t, do(t, srv, http.MethodPost, changesPath, adminToken, `{}`))
	if !chSvc.diffFrom.IsZero() || chSvc.diffHorizon != 0 {
		t.Errorf("defaults = %s + %s, want zero values", chSvc.diffFrom, chSvc.diffHorizon)
	}
	if chSvc.draftLineup != nil || chSvc.draftPolicy != nil {
		t.Error("an empty body must mean 'the saved channel' on both sides")
	}
	if out.Changes == nil || out.Count != 0 || out.Truncated {
		t.Errorf("no changes must render as [] with count 0, got %+v", out)
	}
}

// The response is capped; the count still says how many slots changed.
func TestProgrammingChanges_TruncatesAndCounts(t *testing.T) {
	harness := newChannelsHarness(t)
	srv, chSvc := harness.Server, harness.Channels
	mkChannel(t, srv, "c1", "Movies", 5)
	base := schedule.ActiveRuleAttribution{Label: "Base policy"}
	for i := range 150 {
		at := changesFrom.Add(time.Duration(i) * time.Hour)
		chSvc.diff.Changes = append(chSvc.diff.Changes, channels.SlotChange{
			Start: at, End: at.Add(time.Hour),
			Before: forecastAiring("B"+strconv.Itoa(i), at, base), After: forecastAiring("A"+strconv.Itoa(i), at, base),
		})
	}

	out := decodeChanges(t, do(t, srv, http.MethodPost, changesPath, adminToken, `{}`))
	if out.Count != 150 || !out.Truncated || len(out.Changes) != 100 {
		t.Fatalf("count %d truncated %v rows %d, want 150/true/100", out.Count, out.Truncated, len(out.Changes))
	}
	if out.Changes[99].Before.Title != "B99" {
		t.Errorf("the cap kept %q last, want the earliest 100 in time order", out.Changes[99].Before.Title)
	}
}

// Validation is programming/preview's: the same draft is refused the same way, and the span
// parameters are bounded.
func TestProgrammingChanges_ValidatesLikeThePreview(t *testing.T) {
	harness := newChannelsHarness(t)
	srv := harness.Server
	mkChannel(t, srv, "c1", "Movies", 5)
	for _, tc := range []struct {
		name, query, body string
		want              int
	}{
		{"bad ceiling", "", `{"policy":{"audience":{"ceiling":"TV-BOGUS"}}}`, http.StatusUnprocessableEntity},
		{"bad lineup key", "", `{"lineup":[{"key":"nope","name":"X"}]}`, http.StatusBadRequest},
		{"bad from", "?from=tomorrow", `{}`, http.StatusBadRequest},
		{"horizon over the cap", "?horizonHours=169", `{}`, http.StatusUnprocessableEntity},
		{"negative horizon", "?horizonHours=-1", `{}`, http.StatusUnprocessableEntity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := do(t, srv, http.MethodPost, changesPath+tc.query, adminToken, tc.body)
			if resp.StatusCode != tc.want {
				t.Errorf("→ %d, want %d", resp.StatusCode, tc.want)
			}
			if tc.query != "" {
				return // the span parameters are this endpoint's own
			}
			if got := do(t, srv, http.MethodPost, "/v1/channels/c1/programming/preview", adminToken, tc.body).StatusCode; got != tc.want {
				t.Errorf("programming/preview refuses the same draft with %d, this endpoint with %d", got, tc.want)
			}
		})
	}
}

// Authorization matches programming/preview: any signed-in member, never anonymous.
func TestProgrammingChanges_Authorization(t *testing.T) {
	harness := newChannelsHarness(t)
	srv := harness.Server
	mkChannel(t, srv, "c1", "Movies", 5)
	for _, tc := range []struct {
		name, token string
		want        int
	}{
		{"anonymous", "", http.StatusUnauthorized},
		{"member", memberToken, http.StatusOK},
		{"admin", adminToken, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changes := do(t, srv, http.MethodPost, changesPath, tc.token, `{}`).StatusCode
			preview := do(t, srv, http.MethodPost, "/v1/channels/c1/programming/preview", tc.token, `{}`).StatusCode
			if changes != tc.want || preview != tc.want {
				t.Errorf("changes → %d, preview → %d, want both %d", changes, preview, tc.want)
			}
		})
	}
}

func TestProgrammingChanges_UnknownChannelIs404(t *testing.T) {
	harness := newChannelsHarness(t)
	srv, chSvc := harness.Server, harness.Channels
	chSvc.cycleErr = store.ErrNotFound
	if got := do(t, srv, http.MethodPost, changesPath, adminToken, `{}`).StatusCode; got != http.StatusNotFound {
		t.Errorf("unknown channel → %d, want 404", got)
	}
}
