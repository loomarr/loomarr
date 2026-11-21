package settings

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// guide.timezone is served as-is to every client (the Guide, /v1/household) and read by the server
// to lay the rolling-window grid. time.LoadLocation accepts two names that are not a zone a
// household can pick, "" (UTC) and "Local" (the server's own zone, which a browser's Intl rejects),
// and an unvalidated string accepted anything at all.
func guideTimezoneSetting(t *testing.T) Setting {
	t.Helper()
	s, ok := NewRegistry().Get("guide.timezone")
	if !ok {
		t.Fatal("guide.timezone is not declared")
	}
	return s
}

func TestGuideTimezoneParse(t *testing.T) {
	s := guideTimezoneSetting(t)
	accepted := map[string]string{
		"":                               "",
		"   ":                            "", // padding only is the unset value
		"\t\n":                           "",
		"UTC":                            "UTC",
		"America/New_York":               "America/New_York",
		" America/New_York ":             "America/New_York",
		"\tEurope/Berlin\n":              "Europe/Berlin",
		"Pacific/Auckland":               "Pacific/Auckland",
		"Asia/Kolkata":                   "Asia/Kolkata",
		"America/Argentina/Buenos_Aires": "America/Argentina/Buenos_Aires",
	}
	for raw, want := range accepted {
		got, err := s.parse(raw)
		if err != nil || got != want {
			t.Errorf("parse(%q) = %v, %v; want %q", raw, got, err, want)
		}
	}
	for _, raw := range []string{
		"Local", " Local ", "Local\n",
		"Mars/Olympus", "not a zone", "America/New York", "America/", "America", "/", "UTC+5",
		"../etc/localtime", "America/../Europe/Berlin", "/usr/share/zoneinfo/UTC", `America\New_York`,
		"america/new_york", "Europe/Berlin\x00",
	} {
		if got, err := s.parse(raw); err == nil {
			t.Errorf("parse(%q) = %v, want an error", raw, got)
		}
	}
}

// guide.timezone is checked against the list the setting offers (timezonePresets), not against what
// the host's zoneinfo can load. time.LoadLocation reads the host's database first, so a host that has
// them accepts names no client knows and no household picked: the tz database's rule files and
// leap-second trees (`posixrules`, `right/UTC`, `posix/…`), the placeholder zone (`Factory`), and
// the legacy names. The same value must then validate the same on every host.
func TestGuideTimezoneIsCheckedAgainstTheListNotTheHost(t *testing.T) {
	s := guideTimezoneSetting(t)
	offered := map[string]bool{}
	for _, p := range s.Presets {
		offered[p.Value] = true
	}

	refused := []string{
		// The probe of the 2026-11-13 re-score, and the trees beside it.
		"posixrules", "right/UTC", "Factory", "right/America/New_York", "posix/America/New_York", "posix/UTC",
		"localtime", "Local",
		// Legacy and fixed-offset names are zones Go loads and a household does not pick.
		"EST5EDT", "GMT0", "US/Eastern", "US/Pacific", "GB", "Zulu", "EST", "Etc/GMT+5", "Etc/UTC", "GMT", "UCT",
		// Spelling: the list is exact.
		"utc", "Utc", "america/new_york", "AMERICA/NEW_YORK",
	}
	for _, name := range refused {
		if offered[name] {
			t.Fatalf("test setup: %q is offered", name)
		}
		if _, err := s.parse(name); err == nil {
			t.Errorf("parse(%q) accepted: it is not a timezone the setting offers", name)
		}
		// Padding changes nothing: the same name, trimmed, is the same verdict.
		if _, err := s.parse(" " + name + "\t"); err == nil {
			t.Errorf("parse(%q padded) accepted", name)
		}
	}

	// Exhaustive over Go's own zone names: a name validates exactly when the picker offers it. That
	// is what makes the verdict the same on every host.
	for _, name := range goZoneNames {
		_, err := s.parse(name)
		if (err == nil) != offered[name] {
			t.Errorf("parse(%q) err=%v, but the picker offers it = %v", name, err, offered[name])
		}
	}
	// And every offered zone is one the server can lay a window grid on, whatever the host has.
	t.Setenv("ZONEINFO", t.TempDir()) // a host database that knows nothing: LoadLocation falls to the embedded data
	for name := range offered {
		if name == "" {
			continue
		}
		if _, err := time.LoadLocation(name); err != nil {
			t.Errorf("offered zone %q does not load: %v", name, err)
		}
	}
}

