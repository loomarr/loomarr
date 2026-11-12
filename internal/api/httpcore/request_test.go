package httpcore

import (
	"fmt"
	"net/http"
	"testing"
)

// The login rate-limiter keys on ClientIP. X-Forwarded-For is attacker-controlled unless a
// trusted proxy wrote it: honoring it from a direct client would let that client rotate the header
// per request and defeat the per-IP throttle (§11, S-2).
func clientIPRequest(peer string, forwardedFor ...string) *http.Request {
	r, _ := http.NewRequest(http.MethodPost, "/v1/auth/login", nil)
	r.RemoteAddr = peer
	for _, v := range forwardedFor {
		r.Header.Add("X-Forwarded-For", v)
	}
	return r
}

func TestClientIPIgnoresForwardedForWithoutTrustProxy(t *testing.T) {
	got := Origin{TrustProxy: false}.ClientIP(clientIPRequest("10.0.0.2:44321", "198.51.100.7"))
	if got != "10.0.0.2" {
		t.Fatalf("trust_proxy off: want the socket peer 10.0.0.2, got %q", got)
	}
}

// A client that connects directly is not a proxy, so its X-Forwarded-For is its own claim even
// when trust_proxy is on (a published port next to the proxy's).
func TestClientIPDirectClientCannotChooseItsKey(t *testing.T) {
	for _, forged := range []string{"198.51.100.7", "198.51.100.7, 10.0.0.1", "10.0.0.1", "not-an-ip"} {
		got := Origin{TrustProxy: true}.ClientIP(clientIPRequest("203.0.113.9:44321", forged))
		if got != "203.0.113.9" {
			t.Errorf("direct client sending %q: key = %q, want its own address 203.0.113.9", forged, got)
		}
	}
}

// Proxies append the peer they saw, so the right-most hop that is not itself a proxy is the
// address the nearest trusted proxy actually observed.
func TestClientIPTrustedProxyUsesRightMostUntrustedHop(t *testing.T) {
	for _, tc := range []struct {
		name string
		peer string
		xff  []string
		want string
	}{
		{"loopback proxy, one hop", "127.0.0.1:5000", []string{"203.0.113.9"}, "203.0.113.9"},
		{"ipv6 loopback proxy", "[::1]:5000", []string{"203.0.113.9"}, "203.0.113.9"},
		{"private proxy, client forged a left hop", "10.0.0.2:5000", []string{"198.51.100.7, 203.0.113.9"}, "203.0.113.9"},
		{"chain of two proxies", "10.0.0.2:5000", []string{"203.0.113.9, 172.18.0.4"}, "203.0.113.9"},
		{"header split over two lines", "10.0.0.2:5000", []string{"198.51.100.7", "203.0.113.9"}, "203.0.113.9"},
		{"no header", "10.0.0.2:5000", nil, "10.0.0.2"},
		{"all hops are proxies", "10.0.0.2:5000", []string{"192.168.1.20, 10.0.0.9"}, "192.168.1.20"},
		{"unparseable hop falls back to the peer", "10.0.0.2:5000", []string{"203.0.113.9, garbage"}, "10.0.0.2"},
		{"mapped and padded forms normalize", "10.0.0.2:5000", []string{"::ffff:203.0.113.9"}, "203.0.113.9"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := (Origin{TrustProxy: true}).ClientIP(clientIPRequest(tc.peer, tc.xff...)); got != tc.want {
				t.Errorf("ClientIP = %q, want %q", got, tc.want)
			}
		})
	}
}

// The attack: one real client behind the proxy rotates the first hop on every attempt. The key
// must not move, or the login throttle never fills.
func TestClientIPSpoofedLeftMostHopCannotRotateTheKey(t *testing.T) {
	origin := Origin{TrustProxy: true}
	keys := map[string]bool{}
	for i := range 50 {
		forged := fmt.Sprintf("198.51.100.%d", i+1)
		// nginx's $proxy_add_x_forwarded_for appends the peer it saw to what the client sent.
		keys[origin.ClientIP(clientIPRequest("10.0.0.2:5000", forged+", 203.0.113.9"))] = true
	}
	if len(keys) != 1 || !keys["203.0.113.9"] {
		t.Fatalf("keys = %v, want exactly {203.0.113.9}", keys)
	}
}

// cookie.secure=auto honors X-Forwarded-Proto: https only behind a trusted proxy (§11). A
// direct client's forwarding header is not trustworthy input; without TLS and without a
// trusted proxy, the cookie must NOT be marked Secure or a plain-HTTP LAN client's cookie
// stops being sent.
func TestSecureCookieAutoHonorsForwardedProtoOnlyWhenProxyTrusted(t *testing.T) {
	req := func() *http.Request {
		r, _ := http.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-Forwarded-Proto", "https")
		return r // r.TLS is nil: not a direct TLS request
	}

	if (Origin{CookieSecure: "auto", TrustProxy: false}).SecureCookie(req()) {
		t.Fatal("trust_proxy off: X-Forwarded-Proto must NOT force Secure on a non-TLS request")
	}
	if !(Origin{CookieSecure: "auto", TrustProxy: true}).SecureCookie(req()) {
		t.Fatal("trust_proxy on: X-Forwarded-Proto: https should mark the cookie Secure")
	}

	// The forced modes ignore both the header and the proxy gate.
	if !(Origin{CookieSecure: "true"}).SecureCookie(req()) {
		t.Fatal("cookie.secure=always must force Secure")
	}
	if (Origin{CookieSecure: "false"}).SecureCookie(req()) {
		t.Fatal("cookie.secure=never must never set Secure")
	}
}
