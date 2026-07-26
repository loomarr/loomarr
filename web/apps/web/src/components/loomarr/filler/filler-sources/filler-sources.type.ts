import type { FillerSourceDTO } from "@loomarr/api";

interface FillerSourcesProps {
  sources: FillerSourceDTO[];
  /**
   * Catalog size. Sent by the server rather than summed from `sources` because a clip whose
   * provenance matches no row still belongs to the catalog — summing the rows under-reports.
   */
  total: number;
  onFetch: (kind: string) => void;
  /** Which row's fetch is in flight, so exactly one control shows a pending state. */
  fetching?: string | null;
  error?: string | null;
  className?: string;
}

export type { FillerSourcesProps };
