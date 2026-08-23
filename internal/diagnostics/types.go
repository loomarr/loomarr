package diagnostics

import "time"

// Level is the bounded severity set stored and rendered by Diagnostics.
type Level string

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

// Source identifies which Loomarr runtime observed an event.
type Source string

const (
	SourceServer    Source = "server"
	SourceWeb       Source = "web"
	SourceAndroidTV Source = "android_tv"
)

// Event is the small caller-facing interface for one technical observation. The recorder owns
// identifiers, receipt time, normalization, redaction, JSON encoding, and retained-size accounting.
type Event struct {
	OccurredAt        time.Time
	Level             Level
	Source            Source
	Subsystem         string
	Name              string
	Message           string
	RequestID         string
	PlaybackSessionID string
	ChannelID         string
	ScheduleBlockID   string
	JobID             string
	ProcessRunID      string
	ActorID           string
	InstanceID        string
	Attributes        map[string]any
}

// Record is the normalized persistence shape passed across the recorder's narrow Sink seam. A Sink
// must not accept raw caller attributes: redaction is complete before this value exists.
type Record struct {
	ID                string `json:"id"`
	OccurredAt        int64  `json:"occurredAt"`
	ReceivedAt        int64  `json:"receivedAt"`
	Level             Level  `json:"level"`
	Source            Source `json:"source"`
	Subsystem         string `json:"subsystem,omitempty"`
	Event             string `json:"event"`
	Message           string `json:"message,omitempty"`
	RequestID         string `json:"requestId,omitempty"`
	PlaybackSessionID string `json:"playbackSessionId,omitempty"`
	ChannelID         string `json:"channelId,omitempty"`
	ScheduleBlockID   string `json:"scheduleBlockId,omitempty"`
	JobID             string `json:"jobId,omitempty"`
	ProcessRunID      string `json:"processRunId,omitempty"`
	ActorID           string `json:"actorId,omitempty"`
	InstanceID        string `json:"instanceId,omitempty"`
	AttributesJSON    string `json:"attributesJson"`
	SizeBytes         int64  `json:"sizeBytes"`
}

// ProcessStatus is the durable lifecycle state of one external media process.
type ProcessStatus string

const (
	ProcessRunning   ProcessStatus = "running"
	ProcessSucceeded ProcessStatus = "succeeded"
	ProcessFailed    ProcessStatus = "failed"
	ProcessCancelled ProcessStatus = "cancelled"
)

// ProcessRun is the bounded durable metadata for one external media process. Raw output remains an
// opaque diagnostics-owned file reference; callers and store adapters never resolve the path.
type ProcessRun struct {
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
	CommandSummary    string        `json:"commandSummary,omitempty"`
	StartedAt         int64         `json:"startedAt"`
	EndedAt           int64         `json:"endedAt,omitempty"`
	Status            ProcessStatus `json:"status"`
	ExitCode          *int          `json:"exitCode,omitempty"`
	TerminationReason string        `json:"terminationReason,omitempty"`
	FirstError        string        `json:"firstError,omitempty"`
	LastError         string        `json:"lastError,omitempty"`
	OutputRef         string        `json:"-"`
	OutputBytes       int64         `json:"outputBytes"`
	DiscardedLines    int64         `json:"discardedLines"`
	UpdatedAt         int64         `json:"updatedAt"`
	SizeBytes         int64         `json:"sizeBytes"`
}

// PurgeResult reports logical evidence removed by one retention pass.
type PurgeResult struct {
	Events        int
	ProcessRuns   int
	RetainedBytes int64
}
