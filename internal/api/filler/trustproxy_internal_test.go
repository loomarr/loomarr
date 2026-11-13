package filler

import (
	"net/http"
	"net/netip"
	"testing"

	"github.com/loomarr/loomarr/internal/api/httpcore"
)

func TestEffectiveLocationClientAddressSupportsSocketAndTrustedForwardedForms(t *testing.T) {
	// 10.0.0.2 and 2001:db8::2 are the proxies; documentation addresses stand in for clients.
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.2/32"), netip.MustParsePrefix("2001:db8::/64")}
	tests := []struct {
		name       string
		remoteAddr string
		forwarded  string
		trusted    []netip.Prefix
		want       netip.Addr
	}{
		{name: "IPv4 socket", remoteAddr: "1.1.1.1:44321", want: netip.MustParseAddr("1.1.1.1")},
		{name: "IPv6 socket", remoteAddr: "[2606:4700:4700::1111]:44321", want: netip.MustParseAddr("2606:4700:4700::1111")},
		{name: "raw IPv6", remoteAddr: "2606:4700:4700::1111", want: netip.MustParseAddr("2606:4700:4700::1111")},
		{name: "trusted forwarded IPv4", remoteAddr: "10.0.0.2:44321", forwarded: "1.1.1.1", trusted: trusted, want: netip.MustParseAddr("1.1.1.1")},
		{name: "trusted IPv6 proxy forwarding IPv4", remoteAddr: "[2001:db8::2]:44321", forwarded: "1.1.1.1", trusted: trusted, want: netip.MustParseAddr("1.1.1.1")},
		{name: "trusted IPv6 proxy forwarding IPv6", remoteAddr: "[2001:db8::2]:44321", forwarded: "2606:4700:4700::1111", trusted: trusted, want: netip.MustParseAddr("2606:4700:4700::1111")},
		// Only the trusted proxy's own hop counts, so a second proxy is what gets looked up (a
		// private address, so no suggestion) rather than a hop a client could have written.
		{name: "chain of two proxies", remoteAddr: "10.0.0.2:44321", forwarded: "1.1.1.1, 10.0.0.1", trusted: trusted, want: netip.MustParseAddr("10.0.0.1")},
		{name: "untrusted forwarded address ignored", remoteAddr: "1.1.1.1:44321", forwarded: "8.8.8.8", want: netip.MustParseAddr("1.1.1.1")},
		// With a proxy listed, the header still counts only when the peer is that proxy: a client
		// reaching Loomarr directly cannot claim another country by sending one.
		{name: "direct client's own header ignored", remoteAddr: "1.1.1.1:44321", forwarded: "8.8.8.8", trusted: trusted, want: netip.MustParseAddr("1.1.1.1")},
		// A private peer that is not listed is a client, not a proxy.
		{name: "unlisted private peer's header ignored", remoteAddr: "10.0.0.3:44321", forwarded: "8.8.8.8", trusted: trusted, want: netip.MustParseAddr("10.0.0.3")},
		{name: "unlisted IPv6 peer's header ignored", remoteAddr: "[2001:db8:1::2]:44321", forwarded: "8.8.8.8", trusted: trusted, want: netip.MustParseAddr("2001:db8:1::2")},
		// Proxies append what they saw, so the left-most hop is whatever the client sent.
		{name: "client-sent left-most hop ignored", remoteAddr: "10.0.0.2:44321", forwarded: "9.9.9.9, 1.1.1.1", trusted: trusted, want: netip.MustParseAddr("1.1.1.1")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodGet, "/v1/locations/suggestion", nil)
			if err != nil {
				t.Fatal(err)
			}
			request.RemoteAddr = test.remoteAddr
			request.Header.Set("X-Forwarded-For", test.forwarded)
			got, ok := effectiveLocationClientAddress(request, httpcore.Origin{TrustedProxies: test.trusted})
			if !ok || got != test.want {
				t.Fatalf("effective location address = %v, %v; want %v, true", got, ok, test.want)
			}
		})
	}
}
