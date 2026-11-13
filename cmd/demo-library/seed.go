package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/textproto"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/channels/approval"
	"github.com/loomarr/loomarr/internal/demolibrary"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/people/auth"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
	"github.com/loomarr/loomarr/internal/platform/store/proposalstore"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/proposals/accept"
)

// The demo admin, created only when the store has no admin yet. The docs capture signs in as it.
const (
	demoAdminUser     = "demo"
	demoAdminPassword = "loomarr-demo"
)

type seedConfig struct {
	dir, ffmpeg, base, libraryURL, databaseURL string
}

// SeedResult is DEMO_DIR/seed.json: what the docs capture script (#1572) needs without scraping.
type SeedResult struct {
	Backend       string        `json:"backend"`
	Web           string        `json:"web,omitempty"`
	AdminUser     string        `json:"adminUser"`
	AdminPassword string        `json:"adminPassword,omitempty"`
	AdminName     string        `json:"adminName"`
	Library       string        `json:"library"`
	Channels      []SeedChannel `json:"channels"`
}

// SeedChannel is one seeded demo channel.
type SeedChannel struct {
	ID        string `json:"id"`
	Number    int    `json:"number"`
	Name      string `json:"name"`
	Watermark bool   `json:"watermark"`
}

// seed is idempotent. Every step either finds its result already in place or creates it:
// channels are keyed by deterministic proposal ids, settings are whole-value writes, and icon and
// watermark uploads are content-addressed by the image service.
//
// Channels go through the same approval gate as `make seed` (accept.Approver), because that
// gate is the only way a title becomes `available`; a hand-made lineup over the HTTP API would
// leave every entry pending. Everything the running backend owns (settings, names, numbers,
// icons, watermarks, reconcile) goes over its HTTP API so it hot-applies.
func seed(ctx context.Context, lib *demolibrary.Library, cfg seedConfig, log *slog.Logger) error {
	if cfg.base == "" || cfg.databaseURL == "" {
		return errors.New("seed needs BASE (the backend URL) and DATABASE_URL (its store)")
	}
	if err := demolibrary.Generate(ctx, lib, cfg.dir, cfg.ffmpeg, log); err != nil {
		return err
	}
	api, err := newBackend(cfg.base)
	if err != nil {
		return err
	}
	if err := api.expectOK(ctx, http.MethodGet, cfg.libraryURL+"/System/Info/Public", nil, nil); err != nil {
		return fmt.Errorf("the stand-in media server is not answering at %s (run `make demo-library` first): %w", cfg.libraryURL, err)
	}

	st, err := records.Open(ctx, cfg.databaseURL, true)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = st.Close() }()
	admin, created, err := ensureAdmin(ctx, st)
	if err != nil {
		return err
	}
	if err := api.login(ctx); err != nil {
		return err
	}

	// Transcription is off because the demo interstitials carry music, not speech, and a demo
	// backend has no speech engine: left on, the filler pipeline fails every clip at that stage
	// ("whisper not configured") and none ever reaches a break.
	//
	// The TMDB key is the half of features.suggestions the demo owns (#1941): the stand-in that
	// `serve` mounts at /tmdb/3 ignores it, and a backend started without LOOMARR_TMDB_BASE_URL
	// gets refused by the real TMDB instead of surfacing real titles. The other half, an LLM,
	// comes from the backend's own LLM_* settings.
	if err := api.expectOK(ctx, http.MethodPatch, "/v1/settings", map[string]any{"edits": map[string]string{
		settings.LibraryFlavor.Key(): "emby", settings.LibraryURL.Key(): cfg.libraryURL, settings.LibraryToken.Key(): DemoToken,
		settings.FillerTranscribeEnabled.Key(): "false", settings.TMDBAPIKey.Key(): DemoToken,
	}}, nil); err != nil {
		return fmt.Errorf("point the backend at the demo library: %w", err)
	}

	l := demolibrary.Layout{Dir: cfg.dir}
	items, err := api.scanLibrary(ctx, st, lib, l, log)
	if err != nil {
		return err
	}
	out := SeedResult{Backend: cfg.base, Web: os.Getenv("DEMO_WEB_URL"), AdminUser: demoAdminUser, AdminName: admin.Name, Library: cfg.libraryURL}
	if created || admin.Name == demoAdminUser {
		out.AdminPassword = demoAdminPassword
	}
	approver := accept.NewApprover(st, approval.New(st, nil, nil, slog.New(slog.DiscardHandler)), time.Now)
	for _, c := range lib.Channels {
		id, err := ensureChannel(ctx, st, approver, lib, items, admin.ID, c)
		if err != nil {
			return fmt.Errorf("channel %d %q: %w", c.Number, c.Name, err)
		}
		if err := api.nameChannel(ctx, id, c); err != nil {
			return fmt.Errorf("name channel %d: %w", c.Number, err)
		}
		if err := api.upload(ctx, "/v1/channels/"+id+"/icon", l.Icon(c.Number)); err != nil {
			return fmt.Errorf("icon for channel %d: %w", c.Number, err)
		}
		if c.Watermark {
			if err := api.upload(ctx, "/v1/channels/"+id+"/watermark", l.Watermark()); err != nil {
				return fmt.Errorf("watermark for channel %d: %w", c.Number, err)
			}
		}
		out.Channels = append(out.Channels, SeedChannel{ID: id, Number: c.Number, Name: c.Name, Watermark: c.Watermark})
		log.Info("demo channel ready", "number", c.Number, "name", c.Name, "id", id)
	}
	fillerDir, err := filepath.Abs(lib.FillerDir(l))
	if err != nil {
		return err
	}
	if err := api.seedFiller(ctx, log, fillerDir); err != nil {
		return err
	}
	for _, c := range out.Channels {
		if err := api.expectOK(ctx, http.MethodPost, "/v1/channels/"+c.ID+"/reconcile", nil, nil); err != nil {
			return fmt.Errorf("reconcile channel %d: %w", c.Number, err)
		}
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(cfg.dir, "seed.json"), raw, 0o644); err != nil {
		return err
	}
	log.Info("demo seed complete", "channels", len(out.Channels), "summary", filepath.Join(cfg.dir, "seed.json"))
	return nil
}

