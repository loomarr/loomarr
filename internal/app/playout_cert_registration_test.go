//go:build !windows

package app

import (
	"context"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/playoutcert"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit/execfixture"
)

func TestSyntheticShutdownStopsChildRegisteredLate(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	st, err := store.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "diagnostics.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	manager := diagnostics.NewProcessManager(st, nil, diagnostics.ProcessOptions{OutputDir: t.TempDir()})
	defer func() { _ = manager.Close(context.Background()) }()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	target := &PlayoutCertificationTarget{
		listener: listener, diagnostics: manager,
		parents: make(map[string]*syntheticParent), children: make(map[string]*syntheticChild),
	}
	process := execfixture.POSIX(t, "synthetic-registration-process", "while :; do sleep 1; done")
	parent, err := syntheticRegistrationProcess(ctx, process, manager, diagnostics.ProcessSpec{Purpose: "playout_parent", ChannelID: "channel-a"})
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Stop()
	target.parents["channel-a"] = &syntheticParent{process: parent, generation: 1}
	child, err := syntheticRegistrationProcess(ctx, process, manager, diagnostics.ProcessSpec{
		Purpose: "playout_program", ChannelID: "channel-a", ParentRunID: parent.ProcessRunID(), Target: "baseline",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer child.Stop()

	if _, err := target.stop(); err != nil {
		t.Fatal(err)
	}
	target.registerChild(diagnostics.ProcessSpec{
		Purpose: "playout_program", ChannelID: "channel-a", ParentRunID: parent.ProcessRunID(), Target: "baseline",
	}, child)

	done := make(chan struct{})
	go func() { _ = child.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("child registered after shutdown remained alive")
	}
}

func TestSyntheticShutdownWaitsForLateChildAdmissionFromHTTPHandler(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	st, err := store.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "diagnostics.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	manager := diagnostics.NewProcessManager(st, nil, diagnostics.ProcessOptions{OutputDir: t.TempDir()})
	defer func() { _ = manager.Close(context.Background()) }()
	process := execfixture.POSIX(t, "synthetic-registration-process", "while :; do sleep 1; done")
	parent, err := syntheticRegistrationProcess(ctx, process, manager, diagnostics.ProcessSpec{Purpose: "playout_parent", ChannelID: "channel-a"})
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Stop()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	target := &PlayoutCertificationTarget{
		listener: listener, diagnostics: manager,
		parents:  map[string]*syntheticParent{"channel-a": {process: parent, generation: 1}},
		children: make(map[string]*syntheticChild),
	}
	spec := diagnostics.ProcessSpec{
		Purpose: "playout_program", ChannelID: "channel-a", ParentRunID: parent.ProcessRunID(), Target: "baseline",
	}
	spawned := make(chan *playout.Process, 1)
	releaseRegistration := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseRegistration) }) }
	t.Cleanup(func() {
		release()
		if target.server != nil {
			_ = target.server.Close()
		}
		_ = listener.Close()
	})
	childJoined := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		child, startErr := syntheticRegistrationProcess(ctx, process, manager, spec)
		if startErr != nil {
			http.Error(w, startErr.Error(), http.StatusInternalServerError)
			return
		}
		spawned <- child
		<-releaseRegistration
		target.registerChild(spec, child)
		_ = child.Wait()
		close(childJoined)
		w.WriteHeader(http.StatusNoContent)
	})
	target.server = &http.Server{Handler: handler}
	shutdownStarted := make(chan struct{})
	target.server.RegisterOnShutdown(func() { close(shutdownStarted) })
	go func() { _ = target.server.Serve(listener) }()

	requestDone := make(chan error, 1)
	go func() {
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+listener.Addr().String(), nil)
		if requestErr != nil {
			requestDone <- requestErr
			return
		}
		response, requestErr := http.DefaultClient.Do(request)
		if response != nil {
			_ = response.Body.Close()
		}
		requestDone <- requestErr
	}()
	child := waitValue(t, spawned, "HTTP handler child spawn")
	t.Cleanup(child.Stop)

	stopDone := make(chan struct {
		receipt playoutcert.ShutdownReceipt
		err     error
	}, 1)
	go func() {
		receipt, stopErr := target.stop()
		stopDone <- struct {
			receipt playoutcert.ShutdownReceipt
			err     error
		}{receipt: receipt, err: stopErr}
	}()
	// RegisterOnShutdown runs only after stop has closed registration admission and
	// entered the real http.Server shutdown path. The handler is still blocked.
	waitSignal(t, shutdownStarted, "HTTP server shutdown")
	select {
	case result := <-stopDone:
		t.Fatalf("stop returned before the admitted handler released: %+v, %v", result.receipt, result.err)
	case <-time.After(100 * time.Millisecond):
	}
	release()
	waitSignal(t, childJoined, "late child exit")
	if err := waitError(t, requestDone, "HTTP handler response"); err != nil {
		t.Fatal(err)
	}
	result := waitValue(t, stopDone, "shutdown receipt")
	if result.err != nil || !result.receipt.ServingStopped || !result.receipt.ProcessesExited {
		t.Fatalf("stop receipt = %+v, %v", result.receipt, result.err)
	}
}

func syntheticRegistrationProcess(ctx context.Context, executable string, manager *diagnostics.ProcessManager, spec diagnostics.ProcessSpec) (*playout.Process, error) {
	return playout.StartObserved(ctx, executable, nil, nil, nil, manager, spec)
}
