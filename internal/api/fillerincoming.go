package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/store"
)

const (
	incomingPageLimit          = 20
	defaultRecentlyReadyWindow = 24 * time.Hour
)

var errInvalidIncomingCursor = errors.New("invalid incoming cursor")

type fillerIncomingInput struct {
	PreparingCursor string `query:"preparingCursor" maxLength:"512" doc:"Opaque preparing cursor from the previous page"`
	NeedsHelpCursor string `query:"needsHelpCursor" maxLength:"512" doc:"Opaque Needs-help cursor from the previous page"`
	ReadyCursor     string `query:"readyCursor" maxLength:"512" doc:"Opaque recently-Ready cursor from the previous page"`
}

type fillerIncomingOutput struct {
	Body struct {
		Preparing          IncomingClipGroupDTO `json:"preparing"`
		NeedsHelp          IncomingHelpGroupDTO `json:"needsHelp"`
		RecentlyReady      IncomingClipGroupDTO `json:"recentlyReady"`
		ReadyWindowSeconds int64                `json:"readyWindowSeconds" doc:"Resolved recently-Ready window used for this projection"`
	}
}

// IncomingClipGroupDTO is one bounded newest-first page and its server-counted total.
type IncomingClipGroupDTO struct {
	Rows       []IncomingStatusDTO `json:"rows"`
	Total      int                 `json:"total"`
	NextCursor string              `json:"nextCursor,omitempty" doc:"Opaque cursor for the next newest-first page"`
}

// IncomingHelpGroupDTO is the same bounded contract for exceptional durable choices.
type IncomingHelpGroupDTO struct {
	Rows       []IncomingHelpDTO `json:"rows"`
	Total      int               `json:"total"`
	NextCursor string            `json:"nextCursor,omitempty" doc:"Opaque cursor for the next newest-first page"`
}

// IncomingStatusDTO is the calm row shared by Preparing and recently Ready. Processing is a
// server-owned, browser-safe explanation revealed only after a deliberate details click.
type IncomingStatusDTO struct {
	ClipHash    string                `json:"clipHash"`
	Name        string                `json:"name"`
	From        string                `json:"from,omitempty"`
	DurationMs  int64                 `json:"durationMs"`
	ThumbImage  *ImageDTO             `json:"thumbImage,omitempty" doc:"Extracted still; absent while artwork is not ready"`
	StatusLabel string                `json:"statusLabel"`
	UpdatedAt   string                `json:"updatedAt" doc:"RFC3339"`
	Processing  IncomingProcessingDTO `json:"processing"`
}

type IncomingProcessingDTO struct {
	Attempts        int                          `json:"attempts,omitempty"`
	NextTryAt       string                       `json:"nextTryAt,omitempty" doc:"RFC3339; absent unless this work is scheduled to retry"`
	DiagnosticsHref string                       `json:"diagnosticsHref,omitempty" doc:"Existing operational owner; absent when no deeper evidence exists"`
	Stages          []IncomingProcessingStageDTO `json:"stages"`
}

type IncomingProcessingStageDTO struct {
	Label        string `json:"label"`
	Outcome      string `json:"outcome" enum:"waiting,in_progress,finished,retrying,not_needed,recorded"`
	OutcomeLabel string `json:"outcomeLabel"`
	Note         string `json:"note" doc:"Browser-safe server-owned explanation"`
	At           string `json:"at" doc:"RFC3339"`
	Progress     *int   `json:"progress,omitempty" minimum:"0" maximum:"100" doc:"Measured progress inside this active stage only; absent when unmeasured or inactive"`
}

// IncomingHelpDTO names a real task and the existing destination where its durable mutation is
// completed. Historical classification decisions and operational holds cannot populate it.
type IncomingHelpDTO struct {
	ID          string `json:"id"`
	Kind        string `json:"kind" enum:"split_boundary"`
	ClipHash    string `json:"clipHash"`
	Name        string `json:"name"`
	Question    string `json:"question"`
	ActionLabel string `json:"actionLabel"`
	ActionHref  string `json:"actionHref"`
	CreatedAt   string `json:"createdAt" doc:"RFC3339"`
}

type incomingCursor struct {
	UpdatedAt int64  `json:"updatedAt"`
	ID        string `json:"id"`
}

func (s *Server) registerFillerIncoming(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "filler-incoming", Method: http.MethodGet, Path: "/v1/filler/incoming",
		Summary:     "Filler clips being prepared, needing help, or recently ready",
		Description: "Admin-only hands-off Incoming projection (§10). Each disjoint group is bounded, newest first, and carries its full server-counted total plus an opaque next-page cursor. Needs help contains only a current durable choice; operational and audit history are excluded.",
		Tags:        []string{"filler"},
	}, RoleAdmin), s.fillerIncoming)
}

