import { type ClipDTO, fillerApi } from "@loomarr/api";
import { useMemo } from "react";

// useFillerCatalog — a small id→clip lookup over the whole filler catalog, so the pin/
// exclude lists can render a saved id as its clip name + duration rather than a bare id.
// One list read, memoized into a Map. A miss (an id whose clip isn't in the catalog — e.g.
// a clip removed after being pinned) returns undefined and the list falls back to the id,
// so an override is never hidden. Intentionally NOT the search read the add-flow uses: this
// is the resolve-what's-already-chosen read, always on; that one is query-gated.
const useFillerCatalog = (): { resolve: (id: string) => ClipDTO | undefined; isLoading: boolean } => {
  const list = fillerApi.useListFiller();
  const clips = list.data?.status === 200 ? (list.data.data.clips ?? []) : [];

  // Re-derive when the catalog changes; `clips` is a fresh array each render but its
  // contents only change on a refetch, and a stale label until then is harmless. Keyed on
  // the fetched `list.data`, not `clips`.
  // biome-ignore lint/correctness/useExhaustiveDependencies: keyed on the fetched data
  const byId = useMemo(() => {
    const m = new Map<string, ClipDTO>();
    for (const c of clips) m.set(c.path, c);
    return m;
  }, [list.data]);

  return { resolve: (id) => byId.get(id), isLoading: list.isLoading };
};

export { useFillerCatalog };
