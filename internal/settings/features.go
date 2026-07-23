package settings

import "os"

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
	// derived from settings completeness (config-design §7): it depends on whether the
	// running IMAGE carries yt-dlp + ffmpeg, which only loomarr:filler does. No amount
	// of configuring opens it on loomarr:latest, so the UI copy for this gate must say
	// "run the loomarr:filler image" — never "configure this", which would send an
	// operator to a Settings page that cannot help them.
	Ingest bool
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
		Acquisition: s.requesterConfigured(),
		Suggestions: s.allRequiredSet(FeatureSuggestions),
		Filler:      s.allRequiredSet(FeatureFiller),
		UserSync:    !s.isEmpty("library.flavor"),
		Ingest:      s.ingestAvailable(),
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
func (s *Service) ingestAvailable() bool {
	probe := s.execProbe
	if probe == nil {
		probe = isExecutable
	}
	ytdlp, ffmpeg := s.resolveString("ingest.ytdlp_path"), s.resolveString("ingest.ffmpeg_path")
	if ytdlp == "" || ffmpeg == "" {
		return false
	}
	return probe(ytdlp) && probe(ffmpeg)
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
