package api

import (
	"net/http"
	"net/netip"
	"testing"
)

// The login rate-limiter keys on clientIP (authroutes.go). When Loomarr is NOT behind a
// trusted proxy, X-Forwarded-For is attacker-controlled: honoring it would let a direct
// client rotate the header per request and defeat the per-IP throttle (§11, S-2). So the
// default is to key on the socket peer and ignore the header; only trust_proxy=true honors it.
func TestClientIPHonorsForwardedForOnlyWhenProxyTrusted(t *testing.T) {
	req := func() *http.Request {
		r, _ := http.NewRequest(http.MethodPost, "/v1/auth/login", nil)
		r.RemoteAddr = "203.0.113.9:44321"
		r.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.1")
		return r
	}

	untrusted := (&Server{trustProxy: false}).clientIP(req())
	if untrusted != "203.0.113.9" {
		t.Fatalf("trust_proxy off: want socket peer 203.0.113.9, got %q — a spoofed XFF would bypass the login rate limit", untrusted)
	}

	trusted := (&Server{trustProxy: true}).clientIP(req())
	if trusted != "198.51.100.7" {
		t.Fatalf("trust_proxy on: want first forwarded hop 198.51.100.7, got %q", trusted)
	}
}

func TestEffectiveLocationClientAddressSupportsSocketAndTrustedForwardedForms(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		forwarded  string
		trustProxy bool
		want       netip.Addr
	}{
		{name: "IPv4 socket", remoteAddr: "1.1.1.1:44321", want: netip.MustParseAddr("1.1.1.1")},
		{name: "IPv6 socket", remoteAddr: "[2606:4700:4700::1111]:44321", want: netip.MustParseAddr("2606:4700:4700::1111")},
		{name: "raw IPv6", remoteAddr: "2606:4700:4700::1111", want: netip.MustParseAddr("2606:4700:4700::1111")},
		{name: "trusted forwarded IPv4", remoteAddr: "10.0.0.2:44321", forwarded: "1.1.1.1, 10.0.0.1", trustProxy: true, want: netip.MustParseAddr("1.1.1.1")},
		{name: "untrusted forwarded address ignored", remoteAddr: "1.1.1.1:44321", forwarded: "8.8.8.8", want: netip.MustParseAddr("1.1.1.1")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodGet, "/v1/locations/suggestion", nil)
			if err != nil {
				t.Fatal(err)
			}
			request.RemoteAddr = test.remoteAddr
			request.Header.Set("X-Forwarded-For", test.forwarded)
			got, ok := effectiveLocationClientAddress(request, test.trustProxy)
			if !ok || got != test.want {
				t.Fatalf("effective location address = %v, %v; want %v, true", got, ok, test.want)
			}
		})
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

	if (&Server{cookieSecure: "auto", trustProxy: false}).secureCookie(req()) {
		t.Fatal("trust_proxy off: X-Forwarded-Proto must NOT force Secure on a non-TLS request")
	}
	if !(&Server{cookieSecure: "auto", trustProxy: true}).secureCookie(req()) {
		t.Fatal("trust_proxy on: X-Forwarded-Proto: https should mark the cookie Secure")
	}

	// The forced modes ignore both the header and the proxy gate.
	if !(&Server{cookieSecure: "true"}).secureCookie(req()) {
		t.Fatal("cookie.secure=always must force Secure")
	}
	if (&Server{cookieSecure: "false"}).secureCookie(req()) {
		t.Fatal("cookie.secure=never must never set Secure")
	}
}
