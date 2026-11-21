package httpx

import (
	"net/netip"
	"testing"
)

func TestParseProxyPrefixes(t *testing.T) {
	good := []struct {
		name string
		in   []string
		want []string
	}{
		{"nothing is an empty list", nil, nil},
		{"IPv4 CIDR", []string{"192.0.2.0/24"}, []string{"192.0.2.0/24"}},
		{"IPv6 CIDR", []string{"2001:db8::/64"}, []string{"2001:db8::/64"}},
		{"bare IPv4 is one host", []string{"192.0.2.10"}, []string{"192.0.2.10/32"}},
		{"bare IPv6 is one host", []string{"2001:db8::10"}, []string{"2001:db8::10/128"}},
		{"IPv4-mapped bare address is the IPv4 host", []string{"::ffff:192.0.2.10"}, []string{"192.0.2.10/32"}},
		{"host bits are masked off", []string{"192.0.2.77/24"}, []string{"192.0.2.0/24"}},
		{"padding and empty entries are ignored", []string{" 192.0.2.10 ", "", "  "}, []string{"192.0.2.10/32"}},
		{"order is kept", []string{"127.0.0.0/8", "::1/128"}, []string{"127.0.0.0/8", "::1/128"}},
		{"the whole address space is accepted when the operator writes it", []string{"0.0.0.0/0"}, []string{"0.0.0.0/0"}},
	}
	for _, tc := range good {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseProxyPrefixes(tc.in)
			if err != nil {
				t.Fatalf("ParseProxyPrefixes(%q) = %v", tc.in, err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("ParseProxyPrefixes(%q) = %v, want %v", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != netip.MustParsePrefix(tc.want[i]) {
					t.Errorf("entry %d = %v, want %s", i, got[i], tc.want[i])
				}
			}
		})
	}

	for _, bad := range []string{
		"proxy.example.test", "192.0.2.0/33", "2001:db8::/129", "192.0.2.256", "192.0.2.0/-1",
		"fe80::1%eth0", "::ffff:192.0.2.0/120", "192.0.2.10/", "/24", "192.0.2.10 192.0.2.11",
	} {
		if got, err := ParseProxyPrefixes([]string{"127.0.0.1", bad}); err == nil {
			t.Errorf("ParseProxyPrefixes(%q) = %v, want an error", bad, got)
		}
	}
}