// ensureAdmin returns the store's admin, bootstrapping the demo admin on an empty store.
func ensureAdmin(ctx context.Context, st store.Store) (identitystore.User, bool, error) {
	users, err := st.ListUsers(ctx)
	if err != nil {
		return identitystore.User{}, false, err
	}
	for _, u := range users {
		if u.Role == identitystore.RoleAdmin {
			return u, false, nil
		}
	}
	n := 0
	p := auth.NewProvisioner(st, nil, func() string { n++; return fmt.Sprintf("usr_demo%d", n) }, time.Now)
	u, err := p.Bootstrap(ctx, demoAdminUser, demoAdminPassword)
	return u, err == nil, err
}

// ensureChannel approves the channel's proposal once, keyed by deterministic job and proposal
// ids, and returns the channel the approval bound. items is each title's library item id by its
// stand-in id (scanLibrary).
func ensureChannel(ctx context.Context, st store.Store, approver *accept.Approver, lib *demolibrary.Library, items map[string]string, adminID string, c demolibrary.Channel) (string, error) {
	// The open pack keys its channels apart from the invented library's, so a store seeded with one
	// never mistakes the other's channel for its own: switching content on a kept store fails on the
	// taken channel number instead of renaming an invented channel over the pack's name.
	key := "demo"
	if lib.Content == demolibrary.ContentOpen {
		key = "demo_open"
	}
	jobID := fmt.Sprintf("job_%s_%d", key, c.Number)
	propID := fmt.Sprintf("prop_%s_%d", key, c.Number)
	if ch, err := st.GetChannelByIntentRef(ctx, jobID); err == nil {
		return ch.ID, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return "", err
	}
	if _, err := st.GetProposal(ctx, propID); errors.Is(err, store.ErrNotFound) {
		if err := createProposal(ctx, st, lib, items, adminID, jobID, propID, c); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	stored, err := st.GetProposal(ctx, propID)
	if err != nil {
		return "", err
	}
	approved, err := approver.Approve(ctx, stored, nil, adminID)
	if err != nil {
		return "", fmt.Errorf("approve (the gate): %w", err)
	}
	return approved.ChannelID, nil
}

func createProposal(ctx context.Context, st store.Store, lib *demolibrary.Library, items map[string]string, adminID, jobID, propID string, c demolibrary.Channel) error {
	intent := proposals.Intent{Description: c.Name}
	var lineup []proposals.ProposalItem
	for _, id := range c.Titles {
		t, ok := lib.ByID(id)
		if !ok {
			return fmt.Errorf("unknown demo title %q", id)
		}
		itemID, ok := items[id]
		if !ok {
			return fmt.Errorf("demo title %q is not in the library", t.Name)
		}
		item := proposals.ProposalItem{Name: t.Name, Year: t.Year, InLibrary: true, LibraryItemID: itemID}
		if t.Kind == demolibrary.Series {
			item.MediaType, item.TVDBID = provision.Series, t.ProviderID()
		} else {
			item.MediaType, item.TMDBID = provision.Movie, t.ProviderID()
		}
		lineup = append(lineup, item)
	}
	intentJSON, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	now := time.Now()
	// CreateSuggestionResult writes the done Job, its succeeded Attempt 1 and the submitted
	// Proposal in one transaction, the shape the real suggester worker leaves behind. A raw
	// CreateJob+CreateProposal pair leaves no attempt, so the workflow rejects the job as
	// inconsistent and GET /v1/proposal-jobs 500s on every demo-seeded backend (#1534, #1827).
	if err := st.CreateSuggestionResult(ctx, proposalstore.Job{ID: jobID, Kind: "suggest", Status: "done",
		IntentJSON: string(intentJSON), IntentHash: "demo-" + jobID, CreatedBy: adminID, CreatedAt: now, UpdatedAt: now,
	}, proposalstore.Proposal{ID: propID, JobID: jobID, Status: "submitted", CreatedBy: adminID,
		Body: proposals.Proposal{Intent: intent, Lineup: lineup}, CreatedAt: now, UpdatedAt: now}); err != nil {
		return fmt.Errorf("create suggestion result: %w", err)
	}
	return nil
}

// seedFiller registers the stand-in's filler folder as a library filler source, then runs the sync and
// the ingest pipeline once each, one after the other, so the interstitials reach channel breaks.
// A clip airs only after the pipeline's quality gate marks it Ready, so writing clip rows
// directly would catalogue clips that never air. Run-now works on a paused job and leaves it
// paused, so a lane backend's paused media jobs stay paused.
//
// The jobs run only when this call registers the source. A rerun must not sync again: a
// re-sync of an unchanged library re-adds clips the pipeline has already normalized, and their
// transcodes then fail on a duplicate hash (#1605).
func (b *backend) seedFiller(ctx context.Context, log *slog.Logger, folder string) error {
	type row struct {
		Kind string `json:"kind"`
		URI  string `json:"uri"`
	}
	var list struct {
		Sources []row `json:"sources"`
	}
	if err := b.expectOK(ctx, http.MethodGet, "/v1/filler/sources", nil, &list); err != nil {
		return err
	}
	if slices.ContainsFunc(list.Sources, func(s row) bool { return s.Kind == "library" && s.URI == folder }) {
		log.Info("demo filler source already registered; not syncing again (#1605)")
		return nil
	}
	if err := b.expectOK(ctx, http.MethodPost, "/v1/filler/sources", map[string]string{
		"kind": "library", "uri": folder, "label": "Demo filler",
	}, nil); err != nil {
		return fmt.Errorf("add the demo filler source: %w", err)
	}
	for _, job := range []string{"filler-sync", "filler-pipeline"} {
		if err := b.runJob(ctx, job); err != nil {
			return err
		}
		log.Info("demo filler job finished", "job", job)
	}
	return nil
}

// scanWait bounds the wait for the demo's first scan: a few hundred hard links, every header read,
// and the Enricher's two passes a minute apart (titles, then seasons).
const scanWait = 10 * time.Minute

// scanLibrary puts the demo's two media folders in use and scans them, because playout plays only
// the files Loomarr's own scan found (#2151 step 16). It waits until every demo title is a library
// item named from its catalog record (TMDB's stand-in, through the Enricher) and every file's header
// is read (a programme's runtime is its file's duration), and returns
// each title's library item id by its stand-in id.
func (b *backend) scanLibrary(ctx context.Context, st store.Store, lib *demolibrary.Library, l demolibrary.Layout, log *slog.Logger) (map[string]string, error) {
	for _, dir := range []string{l.Movies(), l.Shows()} {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return nil, err
		}
		if err := b.expectOK(ctx, http.MethodPost, "/v1/library/roots", map[string]string{"path": abs}, nil); err != nil {
			return nil, fmt.Errorf("put %s in use: %w", abs, err)
		}
	}
	if err := b.expectOK(ctx, http.MethodPost, "/v1/library/scan", map[string]any{}, nil); err != nil {
		return nil, fmt.Errorf("start the scan: %w", err)
	}
	deadline := time.Now().Add(scanWait)
	for {
		items, missing, err := demoLibraryItems(ctx, st, lib)
		if err != nil {
			return nil, err
		}
		var status struct {
			HeadersPending int `json:"headersPending"`
		}
		if err := b.expectOK(ctx, http.MethodGet, "/v1/library/status", nil, &status); err != nil {
			return nil, err
		}
		if missing == "" && status.HeadersPending == 0 {
			log.Info("demo library scanned", "titles", len(items))
			return items, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("after %s the scan has not found or named %q, or has %d files left to read",
				scanWait, missing, status.HeadersPending)
		}
		log.Info("waiting for the demo library's scan", "next title", missing, "files to read", status.HeadersPending)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// demoLibraryItems maps each demo title's stand-in id to its library item id, or names the first
// title the scan has not written yet, or written but not yet named from its catalog record.
func demoLibraryItems(ctx context.Context, st store.Store, lib *demolibrary.Library) (map[string]string, string, error) {
	items := make(map[string]string, len(lib.Titles))
	for _, t := range lib.Titles {
		kind, namespace := "movie", "tmdb"
		if t.Kind == demolibrary.Series {
			kind, namespace = "series", "tvdb"
		}
		title, ok, err := st.LibraryTitleByProviderID(ctx, kind, namespace, strconv.Itoa(t.ProviderID()))
		if err != nil {
			return nil, "", err
		}
		if !ok || title.Name != t.Name {
			return nil, t.Name, nil
		}
		if t.Kind == demolibrary.Series {
			named, err := firstEpisodeNamed(ctx, st, title.ID, lib.EpisodesOf(t))
			if err != nil {
				return nil, "", err
			}
			if !named {
				return nil, t.Name, nil
			}
		}
		items[t.ID()] = title.ID
	}
	return items, "", nil
}

// firstEpisodeNamed reports whether a show's first episode carries its catalog name. The Enricher
// fetches a season a pass after its show, so until then the guide would list file names.
func firstEpisodeNamed(ctx context.Context, st store.Store, seriesID string, want []demolibrary.Episode) (bool, error) {
	if len(want) == 0 {
		return true, nil
	}
	eps, _, err := st.LibraryEpisodes(ctx, seriesID)
	if err != nil {
		return false, err
	}
	return len(eps) > 0 && eps[0].Name == want[0].Name, nil
}

// runJob starts a job now and waits for that manual run to finish.
func (b *backend) runJob(ctx context.Context, name string) error {
	started := time.Now()
	if err := b.expectOK(ctx, http.MethodPost, "/v1/jobs/"+name+"/run", map[string]any{}, nil); err != nil {
		return fmt.Errorf("run %s: %w", name, err)
	}
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		var hist struct {
			Recent []struct {
				StartedAt  time.Time `json:"startedAt"`
				FinishedAt time.Time `json:"finishedAt"`
				Result     string    `json:"result"`
				Trigger    string    `json:"trigger"`
			} `json:"recent"`
		}
		if err := b.expectOK(ctx, http.MethodGet, "/v1/jobs/"+name+"/history", nil, &hist); err != nil {
			return err
		}
		for _, r := range hist.Recent {
			if r.Trigger == "manual" && !r.StartedAt.Before(started.Add(-time.Second)) && !r.FinishedAt.IsZero() {
				if r.Result != "ok" {
					return fmt.Errorf("%s finished with %q", name, r.Result)
				}
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("%s did not finish within 10 minutes", name)
}

// nameChannel gives an approved channel its stable demo name, number and group. The approval gate
// numbers channels itself, and the docs cite "channel 101". A channel already named is left alone,
// so a rerun makes no edit (and no reconcile) it does not need.
//
// A channel the seed has just made is still being built by the backend, which moves its revision
// on, so an edit can be refused as made to an older version (409). As a person is told to, the
// seed reads it again and retries.
func (b *backend) nameChannel(ctx context.Context, id string, c demolibrary.Channel) error {
	var err error
	for range nameChannelTries {
		var cur struct {
			Name     string `json:"name"`
			Number   int    `json:"number"`
			Group    string `json:"group"`
			Strategy string `json:"strategy"`
			Revision int64  `json:"revision"`
		}
		if err := b.expectOK(ctx, http.MethodGet, "/v1/channels/"+id, nil, &cur); err != nil {
			return err
		}
		if cur.Name == c.Name && cur.Number == c.Number && cur.Group == c.Group && cur.Strategy == c.Strategy {
			return nil
		}
		edit := map[string]any{"revision": cur.Revision, "name": c.Name, "group": c.Group, "strategy": c.Strategy}
		if cur.Number != c.Number {
			edit["number"] = c.Number // renumbering is unique-checked; only send a real change
		}
		if err = b.expectOK(ctx, http.MethodPatch, "/v1/channels/"+id, edit, nil); !refusedWith(err, http.StatusConflict) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return err
}

// nameChannelTries is how many times nameChannel reads a channel the backend keeps changing.
const nameChannelTries = 5

// backend is a signed-in HTTP client for the Loomarr API.
type backend struct {
	base string
	http *http.Client
}

func newBackend(base string) (*backend, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &backend{base: strings.TrimRight(base, "/"), http: &http.Client{Jar: jar, Timeout: 60 * time.Second}}, nil
}

// login signs in as the demo admin, falling back to the dev sign-in when the store's admin is
// someone else (a lane backend whose first user came from LOOMARR_DEV_LOGIN).
func (b *backend) login(ctx context.Context) error {
	err := b.expectOK(ctx, http.MethodPost, "/v1/auth/login", map[string]string{"username": demoAdminUser, "password": demoAdminPassword}, nil)
	if err == nil {
		return nil
	}
	if devErr := b.expectOK(ctx, http.MethodPost, "/v1/auth/dev-login", nil, nil); devErr != nil {
		return fmt.Errorf("sign in to %s as %q (%v) or with dev login (%v)", b.base, demoAdminUser, err, devErr)
	}
	return nil
}

func (b *backend) expectOK(ctx context.Context, method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(raw)
	}
	return b.send(ctx, method, path, r, "application/json", out)
}

func (b *backend) upload(ctx context.Context, path, file string) error {
	raw, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filepath.Base(file)))
	h.Set("Content-Type", http.DetectContentType(raw)) // the API checks the part's declared type
	part, err := mw.CreatePart(h)
	if err != nil {
		return err
	}
	if _, err := part.Write(raw); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}
	return b.send(ctx, http.MethodPost, path, &buf, mw.FormDataContentType(), nil)
}

// statusError is an answer the backend refused, with its status code.
type statusError struct {
	code int
	msg  string
}

func (e *statusError) Error() string { return e.msg }

// refusedWith reports whether err is the backend's answer with status code.
func refusedWith(err error, code int) bool {
	var se *statusError
	return errors.As(err, &se) && se.code == code
}

func (b *backend) send(ctx context.Context, method, path string, body io.Reader, contentType string, out any) error {
	url := path
	if strings.HasPrefix(path, "/") {
		url = b.base + path
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("X-Loomarr-Csrf", "1")
	resp, err := b.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return &statusError{code: resp.StatusCode,
			msg: fmt.Sprintf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(raw)))}
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}
