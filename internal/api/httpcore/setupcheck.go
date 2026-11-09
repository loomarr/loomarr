package httpcore

// SetupCheck is one integration's checklist result (§13). The setup checklist, the Services panel
// and the restart page's connection checks all report it, so it is shared plumbing, not any one
// area's type.
type SetupCheck struct {
	Name string `json:"name" example:"livetv" doc:"Integration key"`
	OK   bool   `json:"ok"`
	Hint string `json:"hint,omitempty" doc:"Actionable hint when not OK (§13)"`
	// DocHref deep-links this check to its Troubleshooting section so a red check
	// is one click from the exact fix (§13 "failures never blame").
	DocHref string `json:"docHref,omitempty" doc:"Anchor into the embedded Troubleshooting docs"`
	// Target is what this check connected TO — the Services panel shows it beside the state
	// (§12, V31), because "media server: FAIL" without an address makes an operator go
	// looking for which one. Empty when the probe has no configurable endpoint (TMDB) or the
	// setting is unset.
	Target string `json:"target,omitempty" doc:"The URL or path this check probed"`
	// NotSetUp is true when the check is not OK because nobody has set this service up yet
	// (#2781): none of its connection settings are set. That is not a failure — an optional
	// service a fresh install has not touched must not read as broken.
	NotSetUp bool `json:"notSetUp,omitempty" doc:"True when not OK only because none of this service's connection settings are set yet"`
}
