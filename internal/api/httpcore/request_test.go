package httpcore

import (
	"fmt"
	"net/http"
	"net/netip"
	"testing"
)

// The login rate-limiter keys on ClientIP. X-Forwarded-For is attacker-controlled unless a
// trusted proxy wrote it: honoring it from a direct client would let that client rotate the header
// per request and defeat the per-IP throttle (§11, S-2). The trusted proxies are an explicit list
// (`security.trusted_proxies`); a peer outside it is a client, whatever range it is in.
func clientIPRequest(peer string, forwardedFor ...string) *http.Request {
	r, _ := http.NewRequest(http.MethodPost, "/v1/auth/login", nil)
	r.RemoteAddr = peer
	for _, v := range forwardedFor {
		r.Header.Add("X-Forwarded-For", v)
	}
	return r
}

// proxies is an Origin's trusted list written as CIDRs or bare addresses, all invented
// (RFC 5737 / RFC 3849 documentation ranges and a made-up tailnet address).
func proxies(t *testing.T, entries ...string) []netip.Prefix {
	t.Helper()
	var out []netip.Prefix
	for _, entry := range entries {
		if prefix, err := netip.ParsePrefix(entry); err == nil {
			out = append(out, prefix)
			continue
		}
		addr := netip.MustParseAddr(entry)
		out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return out
}

// loopbackOnly is the default of `security.trusted_proxies`.
func loopbackOnly(t *testing.T) Origin {
	t.Helper()
	return Origin{TrustedProxies: proxies(t, "127.0.0.0/8", "::1/128")}
}

func TestClientIPIgnoresForwardedForWithoutAnyTrustedProxy(t *testing.T) {
	for _, peer := range []string{"127.0.0.1:44321", "[::1]:44321", "192.0.2.10:44321"} {
		got := Origin{}.ClientIP(clientIPRequest(peer, "198.51.100.7"))
		want, _ := peerAddr(peer)
		if got != want.String() {
			t.Fatalf("empty list, peer %s: want the socket peer %s, got %q", peer, want, got)
		}
	}
}

// A client that connects directly is not a proxy, so its X-Forwarded-For is its own claim even
// when a proxy is listed (a published port next to the proxy's).
func TestClientIPDirectClientCannotChooseItsKey(t *testing.T) {
	origin := Origin{TrustedProxies: proxies(t, "192.0.2.10")}
	for _, forged := range []string{"198.51.100.7", "198.51.100.7, 10.0.0.1", "10.0.0.1", "not-an-ip"} {
		got := origin.ClientIP(clientIPRequest("203.0.113.9:44321", forged))
		if got != "203.0.113.9" {
			t.Errorf("direct client sending %q: key = %q, want its own address 203.0.113.9", forged, got)
		}
	}
}

// The default trusts the machine itself and nothing else. A private, link-local or shared-address
// peer reaching Loomarr directly is a client: before the list, every such peer counted as a proxy
// and chose its own throttle key with X-Forwarded-For.
func TestClientIPDefaultTrustsLoopbackOnly(t *testing.T) {
	origin := loopbackOnly(t)
	for _, tc := range []struct {
		name string
		peer string
		want string
	}{
		{"IPv4 loopback", "127.0.0.1:5000", "203.0.113.9"},
		{"other IPv4 loopback address", "127.1.2.3:5000", "203.0.113.9"},
		{"IPv6 loopback", "[::1]:5000", "203.0.113.9"},
		{"IPv4-mapped loopback", "[::ffff:127.0.0.1]:5000", "203.0.113.9"},
		{"tailnet device", "100.101.102.103:5000", "100.101.102.103"},
		{"RFC 1918 LAN client", "10.0.0.2:5000", "10.0.0.2"},
		{"RFC 1918 172.16/12", "172.20.0.4:5000", "172.20.0.4"},
		{"RFC 1918 192.168/16", "192.168.1.20:5000", "192.168.1.20"},
		{"IPv4 link-local", "169.254.1.1:5000", "169.254.1.1"},
		{"IPv6 unique-local", "[fd00::2]:5000", "fd00::2"},
		{"IPv6 link-local", "[fe80::2]:5000", "fe80::2"},
		{"documentation IPv4", "192.0.2.10:5000", "192.0.2.10"},
		{"documentation IPv6", "[2001:db8::10]:5000", "2001:db8::10"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := origin.ClientIP(clientIPRequest(tc.peer, "203.0.113.9")); got != tc.want {
				t.Errorf("ClientIP = %q, want %q", got, tc.want)
			}
		})
	}
}

