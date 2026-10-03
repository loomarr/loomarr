import { getMeUrl } from "@loomarr/api/endpoints/auth";
import {
  getHideChannelIdeaUrl,
  getListChannelIdeasUrl,
  getRequestChannelIdeaUrl,
  getUnhideChannelIdeaUrl,
} from "@loomarr/api/endpoints/discovery";
import { getListProposalJobsUrl } from "@loomarr/api/endpoints/proposal-jobs";
import {
  getApproveProposalUrl,
  getBulkApproveProposalsUrl,
  getDenyProposalUrl,
  getListProposalsUrl,
  getSubmitProposalUrl,
} from "@loomarr/api/endpoints/proposals";
import { getListTitlesUrl } from "@loomarr/api/endpoints/titles";
import type { ApproveOutputBody } from "@loomarr/api/models/approveOutputBody";
import type { BulkApproveOutputBody } from "@loomarr/api/models/bulkApproveOutputBody";
import type { ErrorModel } from "@loomarr/api/models/errorModel";
import type { ListChannelIdeasOutputBody } from "@loomarr/api/models/listChannelIdeasOutputBody";
import type { ListOutputBody } from "@loomarr/api/models/listOutputBody";
import type { ListProposalsOutputBody } from "@loomarr/api/models/listProposalsOutputBody";
import type { MeBody } from "@loomarr/api/models/meBody";
import type { ProposalJourneyListOutputBody } from "@loomarr/api/models/proposalJourneyListOutputBody";
import type { RequestChannelIdeaOutputBody } from "@loomarr/api/models/requestChannelIdeaOutputBody";
import type { SubmitOutputBody } from "@loomarr/api/models/submitOutputBody";
import { TitleDTOState } from "@loomarr/api/models/titleDTOState";
import { ApiError, toProblem } from "@loomarr/api/mutator";

import { requestNeedsYou, requestStatus } from "./request-status/request-status";
import type { RequestTab } from "./request-status/request-status.type";
import type {
  DecisionResult,
  RequestEntry,
  RequestsController,
  RequestsPort,
  RequestsSnapshot,
  SubmitBriefResult,
} from "./requests.type";

/** A generating request is re-read this often, as Web's list does. */
const GENERATING_POLL_MS = 2_000;
// GET /v1/titles is a single-state filter, so every acquisition state is fetched and merged.
const TITLE_STATES = Object.values(TitleDTOState);

const createRequestsPort = (request: typeof globalThis.fetch): RequestsPort => {
  // A refusal becomes the same typed ApiError the generated client throws, so `toProblem` words it.
  const send = async <Body>(url: string, method: string, body?: unknown, signal?: AbortSignal) => {
    const response = await request(url, {
      body: body === undefined ? undefined : JSON.stringify(body),
      headers: body === undefined ? undefined : { "Content-Type": "application/json" },
      method,
      signal,
    });
    const text = await response.text();
    const parsed = text ? (JSON.parse(text) as unknown) : undefined;
    if (!response.ok)
      throw new ApiError(
        response.status,
        (parsed as ErrorModel | undefined) ?? { status: response.status },
        response.headers.get("X-Request-Id") ?? undefined,
      );
    return parsed as Body;
  };
  return {
    approve: async (proposalId) => {
      const { channelId } = await send<ApproveOutputBody>(getApproveProposalUrl(proposalId), "POST", {});
      return { channelId };
    },
    approveMany: async (proposalIds) => {
      const out = await send<BulkApproveOutputBody>(getBulkApproveProposalsUrl(), "POST", {
        ids: [...proposalIds],
      });
      return { approved: out.approved, failed: out.results.filter((result) => !result.ok).length };
    },
    deny: async (proposalId, reason) => {
      await send(getDenyProposalUrl(proposalId), "POST", reason ? { reason } : {});
    },
    hideIdea: async (ideaId) => {
      await send(getHideChannelIdeaUrl(ideaId), "PUT");
    },
    loadApprovals: async (signal) =>
      (
        await send<ListProposalsOutputBody>(
          getListProposalsUrl({ status: "submitted" }),
          "GET",
          undefined,
          signal,
        )
      ).proposals ?? [],
    loadIdeas: async (signal) =>
      (await send<ListChannelIdeasOutputBody>(getListChannelIdeasUrl(), "GET", undefined, signal)).ideas ??
      [],
    loadJourneys: async (signal) =>
      (
        await send<ProposalJourneyListOutputBody>(
          getListProposalJobsUrl({ mine: true }),
          "GET",
          undefined,
          signal,
        )
      ).journeys ?? [],
    loadMe: (signal) => send<MeBody>(getMeUrl(), "GET", undefined, signal),
    loadTitles: async (signal) =>
      (
        await Promise.all(
          TITLE_STATES.map((state) =>
            send<ListOutputBody>(getListTitlesUrl({ state }), "GET", undefined, signal),
          ),
        )
      ).flatMap((body) => body.titles ?? []),
    requestIdea: async (ideaId) => {
      const { jobId } = await send<RequestChannelIdeaOutputBody>(getRequestChannelIdeaUrl(ideaId), "POST");
      return { jobId };
    },
    submitBrief: async (intent) => {
      const { jobId } = await send<SubmitOutputBody>(getSubmitProposalUrl(), "POST", intent);
      return { jobId };
    },
    unhideIdea: async (ideaId) => {
      await send(getUnhideChannelIdeaUrl(ideaId), "DELETE");
    },
  };
};

