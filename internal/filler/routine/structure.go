package routine

import (
	"sync"

	"github.com/loomarr/loomarr/internal/filler"
)

// LiveStructureRuntime serves the long-reel runtime for the two long-reel files currently set
// (#1659: they apply live). A path edit builds a complete new runtime (authority, deployment,
// assessment, gate and shadow) and replaces the old one in one assignment, so a reader gets one
// or the other and never a mix.
//
// ⚠ Keyed on the PATHS, not the file contents. Both files are reviewed, hash-pinned evidence; a
// new review is a new file. A failed build is cached like a good one, so a bad path logs once
// rather than on every split, and every proposal stays held until the path changes.
type LiveStructureRuntime struct {
	paths func() (authority, deployment string)
	build func(authority, deployment string) filler.StructureRuntime

	mu                  sync.Mutex
	built               bool
	authority, deployed string
	current             filler.StructureRuntime
}

// NewLiveStructureRuntime returns the live runtime. paths reads the two file paths now; build
// makes a complete runtime from them.
func NewLiveStructureRuntime(
	paths func() (authority, deployment string),
	build func(authority, deployment string) filler.StructureRuntime,
) *LiveStructureRuntime {
	return &LiveStructureRuntime{paths: paths, build: build}
}

func (l *LiveStructureRuntime) Current() filler.StructureRuntime {
	authority, deployment := l.paths()
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.built || authority != l.authority || deployment != l.deployed {
		l.current = l.build(authority, deployment)
		l.built, l.authority, l.deployed = true, authority, deployment
	}
	return l.current
}
