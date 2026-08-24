package diagnostics

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxProcessOutputBytes = defaultOutputPrefix + defaultOutputTail + 64<<10

var (
	ErrInvalidProcessQuery      = errors.New("invalid diagnostic process query")
	ErrProcessNotFound          = errors.New("diagnostic process run not found")
	ErrProcessOutputUnavailable = errors.New("diagnostic process output unavailable")
)

// ProcessQuery is the bounded caller-facing search for active and recent external media work.
type ProcessQuery struct {
	From      int64
	To        int64
	Limit     int
	Cursor    string
	Status    ProcessStatus
	Purpose   string
	ChannelID string
	JobID     string
}

// ProcessStoreQuery is the fully validated persistence query. Results are newest-first.
type ProcessStoreQuery struct {
	From            int64
	To              int64
	Limit           int
	CursorStartedAt int64
	CursorID        string
	Status          ProcessStatus
	Purpose         string
	ChannelID       string
	JobID           string
}

// ProcessRunView deliberately omits the diagnostics-owned output reference, retained-size
// accounting, and command summary. The latter remains redacted but is held for Support bundles,
// not exposed as routine UI detail.
type ProcessRunView struct {
	ID                string        `json:"id"`
	Purpose           string        `json:"purpose"`
	ParentRunID       string        `json:"parentRunId,omitempty"`
	InstanceID        string        `json:"instanceId,omitempty"`
	ChannelID         string        `json:"channelId,omitempty"`
	Target            string        `json:"target,omitempty"`
	ScheduleBlockID   string        `json:"scheduleBlockId,omitempty"`
	JobID             string        `json:"jobId,omitempty"`
	Executable        string        `json:"executable,omitempty"`
	ExecutableVersion string        `json:"executableVersion,omitempty"`
	StartedAt         int64         `json:"startedAt"`
	EndedAt           int64         `json:"endedAt,omitempty"`
	Status            ProcessStatus `json:"status" enum:"running,succeeded,failed,cancelled"`
	ExitCode          *int          `json:"exitCode,omitempty"`
	TerminationReason string        `json:"terminationReason,omitempty"`
	FirstError        string        `json:"firstError,omitempty"`
	LastError         string        `json:"lastError,omitempty"`
	OutputBytes       int64         `json:"outputBytes"`
	DiscardedLines    int64         `json:"discardedLines"`
	UpdatedAt         int64         `json:"updatedAt"`
}

type ProcessPage struct {
	Items      []ProcessRunView `json:"items"`
	NextCursor string           `json:"nextCursor,omitempty"`
}

type ProcessProgressView struct {
	OccurredAt int64   `json:"occurredAt"`
	Frame      int64   `json:"frame"`
	Speed      float64 `json:"speed"`
	OutTimeMS  int64   `json:"outTimeMs"`
}

type ProcessDetail struct {
	Run               ProcessRunView        `json:"run"`
	Progress          []ProcessProgressView `json:"progress"`
	ProgressTruncated bool                  `json:"progressTruncated"`
}

type ProcessOutput struct {
	Content        []byte
	DiscardedLines int64
	Truncated      bool
}

type ProcessReadStore interface {
	QueryDiagnosticProcessRuns(context.Context, ProcessStoreQuery) ([]ProcessRun, error)
	FindDiagnosticProcessRun(context.Context, string) (ProcessRun, bool, error)
	QueryDiagnosticEvents(context.Context, EventStoreQuery) ([]Record, error)
}

type ProcessReadOptions struct {
	OutputDir string
	Now       func() time.Time
}

// ProcessLog is the only read seam for Process-run metadata, progress, and output. It owns query
// bounds, public projection, opaque output references, and filesystem containment.
type ProcessLog struct {
	store     ProcessReadStore
	outputDir string
	now       func() time.Time
}

func NewProcessLog(store ProcessReadStore, opts ProcessReadOptions) *ProcessLog {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &ProcessLog{store: store, outputDir: opts.OutputDir, now: opts.Now}
}

