package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
)

// Channel watermark PREVIEW (#1659 W2): the channel page's On-screen logo preview shows the PNG the
// renderer would air, never a client-side redraw that can drift from it. The backend is the source.

// WatermarkPreviewer draws a channel's bug with its saved policy and the given changes: the rendered
// PNG's path, and whether this host can overlay a bug at all right now.
type WatermarkPreviewer interface {
	Preview(ctx context.Context, channelID string, change schedule.WatermarkPolicy, holiday string) (path string, drawable bool, err error)
	// Options is what the section shows beside the choices at now (WatermarkOptions).
	Options(ctx context.Context, channelID string, now time.Time) (WatermarkOptions, error)
}

type watermarkPreviewInput struct {
	ID      string  `path:"id" example:"ch_abc123"`
	Look    string  `query:"look" enum:"plate,text,outline,small-plate,tint,twotone,badge,lockup,glass,stripe" doc:"Style to draw; omitted is the channel's"`
	Color   string  `query:"color" enum:"era,amber,cyan,green,red,pink,yellow" doc:"Colour to draw; omitted is the channel's"`
	Holiday string  `query:"holiday" enum:"halloween,thanksgiving,christmas,newyear,valentines" doc:"Holiday version to draw; omitted is the everyday logo"`
	Size    float64 `query:"size" minimum:"0" maximum:"0.15" doc:"Bug height as a share of the frame height; omitted (0) is the channel's"`
	Picture string  `query:"picture" enum:"pumpkin,bat,ghost,leaf,holly,snowflake,firework,heart" doc:"The holiday's picture to draw, one of its own; omitted is the channel's"`
}

// watermarkDrawableHeader carries the host's overlay state beside the PNG, so the page learns both
// from one request: "false" while no encoder's self-check has passed.
const watermarkDrawableHeader = "X-Loomarr-Watermark-Drawable"

func (s *Server) registerWatermarkPreview(api huma.API) {
	rawInputOp(api, huma.Operation{
		OperationID: "preview-channel-watermark", Method: http.MethodGet, Path: "/v1/channels/{id}/watermark/preview",
		Summary: "Preview the channel's on-screen logo",
		Description: "Admin only. The PNG the playout renderer would overlay for this channel on a 1080-line frame: " +
			"its saved policy.watermark with the query's look, colour and size, in the named holiday's version. " +
			"The `" + watermarkDrawableHeader + "` header is false while this server cannot overlay a logo.",
		Tags: []string{"channels"},
		Responses: map[string]*huma.Response{"200": {Description: "The bug PNG",
			Content: map[string]*huma.MediaType{"image/png": {}}}},
	}, RoleAdmin, func(w http.ResponseWriter, r *http.Request, in *watermarkPreviewInput) {
		if s.watermarkPreview == nil {
			s.writeProblem(w, r, http.StatusNotImplemented, "Preview unavailable", "This server has no logo renderer.")
			return
		}
		change := schedule.WatermarkPolicy{Look: in.Look, Color: in.Color}
		if in.Size > 0 {
			change.Size = &in.Size
		}
		if in.Picture != "" && in.Holiday != "" {
			change.Motifs = map[string]string{in.Holiday: in.Picture}
		}
		path, drawable, err := s.watermarkPreview.Preview(r.Context(), in.ID, change, in.Holiday)
		if errors.Is(err, store.ErrNotFound) {
			s.writeProblem(w, r, http.StatusNotFound, "Channel not found", "")
			return
		}
		if err != nil {
			s.log.Warn("watermark preview failed", "channel", in.ID, "err", err)
			s.writeProblem(w, r, http.StatusUnprocessableEntity, "Logo not drawn", "The logo could not be drawn.")
			return
		}
		body, err := os.ReadFile(path)
		if err != nil {
			s.writeProblem(w, r, http.StatusInternalServerError, "Logo not read", "The drawn logo could not be read.")
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set(watermarkDrawableHeader, strconv.FormatBool(drawable))
		_, _ = w.Write(body)
	})
}

// WatermarkOptions is what the channel page's On-screen logo section shows beside the choices (#1659
// W2c), all from the backend so the page never re-derives the calendar, the era or the name.
type WatermarkOptions struct {
	AutoCallsign string                   `json:"autoCallsign" doc:"Text the logo shows when the name is left blank, derived from the channel name"`
	Era          string                   `json:"era" doc:"The channel's decade from its Programming era, such as 1980s; empty when it has none"`
	EraPalette   []string                 `json:"eraPalette" doc:"The colours \"Match the era\" uses, in order (amber, cyan, green without an era)"`
	Holidays     []WatermarkHolidayOption `json:"holidays" doc:"The holidays this channel honours, in calendar order; empty when it ignores holidays"`
}

// WatermarkHolidayOption is one holiday the channel's logo follows.
type WatermarkHolidayOption struct {
	ID       string             `json:"id" example:"halloween"`
	Name     string             `json:"name" example:"Halloween"`
	Start    string             `json:"start" format:"date" doc:"First day of the holiday's window that is on today or comes next"`
	End      string             `json:"end" format:"date" doc:"Last day of that window, inclusive"`
	Active   bool               `json:"active" doc:"The holiday is in effect today for this channel: its logo shows the holiday version"`
	Picture  string             `json:"picture" doc:"The picture this channel shows for the holiday: its choice, else the default"`
	Pictures []WatermarkPicture `json:"pictures" doc:"The holiday's pictures, the default first; one means there is no choice"`
}

// WatermarkPicture is a holiday picture's id and plain name.
type WatermarkPicture struct {
	ID   string `json:"id" example:"pumpkin"`
	Name string `json:"name" example:"Pumpkin"`
}

type watermarkOptionsOutput struct {
	Body WatermarkOptions
}

func (s *Server) registerWatermarkOptions(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "get-channel-watermark-options", Method: http.MethodGet, Path: "/v1/channels/{id}/watermark/options",
		Summary:     "On-screen logo options for a channel",
		Description: "Admin only. The name the logo shows when none is set, the channel's era and its colours, and the holidays the logo follows: each one's dates, whether it is in effect today and its pictures.",
		Tags:        []string{"channels"},
	}, RoleAdmin), func(ctx context.Context, in *channelIDInput) (*watermarkOptionsOutput, error) {
		if s.watermarkPreview == nil {
			return nil, errNotImplemented("Logo options unavailable", "This server has no logo renderer.")
		}
		opts, err := s.watermarkPreview.Options(ctx, in.ID, s.clock())
		if errors.Is(err, store.ErrNotFound) {
			return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
		}
		if err != nil {
			return nil, err
		}
		return &watermarkOptionsOutput{Body: opts}, nil
	})
}
