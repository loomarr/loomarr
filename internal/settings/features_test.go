package settings

import (
	"context"
	"os/exec"
	"testing"
)

// featureService builds a Service over the REAL registry with the given db
// overrides, so gating is tested against the actual RequiredFor declarations.
func featureService(t *testing.T, db map[string]string) *Service {
	t.Helper()
	s, err := New(context.Background(), NewRegistry(), fakeLoader{m: db}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.env = func(string) (string, bool) { return "", false } // no env pins in gating tests
	return s
}

// Nothing configured → no feature whose inputs an operator must SUPPLY is available
// (config-design §7).
//
// ⚠ Filler is deliberately excluded: §7's rule is that gating derives from settings
// COMPLETENESS, and a setting carrying a default is complete. `filler.dir` defaults to
// /data/filler (the volume the image already ships), so nothing is missing on a zero-env
// install. Acquisition and Suggestions stay off because their inputs — a Seerr URL, a TMDB
// key — are facts about the operator's world that no default can invent.
func TestFeatures_EmptyIsAllOff(t *testing.T) {
	f := featureService(t, nil).Features()
	if f.Acquisition || f.Suggestions {
		t.Errorf("empty config should gate operator-supplied features off, got %+v", f)
	}
}

// Suggestions need BOTH an LLM provider AND TMDB grounding (config-design §7).
// (llm.provider has a default of "ollama", so the missing piece is tmdb.api_key.)
func TestFeatures_SuggestionsNeedTMDB(t *testing.T) {
	// llm.provider defaults to ollama (set), but tmdb.api_key is empty → gated off.
	if featureService(t, nil).Features().Suggestions {
		t.Error("suggestions should be off without TMDB grounding")
	}
	// Add TMDB → on.
	if !featureService(t, map[string]string{"tmdb.api_key": "tmdbkey"}).Features().Suggestions {
		t.Error("suggestions should be on with a provider + TMDB")
	}
}

// Acquisition is the OR gate: Seerr alone works; Sonarr alone does NOT (needs the
// Radarr pair); Sonarr+Radarr works (config-design §7).
func TestFeatures_AcquisitionOrGate(t *testing.T) {
	cases := []struct {
		name string
		db   map[string]string
		want bool
	}{
		{"nothing", nil, false},
		// Provider defaults to seerr, so the Seerr URL is what counts unless the provider
		// is switched to arr.
		{"seerr only", map[string]string{"seerr.url": "http://seerr:5055"}, true},
		// arr URLs present but provider still seerr → NOT configured: the provider is the
		// explicit switch (§6), so stray arr fields don't silently enable acquisition.
		{"arr urls but provider=seerr", map[string]string{"sonarr.url": "http://sonarr:8989", "radarr.url": "http://radarr:7878"}, false},
		// provider=arr accepts EITHER arr (TV-only or movies-only homelab is valid).
		{"provider=arr, sonarr only", map[string]string{"requester.provider": "arr", "sonarr.url": "http://sonarr:8989"}, true},
		{"provider=arr, radarr only", map[string]string{"requester.provider": "arr", "radarr.url": "http://radarr:7878"}, true},
		{"provider=arr, nothing set", map[string]string{"requester.provider": "arr"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := featureService(t, c.db).Features().Acquisition; got != c.want {
				t.Errorf("Acquisition = %v, want %v", got, c.want)
			}
		})
	}
}

// Filler needs the clip folder (config-design §7).
// ⚠ Filler is ON by default, and the clip root may no longer be emptied: it is a
// generation-scoped storage topology value, not an enable switch. The source's
// explicit enabled setting controls scanning without making every stored catalog
// path lose its root.
//
// The old shape ("off without a dir") was true and load-bearing in the wrong direction: a
// zero-env install opened the Filler page on a single "no folder configured" empty state,
// so every shipped filler capability — catalog, discovery, tagging, splitting — was hidden
// behind a config step nobody was told to take. `database.url` and `backup.dir` both default
// inside /data for exactly this reason; filler was the odd one out for no recorded reason.
func TestFeatures_Filler(t *testing.T) {
	if !featureService(t, nil).Features().Filler {
		t.Error("filler off on a zero-env install — the default drop-folder should turn it on")
	}
	if !featureService(t, map[string]string{"filler.dir": "/srv/clips"}).Features().Filler {
		t.Error("filler off with an explicit dir")
	}
	// A legacy/corrupt empty database value self-heals through the declared default.
	if !featureService(t, map[string]string{"filler.dir": ""}).Features().Filler {
		t.Error("an invalid empty dir did not self-heal to the default clip root")
	}
}

// Available() reflects the set and treats ungated features as always-on.
func TestFeatureSet_Available(t *testing.T) {
	f := FeatureSet{Suggestions: true}
	if !f.Available(FeatureSuggestions) || f.Available(FeatureFiller) {
		t.Error("Available should mirror the set")
	}
	if !f.Available(FeatureNone) {
		t.Error("ungated feature should be available")
	}
}

// UserSync is live-gated on one complete media-server connection. The route and adapter stay
// present while empty, so configuration can enable the next operation without a handler rebuild.
func TestFeatures_UserSyncTracksMediaServerWiring(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]string
		want   bool
	}{
		{name: "empty"},
		{name: "flavor only", values: map[string]string{"library.flavor": "emby"}},
		{name: "missing token", values: map[string]string{
			"library.flavor": "jellyfin", "library.url": "http://jellyfin:8096",
		}},
		{name: "missing flavor", values: map[string]string{
			"library.url": "http://jellyfin:8096", "library.token": "token",
		}},
		{name: "unsupported flavor", values: map[string]string{
			"library.flavor": "plex", "library.url": "http://plex:32400", "library.token": "token",
		}},
		{name: "invalid URL", values: map[string]string{
			"library.flavor": "emby", "library.url": "http://[", "library.token": "token",
		}},
		{name: "unsupported URL scheme", values: map[string]string{
			"library.flavor": "emby", "library.url": "ftp://emby:8096", "library.token": "token",
		}},
		{name: "blank token", values: map[string]string{
			"library.flavor": "emby", "library.url": "http://emby:8096", "library.token": "   ",
		}},
		{name: "complete", values: map[string]string{
			"library.flavor": "jellyfin", "library.url": "http://jellyfin:8096", "library.token": "token",
		}, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := featureService(t, test.values).Features().UserSync; got != test.want {
				t.Fatalf("UserSync = %v, want %v", got, test.want)
			}
		})
	}
}

