import type { DiscoveredClip } from "@loomarr/api";

interface DiscoverPanelProps {
  // Results the search returned, verbatim. ⚠ Typed as the generated DTO rather than
  // re-declared: a hand-mirrored shape is the first step toward showing something the API
  // did not say (contract 1:1).
  items: DiscoveredClip[];
  // Total is what the SOURCE matched, not items.length — an operator judging a search needs
  // the real number, and 54 hits shown 25 at a time is a different situation from 54 total.
  total: number;
  // The caveat the API sends once, about the whole search. Rendered as given rather than
  // hardcoded here, so the backend owns the wording.
  licenceNote?: string;
  query: string;
  onQueryChange: (q: string) => void;
  onSearch: () => void;
  // onAdd hands ids to the ingest path. Absent when this image cannot download, in which case
  // results are still worth showing — see the component.
  onAdd?: (ids: string[]) => void;
  searching?: boolean;
  searched?: boolean;
  className?: string;
}

export type { DiscoverPanelProps };
