package filler

import "errors"

var (
	ErrInvalidSourceReference = errors.New("invalid filler source reference")
	ErrSourceProvider         = errors.New("filler source provider unavailable")
)

// SourceSuggestion is a transient provider result or an exact resolved target. It is not a
// FillerSource and grants no persistence, download, or background-work authority.
type SourceSuggestion struct {
	Provider     string
	TargetType   string
	CanonicalID  string
	CanonicalURL string
	Title        string
	Description  string
	ItemCount    int
	PreviewItems []SourcePreviewItem
}

// SourcePreviewItem is a provider-native example returned while resolving a source. It is
// transient evidence for a person choosing a source, never an acquired or approved clip.
type SourcePreviewItem struct {
	Title      string
	URL        string
	DurationMS int64
}
