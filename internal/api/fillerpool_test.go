package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
)

type poolBody struct {
	Clips       int `json:"clips"`
	Commercials int `json:"commercials"`
	Eligible    int `json:"eligible"`
	Untagged    int `json:"untagged"`
	Channels    []struct {
		ChannelID string `json:"channelId"`
		Name      string `json:"name"`
		Number    int    `json:"number"`
		Level     string `json:"level"`
		Total     int    `json:"total"`
	} `json:"channels"`
}

func getPool(t *testing.T, url, token string) (*http.Response, poolBody) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	var body poolBody
	if res.StatusCode == http.StatusOK {
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
	}
	return res, body
}

// The strip renders what the domain reported, in the order it reported it. Order is
// load-bearing: the diagnosis line names `channels[0]`, so a reshuffle here tells the operator
// to fix a channel that is fine.
func TestFillerPool_RendersCountsAndChannelsInOrder(t *testing.T) {
	srv, _, fp := newPodsServer(t)
	fp.pool = filler.PoolReport{
		Clips: 120, Commercials: 90, Eligible: 61, Untagged: 14,
		Channels: []filler.ChannelCoverage{
			{ChannelID: "ch-3", Name: "Newsreel", Number: 3,
				Report: filler.CoverageReport{Level: filler.MatchBumperCard, Total: 0}},
			{ChannelID: "ch-42", Name: "Cartoons", Number: 42,
				Report: filler.CoverageReport{Level: filler.MatchExact, Total: 44}},
		},
	}

	res, body := getPool(t, srv.URL+"/v1/filler/pool", memberToken)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if body.Clips != 120 || body.Commercials != 90 || body.Eligible != 61 || body.Untagged != 14 {
		t.Errorf("counts = %+v, want clips 120 / commercials 90 / eligible 61 / untagged 14", body)
	}
	if len(body.Channels) != 2 {
		t.Fatalf("got %d channels, want 2", len(body.Channels))
	}
	if body.Channels[0].ChannelID != "ch-3" || body.Channels[0].Level != "bumper_card" {
		t.Errorf("channels[0] = %+v, want the worst channel (ch-3, bumper_card) first", body.Channels[0])
	}
	if body.Channels[1].Level != "exact" || body.Channels[1].Total != 44 {
		t.Errorf("channels[1] = %+v, want exact/44", body.Channels[1])
	}
}

// An install with no channels is a real answer, not a missing one — it is the fresh-install
// state the strip's "Propose a pull" button exists for. The array must be `[]`, never `null`,
// or every client has to guard before iterating.
func TestFillerPool_EmptyChannelsIsAnArrayNotNull(t *testing.T) {
	srv, _, fp := newPodsServer(t)
	fp.pool = filler.PoolReport{Clips: 0}

	res, err := http.DefaultClient.Do(mustGet(t, srv.URL+"/v1/filler/pool", memberToken))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if got := string(raw["channels"]); got != "[]" {
		t.Errorf("channels = %s, want [] — a null here makes every consumer guard", got)
	}
}

func mustGet(t *testing.T, url, token string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}
