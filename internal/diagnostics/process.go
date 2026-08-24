package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultProcessQueue    = 512
	defaultOutputQueue     = 256
	defaultOutputPrefix    = 256 << 10
	defaultOutputTail      = 768 << 10
	defaultOutputFlush     = time.Second
	defaultProgressPeriod  = 10 * time.Second
	defaultProcessTimeout  = 5 * time.Second
	maxCommandSummaryBytes = 4096
)

// ProcessSink is the only persistence role ProcessManager needs.
type ProcessSink interface {
	UpsertDiagnosticProcessRun(context.Context, ProcessRun) error
}

// ProcessStore adds the retention operations that require filesystem coordination.
type ProcessStore interface {
	ProcessSink
	ListDiagnosticRetentionCandidates(context.Context, time.Time, int) ([]RetentionCandidate, error)
	DeleteDiagnosticEvent(context.Context, string) (bool, error)
	DeleteDiagnosticProcessRun(context.Context, string) (bool, error)
	DiagnosticRetainedBytes(context.Context) (int64, error)
}

// ProcessOptions controls the diagnostics service's bounded process-local resources.
type ProcessOptions struct {
	OutputDir      string
	InstanceID     string
	QueueCapacity  int
	OutputCapacity int
	PrefixBytes    int
	TailBytes      int
	FlushInterval  time.Duration
	ProgressPeriod time.Duration
	WriteTimeout   time.Duration
	Now            func() time.Time
	Version        func(context.Context, string) string
	OnFailure      func(error)
}

// ProcessSpec is the stable correlation known when an external process starts.
type ProcessSpec struct {
	Purpose         string
	ParentRunID     string
	ChannelID       string
	Target          string
	ScheduleBlockID string
	JobID           string
	Executable      string
	Args            []string
}

type processSpecContextKey struct{}

// WithProcessSpec carries correlation through an internal adapter without widening its function type.
func WithProcessSpec(ctx context.Context, spec ProcessSpec) context.Context {
	return context.WithValue(ctx, processSpecContextKey{}, spec)
}

// ProcessSpecFromContext returns correlation supplied by the process's owning domain.
func ProcessSpecFromContext(ctx context.Context) (ProcessSpec, bool) {
	spec, ok := ctx.Value(processSpecContextKey{}).(ProcessSpec)
	return spec, ok
}

// ProcessResult describes one external process's terminal state.
type ProcessResult struct {
	Err               error
	Cancelled         bool
	TerminationReason string
}

// ProcessProgress is the media-tool-neutral progress shape persisted as sampled events.
type ProcessProgress struct {
	Frame     int64
	Speed     float64
	OutTimeMS int64
}

// ProcessManager owns Process-run metadata and bounded output files. Callers never receive paths.
type ProcessManager struct {
	sink    ProcessStore
	events  *Recorder
	opts    ProcessOptions
	updates chan ProcessRun
	stop    chan struct{}
	done    chan struct{}
	runs    sync.WaitGroup

	admission sync.RWMutex
	closed    bool
	closeOnce sync.Once
	versions  sync.Map
}

type executableVersion struct {
	done  chan struct{}
	value string
}

// NewProcessManager starts the best-effort Process-run recorder.
func NewProcessManager(sink ProcessStore, events *Recorder, opts ProcessOptions) *ProcessManager {
	opts = processDefaults(opts)
	m := &ProcessManager{
		sink: sink, events: events, opts: opts,
		updates: make(chan ProcessRun, opts.QueueCapacity), stop: make(chan struct{}), done: make(chan struct{}),
	}
	if sink == nil {
		close(m.done)
		return m
	}
	if opts.OutputDir != "" {
		if err := os.MkdirAll(opts.OutputDir, 0o750); err != nil {
			m.fail(fmt.Errorf("create process diagnostics directory: %w", err))
			m.opts.OutputDir = ""
		}
	}
	go m.persist()
	return m
}

