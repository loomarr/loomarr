package llm

import (
	"fmt"
	"slices"
	"strings"
)

// InferenceRole names an independently certified job, not a UI setting.
type InferenceRole string

const (
	RoleLineup        InferenceRole = "lineup"
	RoleFillerText    InferenceRole = "filler_text"
	RoleFillerFrames  InferenceRole = "filler_frames"
	RoleFillerVideo   InferenceRole = "filler_video"
	RoleTranscription InferenceRole = "transcription"
)

type InferenceLane string

const (
	LaneCertification InferenceLane = "certification"
	LaneResilience    InferenceLane = "resilience"
)

// InferenceLimits are hard ceilings carried with a route. Zero means the policy
// does not authorize that resource, not "unlimited".
type InferenceLimits struct {
	MaxInputBytes    int64
	MaxDurationMS    int64
	MaxOutputTokens  int
	MaxAttempts      int
	MaxChargeNanoUSD int64
}

// RoleRoute is one exact, versioned route selected by measured certification.
type RoleRoute struct {
	Provider                 string
	Model                    string
	UpstreamProvider         string
	Modalities               []string
	StructuredOutput         bool
	RequireZDR               bool
	AllowResilienceFallbacks bool
	Limits                   InferenceLimits
}

type RolePolicySnapshot struct {
	Version               string
	CertificationArtifact string
	Routes                map[InferenceRole]RoleRoute
}

// ModelCompatibility is capability evidence only. It does not claim accuracy.
type ModelCompatibility struct {
	Provider          string
	Model             string
	Modalities        []string
	StructuredOutput  bool
	ZDR               bool
	UpstreamProviders []string
}

type CompatibilitySnapshot struct {
	Version string
	Models  []ModelCompatibility
}

// RouteOverride is an Advanced operator choice. It may replace identity but may
// not relax the certified role's capability or privacy requirements.
type RouteOverride struct {
	Provider         string
	Model            string
	UpstreamProvider string
}

type ResolvedRoute struct {
	RoleRoute
	Role                  InferenceRole
	Lane                  InferenceLane
	PolicyVersion         string
	CapabilityVersion     string
	CertificationArtifact string
	Certified             bool
	Overridden            bool
	AllowFallbacks        bool
}

// RouteUnavailableError is an operational hold reason. Callers must not turn it
// into a semantic reject or a review task.
type RouteUnavailableError struct {
	Role   InferenceRole
	Reason string
}

func (e *RouteUnavailableError) Error() string {
	return fmt.Sprintf("inference role %s unavailable: %s", e.Role, e.Reason)
}

// RoleResolver is a pure deep module: callers ask one question while snapshot,
// override, capability, privacy, and lane validation remain local.
type RoleResolver struct {
	policy RolePolicySnapshot
	compat CompatibilitySnapshot
}

func NewRoleResolver(policy RolePolicySnapshot, compat CompatibilitySnapshot) (*RoleResolver, error) {
	if strings.TrimSpace(policy.Version) == "" {
		return nil, fmt.Errorf("role policy version is required")
	}
	if strings.TrimSpace(policy.CertificationArtifact) == "" {
		return nil, fmt.Errorf("role policy certification artifact is required")
	}
	if strings.TrimSpace(compat.Version) == "" {
		return nil, fmt.Errorf("compatibility snapshot version is required")
	}
	if len(policy.Routes) == 0 {
		return nil, fmt.Errorf("role policy requires at least one certified route")
	}
	r := &RoleResolver{policy: policy, compat: compat}
	for role, route := range policy.Routes {
		if err := r.validate(role, route); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *RoleResolver) Resolve(role InferenceRole, lane InferenceLane, override *RouteOverride) (ResolvedRoute, error) {
	base, ok := r.policy.Routes[role]
	if !ok {
		return ResolvedRoute{}, &RouteUnavailableError{Role: role, Reason: "no certified route"}
	}
	if lane != LaneCertification && lane != LaneResilience {
		return ResolvedRoute{}, &RouteUnavailableError{Role: role, Reason: "unknown inference lane"}
	}
	route := base
	overridden := override != nil
	if override != nil {
		route.Provider = strings.TrimSpace(override.Provider)
		route.Model = strings.TrimSpace(override.Model)
		// An omitted override retains a certified provider pin. Advanced controls may
		// choose another exact upstream, but cannot silently relax the constraint.
		if upstream := strings.TrimSpace(override.UpstreamProvider); upstream != "" {
			route.UpstreamProvider = upstream
		}
	}
	if err := r.validate(role, route); err != nil {
		return ResolvedRoute{}, err
	}
	return ResolvedRoute{
		RoleRoute: route, Role: role, Lane: lane,
		PolicyVersion: r.policy.Version, CapabilityVersion: r.compat.Version,
		CertificationArtifact: r.policy.CertificationArtifact,
		Certified:             !overridden && lane == LaneCertification,
		Overridden:            overridden,
		AllowFallbacks:        !overridden && lane == LaneResilience && route.AllowResilienceFallbacks,
	}, nil
}

func (r *RoleResolver) validate(role InferenceRole, route RoleRoute) error {
	if strings.TrimSpace(route.Provider) == "" || strings.TrimSpace(route.Model) == "" {
		return &RouteUnavailableError{Role: role, Reason: "provider and concrete model are required"}
	}
	for _, model := range r.compat.Models {
		if model.Provider != route.Provider || model.Model != route.Model {
			continue
		}
		for _, modality := range route.Modalities {
			if !slices.Contains(model.Modalities, modality) {
				return &RouteUnavailableError{Role: role, Reason: fmt.Sprintf("%s/%s lacks %s input", route.Provider, route.Model, modality)}
			}
		}
		if route.StructuredOutput && !model.StructuredOutput {
			return &RouteUnavailableError{Role: role, Reason: "structured output is not advertised"}
		}
		if route.RequireZDR && !model.ZDR {
			return &RouteUnavailableError{Role: role, Reason: "zero-data-retention routing is unavailable"}
		}
		if route.UpstreamProvider != "" && !slices.Contains(model.UpstreamProviders, route.UpstreamProvider) {
			return &RouteUnavailableError{Role: role, Reason: "pinned upstream provider is unavailable"}
		}
		return nil
	}
	return &RouteUnavailableError{Role: role, Reason: fmt.Sprintf("%s/%s is absent from compatibility snapshot %s", route.Provider, route.Model, r.compat.Version)}
}
