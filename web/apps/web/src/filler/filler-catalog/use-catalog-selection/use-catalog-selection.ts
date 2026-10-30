import type { ClipDTO } from "@loomarr/api/models/clipDTO";
import { useEffect, useRef, useState } from "react";

// The catalog's bulk selection (V35). ⚠ Deliberately NOT in the URL, unlike the filters: a selection
// is a transient intent about the rows in front of you, and a shared link that carried it would hand
// someone else a pre-armed destructive action over clips they never chose.
//
// `resultScope` identifies the result page being shown (filters, parent and page, but not the view).
// Route navigation can change that page without going through a control here, so the selection is
// cleared whenever the scope changes, rather than only in individual filter handlers. View is
// excluded by the caller on purpose: it changes presentation, not the rows on the page.
const useCatalogSelection = (resultScope: string) => {
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const previousResultScope = useRef(resultScope);

  useEffect(() => {
    if (previousResultScope.current === resultScope) return;
    previousResultScope.current = resultScope;
    setSelected(new Set());
  }, [resultScope]);

  const toggle = (hash: string) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (!next.delete(hash)) next.add(hash);
      return next;
    });
  const clear = () => setSelected(new Set());
  // "Select all" over the rows currently RENDERED (V35b). ⚠ Scoped to the rows on screen on purpose:
  // the bulk bar's next control is "Remove from catalog", and selecting clips hidden behind a filter
  // or another page would arm a destructive action over rows nobody looked at.
  //
  // It doubles as the un-select once everything shown is picked, so the control is not a one-way
  // door that needs a second button to undo.
  const allSelected = (rows: readonly ClipDTO[]) =>
    rows.length > 0 && rows.every((clip) => selected.has(clip.hash));
  const selectAll = (rows: readonly ClipDTO[]) =>
    setSelected(allSelected(rows) ? new Set() : new Set(rows.map((clip) => clip.hash)));

  return { selected, toggle, clear, allSelected, selectAll };
};

export { useCatalogSelection };
