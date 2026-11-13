package app

import (
	"fmt"
	"log/slog"
	"net/netip"

	"github.com/loomarr/loomarr/internal/platform/httpx"
	"github.com/loomarr/loomarr/internal/platform/settings"
)

// retiredTrustProxyRanges are the peers the retired `security.trust_proxy` switch treated as a
// reverse proxy: loopback, private, link-local and the shared (Tailscale) address space.
var retiredTrustProxyRanges = []string{
	"127.0.0.0/8", "::1/128",
	"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", // RFC 1918
	"169.254.0.0/16", "fe80::/10", // link-local
	"fc00::/7",      // IPv6 unique-local
	"100.64.0.0/10", // RFC 6598 shared space, where Tailscale numbers its peers
}

// trustedProxies resolves `security.trusted_proxies` into the ranges the API honors forwarding
// headers from.
//
// The retired `security.trust_proxy` switch is carried over only for an install that has it on and
// has never written the list (the list still resolves to its default): the old ranges apply, with a
// warning to narrow them, so a proxy that worked before the upgrade keeps working. The moment the
// operator saves or pins any list, even an empty one, the list alone decides; a stored list that no
// longer parses is a self-healed default and is not migrated either, because falling back to wider
// trust on a damaged value would be the wrong failure direction.
func trustedProxies(set resolved, log *slog.Logger) ([]netip.Prefix, error) {
	key := settings.SecurityTrustedProxies.Key()
	value := settings.SecurityTrustedProxies.Get(set)
	if set.svc != nil && !listWasWritten(set, key) && legacyTrustProxyOn(set, log) {
		log.Warn("TRUST_PROXY / security.trust_proxy is retired: trusting every loopback, private, link-local and "+
			"Tailscale peer as a reverse proxy until security.trusted_proxies (TRUSTED_PROXIES) lists the proxy's address",
			"key", key)
		value = retiredTrustProxyRanges
	}
	prefixes, err := httpx.ParseProxyPrefixes(value)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	return prefixes, nil
}

// listWasWritten reports whether an operator, not the registry default, supplied the list: a
// pinned or stored value, or a stored one that had to be healed.
func listWasWritten(set resolved, key string) bool {
	resolvedValue, ok := set.frozen[key]
	if !ok {
		resolvedValue = set.svc.Resolve(key)
	}
	return resolvedValue.Provenance != settings.ProvenanceDefault || resolvedValue.Caution
}

// legacyTrustProxyOn reads the retired switch strictly: a malformed value is not "on".
func legacyTrustProxyOn(set resolved, log *slog.Logger) bool {
	values, err := set.svc.ResolveMigrationOnly("security.trust_proxy")
	if err != nil {
		log.Warn("ignoring the retired security.trust_proxy setting", "error", err)
		return false
	}
	on, _ := values["security.trust_proxy"].Value.(bool)
	return on
}