func (l *ProcessLog) Query(ctx context.Context, query ProcessQuery) (ProcessPage, error) {
	storeQuery, publicLimit, err := l.validate(query)
	if err != nil {
		return ProcessPage{}, err
	}
	runs, err := l.store.QueryDiagnosticProcessRuns(ctx, storeQuery)
	if err != nil {
		return ProcessPage{}, fmt.Errorf("query diagnostic process runs: %w", err)
	}
	page := ProcessPage{Items: make([]ProcessRunView, 0, min(len(runs), publicLimit))}
	visible := runs
	if len(visible) > publicLimit {
		visible = visible[:publicLimit]
		last := visible[len(visible)-1]
		page.NextCursor = encodeProcessCursor(processCursor{StartedAt: last.StartedAt, ID: last.ID})
	}
	for _, run := range visible {
		page.Items = append(page.Items, processView(run))
	}
	return page, nil
}

func (l *ProcessLog) Get(ctx context.Context, id string) (ProcessDetail, error) {
	run, err := l.find(ctx, id)
	if err != nil {
		return ProcessDetail{}, err
	}
	to := run.EndedAt
	if to == 0 {
		to = l.now().UnixMilli()
	}
	from := run.StartedAt
	if to <= from {
		to = from + 1
	}
	truncated := false
	if to-from > maxEventWindow.Milliseconds() {
		from = to - maxEventWindow.Milliseconds()
		truncated = true
	}
	records, err := l.store.QueryDiagnosticEvents(ctx, EventStoreQuery{
		From: from, To: to, Limit: maxEventLimit + 1, Event: "process.progress", ProcessRunID: run.ID,
	})
	if err != nil {
		return ProcessDetail{}, fmt.Errorf("query diagnostic process progress: %w", err)
	}
	if len(records) > maxEventLimit {
		records = records[:maxEventLimit]
		truncated = true
	}
	progress := make([]ProcessProgressView, 0, len(records))
	for _, record := range records {
		var attributes struct {
			Frame     int64   `json:"frame"`
			Speed     float64 `json:"speed"`
			OutTimeMS int64   `json:"out_time_ms"`
		}
		if err := json.Unmarshal([]byte(record.AttributesJSON), &attributes); err != nil {
			continue
		}
		progress = append(progress, ProcessProgressView{
			OccurredAt: record.OccurredAt, Frame: attributes.Frame, Speed: attributes.Speed, OutTimeMS: attributes.OutTimeMS,
		})
	}
	return ProcessDetail{Run: processView(run), Progress: progress, ProgressTruncated: truncated}, nil
}

func (l *ProcessLog) Output(ctx context.Context, id string) (ProcessOutput, error) {
	run, err := l.find(ctx, id)
	if err != nil {
		return ProcessOutput{}, err
	}
	if l.outputDir == "" || run.OutputRef == "" || filepath.Base(run.OutputRef) != run.OutputRef || strings.ContainsAny(run.OutputRef, `/\\`) {
		return ProcessOutput{}, ErrProcessOutputUnavailable
	}
	path := filepath.Join(l.outputDir, run.OutputRef)
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ProcessOutput{}, ErrProcessOutputUnavailable
		}
		return ProcessOutput{}, fmt.Errorf("read diagnostic process output: %w", err)
	}
	if len(content) > maxProcessOutputBytes {
		return ProcessOutput{}, fmt.Errorf("%w: retained output exceeds safety bound", ErrProcessOutputUnavailable)
	}
	return ProcessOutput{Content: content, DiscardedLines: run.DiscardedLines, Truncated: run.DiscardedLines > 0}, nil
}

func (l *ProcessLog) find(ctx context.Context, id string) (ProcessRun, error) {
	if l == nil || l.store == nil {
		return ProcessRun{}, errors.New("diagnostic process reader unavailable")
	}
	id = strings.TrimSpace(id)
	if id == "" || len(id) > maxFilterBytes {
		return ProcessRun{}, ErrProcessNotFound
	}
	run, found, err := l.store.FindDiagnosticProcessRun(ctx, id)
	if err != nil {
		return ProcessRun{}, fmt.Errorf("find diagnostic process run: %w", err)
	}
	if !found {
		return ProcessRun{}, ErrProcessNotFound
	}
	return run, nil
}