// Purge removes expired evidence first, then oldest completed evidence until within maxBytes.
// An output file is removed before its guarded Process-run row so SQL can never orphan a file.
func (m *ProcessManager) Purge(ctx context.Context, before time.Time, maxBytes int64) (PurgeResult, error) {
	if m == nil || m.sink == nil {
		return PurgeResult{}, nil
	}
	result := PurgeResult{}
	for {
		candidates, err := m.sink.ListDiagnosticRetentionCandidates(ctx, before, 256)
		if err != nil {
			return result, err
		}
		if len(candidates) == 0 {
			break
		}
		for _, candidate := range candidates {
			removed, err := m.deleteCandidate(ctx, candidate)
			if err != nil {
				return result, err
			}
			if removed {
				result.add(candidate.Kind)
			}
		}
	}
	retained, err := m.sink.DiagnosticRetainedBytes(ctx)
	if err != nil {
		return result, err
	}
	for maxBytes > 0 && retained > maxBytes {
		candidates, listErr := m.sink.ListDiagnosticRetentionCandidates(ctx, time.Time{}, 256)
		if listErr != nil {
			return result, listErr
		}
		if len(candidates) == 0 {
			break
		} // active runs may legitimately hold the install above budget.
		removedAny := false
		for _, candidate := range candidates {
			if retained <= maxBytes {
				break
			}
			removed, deleteErr := m.deleteCandidate(ctx, candidate)
			if deleteErr != nil {
				return result, deleteErr
			}
			if removed {
				removedAny = true
				retained -= candidate.SizeBytes
				result.add(candidate.Kind)
			}
		}
		if !removedAny {
			break
		}
	}
	result.RetainedBytes = max(0, retained)
	return result, nil
}

func (m *ProcessManager) deleteCandidate(ctx context.Context, candidate RetentionCandidate) (bool, error) {
	switch candidate.Kind {
	case EvidenceEvent:
		return m.sink.DeleteDiagnosticEvent(ctx, candidate.ID)
	case EvidenceProcessRun:
		if candidate.OutputRef != "" {
			if m.opts.OutputDir == "" {
				return false, errors.New("diagnostic output root is unavailable")
			}
			if filepath.Base(candidate.OutputRef) != candidate.OutputRef || strings.ContainsAny(candidate.OutputRef, `/\\`) {
				return false, fmt.Errorf("refuse unsafe diagnostic output reference %q", candidate.OutputRef)
			}
			if err := os.Remove(filepath.Join(m.opts.OutputDir, candidate.OutputRef)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return false, fmt.Errorf("remove diagnostic process output %s: %w", candidate.ID, err)
			}
		}
		return m.sink.DeleteDiagnosticProcessRun(ctx, candidate.ID)
	default:
		return false, fmt.Errorf("unknown diagnostic retention candidate kind %q", candidate.Kind)
	}
}

func (r *PurgeResult) add(kind EvidenceKind) {
	if kind == EvidenceProcessRun {
		r.ProcessRuns++
	} else {
		r.Events++
	}
}

// Begin admits one Process run without waiting for disk or database I/O.
func (m *ProcessManager) Begin(spec ProcessSpec) *ProcessHandle {
	if m == nil || m.sink == nil {
		return nil
	}
	m.admission.RLock()
	defer m.admission.RUnlock()
	if m.closed {
		return nil
	}
	now := m.opts.Now()
	id := newID(now)
	ref := ""
	if m.opts.OutputDir != "" {
		ref = id + ".log"
	}
	h := &ProcessHandle{
		manager:    m,
		executable: spec.Executable,
		run: ProcessRun{
			ID: id, Purpose: identifier(spec.Purpose), ParentRunID: identifier(spec.ParentRunID),
			InstanceID: identifier(m.opts.InstanceID), ChannelID: identifier(spec.ChannelID),
			Target: sanitizeString(spec.Target, maxNameBytes), ScheduleBlockID: identifier(spec.ScheduleBlockID),
			JobID: identifier(spec.JobID), Executable: filepath.Base(spec.Executable),
			CommandSummary: commandSummary(spec.Executable, spec.Args), StartedAt: now.UnixMilli(),
			Status: ProcessRunning, OutputRef: ref, UpdatedAt: now.UnixMilli(),
		},
		lines: make(chan string, m.opts.OutputCapacity),
	}
	h.run.SizeBytes = processRetainedSize(h.run)
	m.enqueue(h.run)
	m.runs.Add(1)
	go h.capture()
	return h
}

func (m *ProcessManager) enqueue(run ProcessRun) {
	select {
	case m.updates <- run:
	default:
		m.fail(errors.New("diagnostic process metadata queue is full"))
	}
}

func (m *ProcessManager) persist() {
	defer close(m.done)
	write := func(run ProcessRun) {
		ctx, cancel := context.WithTimeout(context.Background(), m.opts.WriteTimeout)
		err := m.sink.UpsertDiagnosticProcessRun(ctx, run)
		cancel()
		if err != nil {
			m.fail(err)
		}
	}
	for {
		select {
		case run := <-m.updates:
			write(run)
		case <-m.stop:
			for {
				select {
				case run := <-m.updates:
					write(run)
				default:
					return
				}
			}
		}
	}
}

