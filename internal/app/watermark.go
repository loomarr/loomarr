package app

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/media/images"
	"github.com/loomarr/loomarr/internal/platform/diagnostics"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/playout/channelbug"
)

// channelWatermarks is the channel-bug service (channelbug.Service) wired to the install's settings,
// with the one method whose answer is an API type: the channel page's options.
type channelWatermarks struct{ *channelbug.Service }

// newChannelWatermarks builds the service before the channel packager that asks it for bugs. The
// image service is built later and bound with withOriginals. Rendered bugs are derived files, kept
// beside the image store rather than inside it (the image GC owns that tree).
func newChannelWatermarks(rootCtx context.Context, st store.Store, set resolved, calendar func() schedule.Calendar, events *diagnostics.Recorder, log *slog.Logger) *channelWatermarks {
	cfg := channelbug.Config{
		FFmpeg:   settings.PlayoutFFmpegPath.Get(set),
		Dir:      filepath.Join(filepath.Dir(filepath.Clean(settings.ImagesDir.Get(set))), "watermarks"),
		Install:  watermarkInstall(set),
		Calendar: calendar,
		Channels: st,
		Log:      log, Lifetime: rootCtx,
	}
	if events != nil {
		cfg.Events = events
	}
	return &channelWatermarks{channelbug.New(cfg)}
}

// watermarkInstall is the install-wide bug settings: playout.watermark_opacity_pct as a fraction
// and playout.watermark_look, resolved per call so a change applies to the next programme item
// without a restart (the rendered-bug cache is keyed by both, so a new value re-renders).
func watermarkInstall(set resolved) func() schedule.WatermarkInstall {
	return func() schedule.WatermarkInstall {
		return schedule.WatermarkInstall{Opacity: float64(settings.PlayoutWatermarkOpacityPct.Get(set)) / 100,
			Look: settings.PlayoutWatermarkLook.Get(set)}
	}
}

// withOriginals binds the image service a custom upload is read from. The build calls it before the
// server takes a request, so no tune can race it; unbound, a channel's bug is the generated Plate.
func (c *channelWatermarks) withOriginals(imgs *images.Service) {
	if imgs != nil {
		c.SetOriginals(imgs)
	}
}

// Options is what the channel page's On-screen logo section shows beside the choices at now.
func (c *channelWatermarks) Options(ctx context.Context, channelID string, now time.Time) (api.WatermarkOptions, error) {
	got, err := c.Service.Options(ctx, channelID, now)
	if err != nil {
		return api.WatermarkOptions{}, err
	}
	out := api.WatermarkOptions{AutoCallsign: got.AutoCallsign, Era: got.Era, EraPalette: got.EraPalette,
		Holidays: []api.WatermarkHolidayOption{}}
	for _, h := range got.Holidays {
		row := api.WatermarkHolidayOption{ID: h.ID, Name: h.Name, Start: h.Start, End: h.End, Active: h.Active, Picture: h.Picture}
		for _, p := range h.Pictures {
			row.Pictures = append(row.Pictures, api.WatermarkPicture{ID: p.ID, Name: p.Name})
		}
		out.Holidays = append(out.Holidays, row)
	}
	return out, nil
}
