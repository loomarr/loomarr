package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/metrics"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/playoutcert"
	"github.com/loomarr/loomarr/internal/prepared"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
)

// PlayoutCertificationConfig configures the disposable application-composed
// target used by the playout certification command and integration tests.
type PlayoutCertificationConfig struct {
	Scope             string
	Channels          []playoutcert.Channel
	FFmpeg            string
	Capacity          int
	Grace             time.Duration
	ProgrammeDuration time.Duration
}

type PlayoutCertificationTarget struct {
	BaseURL     string
	AdminBearer string
	DeviceToken string

	server           *http.Server
	handler          http.Handler
	listener         net.Listener
	origin           *playout.Origin
	diagnostics      *diagnostics.ProcessManager
	store            store.Store
	root             string
	boundaryRecorder *playoutcert.ProgrammeBoundaryRecorder
	lifecycle        syntheticLifecycle
	scope            string
	parentsMu        sync.Mutex
	// stopping closes registration admission before either WaitGroup starts waiting.
	stopping             bool
	parents              map[string]*syntheticParent
	nextParentGeneration uint64
	parentsWG            sync.WaitGroup
	children             map[string]*syntheticChild
	nextChildGeneration  uint64
	childrenWG           sync.WaitGroup
}

type syntheticParent struct {
	process    *playout.Process
	generation uint64
	faulting   bool
}

type syntheticChild struct {
	process          *playout.Process
	parentGeneration uint64
	generation       uint64
	parentRunID      string
	target           string
	faulting         bool
}

