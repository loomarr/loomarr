import * as proposalsApi from "@loomarr/api/endpoints/proposals";
import * as titlesApi from "@loomarr/api/endpoints/titles";
import type { TitleDTO } from "@loomarr/api/models/titleDTO";
import { TitleDTOState } from "@loomarr/api/models/titleDTOState";
import { unwrap } from "@loomarr/api/unwrap";
import { pluralize } from "@loomarr/core/format";
import { useQueries } from "@tanstack/react-query";
import { createFileRoute, Link, Outlet, useLocation } from "@tanstack/react-router";
import { useAuth } from "@/auth/use-auth";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { NavTabs } from "@/components/ui/nav-tabs";
import { useDocumentTitle } from "@/lib/use-document-title";
import { journeyProgress } from "@/queue/journey";

// Queue / My requests (§12, §13) — where a member watches their submission land.
// It deliberately leads with the JOURNEY ("4 of 7 have landed") rather than a table of
// provisioning states, because the member framing §13 asks for is "where is my stuff",
// not "what does `requested` mean". Live via `title` SSE.
//
// The three tabs the v2 mock draws on Queue (`navDefs` hangs a pending-proposal badge off this
// nav entry, which is why the approvals surface lives HERE rather than as its own route).
// ⚠ Each is its own PATH now (V-nav-paths: `/queue/approval`, `/queue/flight`,
// `/queue/history` — it used to be `?tab=`), matching Settings.

// The Queue needs every provisioning state at once (§4) to collapse them into the three
// journey stages the HEADER reports ("N of M have landed"), shown on every tab regardless of
// which one is active — so this layout owns that fetch, not any one panel. GET /v1/titles is a
// single-state FILTER, though — it 400s without a `state` (a deliberate, §7-pinned contract:
// `TestListRequiresState`, so an unbounded "list all titles" can't be issued by accident) — so
// we fan out one query per state and merge. This is the aggregation the endpoint intentionally
// leaves to the client.
//
// ⚠ The flight panel (`flight.tsx`) fetches these SAME queries again for its own list — that is
// not a duplicate network call: React Query dedupes by key, so both reads share one cache entry.
// Keeping the fetch local to whoever renders from it (rather than threading it through context)
// matches how Settings' leaves each call `useSettingsEntries()` independently.
const STATES = Object.values(TitleDTOState);

const TAB_IDS = ["approval", "flight", "history"] as const;
type TabId = (typeof TAB_IDS)[number];

const QueueLayout = () => {
  useDocumentTitle("Queue");
  const { isAdmin } = useAuth();
  // ⚠ From `useLocation`, not `?tab=`: which panel is active is now the PATH (V-nav-paths).
  const { pathname } = useLocation();

  // Pending proposals drive BOTH the approval tab's list and its count, so the two cannot
  // disagree — a tab reading "3" above a list of two is worse than no count. Members never see
  // this tab (approving is admin-only, §11), so the query is gated rather than 403ing in the
  // background on every member's page load.
  const pending = proposalsApi.useListProposals(
    { status: "submitted" },
    { query: { enabled: isAdmin, retry: false } },
  );
  const pendingCount = unwrap(pending.data, (b) => b.proposals?.length) ?? 0;

  const decided = useQueries({
    queries: (["approved", "denied"] as const).map((status) =>
      proposalsApi.getListProposalsQueryOptions({ status }),
    ),
  });
  const historyCount = decided.reduce((n, q) => n + (unwrap(q.data, (b) => b.proposals?.length) ?? 0), 0);
  const stateQueries = useQueries({
    queries: STATES.map((state) => titlesApi.getListTitlesQueryOptions({ state })),
  });

  // All-or-nothing: if ANY state query fails, show the error rather than a board that
  // silently omits a stage — a partial set would make journeyProgress misreport how far
  // along the member actually is.
  const failed = stateQueries.find((q) => q.error);
  if (failed) {
    return (
      <div className="p-6">
        <ErrorState
          error={failed.error}
          onRetry={() => {
            for (const q of stateQueries) q.refetch();
          }}
        />
      </div>
    );
  }

  const rows: TitleDTO[] = stateQueries.flatMap((q) => unwrap(q.data, (b) => b.titles) ?? []);
  const progress = journeyProgress(rows);

  // Members get ONE tab: approving is admin-only (§11), and a history of other people's
  // decisions is not theirs to read. Rendering the bar with a single tab keeps the page's shape
  // stable rather than hiding it entirely for half the users.
  // ⚠ Each tab is a real DESTINATION, so `NavTabs` renders them as links. They were buttons
  // calling `navigate()` — identical to look at, but not middle-clickable, not copyable as a
  // link, and announced to assistive tech as an action rather than a place.
  const tabs = isAdmin
    ? [
        { id: "approval", label: "Needs approval", to: "/queue/approval", count: pendingCount },
        { id: "flight", label: "In flight", to: "/queue/flight", count: rows.length },
        { id: "history", label: "History", to: "/queue/history", count: historyCount },
      ]
    : [{ id: "flight", label: "In flight", to: "/queue/flight", count: rows.length }];

  // The last path segment IS the tab id (`/queue/approval` → "approval"). Falls back to
  // "flight" for a stale/unknown segment rather than rendering a blank panel — the same
  // narrowing `validateSearch` used to do for `?tab=`.
  const activeId: TabId = (TAB_IDS.find((id) => id === pathname.split("/").pop()) ?? "flight") as TabId;

  return (
    <div className="flex h-full flex-col">
      <PageHeader
        title="Queue"
        description={
          rows.length > 0
            ? `${progress.ready} of ${pluralize(progress.total, "title")} ${progress.total === 1 ? "has" : "have"} landed. Channels play what's ready and fill the rest, improving as more arrives.`
            : undefined
        }
      />

      {/* The three tabs (V27). Members see only In flight — approving is admin-only (§11) and a
          history of other people's decisions is not theirs to read, so the bar renders with one
          tab rather than two they would be refused. */}
      <div className="px-6">
        <NavTabs label="Queue sections" linkComponent={Link} tabs={tabs} activeId={activeId} />
      </div>

      <div
        id={`panel-${activeId}`}
        role="tabpanel"
        aria-labelledby={`tab-${activeId}`}
        className="flex flex-1 flex-col gap-6 overflow-auto p-6"
      >
        <Outlet />
      </div>
    </div>
  );
};

const Route = createFileRoute("/_authed/queue")({
  component: QueueLayout,
});

export { Route };
