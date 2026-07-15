package testkit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// MediaServer is the shared Emby/Jellyfin test double (CLAUDE.md: one shared
// mock per service, both flavors). It serves the pinned Phase-0 fixtures so the
// library adapter is tested against ground truth, never the network. It also
// records the auth headers it received, so flavor-specific header construction
// can be asserted.
type MediaServer struct {
	*httptest.Server
	// LastAuthHeader / LastEmbyToken / LastEmbyAuthz capture what the adapter sent
	// on the most recent request, for header-shape assertions.
	LastAuthHeader string
	LastEmbyToken  string
	LastEmbyAuthz  string
	// AdminToken is the token the mock accepts for /Users and /Items.
	AdminToken string
	// GoodUser/GoodPass authenticate successfully via /Users/AuthenticateByName.
	GoodUser string
	GoodPass string
	// PresentTMDB, if set, is an additional tmdb id the mock reports present
	// (beyond the pinned 16153) — lets ingest tests flip a title to confirmed.
	PresentTMDB string
	// ItemRunTimeTicks, if set, is the RunTimeTicks a GET /Items?Ids=<id> lookup
	// returns (§9 ItemDurationMs). 0 ⇒ an empty item list (no runtime).
	ItemRunTimeTicks int64
	// Accounts maps username→(password, isAdmin, disabled) for AuthenticateByName.
	// If nil, GoodUser/GoodPass authenticate as an admin. Lets auth tests model
	// admin vs member vs disabled logins.
	Accounts map[string]Account
	// SearchItems, when set, makes the /Items SearchTerm search RETURN these items
	// (matched case-insensitively by term substring against a stub's Terms) instead
	// of the pinned matrix fixture. Each stub carries the real /Items shape incl.
	// OfficialRating/Genres, so a test can drive a themed intent through the real
	// library→catalog→proposal→policy path with in-library titles that have ratings.
	// The same stubs answer the AnyProviderIdEquals presence check (by tmdb id), so
	// in-library backfill is consistent. Pinned fixtures still serve when unset.
	SearchItems []SearchStub
}

// SearchStub is one in-library item the scriptable search returns. Terms are the
// query substrings it matches (case-insensitive); the rest is the /Items item shape
// the library adapter parses.
type SearchStub struct {
	Terms          []string // query substrings this item answers (e.g. "cartoon", "90s")
	LibraryItemID  string
	Name           string
	Type           string // "Movie" | "Series"
	Year           int
	TMDBID         int
	Genres         []string
	OfficialRating string
	// RunTimeTicks is the item's runtime (Emby ticks; 1 tick = 100ns) returned by
	// the by-id lookup the scheduler uses for program duration (§9). Lets a test give
	// each in-library title a real, distinct runtime so the break interleave fires.
	RunTimeTicks int64
}

// stubRuntime returns the RunTimeTicks a scripted stub declares for a library item
// id (the Ids-filtered duration lookup), or 0 if none.
func (ms *MediaServer) stubRuntime(libraryItemID string) int64 {
	for _, s := range ms.SearchItems {
		if s.LibraryItemID == libraryItemID {
			return s.RunTimeTicks
		}
	}
	return 0
}

// stubPresent reports whether an AnyProviderIdEquals value (e.g. "tmdb.100")
// matches a scripted stub's tmdb id — so a discovered title the catalog also owns
// backfills as in-library consistently with what the search returned.
func (ms *MediaServer) stubPresent(prov string) bool {
	for _, s := range ms.SearchItems {
		if s.TMDBID > 0 && strings.EqualFold(prov, "tmdb."+strconv.Itoa(s.TMDBID)) {
			return true
		}
	}
	return false
}

func (s SearchStub) matches(term string) bool {
	lt := strings.ToLower(term)
	for _, t := range s.Terms {
		if strings.Contains(lt, strings.ToLower(t)) || strings.Contains(strings.ToLower(t), lt) {
			return true
		}
	}
	return false
}

// itemJSON renders the stub as one /Items entry (the real Emby item shape the
// search adapter parses: Id/Name/Type/ProductionYear/Genres/Overview/OfficialRating/
// ProviderIds).
func (s SearchStub) itemJSON() string {
	b, _ := json.Marshal(struct {
		Id             string   `json:"Id"`
		Name           string   `json:"Name"`
		Type           string   `json:"Type"`
		ProductionYear int      `json:"ProductionYear"`
		Genres         []string `json:"Genres"`
		OfficialRating string   `json:"OfficialRating"`
		ProviderIds    struct {
			Tmdb string `json:"Tmdb"`
		} `json:"ProviderIds"`
	}{
		Id: s.LibraryItemID, Name: s.Name, Type: s.Type, ProductionYear: s.Year,
		Genres: s.Genres, OfficialRating: s.OfficialRating,
		ProviderIds: struct {
			Tmdb string `json:"Tmdb"`
		}{Tmdb: strconv.Itoa(s.TMDBID)},
	})
	return string(b)
}

// Account is a media-server login the mock accepts.
type Account struct {
	Password string
	ID       string
	IsAdmin  bool
	Disabled bool
}

