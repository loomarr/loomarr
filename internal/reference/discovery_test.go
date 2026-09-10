package reference

import (
	"context"
	"github.com/loomarr/loomarr/internal/testkit/httpfixture"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

const blockSearch = `{"query":{"search":[{"pageid":7,"title":"Friday Club (TV programming block)","snippet":"Friday Club was a television programming block."}]}}`
const blockMembers = `{"query":{"categorymembers":[{"pageid":7,"title":"Friday Club (TV programming block)"},{"pageid":8,"title":"Alpha House"},{"pageid":9,"title":"Beta Steps (TV series)"}]}}`

func TestDiscoverRequiresExplicitCategoryMembership(t *testing.T) {
	transport := httpfixture.NewScriptedTransport(
		httpfixture.Step{Response: webResponse(200, "application/json", blockSearch)},
		httpfixture.Step{Response: webResponse(200, "application/json", blockMembers)},
	)
	w := NewWeb(&http.Client{Transport: transport})
	w.resolver = fixedResolver{addrs: []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}}
	got, err := w.Discover(context.Background(), "Friday Club")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.TitleAnchors, "|") != "Alpha House|Beta Steps" || !strings.Contains(got.URL, "Category:Friday_Club") {
		t.Fatalf("evidence=%+v", got)
	}
	requests := transport.Requests()
	if len(requests) != 2 {
		t.Fatalf("requests=%d", len(requests))
	}
	first, _ := url.Parse(requests[0].URL)
	last, _ := url.Parse(requests[1].URL)
	if first.Query().Get("srsearch") != `"Friday Club"` || last.Query().Get("cmtitle") != "Category:Friday Club (TV programming block)" || last.Query().Get("cmnamespace") != "0" {
		t.Fatalf("queries=%s %s", first, last)
	}
}

func TestDiscoverRejectsAmbiguityAndMissingMembership(t *testing.T) {
	for _, tc := range []struct {
		name, search, members string
		wantError             bool
		calls                 int
	}{
		{"empty search", `{"query":{"search":[]}}`, "", false, 1},
		{"disambiguation", strings.Replace(blockSearch, "was a television programming block.", "may refer to a programming block.", 1), "", false, 1},
		{"unrelated name", strings.Replace(blockSearch, "Friday Club (TV", "Other Club (TV", 1), "", false, 1},
		{"two blocks", `{"query":{"search":[{"pageid":7,"title":"Friday Club (US)","snippet":"a programming block"},{"pageid":8,"title":"Friday Club (UK)","snippet":"a programming block"}]}}`, "", false, 1},
		{"category lacks subject", blockSearch, strings.Replace(blockMembers, `"pageid":7`, `"pageid":70`, 1), false, 2},
		{"no members", blockSearch, `{"query":{"categorymembers":[]}}`, false, 2},
		{"invalid member", blockSearch, strings.Replace(blockMembers, `"pageid":8`, `"pageid":0`, 1), true, 2},
		{"API error", `{"error":{"code":"ratelimited"}}`, "", true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var steps []httpfixture.Step
			for _, body := range []string{tc.search, tc.members} {
				if body != "" {
					steps = append(steps, httpfixture.Step{Response: webResponse(200, "application/json", body)})
				}
			}
			transport := httpfixture.NewScriptedTransport(steps...)
			w := NewWeb(&http.Client{Transport: transport})
			w.resolver = fixedResolver{addrs: []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}}
			got, err := w.Discover(context.Background(), "Friday Club")
			if (err != nil) != tc.wantError || got.URL != "" || len(got.TitleAnchors) != 0 || transport.Calls() != tc.calls {
				t.Fatalf("got=%+v err=%v calls=%d", got, err, transport.Calls())
			}
		})
	}
}

func TestDiscoverRetainsNetworkAndBodyBounds(t *testing.T) {
	for _, tc := range []struct {
		name, label, ip, body string
		calls                 int
	}{
		{"unsafe label", "Friday Club https://private.example", "203.0.113.10", "", 0},
		{"private DNS", "Friday Club", "127.0.0.1", "", 0},
		{"oversized response", "Friday Club", "203.0.113.10", strings.Repeat(" ", MaxResponseBytes+1), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transport := httpfixture.NewScriptedTransport(httpfixture.Step{Response: webResponse(200, "application/json", tc.body)})
			w := NewWeb(&http.Client{Transport: transport})
			w.resolver = fixedResolver{addrs: []net.IPAddr{{IP: net.ParseIP(tc.ip)}}}
			if _, err := w.Discover(context.Background(), tc.label); err == nil || transport.Calls() != tc.calls {
				t.Fatalf("err=%v calls=%d", err, transport.Calls())
			}
		})
	}
}