// The refusal says what to do about it, whichever name it was.
func TestGuideTimezoneRefusalNamesTheValueAndTheRemedy(t *testing.T) {
	for _, name := range []string{"Factory", "US/Eastern", "posixrules", "Mars/Olympus", "Local"} {
		err := ianaZone(name)
		if err == nil {
			t.Fatalf("ianaZone(%q) accepted", name)
		}
		for _, want := range []string{name, "America/New_York"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("ianaZone(%q) = %q, want it to mention %q", name, err, want)
			}
		}
	}
	if err := ianaZone(42); err == nil {
		t.Error("a non-string was accepted")
	}
}

// A value stored before the check existed heals to the default on read, so no client is handed a
// name its own timezone API throws on, and the server's window grid is not silently on another zone.
func TestGuideTimezoneStoredBadValueHealsToUnset(t *testing.T) {
	// The legacy names an older host's check let through heal the same way: stored data is held to the
	// one rule, and the setting shows its caution.
	for _, stored := range []string{"Mars/Olympus", "not a zone", "Local", " Local ", "posixrules", "right/UTC", "Factory", "EST5EDT", "US/Eastern"} {
		s, err := New(context.Background(), NewRegistry(), fakeLoader{m: map[string]string{"guide.timezone": stored}}, slog.New(slog.DiscardHandler))
		if err != nil {
			t.Fatal(err)
		}
		got := s.Resolve("guide.timezone")
		if got.Value != "" || !got.Caution {
			t.Errorf("stored %q resolves to %q (caution %v), want the empty default with a caution", stored, got.Value, got.Caution)
		}
		if v := GuideTimezone.Get(s); v != "" {
			t.Errorf("stored %q is served as %q by the typed handle", stored, v)
		}
	}
	// A padded but real name is the name.
	s, err := New(context.Background(), NewRegistry(), fakeLoader{m: map[string]string{"guide.timezone": " America/New_York "}}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	if v := GuideTimezone.Get(s); v != "America/New_York" {
		t.Errorf("padded stored value is served as %q, want America/New_York", v)
	}
}

// An operator's pinned value that is wrong fails the boot, naming the key, rather than lurking.
func TestGuideTimezoneBadEnvFailsBoot(t *testing.T) {
	for _, pinned := range []string{"Mars/Olympus", "Local", "not a zone", "posixrules", "right/UTC", "Factory", "EST5EDT", "US/Eastern"} {
		t.Setenv("GUIDE_TIMEZONE", pinned)
		if _, err := New(context.Background(), NewRegistry(), fakeLoader{}, slog.New(slog.DiscardHandler)); err == nil {
			t.Errorf("GUIDE_TIMEZONE=%q booted, want a failure", pinned)
		}
	}
	t.Setenv("GUIDE_TIMEZONE", " Asia/Tokyo ")
	s, err := New(context.Background(), NewRegistry(), fakeLoader{}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("a padded real zone failed the boot: %v", err)
	}
	if v := GuideTimezone.Get(s); v != "Asia/Tokyo" {
		t.Errorf("pinned padded zone = %q, want Asia/Tokyo", v)
	}
}

// Every zone the picker offers is a zone the validator takes: choosing one never saves an error.
func TestGuideTimezoneAcceptsEveryPreset(t *testing.T) {
	s := guideTimezoneSetting(t)
	if len(s.Presets) < 100 {
		t.Fatalf("only %d presets", len(s.Presets))
	}
	for _, p := range s.Presets {
		if _, err := s.parse(p.Value); err != nil {
			t.Errorf("preset %q is refused: %v", p.Value, err)
		}
	}
}
