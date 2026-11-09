package testkit

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/llm"
)

// recordingTB is a testing.TB that runs its cleanups on demand and records what they report, so a
// test can observe the failure the fake raises in t.Cleanup without failing itself.
type recordingTB struct {
	testing.TB
	cleanups []func()
	errs     []string
}

func (r *recordingTB) Helper()                   {}
func (r *recordingTB) Failed() bool              { return false }
func (r *recordingTB) Cleanup(f func())          { r.cleanups = append(r.cleanups, f) }
func (r *recordingTB) Errorf(f string, a ...any) { r.errs = append(r.errs, fmt.Sprintf(f, a...)) }
func (r *recordingTB) runCleanups() {
	for i := len(r.cleanups) - 1; i >= 0; i-- {
		r.cleanups[i]()
	}
}

// A call past the script is an unexpected call: the fake refuses it instead of answering with an
// empty final that a test written around a different call count would pass for.
func TestLLMRefusesACallPastItsScript(t *testing.T) {
	model := NewLLM(t, llm.Response{Content: "one"})
	if resp, err := model.Chat(context.Background(), nil, llm.ChatOptions{}); err != nil || resp.Content != "one" {
		t.Fatalf("scripted call = %+v, %v", resp, err)
	}
	if _, err := model.Chat(context.Background(), nil, llm.ChatOptions{}); !errors.Is(err, ErrUnscripted) {
		t.Fatalf("call past the script = %v, want ErrUnscripted", err)
	}
	empty := NewLLM(t)
	if _, err := empty.Chat(context.Background(), nil, llm.ChatOptions{}); !errors.Is(err, ErrUnscripted) {
		t.Fatalf("call on an empty script = %v, want ErrUnscripted", err)
	}
}

// A scripted turn that never ran fails the test at cleanup: a script longer than the run is a
// path the test believes it covers and does not.
func TestLLMReportsAnUnconsumedScriptInCleanup(t *testing.T) {
	rec := &recordingTB{TB: t}
	model := NewLLM(rec, llm.Response{Content: "one"}, llm.Response{Content: "two"})
	model.Errors = []error{errors.New("boom")}
	if _, err := model.Chat(context.Background(), nil, llm.ChatOptions{}); err == nil {
		t.Fatal("the scripted error was not returned first")
	}
	if _, err := model.Chat(context.Background(), nil, llm.ChatOptions{}); err != nil {
		t.Fatal(err)
	}
	rec.runCleanups()
	if len(rec.errs) != 1 {
		t.Fatalf("cleanup reported %v, want one never-consumed failure", rec.errs)
	}

	done := &recordingTB{TB: t}
	exhausted := NewLLM(done, llm.Response{Content: "one"})
	if _, err := exhausted.Chat(context.Background(), nil, llm.ChatOptions{}); err != nil {
		t.Fatal(err)
	}
	done.runCleanups()
	if len(done.errs) != 0 {
		t.Fatalf("a fully consumed script reported %v", done.errs)
	}
}
