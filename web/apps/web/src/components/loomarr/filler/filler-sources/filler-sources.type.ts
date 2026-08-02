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
  /**
   * Switch a source on or off (V35). ⚠ Off means Loomarr stops scanning, searching and
   * downloading from it — it does NOT remove clips already in the catalog, and the copy beside
   * the switch has to keep saying so.
   *
   * Absent renders no switches at all, which is how a caller that cannot mutate (or an install
   * where the routes are unavailable) shows the same rows without a dead control.
   */
  onToggleEnabled?: (id: string, enabled: boolean) => void;
  /** Which row's switch is in flight, so one toggle disables rather than the whole list. */
  toggling?: string | null;
  error?: string | null;
  className?: string;
}

export type { FillerSourcesProps };