// The probe: a tailnet (or LAN) device rotating its own X-Forwarded-For got a fresh key each time.
func TestClientIPForgedForwardedForFromANonProxyPeerCannotRotateTheKey(t *testing.T) {
	origin := loopbackOnly(t)
	for _, peer := range []string{"100.101.102.103:51000", "10.0.0.2:51000", "[fd7a:115c:a1e0::1]:51000", "[2001:db8::10]:51000"} {
		keys := map[string]bool{}
		for i := range 50 {
			keys[origin.ClientIP(clientIPRequest(peer, fmt.Sprintf("198.51.100.%d", i+1)))] = true
		}
		if len(keys) != 1 {
			t.Errorf("peer %s: %d distinct keys from 50 forged headers, want 1", peer, len(keys))
		}
	}
}

// A proxy appends the peer it saw, so the right-most hop is the address the trusted proxy actually
// observed. Only that one hop is trusted: a hop to its left was written by whoever sent the request,
// and a private address cannot be told apart from a LAN client, so a second proxy in a chain is
// keyed as the client (a stricter throttle, never a forgeable one).
func TestClientIPListedProxyUsesRightMostHop(t *testing.T) {
	origin := Origin{TrustedProxies: proxies(t,
		"127.0.0.0/8", "::1/128", "10.0.0.2", "192.0.2.0/25", "100.101.102.103", "2001:db8:5::/48")}
	for _, tc := range []struct {
		name string
		peer string
		xff  []string
		want string
	}{
		{"loopback proxy, one hop", "127.0.0.1:5000", []string{"203.0.113.9"}, "203.0.113.9"},
		{"ipv6 loopback proxy", "[::1]:5000", []string{"203.0.113.9"}, "203.0.113.9"},
		{"listed host, client forged a left hop", "10.0.0.2:5000", []string{"198.51.100.7, 203.0.113.9"}, "203.0.113.9"},
		{"chain of two proxies keys on the inner proxy", "10.0.0.2:5000", []string{"203.0.113.9, 172.18.0.4"}, "172.18.0.4"},
		{"header split over two lines", "10.0.0.2:5000", []string{"198.51.100.7", "203.0.113.9"}, "203.0.113.9"},
		{"no header", "10.0.0.2:5000", nil, "10.0.0.2"},
		{"LAN client behind the proxy", "10.0.0.2:5000", []string{"192.168.1.20"}, "192.168.1.20"},
		{"LAN client forged a left hop", "10.0.0.2:5000", []string{"198.51.100.7, 192.168.1.20"}, "192.168.1.20"},
		{"unparseable hop falls back to the peer", "10.0.0.2:5000", []string{"203.0.113.9, garbage"}, "10.0.0.2"},
		{"mapped form normalizes", "10.0.0.2:5000", []string{"::ffff:203.0.113.9"}, "203.0.113.9"},
		{"IPv6 client behind an IPv4 proxy", "10.0.0.2:5000", []string{"2001:db8:9::1"}, "2001:db8:9::1"},
		{"bracketed IPv6 hop is not an address", "10.0.0.2:5000", []string{"[2001:db8:9::1]"}, "10.0.0.2"},
		{"tailnet proxy listed by address", "100.101.102.103:5000", []string{"203.0.113.9"}, "203.0.113.9"},
		// A range's edges: 192.0.2.0/25 is .0 to .127.
		{"first address of a listed range", "192.0.2.0:5000", []string{"203.0.113.9"}, "203.0.113.9"},
		{"last address of a listed range", "192.0.2.127:5000", []string{"203.0.113.9"}, "203.0.113.9"},
		{"one past a listed range is a client", "192.0.2.128:5000", []string{"203.0.113.9"}, "192.0.2.128"},
		{"one before a listed range is a client", "192.0.1.255:5000", []string{"203.0.113.9"}, "192.0.1.255"},
		{"next host after a listed host is a client", "10.0.0.3:5000", []string{"203.0.113.9"}, "10.0.0.3"},
		{"previous host before a listed host is a client", "10.0.0.1:5000", []string{"203.0.113.9"}, "10.0.0.1"},
		// IPv6 ranges follow the same rule, and a v4 peer in mapped form meets a v4 entry.
		{"IPv6 proxy inside a listed /48", "[2001:db8:5:ffff::7]:5000", []string{"203.0.113.9"}, "203.0.113.9"},
		{"IPv6 peer just outside the /48", "[2001:db8:6::7]:5000", []string{"203.0.113.9"}, "2001:db8:6::7"},
		{"IPv4-mapped peer meets an IPv4 entry", "[::ffff:10.0.0.2]:5000", []string{"203.0.113.9"}, "203.0.113.9"},
		{"IPv6 proxy with a client on IPv6", "[2001:db8:5::1]:5000", []string{"2001:db8:9::77"}, "2001:db8:9::77"},
		{"raw peer without a port", "10.0.0.2", []string{"203.0.113.9"}, "203.0.113.9"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := origin.ClientIP(clientIPRequest(tc.peer, tc.xff...)); got != tc.want {
				t.Errorf("ClientIP = %q, want %q", got, tc.want)
			}
		})
	}
}

