package httpx

import (
	"fmt"
	"net/netip"
	"strings"
)

// ParseProxyPrefixes reads the `security.trusted_proxies` list: each entry is a CIDR range or a
// single address (a bare address is that one host). Host bits are masked off, so "192.0.2.77/24"
// is 192.0.2.0/24, and an IPv4-mapped address is the IPv4 host it names, since a dual-stack socket
// reports an IPv4 peer in that form. Blank entries are skipped. A host name, a zone, an
// IPv4-mapped range or anything else that is not an address range is an error rather than a
// silently dropped entry: an operator who typed a proxy that then isn't trusted would otherwise
// find every client sharing the proxy's address.
func ParseProxyPrefixes(entries []string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		prefix, err := parseProxyPrefix(entry)
		if err != nil {
			return nil, err
		}
		out = append(out, prefix)
	}
	return out, nil
}

func parseProxyPrefix(entry string) (netip.Prefix, error) {
	if strings.Contains(entry, "/") {
		prefix, err := netip.ParsePrefix(entry)
		if err != nil || prefix.Addr().Zone() != "" {
			return netip.Prefix{}, fmt.Errorf("%q is not an address range such as 192.0.2.0/24", entry)
		}
		if prefix.Addr().Is4In6() {
			return netip.Prefix{}, fmt.Errorf("%q is an IPv4-mapped range: write it as an IPv4 range", entry)
		}
		return prefix.Masked(), nil
	}
	addr, err := netip.ParseAddr(entry)
	if err != nil || addr.Zone() != "" {
		return netip.Prefix{}, fmt.Errorf("%q is not an IP address or address range such as 192.0.2.0/24", entry)
	}
	addr = addr.Unmap()
	return netip.PrefixFrom(addr, addr.BitLen()), nil
}
