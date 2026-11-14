package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite" // the store's own pure-Go driver
)

// Snapshot is everything the report reads, loaded once from a database copy.
type Snapshot struct {
	HomeCountry   string
	Channels      []Channel
	Items         map[string]Item // library item id → facts, for every item an airing names
	Scheduled     []ProgramAiring // scheduled_airings: what reconcile recorded as having started
	Watched       []ProgramAiring // airings: what a viewer was served
	Viewings      []Viewing
	FillerAirings []ClipAiring // filler_airings: watched commercial-break clips
	Clips         map[string]Clip
}

// Channel is one channel row with the parts of its policy the report reads.
type Channel struct {
	ID, Name, Strategy, Status string
	Number                     int
	Ordering                   string
	Applied                    []Relaxation
	Rules                      int
	FillerEra                  []YearRange // empty: the channel names no filler era
	FillerRoles                []string    // empty: the default roles
	Lineup                     []LineupTitle
}

// LineupTitle is one approved lineup entry and how much of it the library holds.
type LineupTitle struct {
	Key, Title string
	Units      int   // episodes of the series, or 1 for a film, found in the library
	RuntimeMs  int64 // their total runtime
}

// Relaxation is one step the scheduler's relaxation ladder applied to the channel's policy.
type Relaxation struct {
	Kind string `json:"kind"`
	From string `json:"from"`
	To   string `json:"to"`
}

// YearRange is an inclusive span of years.
type YearRange struct {
	From int `json:"from"`
	To   int `json:"to"`
}

// Item is a library item an airing names.
type Item struct {
	Kind            string // movie or episode
	Name, Series    string
	Season, Episode int
	RuntimeMs       int64
}

// ProgramAiring is one ledger row.
type ProgramAiring struct {
	ChannelID, ItemID, Key string
	AiredAt, RecordedAt    time.Time
}

// Viewing is one stretch of a household device on a channel.
type Viewing struct {
	ChannelID       string
	Started, Ended  time.Time
	Device, Program string
}

// ClipAiring is one watched airing of a filler clip.
type ClipAiring struct {
	ChannelID, Hash string
	AiredAt         time.Time
}

// Clip is the catalogue facts of one filler clip.
type Clip struct {
	Name, Role, Audience, Country, Placement string
	Era                                      int
	DurationMs                               int64
	Held, Removed                            bool
}

// Airable reports whether the clip can be placed in a break at all: catalogued, Ready, not held
// for review and not removed.
func (c Clip) Airable() bool { return c.Placement != "not_playable" && !c.Held && !c.Removed }

// openSnapshot opens a database copy read-only. A file with a -wal beside it is a live database,
// and the report refuses it: read a copy (AGENTS.md), never the install's own file.
func openSnapshot(path string) (*sql.DB, error) {
	if _, err := os.Stat(path + "-wal"); err == nil {
		return nil, fmt.Errorf("%s has a -wal file, so it is live: snapshot it with VACUUM INTO and pass the copy", path)
	}
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	return sql.Open("sqlite", "file:"+path+"?mode=ro")
}

// Load reads the snapshot.
func Load(ctx context.Context, db *sql.DB) (Snapshot, error) {
	s := Snapshot{Items: map[string]Item{}, Clips: map[string]Clip{}}
	steps := []func(context.Context, *sql.DB, *Snapshot) error{
		loadSettings, loadChannels, loadLineups, loadLedgers, loadViewings, loadFiller, loadItems,
	}
	for _, step := range steps {
		if err := step(ctx, db, &s); err != nil {
			return Snapshot{}, err
		}
	}
	return s, nil
}

