package scan

import (
	"context"
	"log/slog"
	"time"

	"github.com/loomarr/loomarr/internal/library/tmdb"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// PersonPhotosInterval is how often people newly due a photo are looked for.
const PersonPhotosInterval = 5 * time.Minute

// personPhotoPage is how many people one read of the due list returns.
const personPhotoPage = 200

// PersonPhotoStore lists the people whose photo is to be imported.
type PersonPhotoStore interface {
	DueLibraryPersonPhotos(ctx context.Context, after string, limit int) ([]librarystore.LibraryPersonPhoto, error)
	SettleLibraryPersonPhotos(ctx context.Context) error
}

// PersonArtwork records a person's photo in the image service, to be fetched later. source is the
// URL held for them, fetch the one to download.
type PersonArtwork interface {
	PersonPhoto(ctx context.Context, personID, source, fetch string) error
}

// PersonPhotos adopts the cast and crew photos title pages show (#4476). It fetches nothing: the
// image service's fetch job pulls an adopted photo, and backs off when a download fails.
type PersonPhotos struct {
	Store   PersonPhotoStore
	Artwork PersonArtwork
	// Enabled is the scanner's kill switch; nil is on.
	Enabled func() bool
	Log     *slog.Logger
}

// Run adopts until ctx ends.
func (p *PersonPhotos) Run(ctx context.Context) {
	tick := time.NewTicker(PersonPhotosInterval)
	defer tick.Stop()
	for {
		if err := p.Tick(ctx); err != nil && ctx.Err() == nil {
			p.log().Warn("library person photos: tick failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// Tick adopts every photo due. One that cannot be adopted is passed over and tried next tick.
func (p *PersonPhotos) Tick(ctx context.Context) error {
	if p.Enabled != nil && !p.Enabled() {
		return nil
	}
	after := ""
	for {
		due, err := p.Store.DueLibraryPersonPhotos(ctx, after, personPhotoPage)
		if err != nil {
			return err
		}
		if len(due) == 0 {
			return p.Store.SettleLibraryPersonPhotos(ctx)
		}
		for _, d := range due {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := p.Artwork.PersonPhoto(ctx, d.PersonID, d.URL, tmdb.SmallProfile(d.URL)); err != nil {
				p.log().Warn("library person photos: photo not imported", "person", d.PersonID, "err", err)
			}
			after = d.PersonID
		}
	}
}

func (p *PersonPhotos) log() *slog.Logger {
	if p.Log != nil {
		return p.Log
	}
	return slog.Default()
}