func (s *Server) fillerIncoming(ctx context.Context, in *fillerIncomingInput) (*fillerIncomingOutput, error) {
	if s.store == nil {
		return nil, huma.Error501NotImplemented("no store configured")
	}
	at := time.Now().UTC()
	if s.now != nil {
		at = s.now().UTC()
	}
	readyWindow := defaultRecentlyReadyWindow
	if s.liveConfigDuration != nil {
		readyWindow = s.liveConfigDuration("filler.incoming.ready_window")
	}
	out := &fillerIncomingOutput{}
	out.Body.ReadyWindowSeconds = int64(readyWindow / time.Second)
	var err error
	if out.Body.Preparing, err = s.incomingPipelineGroup(ctx, at, 0, in.PreparingCursor, false); err != nil {
		return nil, incomingProjectionError(err)
	}
	if out.Body.RecentlyReady, err = s.incomingPipelineGroup(ctx, at, readyWindow, in.ReadyCursor, true); err != nil {
		return nil, incomingProjectionError(err)
	}
	if out.Body.NeedsHelp, err = s.incomingHelpGroup(ctx, in.NeedsHelpCursor); err != nil {
		return nil, incomingProjectionError(err)
	}
	return out, nil
}

func (s *Server) incomingPipelineGroup(ctx context.Context, at time.Time, readyWindow time.Duration, cursorValue string, ready bool) (IncomingClipGroupDTO, error) {
	beforeAt, beforeID, err := decodeIncomingCursor(cursorValue)
	if err != nil {
		return IncomingClipGroupDTO{}, err
	}
	disposition := filler.DispositionRunning
	filter := filler.PipelineFilter{
		Dispositions:    []filler.Disposition{disposition},
		BeforeUpdatedAt: beforeAt, BeforeClipHash: beforeID, Limit: incomingPageLimit + 1,
	}
	if ready {
		disposition = filler.DispositionReady
		filter.Dispositions = []filler.Disposition{disposition}
		filter.UpdatedAtOrAfter = at.Add(-readyWindow)
	}
	rows, err := s.store.ListClipPipelines(ctx, filter)
	if err != nil {
		return IncomingClipGroupDTO{}, err
	}
	countFilter := filter
	countFilter.BeforeUpdatedAt, countFilter.BeforeClipHash, countFilter.Limit = time.Time{}, "", 0
	total, err := s.store.CountClipPipelines(ctx, countFilter)
	if err != nil {
		return IncomingClipGroupDTO{}, err
	}
	more := len(rows) > incomingPageLimit
	if more {
		rows = rows[:incomingPageLimit]
	}
	clips := make([]store.Clip, 0, len(rows))
	for _, row := range rows {
		clip, clipErr := s.store.GetClip(ctx, row.ClipHash)
		if clipErr != nil {
			return IncomingClipGroupDTO{}, clipErr
		}
		clips = append(clips, clip)
	}
	images := s.clipArtworkResolver(ctx, clips)
	group := IncomingClipGroupDTO{Rows: make([]IncomingStatusDTO, 0, len(rows)), Total: total}
	for index, row := range rows {
		label := friendlyIncomingStage(row.Stage)
		if ready {
			label = "Ready"
		}
		group.Rows = append(group.Rows, incomingStatusDTO(clips[index], row, label, images, at))
	}
	if more {
		last := rows[len(rows)-1]
		group.NextCursor = encodeIncomingCursor(last.UpdatedAt, last.ClipHash)
	}
	return group, nil
}

