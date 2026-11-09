package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
)

type watchHistoryPage struct {
	Viewings []struct {
		ID, UserID, Name, Device, ChannelID, ChannelName, Programme string
		StartedAt, EndedAt                                          time.Time
	}
	NextCursor string
}

func seedWatchHistory(t *testing.T, st store.Store) time.Time {
	t.Helper()
	ctx := context.Background()
	base := time.Unix(1_800_000_000, 0).UTC()
	for id, name := range map[string]string{"u-sam": "Sam", "u-alex": "Alex"} {
		if err := st.UpsertUser(ctx, identitystore.User{ID: id, Name: name, Role: identitystore.RoleMember,
			CreatedAt: base, UpdatedAt: base}); err != nil {
			t.Fatal(err)
		}
	}
	for i, v := range []store.Viewing{
		{UserID: "u-sam", Device: "Living room TV", ChannelID: "ch-1", ChannelName: "Cartoons", Programme: "Bluey"},
		{UserID: "u-alex", Device: "Firefox on macOS", ChannelID: "ch-2", ChannelName: "Movies", Programme: "Up"},
		{UserID: "u-sam", Device: "Living room TV", ChannelID: "ch-2", ChannelName: "Movies", Programme: "Paddington"},
	} {
		v.ID = "v" + string(rune('1'+i))
		v.StartedAt = base.Add(time.Duration(i) * time.Hour)
		v.EndedAt = v.StartedAt.Add(30 * time.Minute)
		if err := st.InsertViewing(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	return base
}

func decodeWatchHistory(t *testing.T, resp *http.Response) watchHistoryPage {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	var page watchHistoryPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	return page
}

func pageIDs(p watchHistoryPage) []string {
	out := []string{}
	for _, v := range p.Viewings {
		out = append(out, v.ID)
	}
	return out
}

// The watch history is the admin's alone (#3010): a member is refused both the read and the clear,
// and an anonymous caller is not signed in.
func TestWatchHistoryIsAdminOnly(t *testing.T) {
	h := newAPIHarness(t)
	seedWatchHistory(t, h.Store)
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		for token, want := range map[string]int{memberToken: http.StatusForbidden, "": http.StatusUnauthorized} {
			resp := h.Do(method, "/v1/household/watch-history", token, "")
			_ = resp.Body.Close()
			if resp.StatusCode != want {
				t.Errorf("%s as %q = %d, want %d", method, token, resp.StatusCode, want)
			}
		}
	}
	rows, err := h.Store.ListViewings(context.Background(), store.ViewingQuery{Limit: 10})
	if err != nil || len(rows) != 3 {
		t.Errorf("after refused clears %d rows, %v; want all 3 kept", len(rows), err)
	}
}

func TestWatchHistoryListsNamedViewingsNewestFirstAndPages(t *testing.T) {
	h := newAPIHarness(t)
	base := seedWatchHistory(t, h.Store)

	page := decodeWatchHistory(t, h.Do(http.MethodGet, "/v1/household/watch-history", adminToken, ""))
	if got := pageIDs(page); len(got) != 3 || got[0] != "v3" || got[2] != "v1" || page.NextCursor != "" {
		t.Fatalf("history = %v cursor %q, want v3 v2 v1 and no cursor", got, page.NextCursor)
	}
	if v := page.Viewings[2]; v.Name != "Sam" || v.Device != "Living room TV" || v.ChannelName != "Cartoons" ||
		v.Programme != "Bluey" || !v.StartedAt.Equal(base) || !v.EndedAt.Equal(base.Add(30*time.Minute)) {
		t.Errorf("v1 = %+v", v)
	}

	first := decodeWatchHistory(t, h.Do(http.MethodGet, "/v1/household/watch-history?limit=2", adminToken, ""))
	if first.NextCursor == "" {
		t.Fatal("a full page has no next cursor")
	}
	second := decodeWatchHistory(t, h.Do(http.MethodGet,
		"/v1/household/watch-history?limit=2&cursor="+url.QueryEscape(first.NextCursor), adminToken, ""))
	if got := append(pageIDs(first), pageIDs(second)...); len(got) != 3 || got[2] != "v1" || second.NextCursor != "" {
		t.Errorf("two pages = %v, last cursor %q; want v3 v2 | v1 and no cursor", got, second.NextCursor)
	}

	sam := decodeWatchHistory(t, h.Do(http.MethodGet, "/v1/household/watch-history?userId=u-sam", adminToken, ""))
	if got := pageIDs(sam); len(got) != 2 || got[0] != "v3" || got[1] != "v1" {
		t.Errorf("Sam's history = %v, want v3 v1", got)
	}
	// v2 runs 1:00–1:30 after base; a window from 0:45 to 1:15 overlaps only it.
	window := "?from=" + url.QueryEscape(base.Add(45*time.Minute).Format(time.RFC3339)) +
		"&to=" + url.QueryEscape(base.Add(75*time.Minute).Format(time.RFC3339))
	inWindow := decodeWatchHistory(t, h.Do(http.MethodGet, "/v1/household/watch-history"+window, adminToken, ""))
	if got := pageIDs(inWindow); len(got) != 1 || got[0] != "v2" {
		t.Errorf("window = %v, want v2", got)
	}

	resp := h.Do(http.MethodGet, "/v1/household/watch-history?cursor=nonsense", adminToken, "")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("a malformed cursor = %d, want 400", resp.StatusCode)
	}
}

func TestWatchHistoryClearsOnePersonOrEveryone(t *testing.T) {
	h := newAPIHarness(t)
	seedWatchHistory(t, h.Store)
	deleted := func(path string) int {
		t.Helper()
		resp := h.Do(http.MethodDelete, path, adminToken, "")
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("DELETE %s = %d, want 200", path, resp.StatusCode)
		}
		var body struct{ Deleted int }
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		return body.Deleted
	}
	if n := deleted("/v1/household/watch-history?userId=u-sam"); n != 2 {
		t.Errorf("clearing Sam removed %d, want 2", n)
	}
	left := decodeWatchHistory(t, h.Do(http.MethodGet, "/v1/household/watch-history", adminToken, ""))
	if got := pageIDs(left); len(got) != 1 || got[0] != "v2" {
		t.Errorf("after clearing Sam = %v, want Alex's v2", got)
	}
	if n := deleted("/v1/household/watch-history"); n != 1 {
		t.Errorf("clearing everyone removed %d, want 1", n)
	}
	if empty := decodeWatchHistory(t, h.Do(http.MethodGet, "/v1/household/watch-history", adminToken, "")); len(empty.Viewings) != 0 {
		t.Errorf("after clearing everyone %d viewings remain", len(empty.Viewings))
	}
}
