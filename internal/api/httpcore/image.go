package httpcore

// ImageDTO is one image as the frontend sees it.
type ImageDTO struct {
	Hash string `json:"hash" doc:"Content hash — the image's identity and its ETag"`
	Role string `json:"role" doc:"poster | backdrop | icon | thumb | logo — decides the width ladder"`
	// Width/Height are the ORIGINAL's. ⚠ Served so an <img> can carry real width/height
	// attributes: browsers derive aspect-ratio from them, which takes cumulative layout shift to
	// zero without the frontend guessing a shape per surface.
	Width  int `json:"width" doc:"The original's width in pixels"`
	Height int `json:"height" doc:"The original's height in pixels"`
	// Placeholder is a base64 ThumbHash (~25 bytes) — the blur preview rendered while the real
	// image loads. Carries alpha, which is why it is not a BlurHash.
	Placeholder string `json:"placeholder" doc:"Base64 ThumbHash for the low-quality placeholder"`
	DominantHex string `json:"dominantHex" doc:"Average colour, for tinting a skeleton"`
	Animated    bool   `json:"animated" doc:"True when the original carries motion; such images have one rendition"`
	// SrcSetAVIF may be empty while the AVIF job catches up — that is a normal state, not an
	// error, and the frontend omits the <source> rather than waiting.
	SrcSetAVIF string `json:"srcSetAvif" doc:"Same-origin srcset for AVIF; empty until the AVIF job has run"`
	SrcSetWebP string `json:"srcSetWebp" doc:"Same-origin srcset for WebP"`
	Src        string `json:"src" doc:"A same-origin path for the <img> fallback: JPEG, or WebP for a logo so its transparency survives"`
}
