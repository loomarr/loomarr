package httpcore

import "github.com/loomarr/loomarr/internal/filler/source"

// EmbedURL is a player for an item of the provider's, or "" when the provider offers none.
func EmbedURL(providers *source.Registry, kind, itemURL string) string {
	if linker, ok := providers.Linker(source.ID(kind)); ok {
		return linker.EmbedURL(itemURL)
	}
	return ""
}
