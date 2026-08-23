// Package buildinfo carries the version stamped into the binary at build time.
//
// It exists because nothing exposed a version before: §16 describes upgrades and
// downgrades and the operator runbook assumes the operator knows what they're running,
// but no endpoint, log line, or UI surface said. A bug report without a version is a bug
// report that starts with a round-trip.
package buildinfo

import "runtime/debug"

// Set by the linker: -ldflags "-X github.com/loomarr/loomarr/internal/buildinfo.version=..."
// A source build leaves these empty, which is why Get() falls back to Go's own embedded
// VCS stamps — `go run ./cmd/loomarr` still reports a usable commit.
var (
	version = ""
	commit  = ""
	builtAt = ""
)

// Info is what the binary knows about itself.
type Info struct {
	// Version is the release tag ("v0.3.1"), or "dev" for an unstamped build.
	Version string `json:"version"`
	// Commit is the git SHA, short form when it came from the linker.
	Commit string `json:"commit,omitempty"`
	// BuiltAt is an RFC3339 timestamp; empty on a source build.
	BuiltAt string `json:"builtAt,omitempty"`
	// Dirty reports that the working tree had uncommitted changes at build time —
	// worth surfacing, since "v0.3.1" and "v0.3.1 plus local edits" are different
	// things when someone is diagnosing a problem.
	Dirty bool `json:"dirty,omitempty"`
}

// Get returns the build info, preferring linker stamps and falling back to the VCS
// information Go embeds automatically.
func Get() Info {
	info := Info{Version: version, Commit: commit, BuiltAt: builtAt}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				if info.Commit == "" {
					info.Commit = s.Value
				}
			case "vcs.time":
				if info.BuiltAt == "" {
					info.BuiltAt = s.Value
				}
			case "vcs.modified":
				info.Dirty = s.Value == "true"
			}
		}
	}
	if info.Version == "" {
		info.Version = "dev"
	}
	if len(info.Commit) > 12 {
		info.Commit = info.Commit[:12]
	}
	return info
}
