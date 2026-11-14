package main

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/testkit"
)

// TestLoadReadsTheInstallSchema runs the report's queries against a fully migrated database, so a
// renamed column fails here rather than in a household's report.
func TestLoadReadsTheInstallSchema(t *testing.T) {
	path := strings.TrimPrefix(testkit.MigratedSQLiteDSN(t), "sqlite://")
	write, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	policy := `{"ordering":"syndication","applied":[{"kind":"seriesMinGap","from":"2h0m0s","to":"0s"}],` +
		`"rules":[{}],"filler":{"era":{"from":1990,"to":1999},"roles":["commercial"]}}`
	aired := time.Date(2026, 11, 12, 1, 0, 0, 0, time.UTC).Unix()
	for _, q := range []string{
		`INSERT INTO settings (key, value) VALUES ('filler.home_country', 'US')`,
		`INSERT INTO channels (id, name, number, strategy, status, policy_json) VALUES ('ch', 'Channel', 7, 'sequential', 'live', '` + policy + `')`,
		`INSERT INTO titles (key, title_json, state) VALUES ('series:tmdb:141', '{}', 'available')`,
		`INSERT INTO channel_lineup (channel_id, position, title_key, entry) VALUES ('ch', 0, 'series:tmdb:141', '{"Title":"A Show"}')`,
		`INSERT INTO library_items (id, kind, name, sort_name, updated_at_ms) VALUES ('show', 'series', 'A Show', 'a show', 1)`,
		`INSERT INTO library_items (id, kind, name, sort_name, updated_at_ms, season, episode, runtime_ms, series_id) VALUES ('ep', 'episode', 'Pilot', 'pilot', 1, 1, 1, 1500000, 'show')`,
		`INSERT INTO library_ids (item_id, namespace, value) VALUES ('show', 'tmdb', '141')`,
		`INSERT INTO scheduled_airings (channel_id, library_item_id, aired_at, recorded_at, key) VALUES ('ch', 'ep', ` + itoa(aired) + `, ` + itoa(aired) + `, 'series:tmdb:141')`,
		`INSERT INTO filler_airings (channel_id, clip_hash, aired_at_ms, recorded_at_ms) VALUES ('ch', 'clip', ` + itoa(aired*1000) + `, ` + itoa(aired*1000) + `)`,
		`INSERT INTO clips (hash, path, name, role, era, duration_ms, updated_at, placement, country) VALUES ('clip', 'c/l/clip.mp4', 'Ad', 'commercial', 1994, 30000, 1, 'break_body', 'US')`,
	} {
		if _, err := write.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := write.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := openSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	s, err := Load(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}

	if s.HomeCountry != "US" || len(s.Channels) != 1 {
		t.Fatalf("home %q, channels %d", s.HomeCountry, len(s.Channels))
	}
	c := s.Channels[0]
	if c.Ordering != "syndication" || c.Rules != 1 || len(c.Applied) != 1 || len(c.FillerEra) != 1 || c.FillerRoles[0] != "commercial" {
		t.Fatalf("policy read as %+v", c)
	}
	if len(c.Lineup) != 1 || c.Lineup[0].Title != "A Show" || c.Lineup[0].Units != 1 || c.Lineup[0].RuntimeMs != 1500000 {
		t.Fatalf("lineup read as %+v", c.Lineup)
	}
	if len(s.Scheduled) != 1 || s.Items["ep"].Series != "A Show" || s.Items["ep"].RuntimeMs != 1500000 {
		t.Fatalf("ledger %+v, item %+v", s.Scheduled, s.Items["ep"])
	}
	if len(s.FillerAirings) != 1 || !s.Clips["clip"].Airable() || s.Clips["clip"].Era != 1994 {
		t.Fatalf("filler %+v, clip %+v", s.FillerAirings, s.Clips["clip"])
	}
}

func TestALiveDatabaseIsRefused(t *testing.T) {
	path := strings.TrimPrefix(testkit.MigratedSQLiteDSN(t), "sqlite://")
	if err := os.WriteFile(path+"-wal", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := openSnapshot(path); err == nil || !strings.Contains(err.Error(), "live") {
		t.Fatalf("opening a database with a -wal beside it: err = %v, want a refusal", err)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
