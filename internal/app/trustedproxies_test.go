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

// The retired trust_proxy switch grants nothing: an install that had it on and wrote no list trusts
// loopback only, like any other, and is warned to list its proxy. Anything written for the list wins.
func TestTrustedProxiesRetiredSwitchGrantsNothing(t *testing.T) {
	notTrusted := []string{"10.0.0.2", "172.16.0.2", "172.31.255.255", "192.168.1.2", "169.254.1.1", "fd00::2", "fe80::2",
		"100.101.102.103", "100.127.255.255", "203.0.113.9", "2001:db8::1"}
	assertLoopbackOnly := func(t *testing.T, got []netip.Prefix) {
		t.Helper()
		for _, addr := range []string{"127.0.0.1", "::1"} {
			if !contains(got, addr) {
				t.Errorf("loopback %s is not trusted", addr)
			}
		}
		for _, addr := range notTrusted {
			if contains(got, addr) {
				t.Errorf("%s is trusted: the retired switch must not widen trust", addr)
			}
		}
	}
	warned := func(t *testing.T, logs string) {
		t.Helper()
		for _, want := range []string{"security.trust_proxy", "security.trusted_proxies", "TRUSTED_PROXIES"} {
			if !strings.Contains(logs, want) {
				t.Errorf("startup warning does not name %q; log = %q", want, logs)
			}
		}
	}

	t.Run("stored on, no list: loopback only, with a warning", func(t *testing.T) {
		var logs bytes.Buffer
		got, err := trustedProxies(visionSet(t, map[string]string{"security.trust_proxy": "true"}), slog.New(slog.NewTextHandler(&logs, nil)))
		if err != nil {
			t.Fatal(err)
		}
		assertLoopbackOnly(t, got)
		warned(t, logs.String())
	})

	t.Run("environment on, no list: loopback only, with a warning", func(t *testing.T) {
		t.Setenv("TRUST_PROXY", "true")
		var logs bytes.Buffer
		got, err := trustedProxies(visionSet(t, nil), slog.New(slog.NewTextHandler(&logs, nil)))
		if err != nil {
			t.Fatal(err)
		}
		assertLoopbackOnly(t, got)
		warned(t, logs.String())
	})

	t.Run("switch off: no warning", func(t *testing.T) {
		var logs bytes.Buffer
		if _, err := trustedProxies(visionSet(t, map[string]string{"security.trust_proxy": "false"}), slog.New(slog.NewTextHandler(&logs, nil))); err != nil {
			t.Fatal(err)
		}
		if logs.Len() != 0 {
			t.Errorf("unexpected log with the switch off: %q", logs.String())
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
