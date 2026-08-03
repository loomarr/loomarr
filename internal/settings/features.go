package settings

import (
	"os"
	"os/exec"
)

// FeatureSet is the computed availability of each gated capability (config-design
// §7). One function produces it; the API 409s, the tab empty states, and the
// wizard checklist all read the SAME set — no drift.
type FeatureSet struct {
	Acquisition bool // a requester is configured (Seerr OR direct Sonarr+Radarr)
	Suggestions bool // an LLM + TMDB grounding are configured
	Filler      bool // a filler drop-folder is configured
	// UserSync reports whether users can be imported/synced from a media server.
	// Unlike the others this gates a route that is CONDITIONALLY REGISTERED
	// (internal/api/usersroutes.go): with no media server, POST /v1/users/sync does not
	// exist at runtime even though it is present in the committed spec, because the spec
	// is generated in schema-only mode where everything registers. Without this flag the
	// generated client happily calls an endpoint that 404s. It must therefore mirror
	// app.go's wiring condition EXACTLY — `library.flavor` set — not merely something
	// close to it, or the flag and the route drift apart again.
	UserSync bool
	// Ingest reports whether clips can be downloaded in-app. It is the ONE gate not
	// derived from settings completeness (config-design §7): it depends on whether
	// yt-dlp + ffmpeg are actually RUNNABLE, which no setting can assert.
	//
	// ⚠ This used to mean "you are on loomarr:latest, switch to loomarr:filler". That
	// two-tag split no longer exists — the single image always ships the tooling (§16) —
	// so OFF now means a DEGRADED install: a custom image without the vendored binaries,
	// or a configured path that is missing or not executable. The UI copy must not send
	// an operator hunting for an image tag that is not published.
	//
	// ⚠ **`Ingest` means "SOME source can download", not "everything works"** (V38b). It is the OR
	// of the two gates below, kept so existing callers that only ask "is downloading possible at
	// all" keep working. Anything reporting WHICH source is available must read `IngestArchive` /
	// `IngestYouTube` instead — see the note on those.
	Ingest bool
	// IngestArchive: archive.org fetches, which use plain HTTP and need only ffmpeg (to probe and
	// thumbnail what they fetched).
	//
	// ⚠ Split out because one flag for both downloaders was a real defect: a missing yt-dlp
	// switched off archive.org too, despite that path never invoking it — and the STARTER PULL is
	// an archive.org collection, so first-run acquisition was blocked by a binary it does not use.
	// The invariant "every ingest needs yt-dlp" was true when written and became false when the
	// archive downloader landed beside it.
	IngestArchive bool
	// IngestYouTube: yt-dlp fetches. Needs yt-dlp AND ffmpeg (yt-dlp shells out to it to combine
	// separate video and audio streams).
	IngestYouTube bool
}

// Available reports whether a named feature is on.
func (f FeatureSet) Available(feature Feature) bool {
	switch feature {
	case FeatureAcquisition:
		return f.Acquisition
	case FeatureSuggestions:
		return f.Suggestions
	case FeatureFiller:
		return f.Filler
	case FeatureUserSync:
		return f.UserSync
	case FeatureIngest:
		return f.Ingest
	// ⚠ Without these two cases the switch falls through to `default: return true` — an UNGATED
	// answer for a gated capability, i.e. the API would report YouTube available on a box with no
	// yt-dlp. A new Feature constant is not enough; it has to be answered here too.
	case FeatureIngestArchive:
		return f.IngestArchive
	case FeatureIngestYouTube:
		return f.IngestYouTube
	default:
		return true // ungated
	}
}

// Features computes the live feature set from settings completeness (config-design
// §7). Suggestions and Filler are simple "every RequiredFor key is set" folds;
// Acquisition is the one OR-shaped gate (Seerr OR the direct Sonarr+Radarr pair),
// so it's evaluated explicitly rather than forced into the generic fold.
func (s *Service) Features() FeatureSet {
	return FeatureSet{
		Acquisition:   s.requesterConfigured(),
		Suggestions:   s.allRequiredSet(FeatureSuggestions),
		Filler:        s.allRequiredSet(FeatureFiller),
		UserSync:      !s.isEmpty("library.flavor"),
		Ingest:        s.ingestArchiveAvailable() || s.ingestYouTubeAvailable(),
		IngestArchive: s.ingestArchiveAvailable(),
		IngestYouTube: s.ingestYouTubeAvailable(),
	}
}

