import { createFileRoute } from "@tanstack/react-router";
import { FillerPage } from "@/filler";

// The clip catalog's filters live in the URL so a filtered/searched view is shareable and
// back-button-aware (the same deep-link contract as the channel-detail `?section=` tabs).
// Each field is narrowed to a known value / dropped, so a hand-typed or stale link can't
// push garbage into the filler list query.
type FillerSearch = {
  // Which section is showing. Absent = Catalog, so the default view has a clean URL and an
  // old bookmark still lands somewhere sensible.
  tab?: string;
  q?: string;
  kind?: string;
  audience?: string;
  untagged?: boolean;
};

const KINDS = ["commercial", "bumper", "station_id", "psa", "trailer", "interstitial"];
const AUDIENCES = ["kids", "family", "general", "late_night"];

// Browsing and previewing filler is readable by any authenticated user (it explains why a
// channel plays what it plays); the mutating actions inside are admin-gated, and the
// server enforces that regardless (§11, §19).
const FillerScreen = () => <FillerPage />;

const Route = createFileRoute("/_authed/filler/")({
  validateSearch: (search: Record<string, unknown>): FillerSearch => {
    const q = typeof search.q === "string" && search.q ? search.q : undefined;
    const kind = KINDS.includes(search.kind as string) ? (search.kind as string) : undefined;
    const audience = AUDIENCES.includes(search.audience as string) ? (search.audience as string) : undefined;
    const untagged = search.untagged === true || search.untagged === "true" ? true : undefined;
    // "incoming" and "sources" are the real alternatives; an unknown value falls back to the
    // catalog rather than rendering an empty page.
    //
    // ⚠ `discover` is deliberately NOT here any more (V35): finding clips is now something you
    // do to a source, so the tab is gone. An old bookmark carrying ?tab=discover therefore lands
    // on Catalog rather than on a blank panel — which is why the fallback is a fallback and not
    // a redirect.
    const tab = search.tab === "sources" || search.tab === "incoming" ? search.tab : undefined;
    return {
      ...(tab ? { tab } : {}),
      ...(q ? { q } : {}),
      ...(kind ? { kind } : {}),
      ...(audience ? { audience } : {}),
      ...(untagged ? { untagged } : {}),
    };
  },
  component: FillerScreen,
});

export type { FillerSearch };
export { Route };
