package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// bearerTransport sends every request with one Bearer token, as an MCP client configured with a
// static header does.
type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(r)
}

func connectMCP(t *testing.T, srv *httptest.Server, token string) (*mcp.ClientSession, error) {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-agent", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:             srv.URL + "/v1/mcp",
		HTTPClient:           &http.Client{Transport: bearerTransport{token: token, base: srv.Client().Transport}},
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}, nil)
	if err == nil {
		t.Cleanup(func() { _ = session.Close() })
	}
	return session, err
}

func mustConnectMCP(t *testing.T, srv *httptest.Server, token string) *mcp.ClientSession {
	t.Helper()
	s, err := connectMCP(t, srv, token)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	return s
}

func toolNames(t *testing.T, s *mcp.ClientSession) []string {
	t.Helper()
	res, err := s.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	return names
}

func callTool(t *testing.T, s *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("tools/call %s: %v", name, err)
	}
	return res
}

func toolText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

var adminOnlyTools = []string{"health", "why_not_playing", "needs_attention", "recent_events", "list_jobs", "job_history", "run_job", "approve_proposal", "deny_proposal"}

// The tool list is the caller's role: a member's agent is never told the admin tools exist, and an
// admin's is not offered the member-only ask its route would refuse.
func TestMCPListsTheToolsTheCallersRoleAllows(t *testing.T) {
	t.Parallel()
	h := newDeviceHarness(t, nil)
	member := pairDevice(t, h.Server, login(t, h.Server, "kid", "pw"))
	admin := pairDevice(t, h.Server, login(t, h.Server, "boss", "pw"))

	memberTools := toolNames(t, mustConnectMCP(t, h.Server, member))
	for _, name := range adminOnlyTools {
		if slices.Contains(memberTools, name) {
			t.Errorf("a member is offered the admin tool %q (tools: %v)", name, memberTools)
		}
	}
	for _, name := range []string{"whats_on", "guide", "star_channel", "request_channel_idea", "who_is_watching"} {
		if !slices.Contains(memberTools, name) {
			t.Errorf("a member is not offered %q (tools: %v)", name, memberTools)
		}
	}

	adminTools := toolNames(t, mustConnectMCP(t, h.Server, admin))
	for _, name := range append([]string{"whats_on", "star_channel"}, adminOnlyTools...) {
		if !slices.Contains(adminTools, name) {
			t.Errorf("an admin is not offered %q (tools: %v)", name, adminTools)
		}
	}
	if slices.Contains(adminTools, "request_channel_idea") {
		t.Errorf("an admin is offered request_channel_idea, whose route refuses admins")
	}
}

// Naming an admin tool the list did not offer gets nothing: the member's server does not have it.
func TestMCPMemberCannotCallAnAdminTool(t *testing.T) {
	t.Parallel()
	h := newDeviceHarness(t, nil)
	member := pairDevice(t, h.Server, login(t, h.Server, "kid", "pw"))
	s := mustConnectMCP(t, h.Server, member)

	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: "needs_attention"})
	if err == nil && !res.IsError {
		t.Fatalf("a member called needs_attention and got a result: %s", toolText(res))
	}
}

// A tool acts as the person who paired the agent: the star lands on that person's channels and
// shows in the app, through the same route the app uses.
func TestMCPToolActsAsTheCaller(t *testing.T) {
	t.Parallel()
	h := newDeviceHarness(t, nil)
	for i, id := range []string{"ch-news", "ch-films", "ch-kids"} {
		seedGridChannel(t, h.Store, id, i+1)
	}
	kidSession := login(t, h.Server, "kid", "pw")
	member := pairDevice(t, h.Server, kidSession)
	s := mustConnectMCP(t, h.Server, member)

	if res := callTool(t, s, "star_channel", map[string]any{"channel": "2"}); res.IsError {
		t.Fatalf("star_channel: %s", toolText(res))
	}
	code, mine := myChannelsCall(t, h.Server, http.MethodGet, "/v1/me/channels", kidSession, "")
	if code != http.StatusOK || len(mine.Favourites) != 1 || mine.Favourites[0].ChannelID != "ch-films" {
		t.Fatalf("after star_channel 2 the person's channels are %d %s, want ch-films starred", code, mine)
	}
	boss := pairDevice(t, h.Server, login(t, h.Server, "boss", "pw"))
	if _, theirs := myChannelsCall(t, h.Server, http.MethodGet, "/v1/me/channels", nil, boss); len(theirs.Favourites) != 0 {
		t.Fatalf("the star landed on someone else too: %s", theirs)
	}

	res := callTool(t, s, "my_channels", nil)
	var out struct {
		Starred []struct {
			Number int    `json:"number"`
			Name   string `json:"name"`
		} `json:"starred"`
	}
	if err := json.Unmarshal([]byte(toolText(res)), &out); err != nil || len(out.Starred) != 1 || out.Starred[0].Number != 2 {
		t.Fatalf("my_channels = %s (%v), want channel 2 starred", toolText(res), err)
	}

	// An ambiguous or unknown channel is a tool error the model can act on, not a guess.
	if res := callTool(t, s, "star_channel", map[string]any{"channel": "ch-"}); !res.IsError || !strings.Contains(toolText(res), "matches 3 channels") {
		t.Fatalf("star_channel 'ch-' = %s, want an error naming the three matches", toolText(res))
	}
	if res := callTool(t, s, "star_channel", map[string]any{"channel": "9"}); !res.IsError || !strings.Contains(toolText(res), "no channel is number 9") {
		t.Fatalf("star_channel 9 = %s, want 'no channel is number 9'", toolText(res))
	}
}

