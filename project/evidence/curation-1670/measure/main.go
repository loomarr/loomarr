//go:build research

// Command measure reads a Loomarr backend's forecast through the HTTP API and reports how
// repetitive each channel is (#1670). It is READ-ONLY: it issues GETs only (plus, for a lane's
// own dev backend, the dev-login POST that creates a session and changes no data).
//
//	go run -tags research ./project/evidence/curation-1670/measure \
//	  -base http://localhost:8080 -days 14 -out /tmp/curation-baseline.json
//
// Auth: a bearer token from $LOOMARR_TOKEN (any signed-in role can read the guide), or
// -dev-login for a worktree backend started with automatic dev login.
//
// What it reads, per channel:
//   - GET /v1/channels                  the channel list
//   - GET /v1/channels/{id}/cycle?at=   the pool: every episode/film the builder placed in the
//     full ordered deck (trace placement facts), before the rolling-window slice
//   - GET /v1/guide?from=&to=           the forecast timeline, one day per request, including
//     each break's assembled commercial pod
//
// ⚠ The guide is a FORECAST from the backend's current state. Two inputs that change as a channel
// really airs are frozen at the moment of measurement: programme recency (airings are recorded
// only when someone watches) and filler exposure. So this measures "what the schedule does if
// nobody watches from now on". The offline simulator (../sim) adds the viewing feedback loop.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/loomarr/loomarr/project/evidence/curation-1670/kit"
)

type channel struct {
	ID       string `json:"id"`
	Number   int    `json:"number"`
	Name     string `json:"name"`
	Strategy string `json:"strategy"`
	Status   string `json:"status"`
}

type cycle struct {
	WindowMs int64 `json:"windowMs"`
	Trace    struct {
		Ordering    string            `json:"ordering"`
		Seed        string            `json:"seed"`
		Truncated   bool              `json:"truncated"`
		Relaxations []json.RawMessage `json:"relaxations"`
		Facts       []struct {
			Stage string `json:"stage"`
		} `json:"facts"`
	} `json:"trace"`
}

type guide struct {
	Channels []struct {
		ChannelID string `json:"channelId"`
		Airings   []struct {
			Kind    string `json:"kind"`
			BlockID string `json:"scheduleBlockId"`
			Title   string `json:"title"`
			Series  string `json:"series"`
			Season  int    `json:"season"`
			Episode int    `json:"episode"`
			ItemID  string `json:"itemId"`
			StartMs int64  `json:"startMs"`
			StopMs  int64  `json:"stopMs"`
			Pod     *struct {
				Entries []struct {
					Hash string `json:"hash"`
					Name string `json:"name"`
				} `json:"entries"`
			} `json:"pod"`
		} `json:"airings"`
	} `json:"channels"`
}

// Result is one channel's row in the output.
type Result struct {
	ID          string      `json:"id"`
	Number      int         `json:"number"`
	Name        string      `json:"name"`
	Ordering    string      `json:"ordering"`
	Seed        string      `json:"seed"`
	WindowH     float64     `json:"windowH"`
	Relaxations int         `json:"relaxations"`
	PoolKnown   bool        `json:"poolKnown"`
	Metrics     kit.Metrics `json:"metrics"`
}

type client struct {
	base  string
	token string
	http  *http.Client
}