func NewPlayoutCertificationTarget(ctx context.Context, config PlayoutCertificationConfig) (*PlayoutCertificationTarget, error) {
	if len(config.Channels) == 0 || len(config.Channels) > 1000 {
		return nil, errors.New("synthetic target requires 1..1000 Channels")
	}
	if config.Capacity <= 0 {
		config.Capacity = 4
	}
	if config.Grace <= 0 {
		config.Grace = 2 * time.Second
	}
	if config.ProgrammeDuration <= 0 {
		config.ProgrammeDuration = 4 * time.Second
	}
	if config.ProgrammeDuration < 2*time.Second || config.ProgrammeDuration > 30*time.Second {
		return nil, errors.New("synthetic programme duration must be within 2s..30s")
	}
	ffmpeg := strings.TrimSpace(config.FFmpeg)
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	if _, err := exec.LookPath(ffmpeg); err != nil {
		return nil, errors.New("synthetic target requires ffmpeg")
	}
	root, err := os.MkdirTemp("", "loomarr-playout-cert-")
	if err != nil {
		return nil, err
	}
	scope := strings.TrimSpace(config.Scope)
	if scope == "" {
		scope = "isolated-playout-cert"
	}
	target := &PlayoutCertificationTarget{root: root, scope: scope, boundaryRecorder: playoutcert.NewProgrammeBoundaryRecorder(), parents: make(map[string]*syntheticParent), children: make(map[string]*syntheticChild)}
	fail := func(err error) (*PlayoutCertificationTarget, error) {
		_ = target.Close(context.Background())
		return nil, err
	}

	st, err := store.Open(ctx, "sqlite://"+filepath.Join(root, "loomarr.db"), true)
	if err != nil {
		return fail(err)
	}
	target.store = st
	for index, channel := range config.Channels {
		row := store.Channel{Channel: schedule.Channel{ID: channel.ID, Name: fmt.Sprintf("Synthetic %03d", index+1), Number: index + 1, Status: schedule.StatusLive}}
		row.Policy.Playout = &schedule.PlayoutPolicy{Backend: schedule.PlayoutBackendInternal}
		if _, err := st.SaveChannel(ctx, row); err != nil {
			return fail(err)
		}
	}

	sources := [2]string{filepath.Join(root, "source-0.mp4"), filepath.Join(root, "source-1.mp4")}
	for variant, source := range sources {
		if err := generateSyntheticSource(ctx, ffmpeg, source, variant); err != nil {
			return fail(err)
		}
	}
	library, err := prepared.NewLibrary(filepath.Join(root, "prepared"))
	if err != nil {
		return fail(err)
	}
	rendition := prepared.RenditionContract{
		VideoCodec: "h264", VideoProfile: "high", VideoLevel: "4.1", PixelFormat: "yuv420p", HDR: "sdr",
		AudioCodec: "aac", AudioLayout: "stereo", Width: 320, Height: 180, FrameRate: 25,
		VideoBitrateKbps: 600, AudioBitrateKbps: 96, SegmentDurationMS: 1000, PackagingVersion: prepared.CurrentPackagingVersion,
	}
	preparedSpecs := make(map[string][2]prepared.Specification)
	preparedIndexes := playoutcert.PreparedChannelIndexes(config.Channels)
	if len(preparedIndexes) > 0 {
		first := config.Channels[preparedIndexes[0]]
		packager := prepared.NewFFmpegPackager(ffmpeg)
		var templates [2]prepared.Publication
		var firstSpecs [2]prepared.Specification
		for variant, source := range sources {
			spec := syntheticPreparedSpecification(first.ID, variant, rendition)
			template, publishErr := library.Publish(ctx, spec, func(buildCtx context.Context, workspace string) (prepared.Output, error) {
				return packager.Package(buildCtx, workspace, prepared.LocalInput(source), 0, rendition)
			})
			if publishErr != nil {
				return fail(publishErr)
			}
			firstSpecs[variant], templates[variant] = spec, template
		}
		preparedSpecs[first.ID] = firstSpecs
		for _, index := range preparedIndexes[1:] {
			channel := config.Channels[index]
			var specs [2]prepared.Specification
			for variant, template := range templates {
				spec := syntheticPreparedSpecification(channel.ID, variant, rendition)
				_, publishErr := library.Publish(ctx, spec, func(_ context.Context, workspace string) (prepared.Output, error) {
					for _, name := range template.Files {
						from, to := filepath.Join(template.Directory, name), filepath.Join(workspace, name)
						if linkErr := os.Link(from, to); linkErr != nil {
							if copyErr := copyRegularFile(from, to); copyErr != nil {
								return prepared.Output{}, copyErr
							}
						}
					}
					return prepared.Output{Files: append([]string(nil), template.Files...)}, nil
				})
				if publishErr != nil {
					return fail(publishErr)
				}
				specs[variant] = spec
			}
			preparedSpecs[channel.ID] = specs
		}
	}

	admin, err := randomCredential()
	if err != nil {
		return fail(err)
	}
	device, err := randomCredential()
	if err != nil {
		return fail(err)
	}
	target.AdminBearer, target.DeviceToken = admin, device
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fail(err)
	}
	target.listener = listener
	target.BaseURL = "http://" + listener.Addr().String()

	logger := slog.New(slog.DiscardHandler)
	processManager := diagnostics.NewProcessManager(st, nil, diagnostics.ProcessOptions{
		OutputDir: filepath.Join(root, "diagnostics"), InstanceID: "synthetic-cert",
	})
	target.diagnostics = processManager
	processLog := diagnostics.NewProcessLog(st, diagnostics.ProcessReadOptions{OutputDir: filepath.Join(root, "diagnostics")})
	scheduleEpoch := time.Now().UTC().Add(-config.ProgrammeDuration / 2)
	programmeSchedule := syntheticProgrammeSchedule{epoch: scheduleEpoch, duration: config.ProgrammeDuration}
	preparedResolver := syntheticPreparedResolver{specs: preparedSpecs, schedule: programmeSchedule}
	preparedCount := len(preparedIndexes)
	preparedStatus := syntheticPreparedStatus{count: preparedCount}
	preparedOrigin := playout.NewPreparedOrigin(library, preparedResolver)
	preparedBlock := preparedOrigin.MPEGTSBlockSource(ffmpeg, logger, processManager)
	liveResolver := syntheticLiveResolver{sources: sources, schedule: programmeSchedule}

	var manager *playout.Manager
	spawner := func(spawnCtx context.Context, channelID string, plan playout.EncodePlan) (*playout.Process, error) {
		sourceID := target.boundaryRecorder.NextSource()
		sourceForParent := syntheticBlockSource(target.BaseURL, device, preparedBlock, manager, target.boundaryRecorder, sourceID)
		process, spawnErr := playout.BlockSpawner(ffmpeg, sourceForParent, logger, processManager)(spawnCtx, channelID, plan)
		if spawnErr != nil {
			return nil, spawnErr
		}
		target.registerParent(channelID, process)
		return process, nil
	}
	manager = playout.NewManager(spawner, func() int { return config.Capacity }, config.Grace, logger).
		WithCostEstimator(func(estimateCtx context.Context, channelID string, plan playout.EncodePlan) int {
			ready, readyErr := preparedOrigin.MPEGTSReady(estimateCtx, channelID, plan)
			if readyErr == nil && ready {
				return 0
			}
			return plan.EstimatedCost()
		})
	recorder := metrics.New(metrics.Options{Version: "synthetic", Revision: "synthetic", Database: "sqlite"})
	manager.WithObserver(recorder)
	origin := playout.NewOrigin(playout.OriginDependencies{Prepared: preparedOrigin, LiveSessions: manager, Observer: recorder})
	target.origin = origin
	handler := api.Router(logger, api.Options{
		Store: st, Auth: api.NewTokenAuthorizer(admin), Log: logger, Metrics: recorder,
		PlayoutSecret: func() string { return device }, Playout: origin, PlayoutObserver: manager,
		PreparedObserver: preparedStatus,
		PlayoutResolver:  liveResolver,
		PlayoutEncoder: func(encodeCtx context.Context, args []string, progress func(playout.Progress)) (*playout.Process, error) {
			spec, _ := diagnostics.ProcessSpecFromContext(encodeCtx)
			process, spawnErr := playout.StartObserved(encodeCtx, ffmpeg, args, logger, progress, processManager, spec)
			if spawnErr != nil {
				return nil, spawnErr
			}
			target.registerChild(spec, process)
			return process, nil
		},
		DiagnosticProcesses: processLog,
		LiveConfig: func(key string) string {
			switch key {
			case "server.public_url":
				return target.BaseURL
			case "playout.backend":
				return schedule.PlayoutBackendInternal
			default:
				return ""
			}
		},
	})
	syntheticHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Go test binaries do not carry the repository VCS stamp. The isolated
		// target therefore owns an explicit synthetic identity while every real
		// target still has to return its linker-stamped Loomarr revision.
		if r.URL.Path == "/v1/system/version" && r.Method == http.MethodGet && r.Header.Get("Authorization") == "Bearer "+admin {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"version": "synthetic", "commit": strings.Repeat("0", 40), "ready": true,
			})
			return
		}
		handler.ServeHTTP(w, r)
	})
	target.server = &http.Server{Handler: syntheticHandler, ReadHeaderTimeout: 5 * time.Second}
	target.handler = syntheticHandler
	go func() { _ = target.server.Serve(listener) }()
	return target, nil
}