// The attack: one real client behind the proxy rotates the first hop on every attempt. The key
// must not move, or the login throttle never fills.
func TestClientIPSpoofedLeftMostHopCannotRotateTheKey(t *testing.T) {
	origin := Origin{TrustedProxies: proxies(t, "10.0.0.2")}
	keys := map[string]bool{}
	for i := range 50 {
		forged := fmt.Sprintf("198.51.100.%d", i+1)
		// nginx's $proxy_add_x_forwarded_for appends the peer it saw to what the client sent.
		keys[origin.ClientIP(clientIPRequest("10.0.0.2:5000", forged+", 203.0.113.9"))] = true
	}
	if len(keys) != 1 || !keys["203.0.113.9"] {
		t.Fatalf("keys = %v, want exactly {203.0.113.9}", keys)
	}

	// The same attack from a client on the LAN: its own address is private, like a proxy's.
	keys = map[string]bool{}
	for i := range 50 {
		forged := fmt.Sprintf("198.51.100.%d", i+1)
		keys[origin.ClientIP(clientIPRequest("10.0.0.2:5000", forged+", 192.168.1.20"))] = true
	}
	if len(keys) != 1 || !keys["192.168.1.20"] {
		t.Fatalf("LAN client: keys = %v, want exactly {192.168.1.20}", keys)
	}
}

// Every consumer of "did a proxy of ours send this?" asks the one question: the client address,
// X-Forwarded-Proto and the geography headers all ride FromTrustedProxy.
func TestFromTrustedProxyIsThePeerCheck(t *testing.T) {
	origin := Origin{TrustedProxies: proxies(t, "192.0.2.10", "2001:db8::/64")}
	for _, tc := range []struct {
		peer string
		want bool
	}{
		{"192.0.2.10:1", true},
		{"[::ffff:192.0.2.10]:1", true},
		{"192.0.2.11:1", false},
		{"[2001:db8::1]:1", true},
		{"[2001:db8:0:1::1]:1", false},
		{"127.0.0.1:1", false},
		{"not an address", false},
		{"", false},
	} {
		if got := origin.FromTrustedProxy(clientIPRequest(tc.peer)); got != tc.want {
			t.Errorf("FromTrustedProxy(%q) = %v, want %v", tc.peer, got, tc.want)
		}
	}
	if (Origin{}).FromTrustedProxy(clientIPRequest("127.0.0.1:1")) {
		t.Error("an empty list must trust nothing, loopback included")
	}
}

// cookie.secure=auto honors X-Forwarded-Proto: https only from a listed proxy (§11). A
// direct client's forwarding header is not trustworthy input; without TLS and without a
// trusted proxy, the cookie must NOT be marked Secure or a plain-HTTP LAN client's cookie
// stops being sent.
func TestSecureCookieAutoHonorsForwardedProtoOnlyFromAListedProxy(t *testing.T) {
	req := func(peer string) *http.Request {
		r, _ := http.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = peer
		r.Header.Set("X-Forwarded-Proto", "https")
		return r // r.TLS is nil: not a direct TLS request
	}
	listed := Origin{CookieSecure: "auto", TrustedProxies: proxies(t, "192.0.2.10", "2001:db8::/64")}

	if (Origin{CookieSecure: "auto"}).SecureCookie(req("127.0.0.1:1")) {
		t.Fatal("no trusted proxy: X-Forwarded-Proto must NOT force Secure on a non-TLS request")
	}
	for _, peer := range []string{"192.0.2.10:1", "[2001:db8::5]:1"} {
		if !listed.SecureCookie(req(peer)) {
			t.Errorf("listed proxy %s: X-Forwarded-Proto: https should mark the cookie Secure", peer)
		}
	}
	for _, peer := range []string{"192.0.2.11:1", "[2001:db8:1::5]:1", "127.0.0.1:1", "100.101.102.103:1"} {
		if listed.SecureCookie(req(peer)) {
			t.Errorf("peer %s is not a listed proxy: its X-Forwarded-Proto must not mark the cookie Secure", peer)
		}
	}

	// The forced modes ignore both the header and the proxy gate.
	if !(Origin{CookieSecure: "true"}).SecureCookie(req("192.0.2.99:1")) {
		t.Fatal("cookie.secure=always must force Secure")
	}
	if (Origin{CookieSecure: "false"}).SecureCookie(req("192.0.2.10:1")) {
		t.Fatal("cookie.secure=never must never set Secure")
	}
}
