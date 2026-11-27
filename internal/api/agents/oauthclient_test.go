package agents

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRedirectRules(t *testing.T) {
	for _, tc := range []struct {
		uri string
		ok  bool
	}{
		{"https://claude.ai/api/mcp/auth_callback", true},
		{"http://127.0.0.1:33418/callback", true},
		{"http://localhost/cb", true},
		{"http://[::1]:8080/cb", true},
		{"http://nas.example/cb", false},        // plain http off this machine
		{"https://claude.ai/cb#frag", false},    // a fragment (RFC 6749 §3.1.2)
		{"https://user:pw@claude.ai/cb", false}, // credentials in the address
		{"claude://callback", false},            // no host
		{"/relative", false},                    // not absolute
	} {
		if _, err := checkRedirect(tc.uri); (err == nil) != tc.ok {
			t.Errorf("checkRedirect(%q) err = %v, want ok=%v", tc.uri, err, tc.ok)
		}
	}
}

// The client ID carries its own registration, and authorize re-validates it rather than trusting
// that registration once did: a hand-made ID with a bad redirect is refused the same way.
func TestClientIDCarriesItsRegistration(t *testing.T) {
	c, err := newOAuthClient("  Claude  ", []string{"https://claude.ai/api/mcp/auth_callback"})
	if err != nil {
		t.Fatal(err)
	}
	back, err := parseClientID(c.id())
	if err != nil || back.Name != "Claude" || back.Redirects[0] != "https://claude.ai/api/mcp/auth_callback" {
		t.Fatalf("round trip = %+v, %v", back, err)
	}
	forged := oauthClient{Name: "x", Redirects: []string{"http://evil.example/cb"}}.id()
	if _, err := parseClientID(forged); err == nil {
		t.Fatal("a hand-made client ID with a plain-http redirect was accepted")
	}
	for _, bad := range []string{"", "lm1_!!!", "other_abc"} {
		if _, err := parseClientID(bad); err == nil {
			t.Errorf("parseClientID(%q) was accepted", bad)
		}
	}
}

func TestRedirectForMatchesExactlyExceptALoopbackPort(t *testing.T) {
	c, _ := newOAuthClient("App", []string{"https://app.example/cb", "http://127.0.0.1:9999/cb"})
	for _, tc := range []struct {
		requested string
		ok        bool
	}{
		{"https://app.example/cb", true},
		{"http://127.0.0.1:41234/cb", true},    // any loopback port (RFC 8252 §7.3)
		{"https://app.example/cb2", false},     // another path
		{"https://app.example:8443/cb", false}, // a non-loopback port is part of the address
		{"http://127.0.0.1:41234/other", false},
		{"", false}, // two registered: the request must say which
	} {
		if _, err := c.redirectFor(tc.requested); (err == nil) != tc.ok {
			t.Errorf("redirectFor(%q) err = %v, want ok=%v", tc.requested, err, tc.ok)
		}
	}
}

func TestDeviceNameShowsWhereTheGrantGoesAndFits(t *testing.T) {
	c, _ := newOAuthClient(strings.Repeat("Ä", 80), []string{"https://claude.ai/cb"})
	name := c.deviceName("https://claude.ai/cb")
	if n := utf8.RuneCountInString(name); n > maxPairDeviceName {
		t.Fatalf("device name is %d characters, the device list allows %d", n, maxPairDeviceName)
	}
	short, _ := newOAuthClient("Claude", []string{"https://claude.ai/cb"})
	if got := short.deviceName("https://claude.ai/cb"); got != "Claude (claude.ai)" {
		t.Fatalf("deviceName = %q, want \"Claude (claude.ai)\"", got)
	}
}

// A long host is the attack on a capped name: padding pushes the owner off the end. The cap must
// cost the client's name and the host's start, never the host's end.
func TestDeviceNameKeepsTheEndOfALongHost(t *testing.T) {
	redirect := "https://claude.ai." + strings.Repeat("a", 60) + ".attacker.example/cb"
	c, _ := newOAuthClient("Claude", []string{redirect})
	name := c.deviceName(redirect)
	if !strings.HasSuffix(name, ".attacker.example)") || utf8.RuneCountInString(name) > maxPairDeviceName {
		t.Fatalf("deviceName = %q (%d characters), want it to end with the real owner and fit", name, utf8.RuneCountInString(name))
	}
}

func TestClientNameCannotReorderOrHideText(t *testing.T) {
	// U+202E reverses what follows; U+200B is invisible; a newline would break the line.
	rlo, zwsp := string(rune(0x202E)), string(rune(0x200B))
	c, _ := newOAuthClient("Cla"+rlo+"ude"+zwsp+"\n  agent\x00", []string{"https://claude.ai/cb"})
	if c.Name != "Claude agent" {
		t.Fatalf("name = %q, want the control and format characters gone", c.Name)
	}
	if blank, _ := newOAuthClient(rlo+zwsp, []string{"https://claude.ai/cb"}); blank.Name != "AI agent" {
		t.Fatalf("a name of only hidden characters = %q, want the default", blank.Name)
	}
}

func TestPKCEValueShape(t *testing.T) {
	for s, want := range map[string]bool{
		strings.Repeat("a", 42):             false,
		strings.Repeat("a", 43):             true,
		strings.Repeat("A1-._~", 21) + "zz": true, // 128
		strings.Repeat("a", 129):            false,
		strings.Repeat("a", 42) + "+":       false,
		strings.Repeat("a", 42) + "=":       false,
	} {
		if got := isPKCEValue(s); got != want {
			t.Errorf("isPKCEValue(%d chars ending %q) = %v, want %v", len(s), s[len(s)-1:], got, want)
		}
	}
}
