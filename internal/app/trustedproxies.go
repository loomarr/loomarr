package app

import (
	"fmt"
	"log/slog"
	"net/netip"

	"github.com/loomarr/loomarr/internal/platform/httpx"
	"github.com/loomarr/loomarr/internal/platform/settings"
)

// trustedProxies resolves `security.trusted_proxies` into the ranges the API honors forwarding
// headers from.
//
// The retired `security.trust_proxy` switch grants nothing. An install that has it on and has never
// written the list trusts loopback only, like any other, and is warned at startup that its proxy
// must now be listed: carrying the old private and tailnet ranges over would keep the forgery hole
// the list closes.
func trustedProxies(set resolved, log *slog.Logger) ([]netip.Prefix, error) {
	key := settings.SecurityTrustedProxies.Key()
	value := settings.SecurityTrustedProxies.Get(set)
	if set.svc != nil && !listWasWritten(set, key) && legacyTrustProxyOn(set, log) {
		log.Warn("TRUST_PROXY / security.trust_proxy is retired and no longer trusts any proxy: only loopback is trusted. "+
			"List the reverse proxy's address in security.trusted_proxies (TRUSTED_PROXIES), or every visitor's "+
			"sign-in throttle keys on the proxy's address",
			"key", key)
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
