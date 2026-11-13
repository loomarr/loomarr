package settings

import (
	"context"
	"log/slog"
	"testing"
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
		"Etc/GMT+5":                      "Etc/GMT+5",
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

// A value stored before the check existed heals to the default on read, so no client is handed a
// name its own timezone API throws on, and the server's window grid is not silently on another zone.
func TestGuideTimezoneStoredBadValueHealsToUnset(t *testing.T) {
	for _, stored := range []string{"Mars/Olympus", "not a zone", "Local", " Local "} {
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
	for _, pinned := range []string{"Mars/Olympus", "Local", "not a zone"} {
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