func loadSettings(ctx context.Context, db *sql.DB, s *Snapshot) error {
	err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'filler.home_country'`).Scan(&s.HomeCountry)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}

// policyJSON is the subset of schedule.ChannelPolicy the report reads. It is decoded here rather
// than through the schedule package so a snapshot from an older install still loads.
type policyJSON struct {
	Ordering string        `json:"ordering"`
	Applied  []Relaxation  `json:"applied"`
	Rules    []any         `json:"rules"`
	Filler   *fillerPolicy `json:"filler"`
}

type fillerPolicy struct {
	Era        *YearRange  `json:"era"`
	EraWindows []YearRange `json:"eraWindows"`
	Roles      []string    `json:"roles"`
}

func loadChannels(ctx context.Context, db *sql.DB, s *Snapshot) error {
	rows, err := db.QueryContext(ctx, `SELECT id, number, name, strategy, status, policy_json FROM channels ORDER BY number`)
	if err != nil {
		return fmt.Errorf("channels: %w", err)
	}
	return each(rows, func(rows *sql.Rows) error {
		var c Channel
		var raw string
		if err := rows.Scan(&c.ID, &c.Number, &c.Name, &c.Strategy, &c.Status, &raw); err != nil {
			return err
		}
		var p policyJSON
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			return fmt.Errorf("channel %s policy: %w", c.ID, err)
		}
		c.Ordering, c.Applied, c.Rules = p.Ordering, p.Applied, len(p.Rules)
		if p.Filler != nil {
			c.FillerRoles = p.Filler.Roles
			c.FillerEra = append(c.FillerEra, p.Filler.EraWindows...)
			if p.Filler.Era != nil {
				c.FillerEra = append(c.FillerEra, *p.Filler.Era)
			}
		}
		s.Channels = append(s.Channels, c)
		return nil
	})
}

// each runs scan on every row, then closes rows. It reports the first of scan's, the iteration's
// and the close's errors.
func each(rows *sql.Rows, scan func(*sql.Rows) error) (err error) {
	defer func() {
		if cerr := rows.Close(); err == nil {
			err = cerr
		}
	}()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// loadLineups reads each channel's lineup and counts what the library holds of each title. A key
// is "<kind>:<namespace>:<value>"; the title is the library item of that kind carrying that id.
func loadLineups(ctx context.Context, db *sql.DB, s *Snapshot) error {
	byID := map[string]*Channel{}
	for i := range s.Channels {
		byID[s.Channels[i].ID] = &s.Channels[i]
	}
	rows, err := db.QueryContext(ctx, `SELECT channel_id, title_key, entry FROM channel_lineup ORDER BY channel_id, position`)
	if err != nil {
		return fmt.Errorf("lineups: %w", err)
	}
	var titles []struct {
		channel string
		title   LineupTitle
	}
	if err := each(rows, func(rows *sql.Rows) error {
		var ch, key, entry string
		if err := rows.Scan(&ch, &key, &entry); err != nil {
			return err
		}
		var e struct {
			Title string `json:"Title"`
		}
		_ = json.Unmarshal([]byte(entry), &e) // a malformed entry still counts, untitled
		titles = append(titles, struct {
			channel string
			title   LineupTitle
		}{ch, LineupTitle{Key: key, Title: e.Title}})
		return nil
	}); err != nil {
		return err
	}
	for _, t := range titles {
		lt := t.title
		if lt.Units, lt.RuntimeMs, err = libraryHolding(ctx, db, lt.Key); err != nil {
			return fmt.Errorf("lineup %s: %w", lt.Key, err)
		}
		if c := byID[t.channel]; c != nil {
			c.Lineup = append(c.Lineup, lt)
		}
	}
	return nil
}

func libraryHolding(ctx context.Context, db *sql.DB, key string) (int, int64, error) {
	kind, namespace, value, ok := splitKey(key)
	if !ok {
		return 0, 0, nil
	}
	const q = `
SELECT count(u.id), coalesce(sum(u.runtime_ms), 0)
FROM library_ids n
JOIN library_items t ON t.id = n.item_id AND t.kind = ?
JOIN library_items u ON (t.kind = 'series' AND u.series_id = t.id AND u.kind = 'episode')
                     OR (t.kind = 'movie' AND u.id = t.id)
WHERE n.namespace = ? AND n.value = ?`
	var units int
	var ms int64
	err := db.QueryRowContext(ctx, q, kind, namespace, value).Scan(&units, &ms)
	return units, ms, err
}

func splitKey(key string) (kind, namespace, value string, ok bool) {
	parts := strings.SplitN(key, ":", 3)
	if len(parts) != 3 || parts[2] == "" {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

func loadLedgers(ctx context.Context, db *sql.DB, s *Snapshot) error {
	for _, l := range []struct {
		table string
		into  *[]ProgramAiring
	}{{"scheduled_airings", &s.Scheduled}, {"airings", &s.Watched}} {
		rows, err := db.QueryContext(ctx, `SELECT channel_id, library_item_id, key, aired_at, recorded_at FROM `+l.table+` ORDER BY aired_at`)
		if err != nil {
			return fmt.Errorf("%s: %w", l.table, err)
		}
		if err := each(rows, func(rows *sql.Rows) error {
			var a ProgramAiring
			var aired, recorded int64
			if err := rows.Scan(&a.ChannelID, &a.ItemID, &a.Key, &aired, &recorded); err != nil {
				return err
			}
			a.AiredAt, a.RecordedAt = time.Unix(aired, 0), time.Unix(recorded, 0)
			*l.into = append(*l.into, a)
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func loadViewings(ctx context.Context, db *sql.DB, s *Snapshot) error {
	rows, err := db.QueryContext(ctx, `SELECT channel_id, device, programme, started_at, ended_at FROM viewings ORDER BY started_at`)
	if err != nil {
		return fmt.Errorf("viewings: %w", err)
	}
	return each(rows, func(rows *sql.Rows) error {
		var v Viewing
		var start, end int64
		if err := rows.Scan(&v.ChannelID, &v.Device, &v.Program, &start, &end); err != nil {
			return err
		}
		v.Started, v.Ended = time.Unix(start, 0), time.Unix(end, 0)
		s.Viewings = append(s.Viewings, v)
		return nil
	})
}

func loadFiller(ctx context.Context, db *sql.DB, s *Snapshot) error {
	rows, err := db.QueryContext(ctx, `SELECT channel_id, clip_hash, aired_at_ms FROM filler_airings ORDER BY aired_at_ms`)
	if err != nil {
		return fmt.Errorf("filler_airings: %w", err)
	}
	if err := each(rows, func(rows *sql.Rows) error {
		var a ClipAiring
		var ms int64
		if err := rows.Scan(&a.ChannelID, &a.Hash, &ms); err != nil {
			return err
		}
		a.AiredAt = time.UnixMilli(ms)
		s.FillerAirings = append(s.FillerAirings, a)
		return nil
	}); err != nil {
		return err
	}
	rows, err = db.QueryContext(ctx, `SELECT hash, name, role, era, audience, country, placement, duration_ms, held, removed_at <> 0 FROM clips`)
	if err != nil {
		return fmt.Errorf("clips: %w", err)
	}
	return each(rows, func(rows *sql.Rows) error {
		var hash string
		var c Clip
		if err := rows.Scan(&hash, &c.Name, &c.Role, &c.Era, &c.Audience, &c.Country, &c.Placement, &c.DurationMs, &c.Held, &c.Removed); err != nil {
			return err
		}
		s.Clips[hash] = c
		return nil
	})
}

// loadItems reads the facts of every library item a ledger row names.
func loadItems(ctx context.Context, db *sql.DB, s *Snapshot) error {
	want := map[string]bool{}
	for _, l := range [][]ProgramAiring{s.Scheduled, s.Watched} {
		for _, a := range l {
			want[a.ItemID] = true
		}
	}
	const q = `
SELECT i.kind, i.name, coalesce(p.name, ''), i.season, i.episode, i.runtime_ms
FROM library_items i LEFT JOIN library_items p ON p.id = i.series_id
WHERE i.id = ?`
	stmt, err := db.PrepareContext(ctx, q)
	if err != nil {
		return fmt.Errorf("library items: %w", err)
	}
	defer func() { _ = stmt.Close() }() // a read-only snapshot: nothing to lose on a failed close
	for id := range want {
		var it Item
		err := stmt.QueryRowContext(ctx, id).Scan(&it.Kind, &it.Name, &it.Series, &it.Season, &it.Episode, &it.RuntimeMs)
		if errors.Is(err, sql.ErrNoRows) {
			continue // an airing of an item since removed from the library: counted, unnamed
		}
		if err != nil {
			return err
		}
		s.Items[id] = it
	}
	return nil
}
