package agents

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The admin tools: running the server. They are listed only to an admin caller, and every one of
// them also goes through an admin-only route, so a member who somehow named one would get the
// route's 403, not the data.

type healthIn struct{}

type healthCheck struct {
	Check  string `json:"check"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type healthOut struct {
	State   string `json:"state"`
	Version string `json:"version"`
	// Problems lists the checks that did not pass. The passing ones are counted, not listed: a
	// model asked "is everything OK" needs the exceptions, and twenty "passed" rows are noise.
	Problems []healthCheck `json:"problems"`
	Passing  int           `json:"passing"`
}

func (t *toolset) health(ctx context.Context, _ *mcp.CallToolRequest, _ healthIn) (*mcp.CallToolResult, healthOut, error) {
	var res struct {
		State   string `json:"state"`
		Version string `json:"version"`
		Checks  []struct {
			Label  string `json:"label"`
			Status string `json:"status"`
			Detail string `json:"detail"`
		} `json:"checks"`
	}
	if err := t.get(ctx, "/v1/diagnostics/health", nil, &res); err != nil {
		return nil, healthOut{}, err
	}
	out := healthOut{State: res.State, Version: res.Version, Problems: []healthCheck{}}
	for _, c := range res.Checks {
		switch c.Status {
		case "passed", "skipped":
			out.Passing++
		default:
			out.Problems = append(out.Problems, healthCheck{Check: c.Label, Status: c.Status, Detail: c.Detail})
		}
	}
	return nil, out, nil
}

type whyIn struct {
	Channel string `json:"channel,omitempty" jsonschema:"a channel number or name; leave empty for every channel that is streaming now"`
}

type streamRow struct {
	Channel string  `json:"channel"`
	Health  string  `json:"health"`
	Reason  string  `json:"reason,omitempty"`
	Viewers int     `json:"viewers"`
	Speed   float64 `json:"speed"`
	Encoder string  `json:"encoder"`
}

type whyOut struct {
	Running bool `json:"playoutRunning"`
	// Channel is set when one channel was asked about: its state, whether it is streaming, and a
	// plain explanation when it is not.
	Channel *channelState `json:"channel,omitempty"`
	Streams []streamRow   `json:"streams"`
}

type channelState struct {
	Channel   string `json:"channel"`
	Status    string `json:"status"`
	Playable  bool   `json:"playableInApp"`
	Programs  int    `json:"programs"`
	Pending   int    `json:"pendingAcquisitions"`
	Streaming bool   `json:"streaming"`
	Note      string `json:"note,omitempty"`
}

// channelNotes explains a channel status that keeps it off the air, in an operator's words.
var channelNotes = map[string]string{
	"building": "The channel is still being built; it plays once its lineup is ready.",
	"empty":    "The channel has no programmes yet, so there is nothing to play.",
	"paused":   "The channel is paused.",
	"drifted":  "The channel's schedule no longer matches its lineup; it needs a rebuild.",
	"detached": "The channel is detached from its playout backend.",
}

func (t *toolset) whyNotPlaying(ctx context.Context, _ *mcp.CallToolRequest, in whyIn) (*mcp.CallToolResult, whyOut, error) {
	var res struct {
		Running  bool `json:"running"`
		Channels []struct {
			ChannelID string  `json:"channelId"`
			Number    int     `json:"number"`
			Name      string  `json:"name"`
			Health    string  `json:"health"`
			Reason    string  `json:"reason"`
			Viewers   int     `json:"viewers"`
			Speed     float64 `json:"speed"`
			Encoder   string  `json:"encoder"`
		} `json:"channels"`
	}
	if err := t.get(ctx, "/v1/playout/status", nil, &res); err != nil {
		return nil, whyOut{}, err
	}
	out := whyOut{Running: res.Running, Streams: []streamRow{}}
	var only *channel
	if in.Channel != "" {
		c, err := t.resolve(ctx, in.Channel)
		if err != nil {
			return nil, out, err
		}
		only = &c
	}
	for _, s := range res.Channels {
		if only != nil && s.ChannelID != only.ID {
			continue
		}
		out.Streams = append(out.Streams, streamRow{
			Channel: fmt.Sprintf("%d %s", s.Number, s.Name), Health: s.Health, Reason: s.Reason,
			Viewers: s.Viewers, Speed: s.Speed, Encoder: s.Encoder,
		})
	}
	if only != nil {
		st := channelState{
			Channel: only.label(), Status: only.Status, Playable: only.InAppPlayable,
			Programs: only.ProgramCount, Pending: only.PendingCount, Streaming: len(out.Streams) > 0,
			Note: channelNotes[only.Status],
		}
		if st.Note == "" && !st.Streaming {
			// Playout is on demand: a channel streams only while someone watches it.
			st.Note = "Nobody is watching it, so it is not streaming. It starts when someone tunes in."
		}
		if !res.Running {
			st.Note = "Playout is not running on this server. " + st.Note
		}
		out.Channel = &st
	}
	return nil, out, nil
}

type attentionIn struct{}

type namedItem struct {
	Name string `json:"name"`
	Year int    `json:"year,omitempty"`
}

type proposalRow struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	Ask  string `json:"ask"`
	By   string `json:"by,omitempty"`
}

type jobRow struct {
	Job     string `json:"job"`
	Title   string `json:"title"`
	Problem string `json:"problem"`
}

type attentionOut struct {
	// Each list holds at most attentionRows; the matching *Count is the full number.
	Proposals      []proposalRow `json:"proposalsWaiting"`
	ProposalCount  int           `json:"proposalCount"`
	Jobs           []jobRow      `json:"jobs"`
	LibraryNeeds   []namedItem   `json:"libraryNeedsAction"`
	NeedsCount     int           `json:"libraryNeedsCount"`
	LibraryOrders  int           `json:"seasonOrdersToChoose"`
	LibraryProblem int           `json:"libraryProblems"`
	// Unavailable names each source this server could not read (no media library connected, the
	// scheduler not running), so an empty list above is not mistaken for "nothing to do".
	Unavailable []string `json:"unavailable,omitempty"`
}

// attentionRows caps each list in needs_attention; the counts beside them stay whole.
const attentionRows = 8

func (t *toolset) needsAttention(ctx context.Context, _ *mcp.CallToolRequest, _ attentionIn) (*mcp.CallToolResult, attentionOut, error) {
	out := attentionOut{Proposals: []proposalRow{}, Jobs: []jobRow{}, LibraryNeeds: []namedItem{}}
	// Each source is read on its own: one that this server cannot serve is reported in
	// Unavailable, and the others still answer. Only when none answers is it the tool's error.
	read := func(source, path string, q url.Values, into any) {
		if err := t.get(ctx, path, q, into); err != nil {
			out.Unavailable = append(out.Unavailable, source+": "+err.Error())
		}
	}

	var props struct {
		Proposals []struct {
			ID            string `json:"id"`
			CreatedByName string `json:"createdByName"`
			Proposal      struct {
				ChannelName string `json:"channelName"`
				Intent      struct {
					Description string `json:"description"`
				} `json:"intent"`
			} `json:"proposal"`
		} `json:"proposals"`
	}
	read("proposals", "/v1/proposals", url.Values{"status": {"submitted"}}, &props)
	out.ProposalCount = len(props.Proposals)
	for _, p := range props.Proposals[:min(attentionRows, len(props.Proposals))] {
		out.Proposals = append(out.Proposals, proposalRow{
			ID: p.ID, Name: untrusted(p.Proposal.ChannelName), Ask: untrusted(p.Proposal.Intent.Description), By: untrusted(p.CreatedByName),
		})
	}

	var jobs struct {
		Jobs []struct {
			Name       string `json:"name"`
			Title      string `json:"title"`
			LastResult string `json:"lastResult"`
			LastError  string `json:"lastError"`
			Overdue    bool   `json:"overdue"`
		} `json:"jobs"`
	}
	read("jobs", "/v1/jobs", nil, &jobs)
	for _, j := range jobs.Jobs {
		var problem string
		switch {
		case j.LastResult == "error":
			problem = "last run failed: " + j.LastError
		case j.Overdue:
			problem = "overdue"
		default:
			continue
		}
		out.Jobs = append(out.Jobs, jobRow{Job: j.Name, Title: j.Title, Problem: problem})
	}

	var needs struct {
		Needs []struct {
			Item namedItem `json:"item"`
		} `json:"needs"`
		Orders   []struct{} `json:"orders"`
		Problems []struct{} `json:"problems"`
	}
	read("library", "/v1/library/needs", nil, &needs)
	out.NeedsCount = len(needs.Needs)
	for _, n := range needs.Needs[:min(attentionRows, len(needs.Needs))] {
		out.LibraryNeeds = append(out.LibraryNeeds, n.Item)
	}
	out.LibraryOrders = len(needs.Orders)
	out.LibraryProblem = len(needs.Problems)
	if len(out.Unavailable) == 3 {
		return nil, out, fmt.Errorf("nothing could be read: %s", strings.Join(out.Unavailable, "; "))
	}
	return nil, out, nil
}

type eventsIn struct {
	Level     string `json:"level,omitempty" jsonschema:"only events at this level: debug, info, warn or error; leave empty for warnings and errors"`
	Subsystem string `json:"subsystem,omitempty" jsonschema:"only events from this subsystem"`
	Channel   string `json:"channel,omitempty" jsonschema:"only events about this channel (number or name)"`
	Text      string `json:"text,omitempty" jsonschema:"only events whose message contains this text"`
	Count     int    `json:"count,omitempty" jsonschema:"how many events, newest first (default 20, at most 100)"`
	Hours     int    `json:"hours,omitempty" jsonschema:"how far back to look, in hours (default 24, at most 168)"`
}

type eventRow struct {
	At        string `json:"at"`
	Level     string `json:"level"`
	Subsystem string `json:"subsystem,omitempty"`
	Event     string `json:"event"`
	Message   string `json:"message,omitempty"`
}

type eventsOut struct {
	TimeZone string     `json:"timeZone"`
	Events   []eventRow `json:"events"`
	More     bool       `json:"more,omitempty"`
}

func (t *toolset) recentEvents(ctx context.Context, _ *mcp.CallToolRequest, in eventsIn) (*mcp.CallToolResult, eventsOut, error) {
	k := t.clock(ctx)
	out := eventsOut{TimeZone: k.zone(), Events: []eventRow{}}
	limit := clampLimit(in.Count, 20, 100)
	// The route's own window is the last hour; "any errors today?" needs more than that.
	from := k.now.Add(-time.Duration(clampLimit(in.Hours, 24, 168)) * time.Hour)
	q := url.Values{
		"limit": {strconv.Itoa(limit)}, "order": {"newest"},
		"from": {strconv.FormatInt(from.UnixMilli(), 10)}, "to": {strconv.FormatInt(k.now.UnixMilli(), 10)},
	}
	if in.Subsystem != "" {
		q.Set("subsystem", in.Subsystem)
	}
	if in.Text != "" {
		q.Set("text", in.Text)
	}
	if in.Channel != "" {
		c, err := t.resolve(ctx, in.Channel)
		if err != nil {
			return nil, out, err
		}
		q.Set("channelId", c.ID)
	}
	// The route filters on one exact level. "Warnings and errors" is therefore two reads, each of
	// the full limit, merged newest first and cut back to the limit.
	levels := []string{strings.ToLower(strings.TrimSpace(in.Level))}
	switch levels[0] {
	case "":
		levels = []string{"warn", "error"}
	case "warning":
		levels[0] = "warn" // the route's word; anything else it does not know is its 422
	}
	type event struct {
		OccurredAt int64  `json:"occurredAt"` // epoch ms
		Level      string `json:"level"`
		Subsystem  string `json:"subsystem"`
		Event      string `json:"event"`
		Message    string `json:"message"`
	}
	var all []event
	for _, level := range levels {
		q.Set("level", level)
		var res struct {
			Items      []event `json:"items"`
			NextCursor string  `json:"nextCursor"`
		}
		if err := t.get(ctx, "/v1/diagnostics/events", q, &res); err != nil {
			return nil, out, err
		}
		all = append(all, res.Items...)
		out.More = out.More || res.NextCursor != ""
	}
	slices.SortStableFunc(all, func(a, b event) int { return cmp.Compare(b.OccurredAt, a.OccurredAt) })
	if len(all) > limit {
		all, out.More = all[:limit], true
	}
	for _, e := range all {
		out.Events = append(out.Events, eventRow{At: k.at(e.OccurredAt), Level: e.Level, Subsystem: e.Subsystem, Event: e.Event, Message: untrusted(e.Message)})
	}
	return nil, out, nil
}

type jobIn struct {
	Job string `json:"job" jsonschema:"the job's id, as needs_attention or list_jobs names it"`
}

type jobsIn struct{}

type jobListRow struct {
	Job     string `json:"job"`
	Title   string `json:"title"`
	LastRun string `json:"lastRun,omitempty"`
	Result  string `json:"lastResult,omitempty"`
	Running bool   `json:"running,omitempty"`
	Paused  bool   `json:"paused,omitempty"`
}

type jobsOut struct {
	TimeZone string       `json:"timeZone"`
	Jobs     []jobListRow `json:"jobs"`
}

func (t *toolset) listJobs(ctx context.Context, _ *mcp.CallToolRequest, _ jobsIn) (*mcp.CallToolResult, jobsOut, error) {
	k := t.clock(ctx)
	var res struct {
		Jobs []struct {
			Name       string `json:"name"`
			Title      string `json:"title"`
			LastRun    string `json:"lastRun"`
			LastResult string `json:"lastResult"`
			Running    bool   `json:"running"`
			Paused     bool   `json:"paused"`
		} `json:"jobs"`
	}
	if err := t.get(ctx, "/v1/jobs", nil, &res); err != nil {
		return nil, jobsOut{}, err
	}
	out := jobsOut{TimeZone: k.zone(), Jobs: make([]jobListRow, 0, len(res.Jobs))}
	for _, j := range res.Jobs {
		out.Jobs = append(out.Jobs, jobListRow{Job: j.Name, Title: j.Title, LastRun: k.stamp(j.LastRun), Result: j.LastResult, Running: j.Running, Paused: j.Paused})
	}
	return nil, out, nil
}

type runRow struct {
	Started  string `json:"started"`
	Duration string `json:"duration"`
	Result   string `json:"result"`
	Error    string `json:"error,omitempty"`
	Trigger  string `json:"trigger"`
}

type historyOut struct {
	TimeZone string   `json:"timeZone"`
	Runs     int      `json:"runs"`
	Failures int      `json:"failures"`
	Recent   []runRow `json:"recent"`
}

func (t *toolset) jobHistory(ctx context.Context, _ *mcp.CallToolRequest, in jobIn) (*mcp.CallToolResult, historyOut, error) {
	job, err := segment("job", in.Job, "list_jobs")
	if err != nil {
		return nil, historyOut{}, err
	}
	k := t.clock(ctx)
	var res struct {
		RunCount     int `json:"runCount"`
		FailureCount int `json:"failureCount"`
		Recent       []struct {
			StartedAt  string `json:"startedAt"`
			DurationMs int64  `json:"durationMs"`
			Result     string `json:"result"`
			Error      string `json:"error"`
			Trigger    string `json:"trigger"`
		} `json:"recent"`
	}
	if err := t.get(ctx, "/v1/jobs/"+job+"/history", nil, &res); err != nil {
		return nil, historyOut{}, err
	}
	out := historyOut{TimeZone: k.zone(), Runs: res.RunCount, Failures: res.FailureCount, Recent: []runRow{}}
	for _, r := range res.Recent {
		out.Recent = append(out.Recent, runRow{
			Started: k.stamp(r.StartedAt), Duration: (time.Duration(r.DurationMs) * time.Millisecond).Round(100 * time.Millisecond).String(),
			Result: r.Result, Error: untrusted(r.Error), Trigger: r.Trigger,
		})
	}
	return nil, out, nil
}

func (t *toolset) runJob(ctx context.Context, _ *mcp.CallToolRequest, in jobIn) (*mcp.CallToolResult, doneOut, error) {
	job, err := segment("job", in.Job, "list_jobs")
	if err != nil {
		return nil, doneOut{}, err
	}
	if err := t.call(ctx, http.MethodPost, "/v1/jobs/"+job+"/run", nil, nil, nil); err != nil {
		return nil, doneOut{}, err
	}
	return nil, doneOut{Done: "Started " + in.Job + ". Check job_history for the result."}, nil
}

type proposalIn struct {
	ProposalID string `json:"proposalId" jsonschema:"the proposal's id, from needs_attention"`
}

type denyIn struct {
	ProposalID string `json:"proposalId" jsonschema:"the proposal's id, from needs_attention"`
	Reason     string `json:"reason,omitempty" jsonschema:"why, shown to the person who asked"`
}

func (t *toolset) approve(ctx context.Context, _ *mcp.CallToolRequest, in proposalIn) (*mcp.CallToolResult, doneOut, error) {
	id, err := segment("proposalId", in.ProposalID, "needs_attention")
	if err != nil {
		return nil, doneOut{}, err
	}
	if err := t.call(ctx, http.MethodPost, "/v1/proposals/"+id+"/approve", nil, map[string]any{}, nil); err != nil {
		return nil, doneOut{}, err
	}
	return nil, doneOut{Done: "Approved. The channel is being made."}, nil
}

func (t *toolset) deny(ctx context.Context, _ *mcp.CallToolRequest, in denyIn) (*mcp.CallToolResult, doneOut, error) {
	id, err := segment("proposalId", in.ProposalID, "needs_attention")
	if err != nil {
		return nil, doneOut{}, err
	}
	body := map[string]string{}
	if in.Reason != "" {
		body["reason"] = in.Reason
	}
	if err := t.call(ctx, http.MethodPost, "/v1/proposals/"+id+"/deny", nil, body, nil); err != nil {
		return nil, doneOut{}, err
	}
	return nil, doneOut{Done: "Denied."}, nil
}

// addAdminTools registers the tools only an admin is offered.
func addAdminTools(s *mcp.Server, t *toolset) {
	mcp.AddTool(s, &mcp.Tool{Name: "health", Annotations: readOnly(),
		Description: "The server's health checks, listing only the ones that are not passing."}, t.health)
	mcp.AddTool(s, &mcp.Tool{Name: "why_not_playing", Annotations: readOnly(),
		Description: "Why a channel is not playing well or at all, or the health of every channel streaming now."}, t.whyNotPlaying)
	mcp.AddTool(s, &mcp.Tool{Name: "needs_attention", Annotations: readOnly(),
		Description: "What needs an admin: channel requests waiting for approval, failing or overdue jobs, and library items needing a decision."}, t.needsAttention)
	mcp.AddTool(s, &mcp.Tool{Name: "recent_events", Annotations: readOnly(),
		Description: "Recent diagnostic events, newest first, warnings and errors by default."}, t.recentEvents)
	mcp.AddTool(s, &mcp.Tool{Name: "list_jobs", Annotations: readOnly(),
		Description: "The scheduled jobs and how each last ran."}, t.listJobs)
	mcp.AddTool(s, &mcp.Tool{Name: "job_history", Annotations: readOnly(),
		Description: "A job's recent runs and failures."}, t.jobHistory)
	mcp.AddTool(s, &mcp.Tool{Name: "run_job", Annotations: changes(false),
		Description: "Run a scheduled job now instead of waiting for its schedule."}, t.runJob)
	mcp.AddTool(s, &mcp.Tool{Name: "approve_proposal", Annotations: changes(true),
		Description: "Approve a channel request, which makes the channel and may start downloads. Confirm with the person first."}, t.approve)
	mcp.AddTool(s, &mcp.Tool{Name: "deny_proposal", Annotations: changes(true),
		Description: "Deny a channel request. Confirm with the person first."}, t.deny)
}