// Ingest is the one gate NOT derived from settings completeness (config-design §7): it
// depends on whether the running IMAGE carries yt-dlp + ffmpeg. The probe is injected so
// this never touches the filesystem (AGENTS.md: unit tests stay off the disk).
func TestFeatures_IngestGatesArchiveAndYouTubeSeparately(t *testing.T) {
	svc := func(db map[string]string, present ...string) *Service {
		s := featureService(t, db)
		set := map[string]bool{}
		for _, p := range present {
			set[p] = true
		}
		s.execProbe = func(path string) bool { return set[path] }
		return s
	}
	paths := map[string]string{"ingest.ytdlp_path": "/bin/yt-dlp", "ingest.ffmpeg_path": "/bin/ffmpeg"}

	// Configured but missing on disk: everything off, and NOT a boot failure — the image is
	// otherwise perfectly usable without ingest.
	if f := svc(paths).Features(); f.Ingest || f.IngestArchive || f.IngestYouTube {
		t.Error("paths configured but absent on disk → every ingest gate must be off")
	}

	// ⚠ THE case this test exists for, and the one the previous version asserted BACKWARDS.
	// archive.org is fetched over plain HTTP; ffmpeg only probes and thumbnails what arrived.
	// Requiring yt-dlp made a source build with ffmpeg report "downloading unavailable" while
	// being perfectly able to fetch — and the STARTER PULL is an archive.org collection, so
	// first-run acquisition was blocked by a binary it never invokes.
	ffmpegOnly := svc(paths, "/bin/ffmpeg").Features()
	if !ffmpegOnly.IngestArchive {
		t.Error("ffmpeg present → archive.org fetches must be available; they never touch yt-dlp")
	}
	if ffmpegOnly.IngestYouTube {
		t.Error("no yt-dlp → YouTube must be off")
	}
	if !ffmpegOnly.Ingest {
		t.Error("Ingest is the OR of the two — one working source means downloading IS possible")
	}

	// ⚠ The other direction is a REAL dependency and stays: yt-dlp shells out to ffmpeg to
	// combine the separate video and audio streams YouTube serves, so a half-present install
	// would accept the request and fail mid-download on exactly the high-resolution sources most
	// worth fetching. The old test asserted both directions; only this one was ever justified.
	ytdlpOnly := svc(paths, "/bin/yt-dlp").Features()
	if ytdlpOnly.IngestYouTube {
		t.Error("yt-dlp without ffmpeg → YouTube must be off (it cannot merge video+audio)")
	}
	if ytdlpOnly.IngestArchive || ytdlpOnly.Ingest {
		t.Error("no ffmpeg → nothing can be probed, so no source is usable")
	}

	// Both present — the normal case on the shipped image.
	if f := svc(paths, "/bin/yt-dlp", "/bin/ffmpeg").Features(); !f.Ingest || !f.IngestArchive || !f.IngestYouTube {
		t.Errorf("both tools present → every gate must be on, got %+v", f)
	}
}

