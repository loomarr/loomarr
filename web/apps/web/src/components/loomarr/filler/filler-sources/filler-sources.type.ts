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
  /**
   * Refresh one source now.
   *
   * ⚠ Takes the source ID, not its kind. It took `kind` while each kind had exactly one row;
   * V38c allows many folders and many libraries, so a kind would put every folder row into
   * "Fetching…" at once and hide which one was really running.
   */
  onFetch: (id: string) => void;
  /** Which row's fetch is in flight (by ID), so exactly one control shows a pending state. */
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
  /**
   * Forget a registered source (V37). ⚠ Only rows with `removable` — the config-backed
   * singletons are configuration, changed in Settings, and the server 409s a delete on them.
   *
   * ⚠ Removing forgets the REGISTRATION, never the clips: they are real files already tagged and
   * possibly pinned into a channel. The control's title has to keep saying so.
   */
  onRemove?: (id: string) => void;
  /** Which row's removal is in flight. */
  removing?: string | null;
  /**
   * Per-row search panel (V35b, the mock's `sv.showSearch`). Finding clips is something you do
   * TO a source, so the search belongs inside the row that owns it rather than in a detached
   * card below the list.
   *
   * ⚠ A render prop, not a `SourceSearch` import: this component stays presentational, and the
   * search's query/results/pagination state is the PAGE's (it owns the discover query and the
   * visible-id de-duplication). Returning `undefined` for a row renders no expander, which is
   * how only the searchable source gets one.
   *
   * ⚠ Today exactly one row is searchable — `remote`, which is where archive.org lives. The
   * mock draws archive as its own peer row (`searchable: id === 'archive'`); that is V37's
   * flattening, and this slot is keyed by row so it survives the change unaltered.
   */
  renderSearch?: (source: FillerSourceDTO) => ReactNode;
  error?: string | null;
  className?: string;
}

export type { FillerSourcesProps };
