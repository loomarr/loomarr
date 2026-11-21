package catalog

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// --- DTOs (schemas are generated from these — §7.1) ---

// TitleDTO is the API view of a provisioning record.
type TitleDTO struct {
	Key       string `json:"key" example:"movie:tmdb:91111867" doc:"Stable identity key"`
	MediaType string `json:"mediaType" enum:"movie,series" example:"movie"`
	TMDBID    int    `json:"tmdbId,omitempty" example:"91111867"`
	TVDBID    int    `json:"tvdbId,omitempty"`
	Name      string `json:"name,omitempty" example:"Ashgrove Anomaly"`
	Year      int    `json:"year,omitempty" example:"2023"`
	State     string `json:"state" enum:"wanted,requested,downloading,available,unavailable" doc:"Provisioning state (§4)"`
	LibraryID string `json:"libraryId,omitempty"`
	// Download progress (§18.1 arr-queue-poll), populated while downloading via the direct
	// arr requester; zero on the Seerr path / once available.
	Progress       float64 `json:"progress,omitempty" doc:"Download completion 0..1 (arr queue poll)"`
	ETAText        string  `json:"etaText,omitempty" doc:"Human time-left from the download client"`
	DownloadStatus string  `json:"downloadStatus,omitempty" doc:"Download-client status (downloading/warning/stalled/…)"`
	// EpisodesHave of EpisodesWanted is Home's On the way "8 of 36 episodes" (#1667).
	EpisodesHave   int `json:"episodesHave,omitempty" doc:"A downloading series' episodes on disk (arr poll); absent for a movie or when unknown"`
	EpisodesWanted int `json:"episodesWanted,omitempty" doc:"The episodes the arr wants for this series (monitored and aired); absent for a movie or when unknown"`
	// LastError is why the reconciler gave up on an `unavailable` title (e.g. "deadline
	// exceeded"), so a client can say so instead of describing it as queued.
	LastError string `json:"lastError,omitempty" example:"deadline exceeded" doc:"Why the title was given up on (unavailable only)"`
	// AvailableAtMs is the title's arrival (Home's New this week, #1663): when the library
	// confirmed a title Loomarr acquired. Absent for a title a channel picked from the library.
	AvailableAtMs int64 `json:"availableAtMs,omitempty" doc:"When the library confirmed this acquired title (Unix ms); absent if it was already in the library"`
	// Channels are the channels whose lineup holds the title, by number, on the list and get
	// reads. Joined server-side from one channel read so no client walks every lineup.
	Channels []TitleChannelDTO `json:"channels,omitempty" doc:"Channels whose lineup holds this title, by number (detached channels excluded)"`
}

// TitleChannelDTO names a channel a title plays on.
type TitleChannelDTO struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Number int    `json:"number"`
}

func toDTO(r provision.Record) TitleDTO {
	var availableAtMs int64
	if !r.AvailableAt.IsZero() {
		availableAtMs = r.AvailableAt.UnixMilli()
	}
	return TitleDTO{
		Key: string(r.Key), MediaType: string(r.Title.MediaType),
		TMDBID: r.Title.TMDBID, TVDBID: r.Title.TVDBID,
		Name: r.Title.Name, Year: r.Title.Year,
		State: string(r.State), LibraryID: r.LibraryID,
		Progress: r.Progress, ETAText: r.ETAText, DownloadStatus: r.DownloadStatus,
		EpisodesHave: r.EpisodesHave, EpisodesWanted: r.EpisodesWanted,
		LastError: r.LastError, AvailableAtMs: availableAtMs,
	}
}

// registerTitles mounts /v1/titles* (§7). Reads are visible to any authenticated
// user; POST/DELETE require admin (§7: enqueuing an acquisition is the approval
// gate's concern).
func (s *Handlers) registerTitles(api huma.API) {
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "enqueue-title", Method: http.MethodPost, Path: "/v1/titles",
		Summary: "Enqueue/ensure a title", Description: "Idempotent by external id (§4); re-enqueueing an unavailable title retries it. Admin only.",
		Tags: []string{"titles"},
	}, httpcore.RoleAdmin), s.enqueueTitle)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "get-title", Method: http.MethodGet, Path: "/v1/titles/{key}",
		Summary: "Get a title's provisioning state", Description: httpcore.OperatorAPI, Tags: []string{"titles"},
	}, httpcore.RoleMember), s.getTitle)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "list-titles", Method: http.MethodGet, Path: "/v1/titles",
		Summary: "List titles, optionally filtered by state", Tags: []string{"titles"},
	}, httpcore.RoleMember), s.listTitles)

}

// --- handlers ---