func (t *PlayoutCertificationTarget) Scope() string {
	if t == nil {
		return ""
	}
	return t.scope
}

func (t *PlayoutCertificationTarget) registerParent(channelID string, process *playout.Process) {
	t.parentsMu.Lock()
	if t.stopping {
		t.parentsMu.Unlock()
		process.Stop()
		return
	}
	t.nextParentGeneration++
	generation := t.nextParentGeneration
	t.parents[channelID] = &syntheticParent{process: process, generation: generation}
	t.parentsWG.Add(1)
	t.parentsMu.Unlock()
	go func() {
		defer t.parentsWG.Done()
		_ = process.Wait()
		t.parentsMu.Lock()
		if current := t.parents[channelID]; current != nil && current.process == process && current.generation == generation {
			delete(t.parents, channelID)
		}
		t.parentsMu.Unlock()
	}()
}

// registerChild admits only a program encoder whose opaque parent correlation
// still names this target's current parent for the same channel. The callback
// is synthetic-only; diagnostics remain correlation, never control authority.
func (t *PlayoutCertificationTarget) registerChild(spec diagnostics.ProcessSpec, process *playout.Process) {
	if t == nil || process == nil || spec.Purpose != "playout_program" || spec.ChannelID == "" || spec.ParentRunID == "" || spec.Target == "" {
		return
	}
	t.parentsMu.Lock()
	if t.stopping {
		t.parentsMu.Unlock()
		process.Stop()
		return
	}
	parent := t.parents[spec.ChannelID]
	if parent == nil || parent.process == nil || parent.faulting || parent.process.ProcessRunID() != spec.ParentRunID {
		t.parentsMu.Unlock()
		return
	}
	t.nextChildGeneration++
	generation := t.nextChildGeneration
	child := &syntheticChild{process: process, parentGeneration: parent.generation, generation: generation, parentRunID: spec.ParentRunID, target: spec.Target}
	t.children[spec.ChannelID] = child
	t.childrenWG.Add(1)
	t.parentsMu.Unlock()
	go func() {
		defer t.childrenWG.Done()
		_ = process.Wait()
		t.parentsMu.Lock()
		if current := t.children[spec.ChannelID]; current == child {
			delete(t.children, spec.ChannelID)
		}
		t.parentsMu.Unlock()
	}()
}