const failure = (error: unknown, fallback: string): string => {
  const { detail, title } = toProblem(error);
  return title || detail || fallback;
};

const createRequestsController = ({ port }: { port: RequestsPort }): RequestsController => {
  let disposed = false;
  let request: AbortController | undefined;
  let ideasRequest: AbortController | undefined;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let snapshot: RequestsSnapshot = {
    approvals: [],
    deciding: [],
    entries: [],
    ideas: [],
    ideasStatus: "loading",
    role: "member",
    status: "loading",
    titles: [],
  };
  const listeners = new Set<() => void>();

  const publish = (next: RequestsSnapshot) => {
    if (disposed) return;
    snapshot = next;
    for (const listener of listeners) listener();
  };
  const patch = (changes: Partial<RequestsSnapshot>) => publish({ ...snapshot, ...changes });

  const refresh = async () => {
    if (disposed) return;
    clearTimeout(timer);
    request?.abort();
    const current = new AbortController();
    request = current;
    try {
      // The role decides whether approvals are read at all: a member's page load must not fire a
      // call that can only 403 (Web gates `usePendingApprovals` the same way).
      const me = await port.loadMe(current.signal);
      const [journeys, titles, approvals] = await Promise.all([
        port.loadJourneys(current.signal),
        port.loadTitles(current.signal),
        me.role === "admin" ? port.loadApprovals(current.signal) : Promise.resolve([]),
      ]);
      if (request !== current) return;
      const entries: RequestEntry[] = journeys.map((journey) => ({
        journey,
        status: requestStatus(journey, titles),
      }));
      publish({
        ...snapshot,
        approvals,
        entries,
        errorMessage: undefined,
        role: me.role,
        status: "ready",
        titles,
      });
      if (entries.some(({ journey }) => journey.milestone === "generating"))
        timer = setTimeout(() => void refresh(), GENERATING_POLL_MS);
    } catch (error) {
      if (request === current && !current.signal.aborted)
        publish({
          ...snapshot,
          errorMessage: failure(error, "The server didn't answer."),
          status: "error",
        });
    }
  };

  const refreshIdeas = async () => {
    if (disposed) return;
    ideasRequest?.abort();
    const current = new AbortController();
    ideasRequest = current;
    try {
      const ideas = await port.loadIdeas(current.signal);
      if (ideasRequest === current) patch({ ideas, ideasStatus: "ready" });
    } catch {
      if (ideasRequest === current && !current.signal.aborted) patch({ ideasStatus: "error" });
    }
  };

  const decide = async (
    ids: readonly string[],
    act: () => Promise<DecisionResult>,
    fallback: string,
  ): Promise<DecisionResult> => {
    patch({ deciding: [...snapshot.deciding, ...ids], notice: undefined });
    let result: DecisionResult;
    try {
      result = await act();
    } catch (error) {
      result = { kind: "failed", message: failure(error, fallback) };
    }
    patch({
      deciding: snapshot.deciding.filter((id) => !ids.includes(id)),
      notice: result.kind === "failed" ? result.message : undefined,
    });
    // A failed decision leaves its proposal in the list; a done one removes it, so read either way.
    await refresh();
    return result;
  };

  return {
    approve: (proposalId) =>
      decide(
        [proposalId],
        async () => ({ channelId: (await port.approve(proposalId)).channelId, kind: "done" }),
        "Couldn't approve that request.",
      ),
    approveMany: (proposalIds) =>
      decide(
        proposalIds,
        async () => {
          const { approved, failed } = await port.approveMany(proposalIds);
          return failed > 0
            ? {
                kind: "failed",
                message: `${approved} approved, ${failed} couldn't be approved. The rest are still waiting.`,
              }
            : { kind: "done" };
        },
        "Couldn't approve those requests.",
      ),
    deny: (proposalId, reason) =>
      decide(
        [proposalId],
        async () => {
          await port.deny(proposalId, reason?.trim() || undefined);
          return { kind: "done" };
        },
        "Couldn't deny that request.",
      ),
    dismissNotice: () => patch({ notice: undefined }),
    dispose: () => {
      if (disposed) return;
      disposed = true;
      clearTimeout(timer);
      request?.abort();
      ideasRequest?.abort();
      listeners.clear();
    },
    getSnapshot: () => snapshot,
    hideIdea: async (ideaId) => {
      const idea = snapshot.ideas.find((candidate) => candidate.id === ideaId);
      patch({ notice: undefined, requestingIdeaId: ideaId });
      try {
        await port.hideIdea(ideaId);
        patch({ hiddenIdea: idea, requestingIdeaId: undefined });
        await refreshIdeas();
      } catch (error) {
        patch({ notice: failure(error, "Couldn't hide that idea."), requestingIdeaId: undefined });
      }
    },
    refresh,
    refreshIdeas,
    requestIdea: async (ideaId) => {
      patch({ notice: undefined, requestingIdeaId: ideaId });
      try {
        await port.requestIdea(ideaId);
        patch({ requestingIdeaId: undefined });
        // The request now shows on its card and joins In progress.
        await Promise.all([refreshIdeas(), refresh()]);
      } catch (error) {
        patch({ notice: failure(error, "Couldn't request that channel."), requestingIdeaId: undefined });
      }
    },
    submitBrief: async (intent): Promise<SubmitBriefResult> => {
      try {
        const { jobId } = await port.submitBrief(intent);
        await refresh();
        return { jobId, kind: "started" };
      } catch (error) {
        const type = toProblem(error).type;
        if (type === "feature_not_configured") return { kind: "unavailable", reason: "ai" };
        if (type === "grounding_not_configured") return { kind: "unavailable", reason: "grounding" };
        return { kind: "failed", message: failure(error, "Couldn't send that request.") };
      }
    },
    subscribe: (listener) => {
      if (disposed) return () => undefined;
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    undoHideIdea: async () => {
      const idea = snapshot.hiddenIdea;
      if (!idea) return;
      try {
        await port.unhideIdea(idea.id);
        patch({ hiddenIdea: undefined });
        await refreshIdeas();
      } catch (error) {
        patch({ notice: failure(error, "Couldn't bring that idea back.") });
      }
    },
  };
};

/** The Needs-you count: an admin's pending decisions plus anyone's failed requests (Web's badge). */
const requestsNeedsYouCount = (snapshot: RequestsSnapshot): number =>
  (snapshot.role === "admin" ? snapshot.approvals.length : 0) +
  snapshot.entries.filter(({ journey }) => requestNeedsYou(journey)).length;

/** The entries on one Requests tab. Needs you also carries an admin's approvals, listed separately. */
const requestsInTab = (snapshot: RequestsSnapshot, tab: RequestTab): readonly RequestEntry[] =>
  snapshot.entries.filter((entry) => entry.status.tab === tab);

export {
  createRequestsController,
  createRequestsPort,
  GENERATING_POLL_MS,
  requestsInTab,
  requestsNeedsYouCount,
};
