package channels

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// /v1/household/watch-history — who watched what, for admins (#3010).
//
// The rows are the recorder's (internal/channels/viewing, history.go): one per person, device,
// channel and programme. Admin only, by the maintainer's rule: knowing who watches what is viewing
// history, and members are not shown a notice or their own. There is no retention; the DELETE is
// the only way rows go, apart from a person's going with them. Who is watching right now is
// `GET /v1/household/viewing`, which names everyone to an admin.

func (s *Handlers) registerWatchHistory(api huma.API) {
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "list-watch-history", Method: http.MethodGet, Path: "/v1/household/watch-history",
		Summary: "Watch history (admin)",
		Description: "Admin only. Who watched which programme on which channel and device, newest first. " +
			"Filter by person and by a time window; a viewing is included when it overlaps the window. " +
			"Counts only in-app playback. Kept until an admin clears it.",
		Tags: []string{"dashboard"},
	}, httpcore.RoleAdmin), s.listWatchHistory)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "clear-watch-history", Method: http.MethodDelete, Path: "/v1/household/watch-history",
		Summary:     "Clear the watch history (admin)",
		Description: "Admin only. Clears one person's watch history, or everyone's when `userId` is omitted.",
		Tags:        []string{"dashboard"},
	}, httpcore.RoleAdmin), s.clearWatchHistory)
}

type watchHistoryListInput struct {
	UserID string    `query:"userId" maxLength:"128" doc:"Only this person's viewings"`
	From   time.Time `query:"from" doc:"Only viewings still going at or after this time"`
	To     time.Time `query:"to" doc:"Only viewings that started before this time"`
	Limit  int       `query:"limit" minimum:"1" maximum:"500" default:"100" doc:"Page size"`
	Cursor string    `query:"cursor" maxLength:"256" doc:"Opaque cursor returned by the previous page"`
}

type watchHistoryEntryDTO struct {
	ID          string    `json:"id"`
	UserID      string    `json:"userId"`
	Name        string    `json:"name" doc:"The person's display name; empty if unknown"`
	Device      string    `json:"device" doc:"What the household calls the device: a paired device's name, or the browser and system"`
	ChannelID   string    `json:"channelId"`
	ChannelName string    `json:"channelName" doc:"The channel's name when the viewing began"`
	Programme   string    `json:"programme" doc:"The programme watched; empty when the guide had none"`
	StartedAt   time.Time `json:"startedAt"`
	EndedAt     time.Time `json:"endedAt" doc:"When the player was last seen; moves forward while the viewing continues"`
}

type watchHistoryListOutput struct {
	Body struct {
		Viewings   []watchHistoryEntryDTO `json:"viewings" doc:"Newest first"`
		NextCursor string                 `json:"nextCursor,omitempty" doc:"Pass as cursor for the next page; absent on the last page"`
	}
}

type watchHistoryClearInput struct {
	UserID string `query:"userId" maxLength:"128" doc:"Clear only this person's history; omit to clear everyone's"`
}

type watchHistoryClearOutput struct {
	Body struct {
		Deleted int `json:"deleted" doc:"How many viewings were removed"`
	}
}

func (s *Handlers) listWatchHistory(ctx context.Context, in *watchHistoryListInput) (*watchHistoryListOutput, error) {
	q := store.ViewingQuery{UserID: in.UserID, From: in.From, To: in.To, Limit: in.Limit + 1}
	if in.Cursor != "" {
		at, id, ok := parseWatchHistoryCursor(in.Cursor)
		if !ok {
			return nil, httpcore.BadRequest("Invalid cursor", "Pass the nextCursor from the previous page unchanged.")
		}
		q.AfterStartedAt, q.AfterID = at, id
	}
	rows, err := s.store.ListViewings(ctx, q)
	if err != nil {
		return nil, err
	}
	out := &watchHistoryListOutput{}
	if len(rows) > in.Limit {
		rows = rows[:in.Limit]
		last := rows[len(rows)-1]
		out.Body.NextCursor = strconv.FormatInt(last.StartedAt.Unix(), 10) + "." + last.ID
	}
	names := map[string]string{}
	if len(rows) > 0 {
		users, err := s.store.ListUsers(ctx)
		if err != nil {
			return nil, err
		}
		for _, u := range users {
			names[u.ID] = u.Name
		}
	}
	out.Body.Viewings = make([]watchHistoryEntryDTO, 0, len(rows))
	for _, v := range rows {
		out.Body.Viewings = append(out.Body.Viewings, watchHistoryEntryDTO{ID: v.ID, UserID: v.UserID,
			Name: names[v.UserID], Device: v.Device, ChannelID: v.ChannelID, ChannelName: v.ChannelName,
			Programme: v.Programme, StartedAt: v.StartedAt, EndedAt: v.EndedAt})
	}
	return out, nil
}

func parseWatchHistoryCursor(raw string) (time.Time, string, bool) {
	at, id, ok := strings.Cut(raw, ".")
	if !ok || id == "" {
		return time.Time{}, "", false
	}
	unix, err := strconv.ParseInt(at, 10, 64)
	if err != nil || unix <= 0 {
		return time.Time{}, "", false
	}
	return time.Unix(unix, 0), id, true
}

func (s *Handlers) clearWatchHistory(ctx context.Context, in *watchHistoryClearInput) (*watchHistoryClearOutput, error) {
	n, err := s.store.DeleteViewings(ctx, in.UserID)
	if err != nil {
		return nil, err
	}
	s.log.Info("watch history cleared", "event", "household.watch_history_cleared", "subsystem", "dashboard",
		"user_id", in.UserID, "deleted", n)
	out := &watchHistoryClearOutput{}
	out.Body.Deleted = n
	return out, nil
}
