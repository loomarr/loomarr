package diagnostics

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	defaultEventWindow = time.Hour
	maxEventWindow     = 24 * time.Hour
	defaultEventLimit  = 100
	maxEventLimit      = 200
	maxCursorBytes     = 512
	maxFilterBytes     = 128
	maxTextBytes       = 256
)

// ErrInvalidEventQuery identifies a caller-controlled filter error. Adapters may safely expose the
// accompanying message as a 400 detail; storage and decoding failures remain separate errors.
var ErrInvalidEventQuery = errors.New("invalid diagnostic event query")

// EventQuery is the small caller-facing read interface. Zero times select the default one-hour
// window; the module validates and resolves every bound before the store adapter sees it.
type EventQuery struct {
	From              int64
	To                int64
	Limit             int
	Cursor            string
	Level             Level
	Source            Source
	Subsystem         string
	RequestID         string
	PlaybackSessionID string
	ChannelID         string
	ScheduleBlockID   string
	JobID             string
	ProcessRunID      string
	InstanceID        string
	Text              string
}

// EventStoreQuery is the fully validated persistence shape. Store adapters must apply every field
// conjunctively and return newest-first records, capped at Limit.
type EventStoreQuery struct {
	From              int64
	To                int64
	Limit             int
	CursorOccurredAt  int64
	CursorID          string
	Level             Level
	Source            Source
	Subsystem         string
	RequestID         string
	PlaybackSessionID string
	ChannelID         string
	ScheduleBlockID   string
	JobID             string
	ProcessRunID      string
	InstanceID        string
	Text              string
}

// EventView is the stable JSON/NDJSON projection. Persistence's encoded attribute object and
// retained-size accounting remain implementation details.
type EventView struct {
	ID                string         `json:"id"`
	OccurredAt        int64          `json:"occurredAt"`
	ReceivedAt        int64          `json:"receivedAt"`
	Level             Level          `json:"level" enum:"debug,info,warn,error"`
	Source            Source         `json:"source" enum:"server,web,android_tv"`
	Subsystem         string         `json:"subsystem,omitempty"`
	Event             string         `json:"event"`
	Message           string         `json:"message,omitempty"`
	RequestID         string         `json:"requestId,omitempty"`
	PlaybackSessionID string         `json:"playbackSessionId,omitempty"`
	ChannelID         string         `json:"channelId,omitempty"`
	ScheduleBlockID   string         `json:"scheduleBlockId,omitempty"`
	JobID             string         `json:"jobId,omitempty"`
	ProcessRunID      string         `json:"processRunId,omitempty"`
	ActorID           string         `json:"actorId,omitempty"`
	InstanceID        string         `json:"instanceId,omitempty"`
	Attributes        map[string]any `json:"attributes"`
}

// EventPage is the shared typed truth returned to UI and agent callers. NextCursor is absent at the
// end of the result set.
type EventPage struct {
	Items      []EventView `json:"items"`
	NextCursor string      `json:"nextCursor,omitempty"`
}

// EventReader is the persistence role required by EventLog. The SQL store and in-memory tests are
// the two adapters at this seam.
type EventReader interface {
	QueryDiagnosticEvents(context.Context, EventStoreQuery) ([]Record, error)
}

// EventLog owns bounded filter validation, opaque pagination, and public projection.
type EventLog struct {
	reader EventReader
	now    func() time.Time
}

func NewEventLog(reader EventReader, now func() time.Time) *EventLog {
	if now == nil {
		now = time.Now
	}
	return &EventLog{reader: reader, now: now}
}

// Query resolves one bounded page. It requests one sentinel row beyond the public page size so
// cursor creation never requires a count or a second store read.
func (l *EventLog) Query(ctx context.Context, query EventQuery) (EventPage, error) {
	if l == nil || l.reader == nil {
		return EventPage{}, errors.New("diagnostic event reader unavailable")
	}
	storeQuery, publicLimit, err := l.validate(query)
	if err != nil {
		return EventPage{}, err
	}
	records, err := l.reader.QueryDiagnosticEvents(ctx, storeQuery)
	if err != nil {
		return EventPage{}, fmt.Errorf("query diagnostic events: %w", err)
	}
	page := EventPage{Items: make([]EventView, 0, min(len(records), publicLimit))}
	visible := records
	if len(visible) > publicLimit {
		visible = visible[:publicLimit]
		last := visible[len(visible)-1]
		page.NextCursor = encodeEventCursor(eventCursor{OccurredAt: last.OccurredAt, ID: last.ID})
	}
	for _, record := range visible {
		view, err := eventView(record)
		if err != nil {
			return EventPage{}, err
		}
		page.Items = append(page.Items, view)
	}
	return page, nil
}

// WriteNDJSON writes exactly the page's EventView records, one JSON object per line. Callers obtain
// the page through Query first, which keeps content negotiation from creating a second truth path.
func WriteNDJSON(w io.Writer, page EventPage) error {
	encoder := json.NewEncoder(w)
	for _, event := range page.Items {
		if err := encoder.Encode(event); err != nil {
			return fmt.Errorf("write diagnostic event NDJSON: %w", err)
		}
	}
	return nil
}