type enqueueInput struct {
	Body struct {
		MediaType string `json:"mediaType" enum:"movie,series"`
		TMDBID    int    `json:"tmdbId,omitempty"`
		TVDBID    int    `json:"tvdbId,omitempty"`
		Name      string `json:"name,omitempty"`
		Year      int    `json:"year,omitempty"`
		Seasons   []int  `json:"seasons,omitempty" maxItems:"200"` // the longest-running series has about sixty seasons
	}
}
type titleOutput struct{ Body TitleDTO }

func (s *Handlers) enqueueTitle(ctx context.Context, in *enqueueInput) (*titleOutput, error) {
	t := provision.Title{
		MediaType: provision.MediaType(in.Body.MediaType),
		TMDBID:    in.Body.TMDBID, TVDBID: in.Body.TVDBID,
		Name: in.Body.Name, Year: in.Body.Year, Seasons: in.Body.Seasons,
	}
	key, err := t.Key()
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusUnprocessableEntity, "Invalid title",
			"That title's identity couldn't be resolved. Check the media type and id, then try again.", err)
	}
	// Idempotent enqueue (§4 inv. 3): only create if absent, else return current — except a title
	// that was given up, which a re-enqueue (the Requests page's Retry) asks for again.
	now := s.clock()
	if existing, err := s.store.GetTitle(ctx, key); err == nil {
		retried, ok := provision.Retry(existing, now)
		if !ok {
			return &titleOutput{Body: toDTO(existing)}, nil
		}
		if err := s.store.UpsertTitle(ctx, retried); err != nil {
			return nil, err
		}
		return &titleOutput{Body: toDTO(retried)}, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	// A wanted title is due at once: the backstop claims only rows with a deadline, and its first
	// claim is what submits the request downstream.
	rec := provision.Record{Key: key, Title: t, State: provision.Wanted, Deadline: now, UpdatedAt: now}
	if err := s.store.UpsertTitle(ctx, rec); err != nil {
		return nil, err
	}
	return &titleOutput{Body: toDTO(rec)}, nil
}

type keyInput struct {
	Key string `path:"key" example:"movie:tmdb:91111867"`
}

func (s *Handlers) getTitle(ctx context.Context, in *keyInput) (*titleOutput, error) {
	rec, err := s.store.GetTitle(ctx, provision.Key(in.Key))
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpcore.NotFound("Title not found", "That title doesn't exist — it may have been removed.")
	}
	if err != nil {
		return nil, err
	}
	on, err := s.titleChannels(ctx)
	if err != nil {
		return nil, err
	}
	dto := toDTO(rec)
	dto.Channels = on[rec.Key]
	return &titleOutput{Body: dto}, nil
}

// titleChannels maps each title key to the channels whose lineup holds it, by number, from one
// channel read. A detached channel no longer plays anything.
func (s *Handlers) titleChannels(ctx context.Context) (map[provision.Key][]TitleChannelDTO, error) {
	channels, err := s.store.ListChannels(ctx) // ordered by number
	if err != nil {
		return nil, err
	}
	out := map[provision.Key][]TitleChannelDTO{}
	for _, ch := range channels {
		if ch.Status == schedule.StatusDetached {
			continue
		}
		seen := map[provision.Key]bool{}
		for _, e := range ch.Lineup {
			if !seen[e.Key] {
				seen[e.Key] = true
				out[e.Key] = append(out[e.Key], TitleChannelDTO{ID: ch.ID, Name: ch.Name, Number: ch.Number})
			}
		}
	}
	return out, nil
}

type listInput struct {
	State string `query:"state" enum:"wanted,requested,downloading,available,unavailable" doc:"Filter by state"`
	Since int64  `query:"since" minimum:"0" doc:"Unix ms. Lists the titles that arrived at or after this time, newest first (Home's New this week). state may be omitted or 'available'."`
}
type listOutput struct {
	Body struct {
		Titles []TitleDTO `json:"titles"`
	}
}

func (s *Handlers) listTitles(ctx context.Context, in *listInput) (*listOutput, error) {
	var (
		recs []provision.Record
		err  error
	)
	switch {
	case in.Since > 0 && in.State != "" && provision.State(in.State) != provision.Available:
		return nil, httpcore.BadRequest("Arrivals are available titles",
			"Only an available title has arrived. Leave out the state, or ask for available ones.")
	case in.Since > 0:
		recs, err = s.store.ListTitlesAvailableSince(ctx, time.UnixMilli(in.Since))
	case in.State == "":
		return nil, httpcore.BadRequest("State required", "Choose a state to filter titles by.")
	default:
		recs, err = s.store.ListTitlesByState(ctx, provision.State(in.State))
	}
	if err != nil {
		return nil, err
	}
	on, err := s.titleChannels(ctx)
	if err != nil {
		return nil, err
	}
	out := &listOutput{}
	out.Body.Titles = make([]TitleDTO, 0, len(recs))
	for _, r := range recs {
		dto := toDTO(r)
		dto.Channels = on[r.Key]
		out.Body.Titles = append(out.Body.Titles, dto)
	}
	return out, nil
}
