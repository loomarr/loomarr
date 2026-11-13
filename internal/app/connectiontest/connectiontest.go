// Package connectiontest holds the named connection probes behind POST /v1/setup/test and the
// Health checks (config-design §8): media server, Tunarr, requester, Sonarr, Radarr, LLM, TMDB and
// the filler library folders.
//
// A probe is a shallow reachability check (a cheap authenticated call), not a full sweep. Every
// probe reads its settings live through Config, so a Test reflects the current configuration.
package connectiontest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/library/mediaserver"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/library/requester"
	"github.com/loomarr/loomarr/internal/library/tmdb"
	"github.com/loomarr/loomarr/internal/platform/llm"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/playout/tunarr"
)

// Config is what the probes read.
type Config struct {
	// Settings is the live settings view every probe reads.
	Settings settings.Reader
	// Library is the application library client; the media-server probe snapshots its live
	// connection, including the stable install device id.
	Library *mediaserver.Client
	// TMDB validates the TMDB key; nil reads as "not configured".
	TMDB *tmdb.Client
	// Tunarr, Arr and Seerr resolve the connections live per probe.
	Tunarr func() tunarr.Config
	Arr    requester.ArrConns
	Seerr  func() (baseURL, apiKey string)
	// Selection is the active LLM selection.
	Selection func() llm.Selection
	// ListModels lists a hosted provider's models; llm.CheckModels when nil. Tests inject it so
	// they never dial out.
	ListModels func(ctx context.Context, baseURL, apiKey string) error
}

// New builds the named probes: check name → live connection probe. An unknown name is the
// caller's to report as "not configured".
func New(c Config) map[string]func(ctx context.Context) (bool, string) {
	list := c.ListModels
	if list == nil {
		list = llm.CheckModels
	}
	set := c.Settings
	hostedModels := &hostedModelCheck{list: list, now: time.Now}
	return map[string]func(ctx context.Context) (bool, string){
		"media_server": func(ctx context.Context) (bool, string) {
			lib := c.Library.Snapshot()
			_, err := lib.Connection().Validate()
			switch {
			case errors.Is(err, mediaserver.ErrConnectionFlavorRequired):
				return false, "set a media server flavor (emby | jellyfin)"
			case errors.Is(err, mediaserver.ErrConnectionURLRequired):
				return false, "set the media server URL"
			case errors.Is(err, mediaserver.ErrConnectionTokenRequired):
				return false, "set the media server API token"
			case err != nil:
				return false, "set a complete media server connection"
			}
			if _, err := lib.ListUsers(ctx); err != nil {
				return false, "could not reach the media server: " + err.Error()
			}
			return true, ""
		},
		"tunarr": func(ctx context.Context) (bool, string) {
			if settings.TunarrURL.Get(set) == "" {
				return false, "set the Tunarr URL"
			}
			prog := tunarr.NewDynamic(c.Tunarr)
			// A GET on a non-existent channel id is a cheap reachability probe: a
			// transport error means unreachable; a clean (not-found) answer means up.
			if _, _, err := prog.GetChannel(ctx, "loomarr-probe"); err != nil {
				return false, "could not reach Tunarr: " + err.Error()
			}
			// Reachable is not enough: every channel create carries a transcode-config
			// uuid, and a missing/unresolvable one makes Tunarr 400 EVERY create while
			// this check reads green (FINDING 5 — a channel that sits `building` forever
			// with nothing the operator can see). Resolving it here (the configured id,
			// or the auto-selected Default) turns that silent failure into an actionable
			// red.
			if _, err := prog.TranscodeConfigID(ctx); err != nil {
				return false, "Tunarr is reachable but no transcode config is usable: " + err.Error()
			}
			return true, ""
		},
		// requester (§6): branch on the selected provider: probe the direct arr(s) or Seerr
		// (URL + API key).
		"requester": func(ctx context.Context) (bool, string) {
			if settings.RequesterProvider.Get(set) == "arr" {
				if settings.SonarrURL.Get(set) == "" && settings.RadarrURL.Get(set) == "" {
					return false, "connect Sonarr and/or Radarr (Where's your media? in setup, Settings → Library later)"
				}
				if err := requester.NewArr(c.Arr).Reachable(ctx); err != nil {
					return false, "could not reach Sonarr/Radarr: " + err.Error()
				}
				return true, ""
			}
			if settings.SeerrURL.Get(set) == "" {
				return false, "set the Seerr URL"
			}
			if err := requester.NewSeerrDynamic(c.Seerr).Reachable(ctx); err != nil {
				return false, "could not reach Seerr: " + err.Error()
			}
			return true, ""
		},
		// sonarr and radarr (#2151 decision 29): their own connections, whichever requester is
		// chosen — the library reads them either way.
		"sonarr": arrCheck("Sonarr", func() (string, string) {
			return settings.SonarrURL.Get(set), settings.SonarrAPIKey.Get(set)
		}),
		"radarr": arrCheck("Radarr", func() (string, string) {
			return settings.RadarrURL.Get(set), settings.RadarrAPIKey.Get(set)
		}),
		// llm (§8/§8.1): local Ollama is probed live (reachable + the selected model
		// pulled). A hosted provider must list its models (#2735): when that list cannot be
		// read the model picker shows "No models returned", and the connection is not OK either.
		// Its key is validated when a model is selected.
		"llm": func(ctx context.Context) (bool, string) {
			sel := c.Selection()
			provider := sel.Provider
			if provider != "" && provider != "ollama" {
				if sel.APIKey == "" {
					return false, "set the " + provider + " API key (AI settings), then Test it"
				}
				if sel.Model == "" {
					return false, "choose a model for " + provider + " (AI settings)"
				}
				base := sel.URL
				if hp, ok := llm.HostedProviderByKey(provider); ok && base == "" {
					base = hp.BaseURL
				}
				if base == "" {
					return false, "set the AI service address (AI settings)"
				}
				if err := hostedModels.check(ctx, base, sel.APIKey); err != nil {
					return false, "no models came back from " + base + "; check the key and address: " + err.Error()
				}
				return true, ""
			}
			base := sel.URL
			if base == "" {
				return false, "set the Ollama URL, or configure a hosted provider in AI settings"
			}
			probe := llm.NewProber(base).Probe(ctx)
			if !probe.Reachable {
				return false, "could not reach the LLM host at " + base
			}
			model := sel.Model
			if model == "" {
				return false, "select a model in AI settings (the model picker, §8.1)"
			}
			for _, m := range probe.PulledModels {
				if m == model {
					return true, ""
				}
			}
			return false, "model " + model + " is not pulled yet — pull it in AI settings"
		},
		// tmdb (§7.2): validate the key with a cheap lookup of a stable known id
		// (The Matrix, tmdb 603). A rejected key surfaces as a non-2xx error.
		"tmdb": func(ctx context.Context) (bool, string) {
			if settings.TMDBAPIKey.Get(set) == "" || c.TMDB == nil {
				return false, "set your TMDB API key"
			}
			if _, err := c.TMDB.Exists(ctx, provision.Movie, 603); err != nil {
				return false, "TMDB rejected the key or was unreachable: " + err.Error()
			}
			return true, ""
		},
		// filler (§10): unlike a remote integration, the dependency is local storage. The probe
		// verifies the catalog root can be listed and written, then checks the effective drop folder
		// (including the derived <root>/_watch default). A non-empty configured path is not health.
		"filler": func(_ context.Context) (bool, string) {
			root := settings.FillerDir.Get(set)
			if root == "" {
				return false, "set the filler clip library folder"
			}
			layout, err := cliplibrary.NewLayout(root, settings.FillerWatchDir.Get(set))
			if err != nil {
				return false, "filler storage layout is unsafe: " + err.Error()
			}
			if err := probeWritableDirectory(layout.ClipDir()); err != nil {
				return false, "clip library is not usable: " + err.Error()
			}
			if err := probeWritableDirectory(layout.WatchDir()); err != nil {
				return false, "drop folder is not usable: " + err.Error()
			}
			return true, ""
		},
	}
}

