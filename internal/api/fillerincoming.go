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

// IncomingStatusDTO is the calm row shared by Preparing and recently Ready. Internal state
// vocabulary stays in Technical, which the browser reveals only after a deliberate details click.
type IncomingStatusDTO struct {
	ClipHash    string               `json:"clipHash"`
	Name        string               `json:"name"`
	From        string               `json:"from,omitempty"`
	DurationMs  int64                `json:"durationMs"`
	ThumbImage  *ImageDTO            `json:"thumbImage,omitempty" doc:"Extracted still; absent while artwork is not ready"`
	StatusLabel string               `json:"statusLabel"`
	Progress    int                  `json:"progress,omitempty" minimum:"-1" maximum:"100"`
	UpdatedAt   string               `json:"updatedAt" doc:"RFC3339"`
	Technical   IncomingTechnicalDTO `json:"technical"`
}

type IncomingTechnicalDTO struct {
	Attempts  int                         `json:"attempts,omitempty"`
	NextTryAt string                      `json:"nextTryAt,omitempty" doc:"RFC3339; absent unless this work is scheduled to retry"`
	Stages    []IncomingTechnicalStageDTO `json:"stages"`
}

type IncomingTechnicalStageDTO struct {
	Label  string `json:"label"`
	Status string `json:"status"`
	Note   string `json:"note,omitempty"`
	At     string `json:"at" doc:"RFC3339"`
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
		ThumbImage: thumb, StatusLabel: label, Progress: row.Progress,
		UpdatedAt: row.UpdatedAt.UTC().Format(time.RFC3339), Technical: incomingTechnicalDTO(row, at),
	}
}

func incomingTechnicalDTO(row filler.ClipPipeline, at time.Time) IncomingTechnicalDTO {
	detail := IncomingTechnicalDTO{Attempts: row.Attempts, Stages: make([]IncomingTechnicalStageDTO, 0, len(row.Stages))}
	if row.Disposition == filler.DispositionRunning && row.NextRun.After(at) {
		detail.NextTryAt = row.NextRun.UTC().Format(time.RFC3339)
	}
	for _, stage := range row.Stages {
		detail.Stages = append(detail.Stages, IncomingTechnicalStageDTO{
			Label: friendlyIncomingStage(stage.Stage), Status: friendlyIncomingStageStatus(stage.Status),
			Note: stage.Note, At: stage.At.UTC().Format(time.RFC3339),
		})
	}
	return detail
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

func friendlyIncomingStageStatus(status filler.StageStatus) string {
	switch status {
	case filler.StatusQueued:
		return "Waiting"
	case filler.StatusRunning:
		return "In progress"
	case filler.StatusDone:
		return "Finished"
	case filler.StatusFailed:
		return "Trying again"
	case filler.StatusSkipped:
		return "Not needed"
	default:
		return "Recorded"
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
