package filler

import (
	"net/http"
	"net/netip"
	"testing"

	"github.com/loomarr/loomarr/internal/api/httpcore"
)

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
		// With trust_proxy on, the header still counts only when the peer is a proxy: a client
		// reaching Loomarr directly cannot claim another country by sending one.
		{name: "direct client's own header ignored", remoteAddr: "1.1.1.1:44321", forwarded: "8.8.8.8", trustProxy: true, want: netip.MustParseAddr("1.1.1.1")},
		// Proxies append what they saw, so the left-most hop is whatever the client sent.
		{name: "client-sent left-most hop ignored", remoteAddr: "10.0.0.2:44321", forwarded: "9.9.9.9, 1.1.1.1", trustProxy: true, want: netip.MustParseAddr("1.1.1.1")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodGet, "/v1/locations/suggestion", nil)
			if err != nil {
				t.Fatal(err)
			}
			request.RemoteAddr = test.remoteAddr
			request.Header.Set("X-Forwarded-For", test.forwarded)
			got, ok := effectiveLocationClientAddress(request, httpcore.Origin{TrustProxy: test.trustProxy})
			if !ok || got != test.want {
				t.Fatalf("effective location address = %v, %v; want %v, true", got, ok, test.want)
			}
		})
	}
}
