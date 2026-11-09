package routine

import (
	"context"
	"sync"
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/filler/structure"
)

// runtimePart stands in for both the assessment and the shadow, tagged with the build that made it.
type runtimePart struct{ build int }

func (runtimePart) Assess(context.Context, split.AssessmentSource) (structure.Artifact, error) {
	return structure.Artifact{}, nil
}

func (runtimePart) NeedsStructureSplitObservation(context.Context, filler.SplitProposal) (bool, error) {
	return false, nil
}

func (runtimePart) ObserveStructureSplit(context.Context, filler.SplitProposal, filler.SplitPartition) error {
	return nil
}

// The long-reel files apply live (#1659). A path edit must rebuild the runtime whole, and a reader
// racing the edit gets the old runtime or the new one, never parts of both.
func TestLiveStructureRuntimeSwapsWholeRuntimesOnPathChange(t *testing.T) {
	var mu sync.Mutex
	authority, deployment := "/reviewed/a.json", "/reviewed/d.json"
	builds := 0
	live := NewLiveStructureRuntime(
		func() (string, string) {
			mu.Lock()
			defer mu.Unlock()
			return authority, deployment
		},
		func(string, string) filler.StructureRuntime {
			builds++ // serialized by the holder's own lock
			part := runtimePart{build: builds}
			return filler.StructureRuntime{Decisioner: part, Shadow: part, Materialization: &filler.StructureMaterializationPolicy{}}
		},
	)
	first := live.Current()
	if again := live.Current(); builds != 1 || again.Materialization != first.Materialization {
		t.Fatalf("unchanged paths rebuilt the runtime: builds=%d", builds)
	}

	var wg sync.WaitGroup
	for reader := 0; reader < 8; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 200 {
				rt := live.Current()
				if rt.Decisioner.(runtimePart).build != rt.Shadow.(runtimePart).build {
					t.Error("one runtime carried parts of two builds")
					return
				}
			}
		}()
	}
	for edit := 0; edit < 50; edit++ {
		mu.Lock()
		if edit%2 == 0 {
			deployment = "/reviewed/d2.json"
		} else {
			deployment = "/reviewed/d.json"
		}
		mu.Unlock()
		live.Current()
	}
	wg.Wait()

	if last := live.Current(); last.Materialization == first.Materialization || builds < 2 {
		t.Fatalf("a path edit did not replace the runtime: builds=%d", builds)
	}
}