// A revoked device is refused at the endpoint itself, before any tool runs: nothing of an MCP
// session outlives the token.
func TestMCPRefusesARevokedToken(t *testing.T) {
	t.Parallel()
	h := newDeviceHarness(t, nil)
	member := pairDevice(t, h.Server, login(t, h.Server, "kid", "pw"))
	s := mustConnectMCP(t, h.Server, member)
	_ = toolNames(t, s)

	if resp := do(t, h.Server, http.MethodDelete, "/v1/auth/device", member, ""); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke current device = %d, want 204", resp.StatusCode)
	}
	if _, err := s.ListTools(context.Background(), nil); err == nil {
		t.Fatal("tools/list still answers after the device was revoked")
	}
	if _, err := connectMCP(t, h.Server, member); err == nil {
		t.Fatal("a revoked token connected")
	}
}

// The endpoint takes a Bearer token only. A signed-in browser session is refused, even with the
// CSRF header, so the one way in is the credential an MCP client really sends.
func TestMCPRefusesABrowserSession(t *testing.T) {
	t.Parallel()
	h := newDeviceHarness(t, nil)
	session := login(t, h.Server, "kid", "pw")
	req, _ := http.NewRequest(http.MethodPost, h.Server.URL+"/v1/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":"probe","method":"tools/list"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("X-Loomarr-Csrf", "1")
	req.AddCookie(session)
	resp, err := h.Server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized || !strings.HasPrefix(resp.Header.Get("WWW-Authenticate"), "Bearer") {
		t.Fatalf("a cookie session got %d (WWW-Authenticate %q), want 401 with a Bearer challenge", resp.StatusCode, resp.Header.Get("WWW-Authenticate"))
	}
}

// A session cookie sent WITH a Bearer token is refused, not ignored. The auth middleware resolves
// the cookie first, so the tool list would follow the cookie's person while every tool's nested
// call carried only the token: two identities in one request.
func TestMCPRefusesACookieAlongsideABearer(t *testing.T) {
	t.Parallel()
	h := newDeviceHarness(t, nil)
	session := login(t, h.Server, "kid", "pw")
	admin := pairDevice(t, h.Server, login(t, h.Server, "boss", "pw"))
	req, _ := http.NewRequest(http.MethodPost, h.Server.URL+"/v1/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":"probe","method":"tools/list"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+admin)
	req.Header.Set("X-Loomarr-Csrf", "1")
	req.AddCookie(session)
	resp, err := h.Server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("a cookie plus a Bearer token got %d, want 400", resp.StatusCode)
	}
}

// Every read tool answers through the real routes, and its reply stays small: a tool result is
// context the model carries for the rest of the conversation.
func TestMCPReadToolsAnswerCompactly(t *testing.T) {
	t.Parallel()
	h := newDeviceHarness(t, nil)
	for i, id := range []string{"ch-news", "ch-films", "ch-kids"} {
		seedGridChannel(t, h.Store, id, i+1)
	}
	admin := pairDevice(t, h.Server, login(t, h.Server, "boss", "pw"))
	s := mustConnectMCP(t, h.Server, admin)

	for _, call := range []struct {
		tool string
		args map[string]any
	}{
		{"list_channels", nil},
		{"whats_on", nil},
		{"whats_on", map[string]any{"channel": "films"}},
		{"tonight", nil},
		{"guide", nil},
		{"guide", map[string]any{"channel": "1", "hours": 2}},
		{"my_channels", nil},
		{"channel_ideas", nil},
		{"who_is_watching", nil},
		{"why_not_playing", map[string]any{"channel": "ch-kids"}},
		{"needs_attention", nil},
		{"recent_events", nil},
		{"list_jobs", nil},
	} {
		res := callTool(t, s, call.tool, call.args)
		text := toolText(res)
		t.Logf("%s %v: %d bytes", call.tool, call.args, len(text))
		if res.IsError {
			// A 501 is a subsystem this harness does not wire (no media library, no scheduler, no
			// event retention); the tool passing the route's own refusal through is the behaviour.
			if !strings.HasPrefix(text, "501: ") {
				t.Errorf("%s %v: %s", call.tool, call.args, text)
			}
			continue
		}
		if len(text) > 4<<10 {
			t.Errorf("%s %v replied %d bytes for three channels; a reply this size will not scale to a real lineup", call.tool, call.args, len(text))
		}
	}
}
