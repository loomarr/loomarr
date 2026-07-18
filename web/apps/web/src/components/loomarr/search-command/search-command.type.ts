import type { SearchResult, SearchScope } from "@loomarr/core";

// SearchResult/SearchScope data contracts live in @loomarr/core — shared with
// packages/fixtures and the future mobile app (§4.2). The palette's prop interface
// (controlled + handlers) stays here; the data types are re-exported for consumers.
interface SearchCommandProps {
  query: string;
  onQueryChange: (query: string) => void;
  results: SearchResult[];
  onSelect?: (result: SearchResult) => void;
  loading?: boolean;
  className?: string;
}

export type { SearchCommandProps, SearchResult, SearchScope };
