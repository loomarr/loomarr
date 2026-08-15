package rustgen

import (
	"context"
	"os"
	"reflect"
	"runtime"
	"time"
)

// Observation is one real worker-process invocation. It carries only bounded vocabulary and
// aggregate sizes; source paths, hashes, URLs and free-form diagnostics never enter telemetry.
type Observation struct {
	Kind         string
	Result       string
	Duration     time.Duration
	InputBytes   int64
	OutputBytes  int64
	PeakRSSBytes int64
}

type observerKey struct{}

// WithObserver attaches a process-boundary observer to ctx. The adapter invokes it exactly once
// for Generate, including refusals and process/protocol failures.
func WithObserver(ctx context.Context, observe func(Observation)) context.Context {
	if observe == nil {
		return ctx
	}
	return context.WithValue(ctx, observerKey{}, observe)
}

func observerFrom(ctx context.Context) func(Observation) {
	observe, _ := ctx.Value(observerKey{}).(func(Observation))
	return observe
}

// processPeakRSSBytes avoids an OS-specific build tag solely for one ProcessState field. Go's
// supported Unix targets expose Maxrss with different units: bytes on Darwin, KiB on Linux.
// Reflection lets other targets report 0 rather than making the worker adapter uncompilable.
func processPeakRSSBytes(state *os.ProcessState) int64 {
	if state == nil {
		return 0
	}
	v := reflect.ValueOf(state.SysUsage())
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return 0
	}
	maxRSS := v.FieldByName("Maxrss")
	if !maxRSS.IsValid() || !maxRSS.CanInt() {
		return 0
	}
	bytes := maxRSS.Int()
	if runtime.GOOS != "darwin" {
		bytes *= 1024
	}
	return bytes
}
