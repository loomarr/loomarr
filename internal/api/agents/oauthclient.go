package agents

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

// An OAuth client of the MCP endpoint, registered dynamically (RFC 7591).
//
// THE CLIENT ID IS THE REGISTRATION. Registration is open: any client may register any name and any
// allowed redirect address, as MCP's dynamic registration expects. A stored registration would
// therefore prove nothing a self-describing ID doesn't, and it would cost a table, a cleanup job,
// and every client's registration lost to a database restore. So the ID encodes the name and the
// redirect addresses, and authorize reads them back out of it.
//
// What keeps a client honest is the person, not the ID: /pair shows the name and the host the grant
// is sent to before anyone approves (oauth-request), and PKCE binds the code to the client that
// started the flow.

const (
	clientIDPrefix      = "lm1_"
	maxClientName       = 40
	maxRedirectURIs     = 8
	maxRedirectURIBytes = 512
)

type oauthClient struct {
	Name      string   `json:"n"`
	Redirects []string `json:"r"`
}

func (c oauthClient) id() string {
	raw, _ := json.Marshal(c)
	return clientIDPrefix + base64.RawURLEncoding.EncodeToString(raw)
}

func parseClientID(id string) (oauthClient, error) {
	rest, ok := strings.CutPrefix(id, clientIDPrefix)
	if !ok {
		return oauthClient{}, errors.New("unknown client_id; register the client first")
	}
	raw, err := base64.RawURLEncoding.DecodeString(rest)
	if err != nil {
		return oauthClient{}, errors.New("malformed client_id")
	}
	var c oauthClient
	if err := json.Unmarshal(raw, &c); err != nil {
		return oauthClient{}, errors.New("malformed client_id")
	}
	// The ID is client-supplied on every request, so it is re-validated, not trusted because
	// registration once checked it.
	return newOAuthClient(c.Name, c.Redirects)
}

// newOAuthClient validates a registration.
func newOAuthClient(name string, redirects []string) (oauthClient, error) {
	name = cleanName(name)
	if name == "" {
		name = "AI agent"
	}
	if len(redirects) == 0 || len(redirects) > maxRedirectURIs {
		return oauthClient{}, fmt.Errorf("give between 1 and %d redirect_uris", maxRedirectURIs)
	}
	for _, r := range redirects {
		if _, err := checkRedirect(r); err != nil {
			return oauthClient{}, err
		}
	}
	return oauthClient{Name: name, Redirects: redirects}, nil
}

// checkRedirect allows an https address, or http to this machine's loopback (a CLI or desktop
// app's local listener, RFC 8252 §7.3). Plain http anywhere else would hand the code to the network.
func checkRedirect(raw string) (*url.URL, error) {
	if len(raw) > maxRedirectURIBytes {
		return nil, errors.New("redirect_uri is too long")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Fragment != "" || u.User != nil {
		return nil, fmt.Errorf("redirect_uri %q is not an absolute address without a fragment", raw)
	}
	switch {
	case u.Scheme == "https":
		return u, nil
	case u.Scheme == "http" && isLoopback(u.Hostname()):
		return u, nil
	default:
		return nil, fmt.Errorf("redirect_uri %q must be https, or http to localhost", raw)
	}
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// redirectFor picks the redirect address an authorization request names: exactly one of the
// client's, except that a loopback address may use any port, because a local app listens on
// whatever port was free (RFC 8252 §7.3). Empty means the client's only address.
func (c oauthClient) redirectFor(requested string) (string, error) {
	if requested == "" {
		if len(c.Redirects) == 1 {
			return c.Redirects[0], nil
		}
		return "", errors.New("redirect_uri is required: the client registered more than one")
	}
	want, err := checkRedirect(requested)
	if err != nil {
		return "", err
	}
	for _, r := range c.Redirects {
		if r == requested {
			return r, nil
		}
		reg, _ := url.Parse(r)
		if reg.Scheme == "http" && want.Scheme == "http" && isLoopback(want.Hostname()) &&
			reg.Hostname() == want.Hostname() && reg.Path == want.Path && reg.RawQuery == want.RawQuery {
			return requested, nil
		}
	}
	return "", errors.New("redirect_uri is not one the client registered")
}

// cleanName is a client's self-chosen name made safe to show a person: no control, format (which
// includes the bidirectional overrides that reorder text) or private-use characters, whitespace
// collapsed, and capped. It is still the client's claim, which is why the host is shown beside it.
func cleanName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case unicode.IsSpace(r):
			b.WriteRune(' ')
		case unicode.In(r, unicode.Cc, unicode.Cf, unicode.Co, unicode.Cs) || r == utf8.RuneError:
		default:
			b.WriteRune(r)
		}
	}
	name = strings.Join(strings.Fields(b.String()), " ")
	return truncateRunes(name, maxClientName)
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// sendsTo is the host a grant is sent to: what the person checks before approving.
func sendsTo(redirect string) string {
	u, err := url.Parse(redirect)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// deviceName is how the grant appears in the device list: the client's own name and the host it
// sends the grant to, so "Claude (claude.ai)" cannot pass for "Claude (attacker.example)".
//
// The HOST is what must survive the length cap, and the end of a host is the part that names who
// owns it: "claude.ai.<padding>.attacker.example" keeps "…attacker.example", never "claude.ai.…".
// The name gives way first.
func (c oauthClient) deviceName(redirect string) string {
	host := sendsTo(redirect)
	if utf8.RuneCountInString(host) > maxHostShown {
		r := []rune(host)
		host = "…" + string(r[len(r)-(maxHostShown-1):])
	}
	suffix := " (" + host + ")"
	return truncateRunes(c.Name, maxPairDeviceName-utf8.RuneCountInString(suffix)) + suffix
}

const (
	// maxPairDeviceName mirrors deviceStartInput's maxLength in internal/api/people/deviceroutes.go.
	maxPairDeviceName = 64
	// maxHostShown leaves the name at least 21 of the 64 characters.
	maxHostShown = 40
)