// FailParent performs the parent-failure drill only for this target's current
// owned parent. It never resolves or signals an arbitrary PID.
func (t *PlayoutCertificationTarget) FailParent(ctx context.Context, request playoutcert.ParentFaultRequest) (playoutcert.ParentFaultReceipt, error) {
	if t == nil || strings.TrimSpace(request.BaseURL) != t.BaseURL {
		return playoutcert.ParentFaultReceipt{}, errors.New("parent fault target mismatch")
	}
	if err := ctx.Err(); err != nil {
		return playoutcert.ParentFaultReceipt{}, err
	}
	t.parentsMu.Lock()
	parent := t.parents[request.ChannelID]
	if parent == nil || parent.process == nil || parent.faulting || request.Generation == 0 || request.Generation != parent.generation {
		t.parentsMu.Unlock()
		return playoutcert.ParentFaultReceipt{}, errors.New("parent fault stale or already ended")
	}
	parent.faulting = true
	process, generation := parent.process, parent.generation
	t.parentsMu.Unlock()
	process.Stop()
	// Stop waits for the owned child to leave; Wait is safe concurrently and
	// establishes that the receipt is issued after an actual exit.
	_ = process.Wait()
	return playoutcert.ParentFaultReceipt{ChannelID: request.ChannelID, Generation: generation, Exited: true}, nil
}

