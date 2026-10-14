import type { FillerSourceDTO } from "@loomarr/api/models/fillerSourceDTO";
import type { ReactNode } from "react";

interface FillerSourcesProps {
  sources: FillerSourceDTO[];
  /**
   * ⚠ `total` (the catalog size) was here and is GONE. This header reads "N of M on" — the
   * mock's `svcOnLine` — and the catalog total belongs to the page header's `watchLine` pill,
   * which gets it from `GET /v1/filler/watch`. Two components reporting the same number is how
   * they start disagreeing.
   */
  /** Open the one source workspace; rows remain a compact index rather than expanding in place. */
  onSelect?: (source: FillerSourceDTO) => void;
  /** Selected row, so the index preserves context behind the source workspace. */
  selectedId?: string;
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
  /** Change provider policy without rewriting any child switch. */
  onToggleProvider?: (kind: "archive" | "youtube", enabled: boolean) => void;
  togglingProvider?: "archive" | "youtube" | null;
  /** Provider-local add flow, rendered with the provider it affects. */
  renderProviderSetup?: (provider: FillerSourceDTO) => ReactNode;
  /** Folder/library add flow, rendered inside the first-class Your files group. */
  renderLocalSetup?: ReactNode;
  error?: string | null;
  className?: string;
}

export type { FillerSourcesProps };
