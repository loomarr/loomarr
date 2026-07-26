import type { PaletteScope, SearchResult } from "@loomarr/core";

// SearchResult / PaletteScope are FE view models (the ⌘K palette merges API candidates +
// channels + help — a superset of the generated SearchScope), so they live in core, not
// the generated client. The palette's prop interface (controlled + handlers) stays here.
interface SearchCommandProps {
  query: string;
  onQueryChange: (query: string) => void;
  results: SearchResult[];
  onSelect?: (result: SearchResult) => void;
  loading?: boolean;
  className?: string;
}

export type { PaletteScope, SearchCommandProps, SearchResult };
