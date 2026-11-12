import * as fillerApi from "@loomarr/api/endpoints/filler";
import * as proposalJobsApi from "@loomarr/api/endpoints/proposal-jobs";
import * as proposalsApi from "@loomarr/api/endpoints/proposals";
import * as titlesApi from "@loomarr/api/endpoints/titles";
import type { ProposalJourneyDTO } from "@loomarr/api/models/proposalJourneyDTO";
import type { TitleDTO } from "@loomarr/api/models/titleDTO";
import { TitleDTOState } from "@loomarr/api/models/titleDTOState";
import { unwrap } from "@loomarr/api/unwrap";
import { type RequestStatus, type RequestTab, requestNeedsYou, requestStatus } from "@loomarr/core/requests";
import { type QueryClient, useQueries } from "@tanstack/react-query";
import { useAuth } from "@/auth/use-auth";
import { usePendingApprovals } from "@/queue/pending-approvals";

// GET /v1/titles is a single-state FILTER (it 400s without `state`, §7), so the acquisition
// states a request's status line needs are fetched one query per state and merged — the same
// fan-out the old Queue made. React Query dedupes by key, so every caller shares one cache entry.
const STATES = Object.values(TitleDTOState);

interface RequestEntry {
  journey: ProposalJourneyDTO;
  status: RequestStatus;
}

// A request an admin has to approve is a card in Needs you (the approval queue lists every submitted
// proposal), so it is not also a row in In progress: one request is in one tab (#2738).
const awaitsThisAdmin = (journey: ProposalJourneyDTO, isAdmin: boolean): boolean =>
  isAdmin && journey.milestone === "awaiting_approval";

// Everything the Requests page and the nav badge read, from ONE place, so the badge, the tab
// counts and the lists cannot disagree.
//
// The requests are the VIEWER's own (`mine`): Proposal Jobs, not Proposal artifacts, because a
// request that is still generating or that failed has no Proposal yet. Approvals are a separate
// admin-only list — deciding is admin-only (§11); reading is global (§342).
const useRequests = () => {
  const { isAdmin } = useAuth();
  const jobs = proposalJobsApi.useListProposalJobs(
    { mine: true },
    {
      query: {
        refetchInterval: (state) => {
          const journeys = unwrap(state.state.data, (body) => body.journeys) ?? [];
          return journeys.some((journey) => journey.milestone === "generating") ? 2_000 : false;
        },
      },
    },
  );
  const titleQueries = useQueries({
    queries: STATES.map((state) => titlesApi.getListTitlesQueryOptions({ state })),
  });
  const pending = usePendingApprovals(isAdmin);

  const titles: TitleDTO[] = titleQueries.flatMap((q) => unwrap(q.data, (b) => b.titles) ?? []);
  const journeys = unwrap(jobs.data, (body) => body.journeys) ?? [];
  const entries: RequestEntry[] = journeys.map((journey) => ({
    journey,
    status: requestStatus(journey, titles),
  }));

  const inTab = (tab: RequestStatus["tab"]) =>
    entries.filter((e) => e.status.tab === tab && !awaitsThisAdmin(e.journey, isAdmin));
  return {
    entries,
    inTab,
    titles,
    /** Approvals waiting on this admin (0 for a member). */
    pendingApprovals: pending.count,
    /** What the Needs you tab and the nav badge count: approvals plus the viewer's failures. */
    needsYouCount: pending.count + inTab("needs-you").length,
    isLoading: jobs.isLoading,
    error: jobs.error ?? titleQueries.find((q) => q.error)?.error ?? null,
    refetch: () => {
      jobs.refetch();
      for (const q of titleQueries) q.refetch();
    },
  };
};

// The nav badge's count on its own: the shell mounts it on EVERY page, so it must not drag the
// per-state titles fan-out along. Same jobs query (shared cache) and the same definition of
// "needs you" as `useRequests().needsYouCount`.
const useNeedsYouCount = (): number => {
  const { isAdmin } = useAuth();
  const jobs = proposalJobsApi.useListProposalJobs({ mine: true });
  const pending = usePendingApprovals(isAdmin);
  const failed = (unwrap(jobs.data, (body) => body.journeys) ?? []).filter(requestNeedsYou).length;
  return pending.count + failed;
};