// CurrentParent exposes the current opaque generation only after validating
// target identity. It does not expose a process ID or any diagnostics handle.
func (t *PlayoutCertificationTarget) CurrentParent(ctx context.Context, request playoutcert.ParentFaultRequest) (uint64, error) {
	if t == nil || strings.TrimSpace(request.BaseURL) != t.BaseURL {
		return 0, errors.New("parent fault target mismatch")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	t.parentsMu.Lock()
	defer t.parentsMu.Unlock()
	parent := t.parents[request.ChannelID]
	if parent == nil || parent.process == nil || parent.faulting {
		return 0, errors.New("parent fault stale or already ended")
	}
	return parent.generation, nil
}

// CurrentChild exposes the opaque current child generation only if it belongs
// to the exact current parent for the requested isolated target.
func (t *PlayoutCertificationTarget) CurrentChild(ctx context.Context, request playoutcert.ChildFaultRequest) (playoutcert.ChildFaultTarget, error) {
	if t == nil || strings.TrimSpace(request.BaseURL) != t.BaseURL {
		return playoutcert.ChildFaultTarget{}, errors.New("child fault target mismatch")
	}
	if err := ctx.Err(); err != nil {
		return playoutcert.ChildFaultTarget{}, err
	}
	t.parentsMu.Lock()
	defer t.parentsMu.Unlock()
	parent := t.parents[request.ChannelID]
	child := t.children[request.ChannelID]
	if parent == nil || child == nil || parent.process == nil || child.process == nil || parent.faulting || child.faulting ||
		parent.generation != child.parentGeneration || parent.process.ProcessRunID() == "" || parent.process.ProcessRunID() != child.parentRunID || child.target == "" {
		return playoutcert.ChildFaultTarget{}, errors.New("child fault stale or already ended")
	}
	return playoutcert.ChildFaultTarget{ParentGeneration: child.parentGeneration, ChildGeneration: child.generation}, nil
}

// FailChild stops exactly the currently registered encoder child. It rejects
// target, parent, and child generation reuse before touching the owned handle.
func (t *PlayoutCertificationTarget) FailChild(ctx context.Context, request playoutcert.ChildFaultRequest) (playoutcert.ChildFaultReceipt, error) {
	if t == nil || strings.TrimSpace(request.BaseURL) != t.BaseURL {
		return playoutcert.ChildFaultReceipt{}, errors.New("child fault target mismatch")
	}
	if err := ctx.Err(); err != nil {
		return playoutcert.ChildFaultReceipt{}, err
	}
	t.parentsMu.Lock()
	parent := t.parents[request.ChannelID]
	child := t.children[request.ChannelID]
	if parent == nil || child == nil || parent.process == nil || child.process == nil || parent.faulting || child.faulting ||
		request.ParentGeneration == 0 || request.ChildGeneration == 0 || request.ParentGeneration != child.parentGeneration || request.ChildGeneration != child.generation ||
		parent.generation != child.parentGeneration || parent.process.ProcessRunID() == "" || parent.process.ProcessRunID() != child.parentRunID || child.target == "" {
		t.parentsMu.Unlock()
		return playoutcert.ChildFaultReceipt{}, errors.New("child fault stale or already ended")
	}
	child.faulting = true
	process := child.process
	t.parentsMu.Unlock()
	process.Stop()
	_ = process.Wait()
	return playoutcert.ChildFaultReceipt{ChannelID: request.ChannelID, ParentGeneration: request.ParentGeneration, ChildGeneration: request.ChildGeneration, Exited: true}, nil
}

// ProgrammeBoundaryWitness returns the isolated target's causal observation
// seam. It is intentionally not available from ordinary production origins.
func (t *PlayoutCertificationTarget) ProgrammeBoundaryWitness() playoutcert.ProgrammeBoundaryWitness {
	if t == nil {
		return nil
	}
	if t.boundaryRecorder == nil {
		return nil
	}
	return t.boundaryRecorder.Witness()
}

func (t *PlayoutCertificationTarget) Close(ctx context.Context) error {
	if t == nil {
		return nil
	}
	return t.lifecycle.close(ctx, t.stop, func() error {
		var result error
		if t.diagnostics != nil {
			if err := t.diagnostics.Close(context.Background()); result == nil {
				result = err
			}
		}
		if t.store != nil {
			if err := t.store.Close(); result == nil {
				result = err
			}
		}
		if t.root != "" {
			if err := os.RemoveAll(t.root); result == nil {
				result = err
			}
		}
		return result
	})
}

// Shutdown performs the target's one terminal transition but leaves the
// retained projections available to SampleStopped until Close disposes them.
func (t *PlayoutCertificationTarget) Shutdown(ctx context.Context, request playoutcert.ShutdownRequest) (playoutcert.ShutdownReceipt, error) {
	if t == nil || strings.TrimSpace(request.BaseURL) != t.BaseURL {
		return playoutcert.ShutdownReceipt{}, errors.New("shutdown target mismatch")
	}
	return t.lifecycle.shutdown(ctx, t.stop)
}

func (t *PlayoutCertificationTarget) stop() (playoutcert.ShutdownReceipt, error) {
	if t.origin != nil {
		t.origin.Quiesce()
	}
	t.parentsMu.Lock()
	t.stopping = true
	t.parentsMu.Unlock()
	t.parentsWG.Wait()
	t.childrenWG.Wait()
	var err error
	if t.server != nil {
		err = t.server.Shutdown(context.Background())
	} else if t.listener != nil {
		err = t.listener.Close()
	}
	return playoutcert.ShutdownReceipt{Scope: t.scope, ServingStopped: err == nil, ProcessesExited: true}, err
}

func (t *PlayoutCertificationTarget) SampleStopped(ctx context.Context, point string) (playoutcert.ResourceSample, error) {
	if t == nil || t.handler == nil {
		return playoutcert.ResourceSample{}, errors.New("shutdown sample unavailable")
	}
	release, err := t.lifecycle.observe(ctx)
	if err != nil {
		return playoutcert.ResourceSample{}, err
	}
	defer release()
	return playoutcert.SampleResources(ctx, playoutcert.Config{
		BaseURL: t.BaseURL, AdminBearer: t.AdminBearer, DeviceToken: t.DeviceToken,
		Client: &http.Client{Transport: syntheticSamplerTransport{handler: t.handler}}, RequestTimeout: 5 * time.Second,
	}, point)
}

// syntheticLifecycle is the owned seam for terminal work. Caller contexts
// bound only their wait; once started, shutdown and disposal publish their
// single actual result after all retained observations have left.
type syntheticLifecycle struct {
	initOnce sync.Once

	shutdownOnce    sync.Once
	shutdownDone    chan struct{}
	shutdownReceipt playoutcert.ShutdownReceipt
	shutdownErr     error

	closeOnce sync.Once
	closeDone chan struct{}
	closeErr  error

	mu               sync.Mutex
	closing          bool
	disposed         bool
	observations     int
	observationsDone chan struct{}
}

func (l *syntheticLifecycle) init() {
	l.initOnce.Do(func() {
		l.shutdownDone = make(chan struct{})
		l.closeDone = make(chan struct{})
		l.observationsDone = make(chan struct{})
	})
}

func (l *syntheticLifecycle) shutdown(ctx context.Context, operation func() (playoutcert.ShutdownReceipt, error)) (playoutcert.ShutdownReceipt, error) {
	l.init()
	select {
	case <-l.shutdownDone:
		return l.shutdownReceipt, l.shutdownErr
	default:
	}
	if err := ctx.Err(); err != nil {
		return playoutcert.ShutdownReceipt{}, err
	}
	l.shutdownOnce.Do(func() {
		go func() {
			l.shutdownReceipt, l.shutdownErr = operation()
			close(l.shutdownDone)
		}()
	})
	if err := waitSyntheticLifecycle(ctx, l.shutdownDone); err != nil {
		return playoutcert.ShutdownReceipt{}, err
	}
	return l.shutdownReceipt, l.shutdownErr
}

func (l *syntheticLifecycle) close(ctx context.Context, shutdown func() (playoutcert.ShutdownReceipt, error), dispose func() error) error {
	l.init()
	select {
	case <-l.closeDone:
		return l.closeErr
	default:
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	l.closeOnce.Do(func() {
		l.mu.Lock()
		l.closing = true
		if l.observations == 0 {
			close(l.observationsDone)
		}
		l.mu.Unlock()
		go func() {
			_, result := l.shutdown(context.Background(), shutdown)
			<-l.observationsDone
			if err := dispose(); result == nil {
				result = err
			}
			l.mu.Lock()
			l.disposed = true
			l.mu.Unlock()
			l.closeErr = result
			close(l.closeDone)
		}()
	})
	if err := waitSyntheticLifecycle(ctx, l.closeDone); err != nil {
		return err
	}
	return l.closeErr
}

func (l *syntheticLifecycle) observe(ctx context.Context) (func(), error) {
	l.init()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if l.closing {
		if l.disposed {
			return nil, errors.New("shutdown sample unavailable after disposal")
		}
		return nil, errors.New("shutdown sample unavailable during disposal")
	}
	select {
	case <-l.shutdownDone:
		l.observations++
		return l.releaseObservation, nil
	default:
		return nil, errors.New("shutdown sample requested before stop")
	}
}

func (l *syntheticLifecycle) releaseObservation() {
	l.mu.Lock()
	l.observations--
	if l.closing && l.observations == 0 {
		close(l.observationsDone)
	}
	l.mu.Unlock()
}

func waitSyntheticLifecycle(ctx context.Context, done <-chan struct{}) error {
	select {
	case <-done:
		return nil
	default:
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		select {
		case <-done:
			return nil
		default:
			return ctx.Err()
		}
	}
}

type syntheticSamplerTransport struct{ handler http.Handler }

func (t syntheticSamplerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	t.handler.ServeHTTP(recorder, request)
	return recorder.Result(), nil
}

type syntheticPreparedResolver struct {
	specs    map[string][2]prepared.Specification
	schedule syntheticProgrammeSchedule
	now      func() time.Time
}

type syntheticPreparedStatus struct{ count int }

func (s syntheticPreparedStatus) Status() prepared.PlannerStatus {
	return prepared.PlannerStatus{
		Available: true, LastRunAt: time.Now(),
		Readiness: prepared.ReadinessSummary{
			Channels: s.count, ReadyChannels: s.count,
			ScheduledBindings: s.count, ReadyBindings: s.count,
		},
	}
}

func (r syntheticPreparedResolver) ResolvePrepared(_ context.Context, request playout.TuneRequest, at time.Time) (playout.PreparedWindow, bool, error) {
	specs, ok := r.specs[request.ChannelID]
	if !ok {
		return playout.PreparedWindow{}, false, nil
	}
	now := at
	if now.IsZero() {
		now = r.currentTime()
	}
	identity, _, offset := r.schedule.airings(now, request.ChannelID)
	spec := specs[r.schedule.variant(now)]
	return playout.PreparedWindow{Current: playout.PreparedAiring{
		Specification: spec, StartedAt: identity.StartedAt, Offset: offset,
		Identity: identity, DiscontinuitySequence: r.schedule.ordinal(now),
	}}, true, nil
}

func (r syntheticPreparedResolver) currentTime() time.Time {
	if r.now != nil {
		return r.now().UTC()
	}
	return time.Now().UTC()
}

type syntheticProgrammeSchedule struct {
	epoch    time.Time
	duration time.Duration
}

func (s syntheticProgrammeSchedule) ordinal(now time.Time) int64 {
	if now.Before(s.epoch) {
		return 0
	}
	return int64(now.Sub(s.epoch) / s.duration)
}

func (s syntheticProgrammeSchedule) airings(now time.Time, channelID string) (playout.AiringIdentity, playout.AiringIdentity, time.Duration) {
	ordinal := s.ordinal(now)
	started := s.epoch.Add(time.Duration(ordinal) * s.duration)
	makeIdentity := func(index int64, start time.Time) playout.AiringIdentity {
		return playout.AiringIdentity{
			StartedAt: start, EndsAt: start.Add(s.duration), Kind: schedule.SlotProgram,
			ContentID:       fmt.Sprintf("%s-programme-%d", channelID, index%2),
			ScheduleBlockID: fmt.Sprintf("synthetic-block-%d", index),
		}
	}
	return makeIdentity(ordinal, started), makeIdentity(ordinal+1, started.Add(s.duration)), now.Sub(started)
}

type syntheticLiveResolver struct {
	sources  [2]string
	schedule syntheticProgrammeSchedule
	now      func() time.Time
}

func (r syntheticLiveResolver) AiringNow(_ context.Context, channelID string) (playout.Airing, string, error) {
	now := r.currentTime()
	identity, _, offset := r.schedule.airings(now, channelID)
	return playout.Airing{StartedAt: identity.StartedAt, Identity: identity.ContentID, ScheduleBlockID: identity.ScheduleBlockID, Kind: identity.Kind, LibraryItemID: channelID, Title: "Synthetic", Offset: offset, Remaining: identity.EndsAt.Sub(now)}, r.sources[r.schedule.variant(now)], nil
}
func (r syntheticLiveResolver) currentTime() time.Time {
	if r.now != nil {
		return r.now().UTC()
	}
	return time.Now().UTC()
}
func (syntheticLiveResolver) Profile(context.Context) playout.Profile {
	p := playout.DefaultProfile()
	p.Width = 320
	p.Height = 180
	p.VideoBitrate = 600
	p.AudioBitrate = 96
	p.Encoder = playout.EncoderSoftware
	return p
}
func (syntheticLiveResolver) AudioTrackFor(context.Context, string, string, string) int { return 0 }
func (syntheticLiveResolver) Tracks(context.Context, string) (playout.MediaTracks, error) {
	return playout.MediaTracks{}, nil
}
func (syntheticLiveResolver) PlanFor(context.Context, string, playout.EncodePlan) (playout.CopyPlan, playout.MediaFormat) {
	return playout.CopyPlan{}, playout.MediaFormat{}
}
func (syntheticLiveResolver) ChannelCodec(context.Context, string) string { return "h264" }

func syntheticBlockSource(base, device string, preparedSource playout.BlockSource, manager *playout.Manager, recorder *playoutcert.ProgrammeBoundaryRecorder, sourceID uint64) playout.BlockSource {
	var broadcast string
	return func(ctx context.Context, blockRequest playout.BlockRequest) (playout.Block, error) {
		channelID := blockRequest.ChannelID
		plan := blockRequest.Plan
		if preparedSource != nil {
			block, err := preparedSource(ctx, blockRequest)
			if err == nil && block.Content != nil {
				if manager != nil && !manager.AdmitProgram(channelID, plan, false) {
					_ = block.Content.Close()
					return playout.Block{}, playout.ErrAtCapacity
				}
				format, valid := playout.ParseBroadcastFormat(block.Format.String())
				if valid && (broadcast == "" || broadcast == format.String()) {
					broadcast = format.String()
					block.Format = format
					block.Content = recorder.WrapBlock(block.Content, channelID, sourceID, block.Identity)
					return block, nil
				}
				_ = block.Content.Close()
			}
			if ctx.Err() != nil {
				return playout.Block{}, ctx.Err()
			}
		}
		if !blockRequest.AiringAt.IsZero() {
			return playout.Block{}, playout.ErrPreparedUnavailable
		}
		query := url.Values{"token": {device}, "plan": {plan.String()}}
		if broadcast != "" {
			query.Set(api.PlayoutBroadcastFormatQuery, broadcast)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/playout/program/"+url.PathEscape(channelID)+"?"+query.Encode(), nil)
		if err != nil {
			return playout.Block{}, err
		}
		if !blockRequest.TimelineOrigin.IsZero() {
			req.Header.Set(api.PlayoutTimelineOriginHeader, blockRequest.TimelineOrigin.UTC().Format(time.RFC3339Nano))
		}
		if spec, ok := diagnostics.ProcessSpecFromContext(ctx); ok && spec.ParentRunID != "" {
			req.Header.Set(api.PlayoutParentProcessRunHeader, spec.ParentRunID)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return playout.Block{}, err
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return playout.Block{}, fmt.Errorf("program endpoint status %d", resp.StatusCode)
		}
		format, ok := playout.ParseBroadcastFormat(resp.Header.Get(api.PlayoutBroadcastFormatHeader))
		if !ok {
			_ = resp.Body.Close()
			return playout.Block{}, errors.New("program endpoint format missing")
		}
		if broadcast == "" {
			broadcast = format.String()
		} else if broadcast != format.String() {
			_ = resp.Body.Close()
			return playout.Block{}, errors.New("program endpoint format changed")
		}
		identity, ok := api.ParsePlayoutAiringIdentity(resp.Header)
		if !ok {
			_ = resp.Body.Close()
			return playout.Block{}, errors.New("program endpoint identity missing")
		}
		return playout.Block{Content: recorder.WrapBlock(resp.Body, channelID, sourceID, identity), Identity: identity, Format: format}, nil
	}
}

func syntheticPreparedSpecification(channelID string, variant int, rendition prepared.RenditionContract) prepared.Specification {
	return prepared.Specification{SourceFingerprint: fmt.Sprintf("synthetic-%s-programme-%d", channelID, variant), Rendition: rendition}
}

func (s syntheticProgrammeSchedule) variant(now time.Time) int {
	return int(s.ordinal(now) % 2)
}

func generateSyntheticSource(ctx context.Context, ffmpeg, output string, variant int) error {
	colour, frequency := "black", "440"
	if variant == 1 {
		colour, frequency = "white", "880"
	}
	filter := fmt.Sprintf("testsrc2=size=320x180:rate=25:duration=60,drawbox=x=0:y=0:w=64:h=64:color=%s:t=fill", colour)
	cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-f", "lavfi", "-i", filter, "-f", "lavfi", "-i", "sine=frequency="+frequency+":sample_rate=48000:duration=60", "-shortest", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-g", "25", "-c:a", "aac", "-b:a", "96k", output)
	if err := cmd.Run(); err != nil {
		return errors.New("generate deterministic synthetic media")
	}
	return nil
}

func randomCredential() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func copyRegularFile(from, to string) error {
	src, err := os.Open(from)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	dst, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

var _ api.PlayoutResolver = syntheticLiveResolver{}
var _ playout.PreparedResolver = syntheticPreparedResolver{}
