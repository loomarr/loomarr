package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/images"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
)

// Channel watermark UPLOAD (#1512 phase 1d): a custom bug, which wins over the automatic image
// (the network's TMDB logo, then the generated Plate). The bug's shape is its alpha, so only PNG
// and WebP are accepted, sniffed from the bytes exactly as the icon upload does, and the image is
// stored with images.RoleWatermark: the renderer reads the ORIGINAL, never a JPEG rung.

var allowedWatermarkTypes = map[string]bool{"image/png": true, "image/webp": true}

type watermarkUploadForm struct {
	File huma.FormFile `form:"file" contentType:"image/png,image/webp" required:"true" doc:"The bug image: PNG or WebP with transparency, under 2 MB."`
}

type uploadWatermarkInput struct {
	ID      string `path:"id" example:"ch_abc123"`
	RawBody huma.MultipartFormFiles[watermarkUploadForm]
}

type uploadWatermarkOutput struct {
	Body struct {
		Image string `json:"image" doc:"The stored image hash, now the channel's policy.watermark.image"`
	}
}

func (s *Server) uploadChannelWatermark(ctx context.Context, in *uploadWatermarkInput) (*uploadWatermarkOutput, error) {
	if _, err := s.store.GetChannel(ctx, in.ID); err != nil {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	}
	file := in.RawBody.Data().File
	data, err := io.ReadAll(io.LimitReader(file, maxIconBytes+1))
	if err != nil {
		return nil, errBadRequest("Couldn't read the image", "The upload was incomplete. Try again.")
	}
	if len(data) > maxIconBytes {
		return nil, apiErr(http.StatusRequestEntityTooLarge, "Image too large", "Watermark images must be under 2 MB.")
	}
	if !allowedWatermarkTypes[iconContentType("", data)] {
		return nil, apiErr(http.StatusUnsupportedMediaType, "Unsupported image type",
			"Use a PNG or WebP with a transparent background.")
	}
	if s.images == nil {
		return nil, apiErr(http.StatusServiceUnavailable, "Images aren't available",
			"The image service isn't configured on this instance.")
	}
	img, err := s.images.Ingest(ctx, bytes.NewReader(data), images.IngestRequest{
		Role: images.RoleWatermark, Visibility: images.VisibilityMember, Origin: images.OriginUpload,
		OwnerKind: ownerKindChannel, OwnerID: in.ID,
	})
	if err != nil {
		return nil, apiErrWithCause(http.StatusInternalServerError, "Couldn't save the watermark",
			"Something went wrong storing the image. Try again.", err)
	}
	// Optimistic save: re-read and retry on a concurrent edit, as the icon upload does.
	for attempt := 0; ; attempt++ {
		ch, err := s.store.GetChannel(ctx, in.ID)
		if err != nil {
			return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
		}
		wm := schedule.WatermarkPolicy{}
		if ch.Policy.Watermark != nil {
			wm = *ch.Policy.Watermark
		}
		wm.Image = img.Hash
		ch.Policy.Watermark = &wm
		_, err = s.store.SaveChannel(ctx, ch)
		if err == nil {
			break
		}
		if !errors.Is(err, store.ErrChannelStale) || attempt == 3 {
			return nil, apiErrWithCause(http.StatusInternalServerError, "Couldn't update the channel",
				"The image was saved but linking it to the channel failed. Try again.", err)
		}
	}
	out := &uploadWatermarkOutput{}
	out.Body.Image = img.Hash
	return out, nil
}