// ⚠ An UNSET path is looked up on PATH (V38b). §15 has always described these as "defaulted to
// the vendored binaries", but the registry defaults were "" and only the Docker image set them —
// so every source build had ingest off even with the tools installed. Doc and code disagreed;
// this is the code catching up.
// ⚠ **Both seams are faked, and that is the point.** Injecting only `execProbe` left the real
// `exec.LookPath` running, so this test asserted the HOST's PATH: it passed on a developer machine
// with ffmpeg installed and failed on every CI runner without it. Green locally, red in CI, from
// V38c until V39 — the whole point of a double is that the answer cannot depend on the machine.
func TestFeatures_UnsetToolPathsFallBackToPathLookup(t *testing.T) {
	s := featureService(t, nil)
	// "It is on PATH": the lookup resolves the bare name, and the probe accepts what it returned.
	s.execLookPath = func(name string) (string, error) { return "/usr/bin/" + name, nil }
	s.execProbe = func(string) bool { return true }

	if !s.Features().IngestArchive {
		t.Error("ffmpeg on PATH with no configured path → archive fetches must be available")
	}
}

// The mirror, and the case CI was really in: nothing on PATH and nothing configured means the
// gate is OFF. Without this, a lookup double that always succeeded would satisfy the test above
// while proving nothing about the branch that matters on a bare install.
func TestFeatures_NothingOnPathLeavesIngestOff(t *testing.T) {
	s := featureService(t, nil)
	s.execLookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	s.execProbe = func(string) bool { return true } // generous on purpose: the lookup must decide

	if f := s.Features(); f.IngestArchive || f.IngestYouTube || f.Ingest {
		t.Errorf("no tools anywhere → every ingest gate must be off, got %+v", f)
	}
}

// ⚠ A new Feature constant that `Available` does not answer falls through to `default: return
// true` — an UNGATED answer for a gated capability. Concretely: the API would report YouTube
// downloads available on a box with no yt-dlp, which is the opposite of the truth and exactly the
// class of silent-wrong-answer this whole split exists to fix.
func TestFeatureSet_AvailableAnswersEveryIngestGate(t *testing.T) {
	off := FeatureSet{} // everything false

	for _, f := range []Feature{FeatureIngest, FeatureIngestArchive, FeatureIngestYouTube} {
		if off.Available(f) {
			t.Errorf("%s reported AVAILABLE on an all-off FeatureSet — Available() has no case for "+
				"it and fell through to the ungated default", f)
		}
	}
}