func (l *EventLog) validate(query EventQuery) (EventStoreQuery, int, error) {
	now := l.now().UnixMilli()
	from, to := query.From, query.To
	switch {
	case from == 0 && to == 0:
		to = now
		from = to - defaultEventWindow.Milliseconds()
	case from == 0:
		from = to - defaultEventWindow.Milliseconds()
	case to == 0:
		if from > int64(^uint64(0)>>1)-defaultEventWindow.Milliseconds() {
			return EventStoreQuery{}, 0, invalidEventQuery("from is out of range")
		}
		to = from + defaultEventWindow.Milliseconds()
	}
	if from < 0 || to <= from {
		return EventStoreQuery{}, 0, invalidEventQuery("from and to must define an increasing time window")
	}
	if to-from > maxEventWindow.Milliseconds() {
		return EventStoreQuery{}, 0, invalidEventQuery("time window cannot exceed 24 hours")
	}
	limit := query.Limit
	if limit == 0 {
		limit = defaultEventLimit
	}
	if limit < 1 || limit > maxEventLimit {
		return EventStoreQuery{}, 0, invalidEventQuery("limit must be between 1 and 200")
	}
	if query.Level != "" {
		switch query.Level {
		case LevelDebug, LevelInfo, LevelWarn, LevelError:
		default:
			return EventStoreQuery{}, 0, invalidEventQuery("unknown level")
		}
	}
	if query.Source != "" {
		switch query.Source {
		case SourceServer, SourceWeb, SourceAndroidTV:
		default:
			return EventStoreQuery{}, 0, invalidEventQuery("unknown source")
		}
	}
	for name, value := range map[string]string{
		"subsystem": query.Subsystem, "requestId": query.RequestID,
		"playbackSessionId": query.PlaybackSessionID, "channelId": query.ChannelID,
		"scheduleBlockId": query.ScheduleBlockID, "jobId": query.JobID,
		"processRunId": query.ProcessRunID, "instanceId": query.InstanceID,
	} {
		if len(value) > maxFilterBytes {
			return EventStoreQuery{}, 0, invalidEventQuery(name + " cannot exceed 128 bytes")
		}
	}
	if len(query.Text) > maxTextBytes {
		return EventStoreQuery{}, 0, invalidEventQuery("text cannot exceed 256 bytes")
	}

	storeQuery := EventStoreQuery{
		From: from, To: to, Limit: limit + 1,
		Level: query.Level, Source: query.Source, Subsystem: strings.TrimSpace(query.Subsystem),
		RequestID: strings.TrimSpace(query.RequestID), PlaybackSessionID: strings.TrimSpace(query.PlaybackSessionID),
		ChannelID: strings.TrimSpace(query.ChannelID), ScheduleBlockID: strings.TrimSpace(query.ScheduleBlockID),
		JobID: strings.TrimSpace(query.JobID), ProcessRunID: strings.TrimSpace(query.ProcessRunID),
		InstanceID: strings.TrimSpace(query.InstanceID), Text: strings.TrimSpace(query.Text),
	}
	if query.Cursor != "" {
		cursor, err := decodeEventCursor(query.Cursor)
		if err != nil || cursor.OccurredAt < from || cursor.OccurredAt > to || cursor.ID == "" || len(cursor.ID) > maxFilterBytes {
			return EventStoreQuery{}, 0, invalidEventQuery("cursor is invalid for this time window")
		}
		storeQuery.CursorOccurredAt, storeQuery.CursorID = cursor.OccurredAt, cursor.ID
	}
	return storeQuery, limit, nil
}

func eventView(record Record) (EventView, error) {
	attributes := map[string]any{}
	if record.AttributesJSON != "" {
		if err := json.Unmarshal([]byte(record.AttributesJSON), &attributes); err != nil {
			return EventView{}, fmt.Errorf("decode diagnostic event %s attributes: %w", record.ID, err)
		}
	}
	return EventView{
		ID: record.ID, OccurredAt: record.OccurredAt, ReceivedAt: record.ReceivedAt,
		Level: record.Level, Source: record.Source, Subsystem: record.Subsystem, Event: record.Event,
		Message: record.Message, RequestID: record.RequestID, PlaybackSessionID: record.PlaybackSessionID,
		ChannelID: record.ChannelID, ScheduleBlockID: record.ScheduleBlockID, JobID: record.JobID,
		ProcessRunID: record.ProcessRunID, ActorID: record.ActorID, InstanceID: record.InstanceID,
		Attributes: attributes,
	}, nil
}

type eventCursor struct {
	OccurredAt int64  `json:"occurredAt"`
	ID         string `json:"id"`
}

func encodeEventCursor(cursor eventCursor) string {
	encoded, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func decodeEventCursor(value string) (eventCursor, error) {
	if len(value) > maxCursorBytes {
		return eventCursor{}, ErrInvalidEventQuery
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return eventCursor{}, err
	}
	var cursor eventCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		return eventCursor{}, err
	}
	return cursor, nil
}

func invalidEventQuery(detail string) error {
	return fmt.Errorf("%w: %s", ErrInvalidEventQuery, detail)
}
