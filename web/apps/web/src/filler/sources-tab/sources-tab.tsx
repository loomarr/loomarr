import * as fillerApi from "@loomarr/api/endpoints/filler";
import { unwrap } from "@loomarr/api/unwrap";
import { SourcesPanel } from "../sources-panel";

// SourcesTab — where clips come from. A thin wrapper that owns the ONE query the Sources tab
// needs, so the shell no longer runs it on behalf of a tab that may not be showing.
//
// ⚠ Admin-only on the server, and the QUERY is gated as well as the tab (`enabled: isAdmin`
// used to live in the shell; the tab is only rendered for an admin, so mounting it IS the gate
// now). A member's request would 403 and fill their console with an error about a surface they
// cannot reach.
//
// ⚠ The page HEADER does not read this. It reads `GET /v1/filler/watch` — counts and verdict
// both. An earlier pass derived the header's health in the browser from this route and was
// wrong twice: it is admin-only, so a member's dot sat permanently grey on a working install,
// and the "every source dark / nothing for days" rule is domain logic that belongs where it can
// be tested against the store. That is why moving this query out of the shell costs the header
// nothing.
const SourcesTab = () => {
  const sourcesQuery = fillerApi.useListFillerSources();
  const sources = unwrap(sourcesQuery.data, (b) => b.sources) ?? [];

  return <SourcesPanel sources={sources} sourcesError={sourcesQuery.error?.detail} />;
};

export { SourcesTab };
