package httpx

import (
	"context"
	"fmt"
	"net"
	"net/netip"
)

// IPLookup resolves a host name to its addresses; *net.Resolver satisfies it.
type IPLookup interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

// DialContextFunc is the dial hook of net/http.Transport.
type DialContextFunc func(context.Context, string, string) (net.Conn, error)

// IsPublicIP is the one answer to "may the server connect to this address on a
// stranger's say-so?". It is true only for a publicly routable unicast address:
// loopback, private, link-local, CGNAT (Tailscale), multicast, reserved and the
// IPv6 transition forms that embed an IPv4 address (mapped, compatible, NAT64,
// 6to4) are all refused. An invalid address is not public.
func IsPublicIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	return publicAddr(addr.Unmap().WithZone(""))
}

var (
	// reservedV4 are the IPv4 ranges the netip predicates do not cover.
	reservedV4 = mustPrefixes(
		"0.0.0.0/8",     // "this network"
		"100.64.0.0/10", // carrier-grade NAT, where Tailscale addresses live
		"192.0.0.0/24",  // IETF protocol assignments
		"198.18.0.0/15", // benchmarking
		"240.0.0.0/4",   // reserved, including the broadcast address
		"255.255.255.255/32",
	)
	// reservedV6 are IPv6 ranges that are never a public host.
	reservedV6 = mustPrefixes(
		"64:ff9b:1::/48", // local-use NAT64: the embedded IPv4 position varies, so refuse it whole
		"100::/64",       // discard-only
		"2001::/32",      // Teredo
	)
	nat64     = netip.MustParsePrefix("64:ff9b::/96")
	v4Compat  = netip.MustParsePrefix("::/96")
	sixToFour = netip.MustParsePrefix("2002::/16")
)

func mustPrefixes(cidrs ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(cidrs))
	for i, c := range cidrs {
		out[i] = netip.MustParsePrefix(c)
	}
	return out
}

func inAny(prefixes []netip.Prefix, addr netip.Addr) bool {
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

func publicAddr(addr netip.Addr) bool {
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
		return false
	}
	if addr.Is4() {
		return !inAny(reservedV4, addr)
	}
	if inAny(reservedV6, addr) {
		return false
	}
	b := addr.As16()
	switch {
	case nat64.Contains(addr), v4Compat.Contains(addr):
		// The low 32 bits are an IPv4 address; "::" and "::1" land in 0.0.0.0/8.
		return publicAddr(netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]}))
	case sixToFour.Contains(addr):
		return publicAddr(netip.AddrFrom4([4]byte{b[2], b[3], b[4], b[5]}))
	}
	return true
}

// PublicDialContext returns a dialer that resolves the host itself, refuses the
// connection if ANY answer is non-public, and dials the first resolved address
// by IP. Pinning the checked address closes the gap where DNS changes between
// validation and connect, so it is the only SSRF defence a client needs for
// stranger-supplied URLs: use it as http.Transport.DialContext.
func PublicDialContext(resolver IPLookup, dial DialContextFunc) DialContextFunc {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("public HTTP dial address: %w", err)
		}
		var addrs []net.IPAddr
		if ip := net.ParseIP(host); ip != nil {
			addrs = []net.IPAddr{{IP: ip}}
		} else {
			addrs, err = resolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("public HTTP resolve %q: %w", host, err)
			}
			if len(addrs) == 0 {
				return nil, fmt.Errorf("public HTTP resolve %q returned no addresses", host)
			}
		}
		for _, addr := range addrs {
			if !IsPublicIP(addr.IP) {
				return nil, fmt.Errorf("public HTTP refuses non-public address for %q", host)
			}
		}
		return dial(ctx, network, net.JoinHostPort(addrs[0].IP.String(), port))
	}
}
