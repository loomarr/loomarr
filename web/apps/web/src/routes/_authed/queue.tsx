import { suggestionsApi, type TitleDTO, TitleDTOState, titlesApi } from "@loomarr/api";
import { pluralize } from "@loomarr/core";
import { useQueries, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { RotateCw } from "lucide-react";
import { useAuth } from "@/auth";
import { CountTabs, EmptyState, ErrorState, StateBadge } from "@/components/loomarr";
import { Button } from "@/components/ui";
import { useDocumentTitle } from "@/lib";
import { ApprovalHistory, journeyProgress, MyRequests, stageOf } from "@/queue";
import { ApprovalQueue } from "@/suggest";

// Queue / My requests (§12, §13) — where a member watches their submission land.
// It deliberately leads with the JOURNEY ("4 of 7 have landed") rather than a table of
// provisioning states, because the member framing §13 asks for is "where is my stuff",
// not "what does `requested` mean". The per-title state badge is still there for the
// operator who wants the detail. Live via `title` SSE.
const STAGE_COPY = {
  waiting: { title: "Waiting", hint: "Approved and queued. Loomarr will request these next." },
  acquiring: { title: "On the way", hint: "Requested from your *arr apps and downloading." },
  ready: { title: "Ready", hint: "In your library and playable on a channel." },
} as const;

const ORDER = ["acquiring", "waiting", "ready"] as const;

// The Queue needs every provisioning state at once (§4) to collapse them into the three
// journey stages. GET /v1/titles is a single-state FILTER, though — it 400s without a
// `state` (a deliberate, §7-pinned contract: `TestListRequiresState`, so an unbounded
// "list all titles" can't be issued by accident) — so we fan out one query per state and
// merge. This is the aggregation the endpoint intentionally leaves to the client.
const STATES = Object.values(TitleDTOState);

// The three tabs the v2 mock draws on Queue (`navDefs` hangs a pending-proposal badge off this
// nav entry, which is why the approvals surface lives HERE rather than as its own route).
// `?tab=` so a tab is linkable — an admin sent "3 need you" can be sent straight to it.
const TAB_IDS = ["approval", "flight", "history"] as const;
type TabId = (typeof TAB_IDS)[number];
type QueueSearch = { tab?: TabId };

const QueueScreen = () => {
  useDocumentTitle("Queue");
  const queryClient = useQueryClient();
  const { isAdmin } = useAuth();
  const navigate = Route.useNavigate();
  const { tab } = Route.useSearch();

  // Pending proposals drive BOTH the approval tab's list and its count, so the two cannot
  // disagree — a tab reading "3" above a list of two is worse than no count. Members never see
  // this tab (approving is admin-only, §11), so the query is gated rather than 403ing in the
  // background on every member's page load.
  const pending = suggestionsApi.useListProposals(
    { status: "submitted" },
    { query: { enabled: isAdmin, retry: false } },
  );
  const pendingCount = pending.data?.status === 200 ? (pending.data.data.proposals?.length ?? 0) : 0;

  const decided = useQueries({
    queries: (["approved", "denied"] as const).map((status) =>
      suggestionsApi.getListProposalsQueryOptions({ status }),
    ),
  });
  const historyCount = decided.reduce(
    (n, q) => n + (q.data?.status === 200 ? (q.data.data.proposals?.length ?? 0) : 0),
    0,
  );
  const stateQueries = useQueries({
    queries: STATES.map((state) => titlesApi.getListTitlesQueryOptions({ state })),
  });
  const retry = titlesApi.useEnqueueTitle({
    mutation: {
      // A re-enqueue moves the title out of `unavailable` and back to `wanted`, and the
      // reconciler may advance it further before we refetch — so invalidate every state
      // query, not just the two ends of the transition. Cheap at household scale.
      onSuccess: () =>
        Promise.all(
          STATES.map((state) =>
            queryClient.invalidateQueries({ queryKey: titlesApi.getListTitlesQueryKey({ state }) }),
          ),
        ),
    },
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

  const isLoading = stateQueries.some((q) => q.isLoading);
  const rows: TitleDTO[] = stateQueries.flatMap((q) =>
    q.data?.status === 200 ? (q.data.data.titles ?? []) : [],
  );
  const progress = journeyProgress(rows);

  // Members get ONE tab: approving is admin-only (§11), and a history of other people's
  // decisions is not theirs to read. Rendering the bar with a single tab keeps the page's shape
  // stable rather than hiding it entirely for half the users.
  const tabs = isAdmin
    ? [
        { id: "approval", label: "Needs approval", count: pendingCount },
        { id: "flight", label: "In flight", count: rows.length },
        { id: "history", label: "History", count: historyCount },
      ]
    : [{ id: "flight", label: "In flight", count: rows.length }];

  // Default to whichever tab has work: an admin with pending requests lands on them, since that
  // is the one queue state that blocks somebody else. Otherwise the titles they came to watch.
  const fallback: TabId = isAdmin && pendingCount > 0 ? "approval" : "flight";
  const active: TabId = tab && tabs.some((t) => t.id === tab) ? tab : fallback;

  return (
    <div className="flex h-full flex-col">
      <header className="border-border border-b px-6 py-4">
        <h1 className="font-semibold text-xl">Queue</h1>
        {rows.length > 0 && (
          <p className="mt-1 text-muted-foreground text-sm">
            {`${progress.ready} of ${pluralize(progress.total, "title")} ${progress.total === 1 ? "has" : "have"} landed.`}{" "}
            Channels play what's ready and fill the rest, improving as more arrives.
          </p>
        )}
      </header>

      {/* The three tabs (V27). Members see only In flight — approving is admin-only (§11) and a
          history of other people's decisions is not theirs to read, so the bar renders with one
          tab rather than two they would be refused. */}
      <div className="px-6">
        <CountTabs
          label="Queue sections"
          tabs={tabs}
          activeId={active}
          onSelect={(id) => void navigate({ search: { tab: id as TabId } })}
        />
      </div>

      <div
        id={`panel-${active}`}
        role="tabpanel"
        aria-labelledby={`tab-${active}`}
        className="flex flex-1 flex-col gap-6 overflow-auto p-6"
      >
        {active === "approval" && <ApprovalQueue />}
        {active === "history" && <ApprovalHistory />}

        {active === "flight" && (
          <>
            {/* The REQUESTS I submitted (V26), above the tracked titles and OUTSIDE the
                empty-titles branch on purpose — a request still waiting for approval has no
                titles yet, so folding it in would show "Nothing in flight" to exactly the
                member who most needs to see their request exists. Renders nothing when there
                are no requests, so the common path is unchanged. */}
            <MyRequests />

            {rows.length === 0 && !isLoading ? (
              <EmptyState
                title="Nothing in flight"
                description="Approved acquisitions show their journey here: from queued, to downloading, to playable."
              />
            ) : (
              <div className="flex flex-col gap-6">
                {ORDER.map((stage) => {
                  const inStage = rows.filter((t) => stageOf(t) === stage);
                  if (inStage.length === 0) return null;
                  return (
                    <section key={stage} className="flex flex-col gap-2">
                      <h2 className="font-semibold text-lg">
                        {STAGE_COPY[stage].title}{" "}
                        <span className="font-normal text-muted-foreground text-sm">({inStage.length})</span>
                      </h2>
                      <p className="text-muted-foreground text-sm">{STAGE_COPY[stage].hint}</p>
                      <ul className="flex flex-col gap-2">
                        {inStage.map((t) => (
                          <li
                            key={t.key}
                            className="flex items-center gap-3 rounded-md border border-border bg-card px-3 py-2.5"
                          >
                            <span className="min-w-0 flex-1 truncate text-sm">
                              {t.name ?? t.key}
                              {t.year ? <span className="text-muted-foreground"> ({t.year})</span> : null}
                            </span>
                            <StateBadge state={t.state} />
                            {/* A given-up title is the one thing a human can act on here:
                            re-enqueue puts it back in the queue (§4). */}
                            {t.state === "unavailable" && (
                              <Button
                                variant="outline"
                                size="sm"
                                disabled={retry.isPending}
                                onClick={() =>
                                  // Re-enqueue by identity, not by key: the enqueue contract
                                  // takes the title's real ids so the reconciler can request
                                  // it again from scratch.
                                  retry.mutate({
                                    data: {
                                      mediaType: t.mediaType,
                                      tmdbId: t.tmdbId,
                                      tvdbId: t.tvdbId,
                                      name: t.name,
                                      year: t.year,
                                    },
                                  })
                                }
                              >
                                <RotateCw aria-hidden />
                                Try again
                              </Button>
                            )}
                          </li>
                        ))}
                      </ul>
                    </section>
                  );
                })}
              </div>
            )}
            {retry.error != null && <ErrorState error={retry.error} className="mt-4" />}
          </>
        )}
      </div>
    </div>
  );
};

const Route = createFileRoute("/_authed/queue")({
  // `?tab=` is deep-linkable; an unknown or absent value narrows to the default so a stale link
  // lands on a real tab rather than a blank panel — the same shape the channel detail uses.
  validateSearch: (search: Record<string, unknown>): QueueSearch => ({
    tab: TAB_IDS.includes(search.tab as TabId) ? (search.tab as TabId) : undefined,
  }),
  component: QueueScreen,
});

export { Route };