// Close drains finished-run workers and accepted metadata updates within ctx.
func (m *ProcessManager) Close(ctx context.Context) error {
	if m == nil || m.sink == nil {
		return nil
	}
	m.closeOnce.Do(func() {
		m.admission.Lock()
		m.closed = true
		m.admission.Unlock()
		go func() { m.runs.Wait(); close(m.stop) }()
	})
	select {
	case <-m.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *ProcessManager) fail(err error) {
	if m != nil && m.opts.OnFailure != nil {
		m.opts.OnFailure(err)
	}
}

// ProcessHandle is the non-blocking producer interface for one external process.
type ProcessHandle struct {
	manager    *ProcessManager
	run        ProcessRun
	executable string
	lines      chan string

	mu             sync.RWMutex
	finished       bool
	result         ProcessResult
	firstOutput    string
	lastOutput     string
	discardedQueue atomic.Int64
	progressAt     time.Time
	progressSpeed  float64
}

// ID is the correlation identity children and Diagnostic events carry.
func (h *ProcessHandle) ID() string {
	if h == nil {
		return ""
	}
	return h.run.ID
}

// RecordOutput never waits for file I/O. Saturation drops the line and increments metadata.
func (h *ProcessHandle) RecordOutput(line string) {
	if h == nil {
		return
	}
	line = sanitizeProcessOutput(line)
	if line == "" {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.finished {
		h.discardedQueue.Add(1)
		return
	}
	if h.firstOutput == "" {
		h.firstOutput = line
	}
	h.lastOutput = line
	line = "[" + h.manager.opts.Now().UTC().Format(time.RFC3339Nano) + "] " + line
	select {
	case h.lines <- line:
	default:
		h.discardedQueue.Add(1)
	}
}

// ObserveProgress downsamples progress independently of the existing live-health callback.
func (h *ProcessHandle) ObserveProgress(sample ProcessProgress) {
	if h == nil || h.manager.events == nil {
		return
	}
	now := h.manager.opts.Now()
	h.mu.Lock()
	emit := h.progressAt.IsZero() || now.Sub(h.progressAt) >= h.manager.opts.ProgressPeriod ||
		abs(sample.Speed-h.progressSpeed) >= 0.25
	if emit {
		h.progressAt, h.progressSpeed = now, sample.Speed
	}
	h.mu.Unlock()
	if !emit {
		return
	}
	h.manager.events.Record(context.Background(), Event{
		OccurredAt: now, Level: LevelInfo, Source: SourceServer, Subsystem: "process",
		Name: "process.progress", ProcessRunID: h.run.ID, ChannelID: h.run.ChannelID,
		ScheduleBlockID: h.run.ScheduleBlockID, JobID: h.run.JobID, InstanceID: h.run.InstanceID,
		Attributes: map[string]any{"frame": sample.Frame, "speed": sample.Speed, "out_time_ms": sample.OutTimeMS},
	})
}

// Finish records the terminal result and returns immediately; the worker drains accepted output.
func (h *ProcessHandle) Finish(result ProcessResult) {
	if h == nil {
		return
	}
	h.mu.Lock()
	if h.finished {
		h.mu.Unlock()
		return
	}
	h.finished = true
	h.result = result
	close(h.lines)
	h.mu.Unlock()
}

func (h *ProcessHandle) capture() {
	defer h.manager.runs.Done()
	if version := h.manager.executableVersion(h.executable); version != "" {
		h.run.ExecutableVersion = sanitizeString(version, maxNameBytes)
		h.snapshotMetadata(0)
	}
	ticker := time.NewTicker(h.manager.opts.FlushInterval)
	defer ticker.Stop()
	var prefix [][]byte
	var tail [][]byte
	prefixBytes, tailBytes := 0, 0
	discardedRetention := int64(0)
	flush := func() {
		if h.run.OutputRef == "" {
			return
		}
		path := filepath.Join(h.manager.opts.OutputDir, h.run.OutputRef)
		tmp := path + ".tmp"
		file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
		if err != nil {
			h.manager.fail(err)
			return
		}
		for _, line := range prefix {
			_, err = file.Write(line)
			if err != nil {
				break
			}
		}
		queueDrops := h.discardedQueue.Load()
		if err == nil && (discardedRetention > 0 || queueDrops > 0) {
			_, err = fmt.Fprintf(file,
				"\n--- discarded diagnostic output: %d lines evicted by the rolling cap, %d lines dropped before disk; rolling tail follows ---\n",
				discardedRetention, queueDrops)
		}
		if err == nil {
			for _, line := range tail {
				_, err = file.Write(line)
				if err != nil {
					break
				}
			}
		}
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			// Windows cannot atomically replace an existing destination with os.Rename. Process
			// output is diagnostic evidence rather than a correctness-bearing publication, so a
			// brief replacement gap is preferable to silently losing every flush after the first.
			if runtime.GOOS == "windows" {
				_ = os.Remove(path)
			}
			err = os.Rename(tmp, path)
		}
		if err != nil {
			_ = os.Remove(tmp)
			h.manager.fail(err)
			return
		}
		if info, statErr := os.Stat(path); statErr == nil {
			h.run.OutputBytes = info.Size()
		}
	}
	add := func(text string) {
		line := append([]byte(text), '\n')
		if prefixBytes+len(line) <= h.manager.opts.PrefixBytes {
			prefix = append(prefix, line)
			prefixBytes += len(line)
			return
		}
		if len(line) > h.manager.opts.TailBytes {
			line = append([]byte(nil), line[len(line)-h.manager.opts.TailBytes:]...)
			discardedRetention++
		}
		tail = append(tail, line)
		tailBytes += len(line)
		for tailBytes > h.manager.opts.TailBytes && len(tail) > 0 {
			tailBytes -= len(tail[0])
			tail = tail[1:]
			discardedRetention++
		}
	}
	for {
		select {
		case line, ok := <-h.lines:
			if !ok {
				flush()
				h.finishMetadata(discardedRetention)
				return
			}
			add(line)
		case <-ticker.C:
			flush()
			h.snapshotMetadata(discardedRetention)
		}
	}
}

func (m *ProcessManager) executableVersion(executable string) string {
	if executable == "" {
		return ""
	}
	entry := &executableVersion{done: make(chan struct{})}
	actual, loaded := m.versions.LoadOrStore(executable, entry)
	cached := actual.(*executableVersion)
	if !loaded {
		cached.value = m.opts.Version(context.Background(), executable)
		close(cached.done)
	}
	<-cached.done
	return cached.value
}

func (h *ProcessHandle) snapshotMetadata(discardedRetention int64) {
	h.run.DiscardedLines = discardedRetention + h.discardedQueue.Load()
	h.run.UpdatedAt = h.manager.opts.Now().UnixMilli()
	h.run.SizeBytes = processRetainedSize(h.run) + h.run.OutputBytes
	h.manager.enqueue(h.run)
}

func (h *ProcessHandle) finishMetadata(discardedRetention int64) {
	now := h.manager.opts.Now()
	h.mu.RLock()
	result := h.result
	firstOutput, lastOutput := h.firstOutput, h.lastOutput
	h.mu.RUnlock()
	h.run.EndedAt = now.UnixMilli()
	h.run.UpdatedAt = h.run.EndedAt
	h.run.DiscardedLines = discardedRetention + h.discardedQueue.Load()
	h.run.TerminationReason = sanitizeString(result.TerminationReason, maxMessageBytes)
	h.run.FirstError = firstOutput
	h.run.LastError = lastOutput
	if result.Cancelled {
		h.run.Status = ProcessCancelled
	} else if result.Err != nil {
		h.run.Status = ProcessFailed
	} else {
		h.run.Status = ProcessSucceeded
	}
	if result.Err != nil {
		errText := sanitizeProcessOutput(result.Err.Error())
		if h.run.FirstError == "" {
			h.run.FirstError = errText
		}
		h.run.LastError = errText
		var exitErr *exec.ExitError
		if errors.As(result.Err, &exitErr) {
			code := exitErr.ExitCode()
			h.run.ExitCode = &code
		}
	}
	h.run.SizeBytes = processRetainedSize(h.run) + h.run.OutputBytes
	h.manager.enqueue(h.run)
}

func processDefaults(opts ProcessOptions) ProcessOptions {
	if opts.QueueCapacity <= 0 {
		opts.QueueCapacity = defaultProcessQueue
	}
	if opts.OutputCapacity <= 0 {
		opts.OutputCapacity = defaultOutputQueue
	}
	if opts.PrefixBytes <= 0 {
		opts.PrefixBytes = defaultOutputPrefix
	}
	if opts.TailBytes <= 0 {
		opts.TailBytes = defaultOutputTail
	}
	if opts.FlushInterval <= 0 {
		opts.FlushInterval = defaultOutputFlush
	}
	if opts.ProgressPeriod <= 0 {
		opts.ProgressPeriod = defaultProgressPeriod
	}
	if opts.WriteTimeout <= 0 {
		opts.WriteTimeout = defaultProcessTimeout
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Version == nil {
		opts.Version = detectExecutableVersion
	}
	return opts
}

func detectExecutableVersion(ctx context.Context, executable string) string {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, executable, "-version").Output() //nolint:gosec // configured executable
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(out), "\n")
	return truncate(strings.TrimSpace(line), maxNameBytes)
}

