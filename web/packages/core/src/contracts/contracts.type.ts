// FE-only view models with NO 1:1 API equivalent. Everything that used to mirror a DTO
// here (Clip, the proposal shapes, media/state enums) now uses the orval-generated type
// from @loomarr/api directly (§12) — mirroring a generated type is drift-risk for no gain
// (packages/api runs on RN too, §4.2).
//
// The ⌘K palette merges API search candidates + local channels + help links into one
// list, so its scope is a *superset* of the API's SearchScope — named PaletteScope to
// avoid colliding with the generated one.
type PaletteScope = "library" | "tmdb" | "clips" | "channels" | "help";

interface SearchResult {
  id: string;
  scope: PaletteScope;
  name: string;
  meta?: string;
  inLibrary?: boolean;
}

export type { PaletteScope, SearchResult };
