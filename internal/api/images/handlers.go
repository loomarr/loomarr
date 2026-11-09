// Package images serves the image service's HTTP surface (§22): a rendition's bytes, an image's
// record and uploads. It also renders image records for the other routes that embed them.
package images

import (
	"log/slog"

	"github.com/danielgtaylor/huma/v2"
)

// Deps is what the image routes need from the rest of the server. Images is optional: nil answers
// the byte route 404 and the record route absent, the honest answer where the service is not wired.
type Deps struct {
	Log    *slog.Logger
	Images ImageService
}

// Handlers serves the image routes.
type Handlers struct {
	log    *slog.Logger
	images ImageService
}

// New returns the image handlers over d.
func New(d Deps) *Handlers { return &Handlers{log: d.Log, images: d.Images} }

// Register mounts every image route on api.
func (s *Handlers) Register(api huma.API) { s.registerImages(api) }