// hostedModelReuse is how long a successful model listing is reused. Current Health runs this
// probe every 30 seconds by default, and OpenRouter's catalog is large; a changed key or address
// is a new listing, so a fix is still checked at once.
const hostedModelReuse = 10 * time.Minute

// hostedModelCheck remembers the last successful listing for one address and key. A failure is
// never remembered, so a broken provider is re-checked on every probe.
type hostedModelCheck struct {
	mu        sync.Mutex
	base, key string
	at        time.Time
	list      func(ctx context.Context, baseURL, apiKey string) error
	now       func() time.Time
}

func (h *hostedModelCheck) check(ctx context.Context, base, key string) error {
	h.mu.Lock()
	fresh := h.base == base && h.key == key && !h.at.IsZero() && h.now().Sub(h.at) < hostedModelReuse
	h.mu.Unlock()
	if fresh {
		return nil
	}
	if err := h.list(ctx, base, key); err != nil {
		return err
	}
	h.mu.Lock()
	h.base, h.key, h.at = base, key, h.now()
	h.mu.Unlock()
	return nil
}

// arrCheck is one arr's Test: its address and key answer, nothing else is touched.
func arrCheck(name string, conn func() (string, string)) func(context.Context) (bool, string) {
	conns := requester.ArrConns{Sonarr: conn, Radarr: noArr}
	if name == "Radarr" {
		conns = requester.ArrConns{Sonarr: noArr, Radarr: conn}
	}
	return func(ctx context.Context) (bool, string) {
		if base, _ := conn(); strings.TrimSpace(base) == "" {
			return false, "set the " + name + " URL"
		}
		if err := requester.NewArr(conns).Reachable(ctx); err != nil {
			return false, err.Error()
		}
		return true, ""
	}
}

// noArr is an arr that is not configured; Reachable skips it.
func noArr() (string, string) { return "", "" }

func probeWritableDirectory(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s does not exist", dir)
		}
		return fmt.Errorf("inspect %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a folder", dir)
	}
	f, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}
	_, readErr := f.Readdirnames(1)
	closeErr := f.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return fmt.Errorf("read %s: %w", dir, readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close %s: %w", dir, closeErr)
	}
	probe, err := os.CreateTemp(dir, ".loomarr-health-*")
	if err != nil {
		return fmt.Errorf("write %s: %w", dir, err)
	}
	name := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("write %s: %w", dir, err)
	}
	if err := os.Remove(name); err != nil {
		return fmt.Errorf("clean up test file in %s: %w", dir, err)
	}
	return nil
}
