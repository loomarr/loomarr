package api

import (
	"context"
	"strconv"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/provision"
)

// attachTitleArt sets each airing's LogoImage and BackdropImage from the Library (#2567): the art
// of the title its provisioning key names, so an episode takes its show's. One store read and one
// image read per distinct title for every airing passed, however many channels they span.
//
// An image still being fetched has no bytes and is left off, like a title with none: the player
// draws its plain card rather than a broken one.
func (s *Server) attachTitleArt(ctx context.Context, rows ...[]GuideAiring) {
	if s.store == nil || s.images == nil {
		return
	}
	idOf := map[provision.Key]store.LibraryProviderID{}
	ids := []store.LibraryProviderID{}
	for _, airings := range rows {
		for _, a := range airings {
			if _, seen := idOf[a.titleKey]; seen || a.titleKey == "" {
				continue
			}
			mt, namespace, n, ok := provision.ParseKey(a.titleKey)
			if !ok {
				continue
			}
			id := store.LibraryProviderID{Kind: string(mt), Namespace: namespace, Value: strconv.Itoa(n)}
			idOf[a.titleKey] = id
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return
	}
	art, err := s.store.LibraryTitleArt(ctx, ids)
	if err != nil {
		s.log.Warn("guide: title art lookup failed", "err", err)
		return
	}
	images := map[string]*httpcore.ImageDTO{}
	image := func(hash string) *httpcore.ImageDTO {
		if hash == "" {
			return nil
		}
		if dto, seen := images[hash]; seen {
			return dto
		}
		dto := s.libraryImage(ctx, hash)
		images[hash] = dto
		return dto
	}
	for _, airings := range rows {
		for i := range airings {
			id, ok := idOf[airings[i].titleKey]
			if !ok {
				continue
			}
			a := art[id]
			airings[i].LogoImage = image(a.Logo)
			airings[i].BackdropImage = image(a.Backdrop)
		}
	}
}
