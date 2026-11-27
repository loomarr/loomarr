package main

import (
	"context"
	"errors"
	"net"
	"reflect"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// fakeGeneration records what the process does to a built application.
type fakeGeneration struct {
	events       []string
	quiesceErr   error
	shutdownErr  error
	finalizeLive bool
}

func (g *fakeGeneration) Quiesce(context.Context) error {
	g.events = append(g.events, "quiesce")
	return g.quiesceErr
}

func (g *fakeGeneration) Shutdown(ctx context.Context) error {
	g.events = append(g.events, "shutdown")
	g.finalizeLive = ctx.Err() == nil
	return g.shutdownErr
}

// A generation that was built but cannot be served has workers running and stops registered.
// Without a shutdown the deferred store close runs under live workers and River never stops.
func TestAcquireListenerShutsTheBuiltGenerationDownWhenListenFails(t *testing.T) {
	for _, network := range []struct{ name, host string }{
		{"IPv4", "127.0.0.1"},
		{"IPv6", "::1"},
		{"wildcard", ""},
	} {
		t.Run(network.name, func(t *testing.T) {
			taken, err := net.Listen("tcp", net.JoinHostPort(network.host, "0"))
			if err != nil {
				t.Skipf("cannot listen on %s: %v", network.name, err)
			}
			defer func() { _ = taken.Close() }()
			addr := net.JoinHostPort(network.host, strconv.Itoa(taken.Addr().(*net.TCPAddr).Port))

			abandoned := 0
			listener, err := acquireListener(addr, func() error { abandoned++; return nil })
			if listener != nil {
				_ = listener.Close()
				t.Fatal("listened on a taken address")
			}
			if !errors.Is(err, syscall.EADDRINUSE) {
				t.Fatalf("error = %v, want the listen failure (address in use)", err)
			}
			if abandoned != 1 {
				t.Fatalf("the built generation was shut down %d times, want exactly once", abandoned)
			}
		})
	}

	t.Run("a malformed address", func(t *testing.T) {
		abandoned := 0
		listener, err := acquireListener("not-an-address", func() error { abandoned++; return nil })
		if listener != nil || err == nil {
			t.Fatalf("listener = %v, err = %v; want a failure", listener, err)
		}
		if abandoned != 1 {
			t.Fatalf("shut down %d times, want once", abandoned)
		}
	})

	t.Run("a failing shutdown does not hide the listen failure", func(t *testing.T) {
		taken, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = taken.Close() }()
		shutdownErr := errors.New("workers still running")
		_, err = acquireListener(taken.Addr().String(), func() error { return shutdownErr })
		if !errors.Is(err, syscall.EADDRINUSE) {
			t.Fatalf("error = %v, want the listen failure first", err)
		}
		if !errors.Is(err, shutdownErr) {
			t.Fatalf("error = %v, want the shutdown failure reported too", err)
		}
	})

	t.Run("a free address leaves the generation running", func(t *testing.T) {
		abandoned := 0
		listener, err := acquireListener("127.0.0.1:0", func() error { abandoned++; return nil })
		if err != nil {
			t.Fatal(err)
		}
		_ = listener.Close()
		if abandoned != 0 {
			t.Fatalf("a generation that listened was shut down %d times", abandoned)
		}
	})
}

// The same teardown serves a failed listen, a serve error and the operator's stop: cancel the
// generation's context, quiesce, drain HTTP, then finalize on a live budget.
func TestStopGenerationEndsWhatBuildStarted(t *testing.T) {
	t.Run("runs every step in order", func(t *testing.T) {
		generation := &fakeGeneration{}
		cancelled := false
		httpDrained := false
		err := stopGeneration(
			func() { cancelled = true; generation.events = append(generation.events, "cancel root") },
			time.Second,
			func(context.Context) error {
				httpDrained = true
				generation.events = append(generation.events, "drain http")
				return nil
			},
			generation,
		)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"cancel root", "quiesce", "drain http", "shutdown"}
		if !cancelled || !httpDrained || !reflect.DeepEqual(generation.events, want) {
			t.Fatalf("events = %v, want %v", generation.events, want)
		}
	})

	t.Run("no HTTP server yet", func(t *testing.T) {
		generation := &fakeGeneration{}
		if err := stopGeneration(func() {}, time.Second, nil, generation); err != nil {
			t.Fatal(err)
		}
		if want := []string{"quiesce", "shutdown"}; !reflect.DeepEqual(generation.events, want) {
			t.Fatalf("events = %v, want %v", generation.events, want)
		}
	})

	t.Run("finalizes on a live budget when the drain budget is spent", func(t *testing.T) {
		for _, budget := range []time.Duration{0, time.Nanosecond} {
			generation := &fakeGeneration{}
			err := stopGeneration(func() {}, budget,
				func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }, generation)
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("budget %v: error = %v, want the drain's deadline reported", budget, err)
			}
			if !generation.finalizeLive {
				t.Errorf("budget %v: the generation was finalized on a dead context", budget)
			}
		}
	})

	t.Run("reports every failing step", func(t *testing.T) {
		quiesceErr, shutdownErr := errors.New("quiesce"), errors.New("shutdown")
		generation := &fakeGeneration{quiesceErr: quiesceErr, shutdownErr: shutdownErr}
		err := stopGeneration(func() {}, time.Second, nil, generation)
		if !errors.Is(err, quiesceErr) || !errors.Is(err, shutdownErr) {
			t.Fatalf("error = %v, want both failures", err)
		}
	})
}