func (l *ProcessLog) validate(query ProcessQuery) (ProcessStoreQuery, int, error) {
	if l == nil || l.store == nil {
		return ProcessStoreQuery{}, 0, errors.New("diagnostic process reader unavailable")
	}
	from, to := query.From, query.To
	now := l.now().UnixMilli()
	if from == 0 && to == 0 {
		to, from = now, now-defaultEventWindow.Milliseconds()
	} else if from == 0 {
		from = to - defaultEventWindow.Milliseconds()
	} else if to == 0 {
		to = min(now, from+defaultEventWindow.Milliseconds())
	}
	if from < 0 || to <= from || to-from > maxEventWindow.Milliseconds() {
		return ProcessStoreQuery{}, 0, invalidProcessQuery("from and to must define a window of at most 24 hours")
	}
	limit := query.Limit
	if limit == 0 {
		limit = defaultEventLimit
	}
	if limit < 1 || limit > maxEventLimit {
		return ProcessStoreQuery{}, 0, invalidProcessQuery("limit must be between 1 and 200")
	}
	if query.Status != "" {
		switch query.Status {
		case ProcessRunning, ProcessSucceeded, ProcessFailed, ProcessCancelled:
		default:
			return ProcessStoreQuery{}, 0, invalidProcessQuery("unknown status")
		}
	}
	for name, value := range map[string]string{"purpose": query.Purpose, "channelId": query.ChannelID, "jobId": query.JobID} {
		if len(value) > maxFilterBytes {
			return ProcessStoreQuery{}, 0, invalidProcessQuery(name + " cannot exceed 128 bytes")
		}
	}
	validated := ProcessStoreQuery{
		From: from, To: to, Limit: limit + 1, Status: query.Status,
		Purpose: strings.TrimSpace(query.Purpose), ChannelID: strings.TrimSpace(query.ChannelID), JobID: strings.TrimSpace(query.JobID),
	}
	if query.Cursor != "" {
		cursor, err := decodeProcessCursor(query.Cursor)
		if err != nil || cursor.StartedAt < from || cursor.StartedAt > to || cursor.ID == "" || len(cursor.ID) > maxFilterBytes {
			return ProcessStoreQuery{}, 0, invalidProcessQuery("cursor is invalid for this time window")
		}
		validated.CursorStartedAt, validated.CursorID = cursor.StartedAt, cursor.ID
	}
	return validated, limit, nil
}

func processView(run ProcessRun) ProcessRunView {
	return ProcessRunView{
		ID: run.ID, Purpose: run.Purpose, ParentRunID: run.ParentRunID, InstanceID: run.InstanceID,
		ChannelID: run.ChannelID, Target: run.Target, ScheduleBlockID: run.ScheduleBlockID, JobID: run.JobID,
		Executable: run.Executable, ExecutableVersion: run.ExecutableVersion, StartedAt: run.StartedAt,
		EndedAt: run.EndedAt, Status: run.Status, ExitCode: run.ExitCode, TerminationReason: run.TerminationReason,
		FirstError: run.FirstError, LastError: run.LastError, OutputBytes: run.OutputBytes,
		DiscardedLines: run.DiscardedLines, UpdatedAt: run.UpdatedAt,
	}
}

type processCursor struct {
	StartedAt int64  `json:"startedAt"`
	ID        string `json:"id"`
}

func encodeProcessCursor(cursor processCursor) string {
	encoded, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func decodeProcessCursor(value string) (processCursor, error) {
	if len(value) > maxCursorBytes {
		return processCursor{}, ErrInvalidProcessQuery
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return processCursor{}, err
	}
	var cursor processCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		return processCursor{}, err
	}
	return cursor, nil
}

func invalidProcessQuery(detail string) error {
	return fmt.Errorf("%w: %s", ErrInvalidProcessQuery, detail)
}
