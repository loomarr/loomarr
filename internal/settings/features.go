package settings

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

// requesterConfigured is the OR gate (config-design §7): Seerr configured, OR the
// direct Sonarr+Radarr pair both configured. A requester URL is what "configured"
// means (the key is present); keys are validated on save.
func (s *Service) requesterConfigured() bool {
	seerr := !s.isEmpty("seerr.url")
	direct := !s.isEmpty("sonarr.url") && !s.isEmpty("radarr.url")
	return seerr || direct
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