// allRequiredSet reports whether every registry key with RequiredFor == feature
// resolves to a non-empty value. A feature with no required keys is trivially on.
func (s *Service) allRequiredSet(feature Feature) bool {
	for _, set := range s.reg.All() {
		if set.Required == feature && s.isEmpty(set.Key) {
			return false
		}
	}
	return true
}

// requesterConfigured is the acquisition gate (config-design §7), now provider-aware.
// The `requester.provider` selects which backend counts: `seerr` → the Seerr URL;
// `arr` → EITHER Sonarr or Radarr (a TV-only or movies-only homelab is valid — you
// don't need both to acquire the one kind you have). Absent/unknown provider falls
// back to the historical OR (Seerr, or either arr) so an install predating the
// selector still reads as configured. A URL present is what "configured" means.
func (s *Service) requesterConfigured() bool {
	seerr := !s.isEmpty("seerr.url")
	arr := !s.isEmpty("sonarr.url") || !s.isEmpty("radarr.url")
	switch s.resolveString("requester.provider") {
	case "seerr":
		return seerr
	case "arr":
		return arr
	default:
		return seerr || arr
	}
}

// resolveString resolves a key to its string value ("" when unset or non-string).
func (s *Service) resolveString(key string) string {
	if v, ok := s.Resolve(key).Value.(string); ok {
		return v
	}
	return ""
}

// isEmpty reports whether a key resolves to an empty/zero value (unset). Used only
// for gating — an empty string, empty URL, or a nil default all count as "not set".
func (s *Service) isEmpty(key string) bool {
	r := s.Resolve(key)
	switch v := r.Value.(type) {
	case nil:
		return true
	case string:
		return v == ""
	default:
		return false // non-string required keys aren't part of any current gate
	}
}

// ingestAvailable reports whether BOTH ingest tools resolve to runnable executables.
// Both are required: yt-dlp cannot merge separate video/audio streams without ffmpeg,
// so a half-configured image would accept an ingest request and then fail mid-download
// on exactly the high-resolution sources most worth fetching.
//
// The paths come from settings (so an operator can point at a newer yt-dlp), but the
// CHECK is a filesystem probe — which is why this gate is environment-derived rather
// than completeness-derived. execProbe is injectable so tests never touch the disk.
// ingestArchiveAvailable reports whether archive.org fetches can run: HTTP plus ffmpeg.
//
// ⚠ It deliberately does NOT consult yt-dlp. `clipfetch` routes archive.org through a plain
// net/http downloader; requiring yt-dlp here is what made a source build with ffmpeg installed
// report "downloading unavailable" while being able to fetch perfectly well.
func (s *Service) ingestArchiveAvailable() bool {
	return s.toolRunnable("ingest.ffmpeg_path")
}

// ingestYouTubeAvailable reports whether yt-dlp fetches can run. Both binaries: yt-dlp shells out
// to ffmpeg to combine the separate video and audio streams YouTube serves.
func (s *Service) ingestYouTubeAvailable() bool {
	return s.toolRunnable("ingest.ytdlp_path") && s.toolRunnable("ingest.ffmpeg_path")
}

// toolRunnable resolves one tool path and reports whether it names something executable.
//
// ⚠ An UNSET path falls back to a `PATH` lookup (V38b). §15 has always described these as
// "defaulted to the vendored binaries", but the registry defaults were the empty string and only
// the Docker image set them — so every source build had ingest off even with the tools installed,
// and the doc and the code disagreed. Looking on PATH is what makes the documented behaviour true.
func (s *Service) toolRunnable(key string) bool {
	probe := s.execProbe
	if probe == nil {
		probe = isExecutable
	}
	if p := s.resolveString(key); p != "" {
		return probe(p)
	}
	// Unset: find it the way a shell would. LookPath already checks the execute bit, but the
	// probe still runs so a test double sees every candidate.
	name := map[string]string{"ingest.ytdlp_path": "yt-dlp", "ingest.ffmpeg_path": "ffmpeg"}[key]
	if name == "" {
		return false
	}
	found, err := exec.LookPath(name)
	return err == nil && probe(found)
}

// isExecutable reports whether path names an existing file with an execute bit. A
// configured-but-missing path is treated as unavailable rather than an error: the
// operator sees the feature off and the reason in the checklist, instead of a boot
// failure on an image that is otherwise perfectly usable without ingest.
func isExecutable(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return false
	}
	return fi.Mode().Perm()&0o111 != 0
}
