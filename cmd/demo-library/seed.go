package main

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/auth"
	"github.com/loomarr/loomarr/internal/binder"
	"github.com/loomarr/loomarr/internal/demolibrary"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/suggest"
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
// Channels go through the same approval gate as `make seed` (suggest.Approver), because that
// gate is the only way a title becomes `available`; a hand-made lineup over the HTTP API would
// leave every entry pending. Everything the running backend owns (settings, names, numbers,
// icons, watermarks, reconcile) goes over its HTTP API so it hot-applies.
func seed(ctx context.Context, cfg seedConfig, log *slog.Logger) error {
	if cfg.base == "" || cfg.databaseURL == "" {
		return errors.New("seed needs BASE (the backend URL) and DATABASE_URL (its store)")
	}
	if err := demolibrary.Generate(ctx, cfg.dir, cfg.ffmpeg, log); err != nil {
		return err
	}
	api, err := newBackend(cfg.base)
	if err != nil {
		return err
	}
	if err := api.expectOK(ctx, http.MethodGet, cfg.libraryURL+"/System/Info/Public", nil, nil); err != nil {
		return fmt.Errorf("the stand-in media server is not answering at %s (run `make demo-library` first): %w", cfg.libraryURL, err)
	}

	st, err := store.Open(ctx, cfg.databaseURL, true)
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

	if err := api.expectOK(ctx, http.MethodPatch, "/v1/settings", map[string]any{"edits": map[string]string{
		"library.flavor": "emby", "library.url": cfg.libraryURL, "library.token": DemoToken,
	}}, nil); err != nil {
		return fmt.Errorf("point the backend at the demo library: %w", err)
	}

	l := demolibrary.Layout{Dir: cfg.dir}
	out := SeedResult{Backend: cfg.base, Web: os.Getenv("DEMO_WEB_URL"), AdminUser: demoAdminUser, AdminName: admin.Name, Library: cfg.libraryURL}
	if created || admin.Name == demoAdminUser {
		out.AdminPassword = demoAdminPassword
	}
	approver := suggest.NewApprover(st, binder.New(st, nil, nil, slog.New(slog.DiscardHandler)), time.Now)
	for _, c := range demolibrary.Channels {
		id, err := ensureChannel(ctx, st, approver, admin.ID, c)
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
	if err := seedFiller(ctx, st, l); err != nil {
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
func ensureAdmin(ctx context.Context, st store.Store) (store.User, bool, error) {
	users, err := st.ListUsers(ctx)
	if err != nil {
		return store.User{}, false, err
	}
	for _, u := range users {
		if u.Role == store.RoleAdmin {
			return u, false, nil
		}
	}
	n := 0
	p := auth.NewProvisioner(st, nil, func() string { n++; return fmt.Sprintf("usr_demo%d", n) }, time.Now)
	u, err := p.Bootstrap(ctx, demoAdminUser, demoAdminPassword)
	return u, err == nil, err
}

// ensureChannel approves the channel's proposal once, keyed by deterministic job and proposal
// ids, and returns the channel the approval bound.
func ensureChannel(ctx context.Context, st store.Store, approver *suggest.Approver, adminID string, c demolibrary.Channel) (string, error) {
	jobID := fmt.Sprintf("job_demo_%d", c.Number)
	propID := fmt.Sprintf("prop_demo_%d", c.Number)
	if ch, err := st.GetChannelByIntentRef(ctx, jobID); err == nil {
		return ch.ID, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return "", err
	}
	if _, err := st.GetProposal(ctx, propID); errors.Is(err, store.ErrNotFound) {
		if err := createProposal(ctx, st, adminID, jobID, propID, c); err != nil {
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

func createProposal(ctx context.Context, st store.Store, adminID, jobID, propID string, c demolibrary.Channel) error {
	intent := suggest.Intent{Description: c.Name}
	var lineup []suggest.ProposalItem
	for _, id := range c.Titles {
		t, ok := demolibrary.ByID(id)
		if !ok {
			return fmt.Errorf("unknown demo title %q", id)
		}
		item := suggest.ProposalItem{Name: t.Name, Year: t.Year, InLibrary: true, LibraryItemID: t.ID()}
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
	propJSON, err := json.Marshal(suggest.Proposal{Intent: intent, Lineup: lineup})
	if err != nil {
		return err
	}
	now := time.Now()
	if err := st.CreateJob(ctx, store.Job{ID: jobID, Kind: "suggest", Status: "done", IntentJSON: string(intentJSON),
		IntentHash: "demo-" + jobID, CreatedBy: adminID, CreatedAt: now, UpdatedAt: now}); err != nil {
		return fmt.Errorf("create job: %w", err)
	}
	return st.CreateProposal(ctx, store.Proposal{ID: propID, JobID: jobID, Status: "submitted", CreatedBy: adminID,
		ProposalJSON: string(propJSON), CreatedAt: now, UpdatedAt: now})
}

// seedFiller adds the generated interstitials to the clip catalogue. Clips are not gated titles;
// a direct upsert is how `make seed` builds its catalogue too. Each is keyed on its content hash.
func seedFiller(ctx context.Context, st store.Store, l demolibrary.Layout) error {
	now := time.Now()
	for _, f := range demolibrary.Fillers {
		path, err := filepath.Abs(l.Filler(f.ID))
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hash := fmt.Sprintf("%x", sha256.Sum256(raw))
		clip := filler.Clip{Hash: hash, Path: path, Name: f.Name, Kind: filler.Kind(f.Kind), Audience: filler.General,
			DurationMs: int64(f.Duration) * 1000, Source: "demo"}
		if err := st.UpsertClip(ctx, store.Clip{Clip: clip, UpdatedAt: now}); err != nil {
			return fmt.Errorf("filler %q: %w", f.Name, err)
		}
		if err := st.SetClipTags(ctx, hash, []string{f.Tag}); err != nil {
			return fmt.Errorf("tag filler %q: %w", f.Name, err)
		}
	}
	return nil
}

// nameChannel gives an approved channel its stable demo name, number and group. The approval gate
// numbers channels itself, and the docs cite "channel 101". A channel already named is left alone,
// so a rerun makes no edit (and no reconcile) it does not need.
func (b *backend) nameChannel(ctx context.Context, id string, c demolibrary.Channel) error {
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
	return b.expectOK(ctx, http.MethodPatch, "/v1/channels/"+id, edit, nil)
}

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
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(raw)))
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}
