import { createFileRoute, redirect } from "@tanstack/react-router";
import { FillerPage } from "@/filler";

// The clip catalog's filters live in the URL so a filtered/searched view is shareable and
// back-button-aware (the same deep-link contract as the channel-detail section routes).
// Each field is narrowed to a known value / dropped, so a hand-typed or stale link can't
// push garbage into the filler list query.
//
// ⚠ `tab` is GONE from this type (V-nav-paths): which section shows is now the PATH
// (`/filler`, `/filler/incoming`, `/filler/sources`), matching Settings. These four stay
// query params because they are catalog FILTERS, not navigation — a filtered/searched view
// needs to be shareable independent of which section it was reached from.
type FillerSearch = {
  q?: string;
  kind?: string;
  audience?: string;
  untagged?: boolean;
  // Grid (default, absent) or the dense list (V35b). In the URL for the same reason the
  // filters are: a shared link should show what the sender was looking at. ⚠ Selection is
  // deliberately NOT here — see the note on `selected` in filler-page.
  view?: string;
  // Which page of the catalog (§10 V51d), 1-based. In the URL because a pager that the back
  // button cannot undo is a trap: paging to 7 and pressing back should return to 6, not leave
  // the catalog entirely.
  //
  // ⚠ Dropped when it is 1, like `view: "grid"`, so the common link stays clean. ⚠ `pageSize`
  // is deliberately NOT here — it is one constant for both views, so switching grid/list cannot
  // renumber the pages under the operator.
  page?: number;
  // A composite's content hash. Present means the catalog is showing the airable segments filed
  // beneath that compilation; absent means the normal top-level catalog hierarchy.
  parent?: string;
};

const KINDS = ["commercial", "bumper", "station_id", "psa", "trailer", "interstitial"];
const AUDIENCES = ["kids", "family", "general", "late_night"];

const validateCatalogSearch = (search: Record<string, unknown>): FillerSearch => {
  const q = typeof search.q === "string" && search.q ? search.q : undefined;
  const kind = KINDS.includes(search.kind as string) ? (search.kind as string) : undefined;
  const audience = AUDIENCES.includes(search.audience as string) ? (search.audience as string) : undefined;
  const untagged = search.untagged === true || search.untagged === "true" ? true : undefined;
  // Only "list" is carried; "grid" is the default and stays out of the URL so the common
  // view has a clean link. An unknown value falls back to the grid rather than erroring.
  const view = search.view === "list" ? "list" : undefined;
  // ⚠ Clamped to a whole number ≥ 1 and dropped when it is 1. A hand-typed `?page=0` or
  // `?page=abc` must land on page one rather than sending `offset=-60` (a 422) or NaN.
  const rawPage = Number(search.page);
  const page = Number.isFinite(rawPage) && rawPage > 1 ? Math.floor(rawPage) : undefined;
  const parent = typeof search.parent === "string" && search.parent.length <= 128 ? search.parent : undefined;
  return {
    ...(q ? { q } : {}),
    ...(kind ? { kind } : {}),
    ...(audience ? { audience } : {}),
    ...(untagged ? { untagged } : {}),
    ...(view ? { view } : {}),
    ...(page ? { page } : {}),
    ...(parent ? { parent } : {}),
  };
};

// Browsing and previewing filler is readable by any authenticated user (it explains why a
// channel plays what it plays); the mutating actions inside are admin-gated, and the
// server enforces that regardless (§11, §19).
const FillerScreen = () => <FillerPage tab="catalog" />;

const Route = createFileRoute("/_authed/filler/")({
  // ⚠ The legacy `?tab=` carrier. It is NARROWED here and redirected in `beforeLoad` below —
  // never dropped, or the redirect would have nothing to read.
  validateSearch: (search: Record<string, unknown>): FillerSearch & { tab?: "incoming" | "sources" } => ({
    ...validateCatalogSearch(search),
    ...(search.tab === "incoming" || search.tab === "sources" ? { tab: search.tab } : {}),
  }),
  // ⚠ `?tab=incoming` / `?tab=sources` are old bookmarks and shared links (pre-nav-paths). The
  // section is a path segment now, so they redirect to it, carrying every other search param
  // through unchanged.
  //
  // ⚠ **In `beforeLoad`, NOT in `validateSearch`.** Throwing a `redirect` from the validator
  // does not navigate — the router is still parsing the URL at that point and treats the throw
  // as a parse failure, so the page rendered BLANK at the old address. That is the exact
  // bookmark-compatibility promise this code exists to keep, and every unit test passed while
  // it was broken; only loading the URL in a browser showed it.
  beforeLoad: ({ search }) => {
    if (search.tab) {
      const { tab, ...rest } = search;
      throw redirect({ to: `/filler/${tab}`, search: rest });
    }
  },
  component: FillerScreen,
});

export type { FillerSearch };
export { Route, validateCatalogSearch };
