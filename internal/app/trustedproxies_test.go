package app

import (
	"bytes"
	"log/slog"
	"net/netip"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/settings"
)

func contains(prefixes []netip.Prefix, addr string) bool {
	a := netip.MustParseAddr(addr)
	for _, p := range prefixes {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// The default trusts the machine itself and nothing else.
func TestTrustedProxiesDefaultIsLoopbackOnly(t *testing.T) {
	got, err := trustedProxies(visionSet(t, nil), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	for _, addr := range []string{"127.0.0.1", "127.255.255.255", "::1"} {
		if !contains(got, addr) {
			t.Errorf("default does not trust %s", addr)
		}
	}
	for _, addr := range []string{"10.0.0.2", "172.16.0.2", "192.168.1.2", "100.101.102.103", "169.254.1.1", "fd00::2", "fe80::2", "192.0.2.10", "2001:db8::1", "::2"} {
		if contains(got, addr) {
			t.Errorf("default trusts %s, want loopback only", addr)
		}
	}
}

func TestTrustedProxiesReadsTheConfiguredList(t *testing.T) {
	got, err := trustedProxies(visionSet(t, map[string]string{
		"security.trusted_proxies": "192.0.2.10, 198.51.100.0/24, 2001:db8:5::/48",
	}), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	for addr, want := range map[string]bool{
		"192.0.2.10": true, "192.0.2.11": false, "198.51.100.0": true, "198.51.100.255": true, "198.51.101.0": false,
		"2001:db8:5:ffff::1": true, "2001:db8:6::1": false, "127.0.0.1": false, "::1": false,
	} {
		if contains(got, addr) != want {
			t.Errorf("trusts %s = %v, want %v", addr, !want, want)
		}
	}
}

// An operator who clears the list trusts no peer, loopback included.
func TestTrustedProxiesEmptyListTrustsNothing(t *testing.T) {
	got, err := trustedProxies(visionSet(t, map[string]string{"security.trusted_proxies": ""}), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("explicit empty list trusts %v, want none", got)
	}
}

// A stored list that no longer parses self-heals in the settings layer to the default, never to a
// wider trust.
func TestTrustedProxiesMalformedStoredListFallsBackToLoopback(t *testing.T) {
	got, err := trustedProxies(visionSet(t, map[string]string{"security.trusted_proxies": "proxy.example.test"}), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(got, "127.0.0.1") || contains(got, "10.0.0.2") || contains(got, "192.0.2.10") {
		t.Fatalf("malformed list trusts %v, want the loopback default", got)
	}
}

// The retired trust_proxy switch: an install that had it on keeps a working proxy until the
// operator writes the list, and is told so. Anything the operator wrote for the list wins.
func TestTrustedProxiesMigratesTheRetiredSwitch(t *testing.T) {
	legacyRanges := []string{"127.0.0.1", "::1", "10.0.0.2", "172.16.0.2", "172.31.255.255", "192.168.1.2", "169.254.1.1", "fd00::2", "fe80::2", "100.101.102.103", "100.127.255.255"}
	public := []string{"203.0.113.9", "198.51.100.7", "100.63.255.255", "100.128.0.1", "172.32.0.1", "2001:db8::1", "2606:4700:4700::1111"}

	t.Run("stored on, no list: the old ranges, with a warning", func(t *testing.T) {
		var logs bytes.Buffer
		log := slog.New(slog.NewTextHandler(&logs, nil))
		got, err := trustedProxies(visionSet(t, map[string]string{"security.trust_proxy": "true"}), log)
		if err != nil {
			t.Fatal(err)
		}
		for _, addr := range legacyRanges {
			if !contains(got, addr) {
				t.Errorf("migrated list does not trust %s, which the switch did", addr)
			}
		}
		for _, addr := range public {
			if contains(got, addr) {
				t.Errorf("migrated list trusts %s, which the switch never did", addr)
			}
		}
		if !strings.Contains(logs.String(), "security.trusted_proxies") {
			t.Errorf("no warning naming the setting to narrow; log = %q", logs.String())
		}
	})

	t.Run("environment on, no list", func(t *testing.T) {
		t.Setenv("TRUST_PROXY", "true")
		got, err := trustedProxies(visionSet(t, nil), slog.New(slog.DiscardHandler))
		if err != nil {
			t.Fatal(err)
		}
		if !contains(got, "10.0.0.2") || !contains(got, "100.101.102.103") {
			t.Fatalf("env TRUST_PROXY=true with no list trusts %v, want the old ranges", got)
		}
	})

	t.Run("switch off stays the loopback default", func(t *testing.T) {
		got, err := trustedProxies(visionSet(t, map[string]string{"security.trust_proxy": "false"}), slog.New(slog.DiscardHandler))
		if err != nil {
			t.Fatal(err)
		}
		if contains(got, "10.0.0.2") || !contains(got, "127.0.0.1") {
			t.Fatalf("switch off trusts %v, want loopback only", got)
		}
	})

	t.Run("a written list wins over the switch", func(t *testing.T) {
		got, err := trustedProxies(visionSet(t, map[string]string{
			"security.trust_proxy": "true", "security.trusted_proxies": "192.0.2.10",
		}), slog.New(slog.DiscardHandler))
		if err != nil {
			t.Fatal(err)
		}
		if !contains(got, "192.0.2.10") || contains(got, "10.0.0.2") || contains(got, "127.0.0.1") {
			t.Fatalf("list beside the switch trusts %v, want only 192.0.2.10", got)
		}
	})

	t.Run("an emptied list wins over the switch", func(t *testing.T) {
		got, err := trustedProxies(visionSet(t, map[string]string{
			"security.trust_proxy": "true", "security.trusted_proxies": "",
		}), slog.New(slog.DiscardHandler))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Fatalf("emptied list beside the switch trusts %v, want none", got)
		}
	})

	t.Run("a malformed stored list does not fall back to the old ranges", func(t *testing.T) {
		got, err := trustedProxies(visionSet(t, map[string]string{
			"security.trust_proxy": "true", "security.trusted_proxies": "proxy.example.test",
		}), slog.New(slog.DiscardHandler))
		if err != nil {
			t.Fatal(err)
		}
		if contains(got, "10.0.0.2") {
			t.Fatalf("malformed list widened to the old ranges: %v", got)
		}
	})

	t.Run("a malformed switch is not on", func(t *testing.T) {
		got, err := trustedProxies(visionSet(t, map[string]string{"security.trust_proxy": "perhaps"}), slog.New(slog.DiscardHandler))
		if err != nil {
			t.Fatal(err)
		}
		if contains(got, "10.0.0.2") {
			t.Fatalf("malformed switch trusts %v", got)
		}
	})
}

// The list is read once per generation: a save made after boot waits for the next one, and the
// settings page says so, because the HTTP layer holds the ranges it was built with.
func TestTrustedProxiesAreFrozenForTheGeneration(t *testing.T) {
	set := visionSet(t, map[string]string{"security.trusted_proxies": "192.0.2.10"})
	frozen, _ := set.freeze(settings.NewRegistry().RestartKeys()...)
	set.svc.SetDB(map[string]string{"security.trusted_proxies": "198.51.100.7"})
	got, err := trustedProxies(frozen, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(got, "192.0.2.10") || contains(got, "198.51.100.7") {
		t.Fatalf("frozen generation trusts %v, want the list it started with", got)
	}
}

// No settings service is a supported degraded boot: the default still holds.
func TestTrustedProxiesWithoutAServiceIsLoopbackOnly(t *testing.T) {
	got, err := trustedProxies(resolved{}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(got, "127.0.0.1") || contains(got, "10.0.0.2") {
		t.Fatalf("no service trusts %v, want loopback only", got)
	}
}