func (s *Server) incomingHelpGroup(ctx context.Context, cursorValue string) (IncomingHelpGroupDTO, error) {
	beforeAt, beforeID, err := decodeIncomingCursor(cursorValue)
	if err != nil {
		return IncomingHelpGroupDTO{}, err
	}
	proposals, err := s.store.ListReadySplitProposalsAfter(ctx, filler.SplitProposalCursor{
		BeforeCreatedAt: beforeAt, BeforeID: beforeID,
	}, incomingPageLimit+1)
	if err != nil {
		return IncomingHelpGroupDTO{}, err
	}
	more := len(proposals) > incomingPageLimit
	if more {
		proposals = proposals[:incomingPageLimit]
	}
	group := IncomingHelpGroupDTO{Rows: make([]IncomingHelpDTO, 0, len(proposals))}
	if group.Total, err = s.store.CountReadySplitProposals(ctx); err != nil {
		return IncomingHelpGroupDTO{}, err
	}
	for _, proposal := range proposals {
		clip, clipErr := s.store.GetClip(ctx, proposal.ClipHash)
		if clipErr != nil {
			return IncomingHelpGroupDTO{}, clipErr
		}
		group.Rows = append(group.Rows, IncomingHelpDTO{
			ID: proposal.ID, Kind: "split_boundary", ClipHash: proposal.ClipHash, Name: clip.Name,
			Question: "Where should this compilation be split?", ActionLabel: "Review clips",
			ActionHref: "/filler/splits/" + url.PathEscape(proposal.ID),
			CreatedAt:  proposal.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	if more {
		last := proposals[len(proposals)-1]
		group.NextCursor = encodeIncomingCursor(last.CreatedAt, last.ID)
	}
	return group, nil
}

func incomingStatusDTO(clip store.Clip, row filler.ClipPipeline, label string, image func(string) *ImageDTO, at time.Time) IncomingStatusDTO {
	var thumb *ImageDTO
	if image != nil {
		thumb = image(clip.Hash)
	}
	return IncomingStatusDTO{
		ClipHash: clip.Hash, Name: clip.Name, From: clip.Source, DurationMs: clip.DurationMs,
		ThumbImage: thumb, StatusLabel: label,
		UpdatedAt: row.UpdatedAt.UTC().Format(time.RFC3339), Processing: incomingProcessingDTO(row, at),
	}
}

func incomingProcessingDTO(row filler.ClipPipeline, at time.Time) IncomingProcessingDTO {
	detail := IncomingProcessingDTO{Attempts: row.Attempts, Stages: make([]IncomingProcessingStageDTO, 0, len(row.Stages)+1)}
	if row.Disposition == filler.DispositionRunning && row.Status == filler.StatusFailed && row.NextRun.After(at) {
		detail.NextTryAt = row.NextRun.UTC().Format(time.RFC3339)
	}
	currentRecorded := false
	for _, stage := range row.Stages {
		detail.Stages = append(detail.Stages, incomingProcessingStage(stage.Stage, stage.Status, stage.At, nil))
		if stage.Stage == row.Stage && stage.Status == row.Status {
			currentRecorded = true
		}
		if stage.Status == filler.StatusFailed {
			detail.DiagnosticsHref = "/filler/manage#diagnostics"
		}
	}
	// A running rung is intentionally absent from the stored completed-stage ladder. Append its
	// current snapshot without replacing a prior failed attempt; both are true and useful. Queued,
	// failed, and terminal snapshots are appended only when the store has not already recorded the
	// same state.
	if !currentRecorded {
		var progress *int
		if row.Status == filler.StatusRunning && row.Progress >= 0 {
			measured := row.Progress
			progress = &measured
		}
		detail.Stages = append(detail.Stages, incomingProcessingStage(row.Stage, row.Status, row.UpdatedAt, progress))
	}
	if row.Status == filler.StatusFailed {
		detail.DiagnosticsHref = "/filler/manage#diagnostics"
	}
	return detail
}

func incomingProcessingStage(stage filler.StageID, status filler.StageStatus, at time.Time, progress *int) IncomingProcessingStageDTO {
	outcome, label, note := friendlyIncomingStageOutcome(status)
	return IncomingProcessingStageDTO{
		Label: friendlyIncomingProcessingStage(stage), Outcome: outcome, OutcomeLabel: label,
		Note: note, At: at.UTC().Format(time.RFC3339), Progress: progress,
	}
}

func friendlyIncomingProcessingStage(stage filler.StageID) string {
	switch stage {
	case filler.StageProbe:
		return "Inspecting the file"
	case filler.StageTranscode:
		return "Preparing playback"
	case filler.StageScreen:
		return "Checking video safety"
	case filler.StageSplit:
		return "Checking for separate clips"
	case filler.StageLanguage:
		return "Detecting the language"
	case filler.StageTranscribe:
		return "Listening for speech"
	case filler.StageTag:
		return "Adding clip details"
	case filler.StageVision:
		return "Checking the picture"
	case filler.StageScore:
		return "Finishing"
	default:
		return "Getting ready"
	}
}

func friendlyIncomingStage(stage filler.StageID) string {
	switch stage {
	case filler.StageProbe, filler.StageTranscode, filler.StageScreen:
		return "Checking video"
	case filler.StageSplit:
		return "Checking for separate clips"
	case filler.StageLanguage, filler.StageTranscribe, filler.StageTag, filler.StageVision:
		return "Adding details"
	case filler.StageScore:
		return "Finishing"
	default:
		return "Getting ready"
	}
}

func friendlyIncomingStageOutcome(status filler.StageStatus) (string, string, string) {
	switch status {
	case filler.StatusQueued:
		return "waiting", "Waiting", "Loomarr saved its place and will continue automatically."
	case filler.StatusRunning:
		return "in_progress", "In progress", "Loomarr is working on this step now."
	case filler.StatusDone:
		return "finished", "Finished", "This step finished successfully."
	case filler.StatusFailed:
		return "retrying", "Trying again", "This step did not finish. Loomarr will try again automatically."
	case filler.StatusSkipped:
		return "not_needed", "Not needed", "This step was not needed for this clip."
	default:
		return "recorded", "Recorded", "Loomarr recorded this step."
	}
}

func encodeIncomingCursor(at time.Time, id string) string {
	raw, _ := json.Marshal(incomingCursor{UpdatedAt: at.Unix(), ID: id})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeIncomingCursor(value string) (time.Time, string, error) {
	if value == "" {
		return time.Time{}, "", nil
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil {
		return time.Time{}, "", errInvalidIncomingCursor
	}
	var cursor incomingCursor
	if err := json.Unmarshal(raw, &cursor); err != nil || cursor.UpdatedAt <= 0 || cursor.ID == "" {
		return time.Time{}, "", errInvalidIncomingCursor
	}
	return time.Unix(cursor.UpdatedAt, 0).UTC(), cursor.ID, nil
}

func incomingProjectionError(err error) error {
	if errors.Is(err, errInvalidIncomingCursor) {
		return huma.Error422UnprocessableEntity("Invalid Incoming cursor")
	}
	return huma.Error500InternalServerError("project incoming", err)
}
