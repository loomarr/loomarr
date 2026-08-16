import type { FillerSourceDTO } from "@loomarr/api/models/fillerSourceDTO";

interface SourcesPanelProps {
  // ⚠ `sourcesQuery` itself stays in filler-page: the page header reads the same list, and a
  // second `useListFillerSources` here would double-fetch it. `sources`/`sourcesError` are that
  // one query's shape, handed down rather than re-queried.
  //
  // ⚠ `total` (the catalog size) was here and is gone with it — the page header's `watchLine`
  // pill owns that number now, from `GET /v1/filler/watch`.
  sources: FillerSourceDTO[];
  /** `sourcesQuery.error?.detail`, folded into this panel's own error line alongside it. */
  sourcesError?: string | null;
}

export type { SourcesPanelProps };
