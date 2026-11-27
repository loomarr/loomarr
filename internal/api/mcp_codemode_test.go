package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// bossDeviceID is the id the device list gives the admin's only paired device.
func bossDeviceID(t *testing.T, base string, session *http.Cookie) string {
	t.Helper()
	resp := authed(t, http.MethodGet, base+"/v1/auth/devices", session, "")
	defer func() { _ = resp.Body.Close() }()
	var list struct {
		Devices []struct {
			ID string `json:"id"`
		} `json:"devices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil || len(list.Devices) != 1 {
		t.Fatalf("device list = %+v (err %v), want the one paired device", list, err)
	}
	return list.Devices[0].ID
}

func setCodeMode(t *testing.T, base, id string, session *http.Cookie, bearer string, on bool) int {
	t.Helper()
	body := `{"on":false}`
	if on {
		body = `{"on":true}`
	}
	req, _ := http.NewRequest(http.MethodPut, base+"/v1/agents/code-mode/"+id, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Loomarr-Csrf", "1")
	if session != nil {
		req.AddCookie(session)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode
}

func refusesTool(t *testing.T, s *mcp.ClientSession, name string, args map[string]any) {
	t.Helper()
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err == nil && !res.IsError {
		t.Fatalf("%s ran: %s", name, toolText(res))
	}
}

// Code Mode is an admin's, and off until that admin turns it on for the device, from a browser.
func TestMCPCodeModeIsAnAdminsAndOffUntilTurnedOn(t *testing.T) {
	t.Parallel()
	h := newDeviceHarness(t, nil)
	kid := pairDevice(t, h.Server, login(t, h.Server, "kid", "pw"))
	bossSession := login(t, h.Server, "boss", "pw")
	boss := pairDevice(t, h.Server, bossSession)
	id := bossDeviceID(t, h.Server.URL, bossSession)
	script := map[string]any{"code": `return 1`}

	memberSession := mustConnectMCP(t, h.Server, kid)
	if tools := toolNames(t, memberSession); slices.Contains(tools, "execute") || slices.Contains(tools, "search") {
		t.Fatalf("a member is offered Code Mode: %v", tools)
	}
	refusesTool(t, memberSession, "execute", script)

	off := mustConnectMCP(t, h.Server, boss)
	if tools := toolNames(t, off); slices.Contains(tools, "execute") {
		t.Fatalf("an admin's device is offered execute with Code Mode off: %v", tools)
	}
	refusesTool(t, off, "execute", script)

	// The device itself cannot turn it on, even with its admin token; a member cannot at all.
	if code := setCodeMode(t, h.Server.URL, id, nil, boss, true); code != http.StatusForbidden {
		t.Fatalf("the device turning Code Mode on = %d, want 403", code)
	}
	if code := setCodeMode(t, h.Server.URL, id, login(t, h.Server, "kid", "pw"), "", true); code != http.StatusForbidden {
		t.Fatalf("a member turning Code Mode on = %d, want 403", code)
	}
	if code := setCodeMode(t, h.Server.URL, id, bossSession, "", true); code != http.StatusNoContent && code != http.StatusOK {
		t.Fatalf("the admin turning Code Mode on = %d", code)
	}

	on := mustConnectMCP(t, h.Server, boss)
	if tools := toolNames(t, on); !slices.Contains(tools, "execute") || !slices.Contains(tools, "search") {
		t.Fatalf("Code Mode on, but the tools are %v", tools)
	}
	// Still never a member's, even once the admin's switch exists.
	if tools := toolNames(t, mustConnectMCP(t, h.Server, kid)); slices.Contains(tools, "execute") {
		t.Fatalf("a member is offered execute: %v", tools)
	}

	if code := setCodeMode(t, h.Server.URL, id, bossSession, "", false); code != http.StatusNoContent && code != http.StatusOK {
		t.Fatalf("turning Code Mode off = %d", code)
	}
	refusesTool(t, mustConnectMCP(t, h.Server, boss), "execute", script)
}

// One script reads three routes and joins them; a route the role refuses gives the script the
// same 403 the HTTP route gives; and the endpoint itself is out of a script's reach.
func TestMCPCodeModeScriptCallsTheAPIAsTheAdmin(t *testing.T) {
	t.Parallel()
	h := newDeviceHarness(t, nil)
	for i, id := range []string{"ch-news", "ch-films"} {
		seedGridChannel(t, h.Store, id, i+1)
	}
	bossSession := login(t, h.Server, "boss", "pw")
	boss := pairDevice(t, h.Server, bossSession)
	if code := setCodeMode(t, h.Server.URL, bossDeviceID(t, h.Server.URL, bossSession), bossSession, "", true); code >= 300 {
		t.Fatalf("turning Code Mode on = %d", code)
	}
	s := mustConnectMCP(t, h.Server, boss)

	found := toolText(callTool(t, s, "search", map[string]any{"query": "channels list"}))
	if !strings.Contains(found, `"/v1/channels"`) {
		t.Fatalf("search did not find the channel list: %s", found)
	}

	res := callTool(t, s, "execute", map[string]any{"code": `
const [me, channels, household] = await Promise.all([
  loomarr.request("GET", "/v1/auth/me"),
  loomarr.request("GET", "/v1/channels"),
  loomarr.request("GET", "/v1/household"),
]);
return { who: me.body.name, channels: channels.body.channels.map(c => c.id).sort(), zoned: household.status === 200 };`})
	if res.IsError {
		t.Fatalf("execute: %s", toolText(res))
	}
	var out struct {
		Result struct {
			Who      string   `json:"who"`
			Channels []string `json:"channels"`
			Zoned    bool     `json:"zoned"`
		} `json:"result"`
		Requests []string `json:"requests"`
	}
	if err := json.Unmarshal([]byte(toolText(res)), &out); err != nil {
		t.Fatalf("execute reply %s: %v", toolText(res), err)
	}
	if out.Result.Who != "boss" || !slices.Equal(out.Result.Channels, []string{"ch-films", "ch-news"}) || !out.Result.Zoned || len(out.Requests) != 3 {
		t.Fatalf("execute = %+v", out)
	}

	// An admin may not ask for an idea (that is a member's request); the route says 403 to the
	// device over HTTP, and says the same to the script.
	req, _ := http.NewRequest(http.MethodPost, h.Server.URL+"/v1/discovery/ideas/any/request", nil)
	req.Header.Set("Authorization", "Bearer "+boss)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	res = callTool(t, s, "execute", map[string]any{"code": `return (await loomarr.request("POST", "/v1/discovery/ideas/any/request")).status`})
	if res.IsError || !strings.Contains(toolText(res), `"result":`+itoa(resp.StatusCode)) || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("over HTTP %d; in a script %s; want the same 403", resp.StatusCode, toolText(res))
	}

	for _, path := range []string{
		"/v1/mcp", "/v1/agents/code-mode", "/v1/oauth/token", "https://elsewhere.example/v1/x",
		// Encoded so a single decode misses it, and the router's own parse finds it.
		"/v1/m%2563p", "/v1/m%63p", "/v1/oauth%2Fcontinue",
	} {
		res = callTool(t, s, "execute", map[string]any{"code": `return (await loomarr.request("POST", "` + path + `", {})).status`})
		if !res.IsError {
			t.Errorf("a script reached %s: %s", path, toolText(res))
		}
	}
}
