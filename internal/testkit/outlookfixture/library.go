// Package outlookfixture provides shared read-only library observations for tests.
package outlookfixture

import (
	"context"
	"maps"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/schedule"
)

// OutlookLibrary supplies read-only metadata and episodes for proposal observation
// tests. Its recorded requests make accidental extra resolution visible.
type OutlookLibrary struct {
	Metadata      map[string]library.ItemMetadata
	MetadataError error
	Episodes      map[string][]schedule.ResolvedProgram
	EpisodeError  error
	MetadataReads [][]string
	EpisodeReads  []string
}

func (l *OutlookLibrary) ItemMetadataByID(_ context.Context, ids []string) (map[string]library.ItemMetadata, error) {
	l.MetadataReads = append(l.MetadataReads, append([]string(nil), ids...))
	return maps.Clone(l.Metadata), l.MetadataError
}

func (l *OutlookLibrary) ResolveEpisodes(_ context.Context, id string) ([]schedule.ResolvedProgram, error) {
	l.EpisodeReads = append(l.EpisodeReads, id)
	return append([]schedule.ResolvedProgram(nil), l.Episodes[id]...), l.EpisodeError
}