var (
	urlCredentialPattern  = regexp.MustCompile(`(?i)(https?://)[^/@\s]+@`)
	embeddedURLPattern    = regexp.MustCompile(`(?i)https?://[^\s'"<>]+`)
	sensitiveValuePattern = regexp.MustCompile(`(?i)(authorization|cookie|token|api[_-]?key|password|secret|signature)([=: ]+)([^\s,;]+)`)
	bearerValuePattern    = regexp.MustCompile(`(?i)(bearer\s+)([^\s,;]+)`)
	absPathPattern        = regexp.MustCompile(`(^|[\s='"])(/[A-Za-z0-9._~!$&()+,;=:@%/-]+)`)
	windowsPathPattern    = regexp.MustCompile(`(?i)(^|[\s='"])([a-z]:\\[^\s'"]+)`)
)

func sanitizeProcessOutput(value string) string {
	value = strings.TrimSpace(value)
	value = urlCredentialPattern.ReplaceAllString(value, `${1}[redacted]@`)
	value = bearerValuePattern.ReplaceAllString(value, `${1}[redacted]`)
	value = sensitiveValuePattern.ReplaceAllString(value, `${1}${2}[redacted]`)
	value = embeddedURLPattern.ReplaceAllStringFunc(value, func(raw string) string {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" {
			return "[url]"
		}
		return parsed.Scheme + "://" + parsed.Host + "/[redacted]"
	})
	value = absPathPattern.ReplaceAllString(value, `${1}[path]`)
	value = windowsPathPattern.ReplaceAllString(value, `${1}[path]`)
	return truncate(value, maxMessageBytes)
}

