package httpx

import (
	"net"
	"testing"
)

func TestIsPublicIPClassifiesEveryNonPublicRange(t *testing.T) {
	for _, tc := range []struct {
		name, ip string
		public   bool
	}{
		// Public.
		{"cloudflare v4", "1.1.1.1", true},
		{"google v4", "8.8.8.8", true},
		{"documentation TEST-NET-3 stays routable", "203.0.113.10", true},
		{"public v6", "2606:4700:4700::1111", true},
		{"mapped public v4", "::ffff:8.8.8.8", true},
		{"nat64 public v4", "64:ff9b::808:808", true},
		{"6to4 public v4", "2002:808:808::1", true},
		{"just below CGNAT", "100.63.255.255", true},
		{"just above CGNAT", "100.128.0.0", true},
		{"just below benchmarking", "198.17.255.255", true},
		{"just above benchmarking", "198.20.0.0", true},

		// IPv4 special ranges.
		{"this network", "0.1.2.3", false},
		{"unspecified", "0.0.0.0", false},
		{"loopback", "127.0.0.1", false},
		{"loopback high", "127.255.255.254", false},
		{"private 10", "10.0.0.4", false},
		{"private 172", "172.16.0.1", false},
		{"private 192", "192.168.1.20", false},
		{"link-local", "169.254.169.254", false},
		{"CGNAT low", "100.64.0.1", false},
		{"CGNAT tailscale", "100.100.100.100", false},
		{"CGNAT high", "100.127.255.254", false},
		{"IETF protocol assignments", "192.0.0.8", false},
		{"benchmarking low", "198.18.0.1", false},
		{"benchmarking high", "198.19.255.254", false},
		{"multicast", "224.0.0.1", false},
		{"reserved", "240.0.0.1", false},
		{"broadcast", "255.255.255.255", false},

		// IPv6 special ranges.
		{"v6 unspecified", "::", false},
		{"v6 loopback", "::1", false},
		{"v6 link-local", "fe80::1", false},
		{"v6 unique-local fc", "fc00::1", false},
		{"v6 unique-local fd", "fd12:3456::1", false},
		{"v6 multicast", "ff02::1", false},
		{"v6 discard-only", "100::1", false},
		{"teredo", "2001:0:4136:e378:8000:63bf:3fff:fdd2", false},

		// Embedded IPv4 forms of every v4 rule.
		{"mapped loopback", "::ffff:127.0.0.1", false},
		{"mapped private", "::ffff:10.0.0.4", false},
		{"mapped CGNAT", "::ffff:100.64.0.1", false},
		{"mapped link-local", "::ffff:169.254.169.254", false},
		{"compatible loopback", "::7f00:1", false},
		{"compatible private", "::a00:4", false},
		{"compatible CGNAT", "::6440:1", false},
		{"nat64 CGNAT", "64:ff9b::6440:1", false},
		{"nat64 private", "64:ff9b::a00:1", false},
		{"nat64 loopback", "64:ff9b::7f00:1", false},
		{"nat64 benchmarking", "64:ff9b::c612:1", false},
		{"nat64 local-use public v4", "64:ff9b:1::808:808", false},
		{"nat64 local-use private", "64:ff9b:1::a00:1", false},
		{"6to4 loopback", "2002:7f00:1::1", false},
		{"6to4 private", "2002:a00:1::1", false},
		{"6to4 CGNAT", "2002:6440:1::1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			if ip == nil {
				t.Fatalf("bad test address %q", tc.ip)
			}
			if got := IsPublicIP(ip); got != tc.public {
				t.Fatalf("IsPublicIP(%s) = %v, want %v", tc.ip, got, tc.public)
			}
		})
	}
}

func TestIsPublicIPRefusesAnInvalidAddress(t *testing.T) {
	if IsPublicIP(nil) || IsPublicIP(net.IP{1, 2, 3}) {
		t.Fatal("an invalid address must never read as public")
	}
}

func TestIsPublicIPAcceptsTheFourByteForm(t *testing.T) {
	if !IsPublicIP(net.IPv4(8, 8, 8, 8).To4()) || IsPublicIP(net.IPv4(100, 64, 0, 1).To4()) {
		t.Fatal("4-byte addresses are classified by their v4 value")
	}
}
