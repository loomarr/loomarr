import { type ClipDTO, fillerApi, unwrap } from "@loomarr/api";
import { useMemo } from "react";

// useFillerCatalog — a small id→clip lookup so the pin/exclude lists can render a saved id as its
// clip name + duration rather than a bare id. A miss (an id whose clip isn't in the catalog — e.g.
// a clip removed after being pinned) returns undefined and the list falls back to the id, so an
// override is never hidden.
//
// ⚠ **It asks for exactly the ids it holds** (`hashes`, §10 V51d) rather than loading the catalog
// and building a map. It used to do the latter, which paging made silently WRONG rather than
// merely wasteful: with a 100-row default page, an install with more clips than that would resolve
// the pins that happened to land on page one and render the rest as bare hashes — an override that
// looks like it has gone missing.
//
// ⚠ Held and removed clips are deliberately NOT opted into. A pinned clip that was later removed
// from the catalog SHOULD fail to resolve: the fallback renders the raw id, which is the honest
// signal that the override points at something no longer airable.
//
// Intentionally NOT the search read the add-flow uses: this is the resolve-what's-already-chosen
// read, always on; that one is query-gated.
const useFillerCatalog = (
  ids: string[],
): { resolve: (id: string) => ClipDTO | undefined; isLoading: boolean } => {
  // ⚠ Sorted and joined into the query key's own input, so re-ordering the same pins does not
  // look like a different request. TanStack keys on the params object, and `["a","b"]` and
  // `["b","a"]` are two cache entries for one answer.
  const hashes = [...new Set(ids)].sort();
  const list = fillerApi.useListFiller(
    { hashes, limit: Math.max(hashes.length, 1) },
    // Nothing pinned or excluded means nothing to resolve — and an empty `hashes` would read as
    // "no hash filter" on the wire, i.e. a full catalog page fetched to answer no question.
    { query: { enabled: hashes.length > 0 } },
  );
  const clips = unwrap(list.data, (b) => b.clips) ?? [];

  // Re-derive when the fetched data changes; `clips` is a fresh array each render but its
  // contents only change on a refetch. Keyed on the fetched `list.data`, not `clips`.
  // biome-ignore lint/correctness/useExhaustiveDependencies: keyed on the fetched data
  const byId = useMemo(() => {
    const m = new Map<string, ClipDTO>();
    for (const c of clips) m.set(c.hash, c);
    return m;
  }, [list.data]);

  return { resolve: (id) => byId.get(id), isLoading: list.isLoading };
};

export { useFillerCatalog };
