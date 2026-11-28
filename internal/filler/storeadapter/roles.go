package storeadapter

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// The store roles below are what the adapters read and write beyond the filler extension
// (records.Store). Each names only the methods its adapter calls.

type clipLister interface {
	ListClips(ctx context.Context, filter clipstore.ClipFilter) ([]clipstore.Clip, error)
}

type languageStore interface {
	clipLister
	SetClipLanguage(ctx context.Context, path, language string, at time.Time) error
	SetClipsRemoved(ctx context.Context, paths []string, at time.Time) (int, error)
}

type transcribeStore interface {
	clipLister
	SetClipTranscript(ctx context.Context, path, transcript string, at time.Time) error
}

type catalogStore interface {
	clipLister
	ListClipVariants(ctx context.Context) (map[string][]string, error)
}

type clipPathReader interface {
	GetClipByPath(ctx context.Context, path string) (clipstore.Clip, error)
}

type imageRefDeleter interface {
	DeleteImageRefs(ctx context.Context, ownerKind, ownerID string) error
}
