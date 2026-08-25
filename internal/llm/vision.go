package llm

import "context"

// Image input over the chat API (§8/§10 V44 vision tier). The visual tier reads a clip
// nobody narrates — a wordless spot whose only signal is an on-screen logo, visible text,
// or a black-and-white transfer that dates it — and asks a model for the brand/category/
// visibleText it can literally SEE. It is grounded exactly as text tags are: a brand the
// frame does not show is dropped, never persisted (the era rule, generalised — §8, §10).
//
// This mirrors audio.go's separate-method design (`AskAboutImages`, not a flag on `Chat`),
// and for the same reason: the multimodal request shape is genuinely different (content
// becomes an ARRAY of typed parts on the hosted path), and forcing that into the text path
// would complicate every existing caller for one feature. The one deliberate exception is
// Ollama, whose per-message `images` field lives ON the shared `Chat` wire type — the single
// V44 change to the hot path, and the one guarded by a test proving an image-free request
// serialises byte-for-byte as before.

// VisionProvider is the narrow multimodal seam the vision-tagging job calls (§10 V44). It is
// deliberately SEPARATE from Provider rather than a method on it: Provider models the grounded
// tool-calling chat loop, and this is a single stateless question about a handful of frames
// that wants none of Tools/JSONMode/streaming. Both providers implement it — the hosted
// OpenAI-compatible path (frames as `image_url` data URIs) and the local Ollama path (frames
// in the per-message `images` array) — so a fully-local install gets visual tagging too,
// without egress or per-clip cost (§14).
//
// Callers type-assert for it, the same way they type-assert for Warmer: a provider that does
// not implement it simply has no vision, and the job falls back to the text-only tiers.
type VisionProvider interface {
	// AskAboutImages asks one question about one clip's keyframes and returns the model's
	// answer as a Response (the same envelope Chat returns, so the caller parses grounded
	// tags out of Content identically to a text tagging turn — vision never uses ToolCalls).
	//
	// The jpegs are bounded near-full-resolution semantic frames from ffmpeg (§10 V61),
	// NOT the 320px presentation artwork or 9×8 grayscale dHash frames. They are sent
	// INLINE as base64; the evidence router selects only the frames a claim still needs.
	AskAboutImages(ctx context.Context, prompt string, jpegs [][]byte) (Response, error)
}

// --- hosted wire types (OpenAI-compatible multimodal content parts) ---
//
// A separate request type rather than making `openaiMessage.Content` an `any`: that field is
// marshalled on every text request in the app, and widening its type to serve one caller would
// put a runtime-typed field on the hot path for no benefit there (the same call audio.go makes).

// visionChatReq mirrors openaiChatReq but with multimodal content parts.
type visionChatReq struct {
	Model     string          `json:"model"`
	Messages  []visionMessage `json:"messages"`
	MaxTokens int             `json:"max_tokens,omitempty"`
}

type visionMessage struct {
	Role    string       `json:"role"`
	Content []visionPart `json:"content"`
}

// visionPart is one element of a multimodal content array. Exactly one of Text/ImageURL is
// set, selected by Type — the shape OpenAI defined and every compatible gateway copies.
type visionPart struct {
	Type     string           `json:"type"` // "text" | "image_url"
	Text     string           `json:"text,omitempty"`
	ImageURL *visionPartImage `json:"image_url,omitempty"`
}

type visionPartImage struct {
	// URL is a FULL data URI ("data:image/jpeg;base64,<...>"), unlike the audio part next to
	// it in the same spec, which takes bare base64. Getting this wrong produces a 400 that
	// names neither field — see audio.go's audioPartAudio for the mirror-image warning.
	URL string `json:"url"`
}

// Compile-time proof both providers satisfy the vision seam. (Placed here so the two impls,
// which live in openai.go and ollama.go, are asserted against the interface that defines them.)
var (
	_ VisionProvider = (*OpenAI)(nil)
	_ VisionProvider = (*Ollama)(nil)
)