// NewMediaServer starts a mock media server serving the pinned fixtures.
func NewMediaServer(t testing.TB) *MediaServer {
	t.Helper()
	ms := &MediaServer{AdminToken: "test-admin-token", GoodUser: "Matt", GoodPass: "correct-horse"}
	mux := http.NewServeMux()

	// /Items — presence lookup (§6) AND term search (§7.2). SearchTerm branches
	// to the search fixture; otherwise AnyProviderIdEquals decides present/absent.
	mux.HandleFunc("GET /Items", func(w http.ResponseWriter, r *http.Request) {
		ms.capture(r)
		// Filler-library read (§10): scoped by ParentId → serve the filler fixture.
		if pid := r.URL.Query().Get("ParentId"); pid != "" {
			_, _ = w.Write(Fixture(t, "emby/filler_library.json"))
			return
		}
		// Single-item runtime lookup (§9 ItemDurationMs): GET /Items?Ids=<id>&
		// Fields=RunTimeTicks. Emby rejects the bare /Items/<id> path unless user-
		// scoped, so the adapter uses this Ids-filtered list. A configured
		// ItemRunTimeTicks answers with that runtime; else an empty list (0 → the
		// scheduler falls back, never dead air).
		if ids := r.URL.Query().Get("Ids"); ids != "" {
			// A scripted stub's runtime (per library item id) takes precedence, so a
			// test can give each title a distinct real runtime; else the global
			// ItemRunTimeTicks; else an empty item list.
			ticks := ms.ItemRunTimeTicks
			if t := ms.stubRuntime(ids); t > 0 {
				ticks = t
			}
			if ticks == 0 {
				_, _ = w.Write([]byte(`{"Items":[],"TotalRecordCount":0}`))
				return
			}
			_, _ = fmt.Fprintf(w, `{"Items":[{"Id":%q,"RunTimeTicks":%d}],"TotalRecordCount":1}`, ids, ticks)
			return
		}
		if term := r.URL.Query().Get("SearchTerm"); term != "" {
			// Scriptable stubs (with OfficialRating/genres) take precedence when set,
			// so a test can drive a themed intent through real in-library titles.
			if len(ms.SearchItems) > 0 {
				var items []string
				for _, s := range ms.SearchItems {
					if s.matches(term) {
						items = append(items, s.itemJSON())
					}
				}
				_, _ = fmt.Fprintf(w, `{"Items":[%s],"TotalRecordCount":%d}`, strings.Join(items, ","), len(items))
				return
			}
			// The pinned search fixture answers "matrix"; any other term → empty.
			if strings.EqualFold(term, "matrix") {
				_, _ = w.Write(Fixture(t, "emby/search_matrix.json"))
			} else {
				_, _ = w.Write([]byte(`{"Items":[],"TotalRecordCount":0}`))
			}
			return
		}
		prov := r.URL.Query().Get("AnyProviderIdEquals")
		// The pinned present fixture is tmdb.16153 (Phase 0); a test may add one
		// more present id via PresentTMDB. A scripted SearchStub's tmdb id also
		// reports present (so in-library backfill on discovery is consistent).
		present := strings.EqualFold(prov, "tmdb.16153") ||
			(ms.PresentTMDB != "" && strings.EqualFold(prov, "tmdb."+ms.PresentTMDB)) ||
			ms.stubPresent(prov)
		if present {
			_, _ = w.Write(Fixture(t, "emby/lookup_present.json"))
			return
		}
		_, _ = w.Write(Fixture(t, "emby/lookup_absent.json"))
	})

	// /Users — list for import/sync (§11).
	mux.HandleFunc("GET /Users", func(w http.ResponseWriter, r *http.Request) {
		ms.capture(r)
		_, _ = w.Write(Fixture(t, "emby/users_list.json"))
	})

	// /Users/AuthenticateByName — credential check (§11).
	mux.HandleFunc("POST /Users/AuthenticateByName", func(w http.ResponseWriter, r *http.Request) {
		ms.capture(r)
		var body struct{ Username, Pw string }
		_ = json.NewDecoder(r.Body).Decode(&body)

		// Configurable accounts take precedence; otherwise the default admin.
		if ms.Accounts != nil {
			if acct, ok := ms.Accounts[body.Username]; ok && acct.Password == body.Pw {
				id := acct.ID
				if id == "" {
					id = "user-" + body.Username
				}
				resp := map[string]any{
					"AccessToken": "ms-session-token",
					"User": map[string]any{
						"Id": id, "Name": body.Username,
						"Policy": map[string]any{"IsAdministrator": acct.IsAdmin, "IsDisabled": acct.Disabled},
					},
				}
				_ = json.NewEncoder(w).Encode(resp)
				return
			}
		} else if body.Username == ms.GoodUser && body.Pw == ms.GoodPass {
			resp := map[string]any{
				"AccessToken": "ms-session-token",
				"User": map[string]any{
					"Id": "b07b7a15f38343c39f0da4283e896acc", "Name": ms.GoodUser,
					"Policy": map[string]any{"IsAdministrator": true, "IsDisabled": false},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		// Bad creds: the real Emby returns 401 (Phase 0 pinned this).
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write(Fixture(t, "emby/auth_badpw_response.json"))
	})

	// /Sessions/Logout — best-effort token discard (§11).
	mux.HandleFunc("POST /Sessions/Logout", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	ms.Server = httptest.NewServer(mux)
	return ms
}

func (ms *MediaServer) capture(r *http.Request) {
	ms.LastAuthHeader = r.Header.Get("Authorization")
	ms.LastEmbyToken = r.Header.Get("X-Emby-Token")
	ms.LastEmbyAuthz = r.Header.Get("X-Emby-Authorization")
}