func (c *client) get(path string, out any) error {
	req, err := http.NewRequest(http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		return fmt.Errorf("GET %s: %s: %s", path, res.Status, b)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func main() {
	base := flag.String("base", "http://localhost:8080", "backend base URL")
	days := flag.Int("days", 14, "days to forecast")
	fromS := flag.String("from", "", "RFC3339 start (default: now, truncated to the hour)")
	out := flag.String("out", "", "write JSON results here as well")
	devLogin := flag.Bool("dev-login", false, "create a session via /v1/auth/dev-login (worktree backends only)")
	pause := flag.Duration("pause", 250*time.Millisecond, "sleep between guide requests (be gentle with a live install)")
	flag.Parse()

	jar, _ := cookiejar.New(nil)
	c := &client{base: strings.TrimRight(*base, "/"), token: os.Getenv("LOOMARR_TOKEN"),
		http: &http.Client{Timeout: 5 * time.Minute, Jar: jar}}
	if *devLogin {
		req, _ := http.NewRequest(http.MethodPost, c.base+"/v1/auth/dev-login", nil)
		req.Header.Set("X-Loomarr-Csrf", "1")
		res, err := c.http.Do(req)
		if err != nil || res.StatusCode != http.StatusOK {
			fail("dev-login failed: %v %v", err, res)
		}
		_ = res.Body.Close()
	}

	from := time.Now().Truncate(time.Hour)
	if *fromS != "" {
		t, err := time.Parse(time.RFC3339, *fromS)
		if err != nil {
			fail("-from: %v", err)
		}
		from = t
	}
	to := from.Add(time.Duration(*days) * 24 * time.Hour)

	var list struct {
		Channels []channel `json:"channels"`
	}
	if err := c.get("/v1/channels", &list); err != nil {
		fail("%v", err)
	}

	results := make([]Result, 0, len(list.Channels))
	pools := map[string]Result{}
	for _, ch := range list.Channels {
		r := Result{ID: ch.ID, Number: ch.Number, Name: ch.Name}
		var cy cycle
		if err := c.get("/v1/channels/"+ch.ID+"/cycle?at="+url.QueryEscape(from.Format(time.RFC3339)), &cy); err != nil {
			fmt.Fprintf(os.Stderr, "cycle %s: %v\n", ch.ID, err)
		} else {
			r.Ordering, r.Seed, r.Relaxations = cy.Trace.Ordering, cy.Trace.Seed, len(cy.Trace.Relaxations)
			r.WindowH = float64(cy.WindowMs) / 3.6e6
			n := 0
			for _, f := range cy.Trace.Facts {
				if f.Stage == "placement" {
					n++
				}
			}
			r.PoolKnown = !cy.Trace.Truncated && n > 0
			if r.PoolKnown {
				r.Metrics.PoolUnits = n
			}
		}
		pools[ch.ID] = r
	}

	// One day per request: the guide caps a request at 7 days and re-resolves each rolling
	// window inside it; a day keeps each response small. A programme crossing a request edge is
	// returned by both requests with its true start, so blocks are de-duplicated by identity.
	airings := map[string][]kit.Airing{}
	seen := map[string]bool{}
	for d := from; d.Before(to); d = d.Add(24 * time.Hour) {
		var g guide
		path := fmt.Sprintf("/v1/guide?from=%d&to=%d", d.UnixMilli(), d.Add(24*time.Hour).UnixMilli())
		if err := c.get(path, &g); err != nil {
			fail("%v", err)
		}
		for _, row := range g.Channels {
			for _, a := range row.Airings {
				id := fmt.Sprintf("%s|%d|%s", row.ChannelID, a.StartMs, a.Kind)
				if seen[id] {
					continue
				}
				seen[id] = true
				ka := kit.Airing{Kind: a.Kind, Start: time.UnixMilli(a.StartMs), Stop: time.UnixMilli(a.StopMs)}
				switch a.Kind {
				case "program":
					ka.Title = a.Title
					if a.Series != "" {
						ka.Title = a.Series
					}
					ka.Unit = a.ItemID
					if ka.Unit == "" {
						ka.Unit = fmt.Sprintf("%s|%d|%d|%s", a.Series, a.Season, a.Episode, a.Title)
					}
				case "filler":
					if a.Pod != nil {
						for _, e := range a.Pod.Entries {
							id := e.Hash
							if id == "" {
								id = "card:" + e.Name
							}
							ka.Clips = append(ka.Clips, id)
						}
					}
				}
				airings[row.ChannelID] = append(airings[row.ChannelID], ka)
			}
		}
		time.Sleep(*pause)
	}

	for _, ch := range list.Channels {
		r := pools[ch.ID]
		r.Metrics = kit.Measure(airings[ch.ID], r.Metrics.PoolUnits, from, to)
		results = append(results, r)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Number < results[j].Number })

	fmt.Printf("forecast %s → %s (%d days), %s\n\n", from.Format(time.RFC3339), to.Format(time.RFC3339), *days, c.base)
	print(os.Stdout, results)
	if *out != "" {
		b, err := json.MarshalIndent(map[string]any{"from": from, "to": to, "base": c.base, "channels": results}, "", "  ")
		if err != nil {
			fail("encode: %v", err)
		}
		if err := os.WriteFile(*out, b, 0o644); err != nil {
			fail("%v", err)
		}
	}
}

func print(w io.Writer, rs []Result) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(tw, "ch\torder\twin h\tpool\taired\tcover\tR24h\tR7d\tgap p10\tp50\tp90\tmax/day\tadjRep\tclips/h\tclips\tC60m\tC24h\t")
	for _, r := range rs {
		m := r.Metrics
		fmt.Fprintf(tw, "%d\t%s\t%.0f\t%d\t%d\t%s\t%s\t%s\t%.1f\t%.1f\t%.1f\t%d\t%s\t%.1f\t%d\t%s\t%s\t\n",
			r.Number, r.Ordering, r.WindowH, m.PoolUnits, m.DistinctUnits, pc(m.Coverage),
			pc(m.RepeatRate24h), pc(m.RepeatRate7d), m.ReairGapP10H, m.ReairGapP50H, m.ReairGapP90H,
			m.MaxUnitPerDay, pc(m.RepeatedAdjacency), m.ClipsPerHour, m.DistinctClips,
			pc(m.ClipRepeat60m), pc(m.ClipRepeat24h))
	}
	_ = tw.Flush()
}

func pc(f float64) string {
	if f < 0 || math.IsNaN(f) {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", 100*f)
}

func fail(f string, a ...any) {
	fmt.Fprintf(os.Stderr, "measure: "+f+"\n", a...)
	os.Exit(1)
}
