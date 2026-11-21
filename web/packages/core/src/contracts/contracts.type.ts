// FE-only view models with NO 1:1 API equivalent. Anything that has a DTO (Clip, the
// proposal shapes, media/state enums) uses the orval-generated type from @loomarr/api
// directly (§12), not a mirror here — mirroring a generated type is drift-risk for no gain
// (packages/api runs on RN too, §4.2).
//
// The ⌘K palette merges API search candidates + local channels + help links into one
// list, so its scope is a *superset* of the API's SearchScope — named PaletteScope to
// avoid colliding with the generated one.
type PaletteScope = "diagnostics" | "library" | "tmdb" | "clips" | "channels" | "help";

interface SearchResult {
  id: string;
  scope: PaletteScope;
  name: string;
  meta?: string;
  inLibrary?: boolean;
}

export type { PaletteScope, SearchResult };