func commandSummary(executable string, args []string) string {
	out := []string{filepath.Base(executable)}
	sensitiveNext := false
	inputNext := false
	for _, arg := range args {
		if sensitiveNext {
			out = append(out, "[redacted]")
			sensitiveNext = false
			continue
		}
		if inputNext {
			out = append(out, "[input]")
			inputNext = false
			continue
		}
		lower := strings.ToLower(arg)
		out = append(out, redactCommandArg(arg))
		if lower == "-i" {
			inputNext = true
		}
		if lower == "-headers" || lower == "-http_proxy" || lower == "-metadata" {
			sensitiveNext = true
		}
	}
	return truncate(strings.Join(out, " "), maxCommandSummaryBytes)
}

func redactCommandArg(arg string) string {
	if arg == "pipe:0" || arg == "pipe:1" || strings.HasPrefix(arg, "pipe:") {
		return arg
	}
	if filepath.IsAbs(arg) || windowsPathPattern.MatchString(" "+arg) {
		return "[path]"
	}
	if strings.Contains(arg, "://") {
		parts := strings.SplitN(arg, "://", 2)
		host := strings.SplitN(strings.SplitN(parts[1], "/", 2)[0], "@", 2)
		return parts[0] + "://" + host[len(host)-1] + "/[redacted]"
	}
	return sanitizeProcessOutput(arg)
}

func processRetainedSize(run ProcessRun) int64 {
	exit := 0
	if run.ExitCode != nil {
		exit = len(strconv.Itoa(*run.ExitCode))
	}
	return int64(len(run.ID) + len(run.Purpose) + len(run.ParentRunID) + len(run.InstanceID) + len(run.ChannelID) +
		len(run.Target) + len(run.ScheduleBlockID) + len(run.JobID) + len(run.Executable) + len(run.ExecutableVersion) +
		len(run.CommandSummary) + len(run.Status) + len(run.TerminationReason) + len(run.FirstError) + len(run.LastError) +
		len(run.OutputRef) + exit + 64)
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}