// The Needs you count read imperatively, for a route's `beforeLoad` — the same number the tab and
// the nav badge show: an admin's pending approvals plus the viewer's failed requests.
const fetchNeedsYouCount = async (queryClient: QueryClient, isAdmin: boolean): Promise<number> => {
  const [jobs, proposals, pulls] = await Promise.all([
    queryClient.ensureQueryData(proposalJobsApi.getListProposalJobsQueryOptions({ mine: true })),
    isAdmin
      ? queryClient.ensureQueryData(proposalsApi.getListProposalsQueryOptions({ status: "submitted" }))
      : undefined,
    isAdmin
      ? queryClient.ensureQueryData(fillerApi.getListFillerPullsQueryOptions({ status: "pending" }))
      : undefined,
  ]);
  // `?? []` like every other reader here: this runs in a route's beforeLoad, so a throw is not a
  // wrong count but the whole Requests page replaced by the router's error boundary.
  const failed = ((jobs.status === 200 ? jobs.data.journeys : undefined) ?? []).filter(
    requestNeedsYou,
  ).length;
  return (
    failed +
    (proposals?.status === 200 ? (proposals.data.proposals?.length ?? 0) : 0) +
    (pulls?.status === 200 ? (pulls.data.pulls?.length ?? 0) : 0)
  );
};

// The tab /requests opens on, read imperatively for its `beforeLoad`: the first of Needs you, In
// progress and Done that has something in it, so the page never opens on an empty tab while another
// has items (#2738). Needs you is answered first and alone: when it has items nothing else is read.
// With nothing anywhere it is In progress, and the page's own empty state takes over.
const fetchLandingTab = async (queryClient: QueryClient, isAdmin: boolean): Promise<RequestTab> => {
  if ((await fetchNeedsYouCount(queryClient, isAdmin)) > 0) return "needs-you";
  const [jobs, ...titleAnswers] = await Promise.all([
    queryClient.ensureQueryData(proposalJobsApi.getListProposalJobsQueryOptions({ mine: true })),
    ...STATES.map((state) => queryClient.ensureQueryData(titlesApi.getListTitlesQueryOptions({ state }))),
  ]);
  const titles = titleAnswers.flatMap((answer) => (answer.status === 200 ? answer.data.titles : []) ?? []);
  const journeys = ((jobs.status === 200 ? jobs.data.journeys : undefined) ?? []).filter(
    (journey) => !awaitsThisAdmin(journey, isAdmin),
  );
  const tabs = journeys.map((journey) => requestStatus(journey, titles).tab);
  return !tabs.includes("in-progress") && tabs.includes("done") ? "done" : "in-progress";
};

// What `useRequests` reads, ensured imperatively for the Requests layout's `loader`, so the tabs
// open on their lists. The viewer's own jobs and the per-state titles are what the lists are made
// of, so a failure there reaches the route's error view. The admin's approvals only add to the Needs
// you count and are read with `retry: false` by the page, so they are warmed and may fail quietly.
const ensureRequests = async (queryClient: QueryClient, isAdmin: boolean): Promise<void> => {
  await Promise.all([
    queryClient.ensureQueryData(proposalJobsApi.getListProposalJobsQueryOptions({ mine: true })),
    ...STATES.map((state) => queryClient.ensureQueryData(titlesApi.getListTitlesQueryOptions({ state }))),
    isAdmin
      ? queryClient.prefetchQuery(proposalsApi.getListProposalsQueryOptions({ status: "submitted" }))
      : undefined,
    isAdmin
      ? queryClient.prefetchQuery(fillerApi.getListFillerPullsQueryOptions({ status: "pending" }))
      : undefined,
  ]);
};

export type { RequestEntry };
export { ensureRequests, fetchLandingTab, useNeedsYouCount, useRequests };
