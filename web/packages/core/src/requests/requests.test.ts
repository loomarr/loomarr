import type { ProposalDTO } from "@loomarr/api/models/proposalDTO";
import type { ProposalJourneyDTO } from "@loomarr/api/models/proposalJourneyDTO";
import { ApiError } from "@loomarr/api/mutator";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  createRequestsController,
  createRequestsPort,
  GENERATING_POLL_MS,
  requestsInTab,
  requestsNeedsYouCount,
} from "./requests";
import type { RequestsPort } from "./requests.type";

const journey = (over: Partial<ProposalJourneyDTO> = {}): ProposalJourneyDTO => ({
  actions: [],
  attempts: [],
  createdAt: "2026-10-01T18:00:00Z",
  intent: { description: "Rainy-day detective stories" },
  jobId: "job-1",
  milestone: "live",
  updatedAt: "2026-10-01T18:00:00Z",
  version: 1,
  ...over,
});
const failed = journey({ jobId: "job-failed", milestone: "failed" });
const generating = journey({ jobId: "job-generating", milestone: "generating" });
const approval = { id: "approval-1", jobId: "job-x", status: "submitted" } as unknown as ProposalDTO;

const refusal = (type: string, title: string) => new ApiError(422, { title, type });

const port = (over: Partial<RequestsPort> = {}): RequestsPort => ({
  approve: vi.fn(async () => ({ channelId: "channel-1" })),
  approveMany: vi.fn(async () => ({ approved: 2, failed: 0 })),
  deny: vi.fn(async () => undefined),
  hideIdea: vi.fn(async () => undefined),
  loadApprovals: vi.fn(async () => [approval]),
  loadIdeas: vi.fn(async () => []),
  loadJourneys: vi.fn(async () => [failed, journey()]),
  loadMe: vi.fn(async () => ({ role: "member" }) as never),
  loadTitles: vi.fn(async () => []),
  requestIdea: vi.fn(async () => ({ jobId: "job-new" })),
  submitBrief: vi.fn(async () => ({ jobId: "job-new" })),
  unhideIdea: vi.fn(async () => undefined),
  ...over,
});

afterEach(() => vi.useRealTimers());

describe("requests controller", () => {
  it("reads a member's own requests into Web's tabs and never asks for approvals", async () => {
    const fake = port();
    const controller = createRequestsController({ port: fake });
    expect(controller.getSnapshot().status).toBe("loading");
    await controller.refresh();

    const snapshot = controller.getSnapshot();
    expect(snapshot.status).toBe("ready");
    expect(snapshot.role).toBe("member");
    expect(requestsInTab(snapshot, "needs-you").map((e) => e.journey.jobId)).toEqual(["job-failed"]);
    expect(requestsInTab(snapshot, "done").map((e) => e.journey.jobId)).toEqual(["job-1"]);
    expect(fake.loadApprovals).not.toHaveBeenCalled();
    expect(requestsNeedsYouCount(snapshot)).toBe(1);
  });

  it("gives an admin the pending approvals, counted with failed requests", async () => {
    const controller = createRequestsController({
      port: port({ loadMe: vi.fn(async () => ({ role: "admin" }) as never) }),
    });
    await controller.refresh();
    expect(controller.getSnapshot().approvals).toEqual([approval]);
    expect(requestsNeedsYouCount(controller.getSnapshot())).toBe(2);
  });

  it("reports the server's sentence when the list can't load, and recovers on retry", async () => {
    const loadJourneys = vi
      .fn()
      .mockRejectedValueOnce(new ApiError(503, { title: "The server didn't answer." }))
      .mockResolvedValue([journey()]);
    const controller = createRequestsController({ port: port({ loadJourneys }) });
    await controller.refresh();
    expect(controller.getSnapshot()).toMatchObject({
      errorMessage: "The server didn't answer.",
      status: "error",
    });
    await controller.refresh();
    expect(controller.getSnapshot()).toMatchObject({ errorMessage: undefined, status: "ready" });
  });

  it("re-reads every two seconds while a request is generating, then stops", async () => {
    vi.useFakeTimers();
    const loadJourneys = vi
      .fn()
      .mockResolvedValueOnce([generating])
      .mockResolvedValueOnce([generating])
      .mockResolvedValue([journey({ jobId: "job-generating", milestone: "awaiting_approval" })]);
    const controller = createRequestsController({ port: port({ loadJourneys }) });
    await controller.refresh();
    expect(loadJourneys).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(GENERATING_POLL_MS);
    expect(loadJourneys).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(GENERATING_POLL_MS);
    expect(loadJourneys).toHaveBeenCalledTimes(3);
    await vi.advanceTimersByTimeAsync(GENERATING_POLL_MS * 3);
    expect(loadJourneys).toHaveBeenCalledTimes(3);
    controller.dispose();
  });

  it("stops polling when disposed", async () => {
    vi.useFakeTimers();
    const loadJourneys = vi.fn(async () => [generating]);
    const controller = createRequestsController({ port: port({ loadJourneys }) });
    await controller.refresh();
    controller.dispose();
    await vi.advanceTimersByTimeAsync(GENERATING_POLL_MS * 2);
    expect(loadJourneys).toHaveBeenCalledTimes(1);
  });

  it("requests an idea, then re-reads both the ideas and the requests", async () => {
    const fake = port();
    const controller = createRequestsController({ port: fake });
    await controller.requestIdea("idea-1");
    expect(fake.requestIdea).toHaveBeenCalledWith("idea-1");
    expect(fake.loadIdeas).toHaveBeenCalled();
    expect(fake.loadJourneys).toHaveBeenCalled();
    expect(controller.getSnapshot()).toMatchObject({ notice: undefined, requestingIdeaId: undefined });
  });

  it("keeps a refused idea request as a one-time notice", async () => {
    const requestIdea = vi
      .fn()
      .mockRejectedValue(new ApiError(403, { title: "Channel ideas are for members" }));
    const controller = createRequestsController({ port: port({ requestIdea }) });
    await controller.requestIdea("idea-1");
    expect(controller.getSnapshot().notice).toBe("Channel ideas are for members");
    controller.dismissNotice();
    expect(controller.getSnapshot().notice).toBeUndefined();
  });

  it("hides an idea and brings it back with Undo", async () => {
    const idea = { id: "idea-1", name: "Fireside Mysteries" } as never;
    const fake = port({ loadIdeas: vi.fn(async () => [idea]) });
    const controller = createRequestsController({ port: fake });
    await controller.refreshIdeas();
    await controller.hideIdea("idea-1");
    expect(controller.getSnapshot().hiddenIdea).toBe(idea);
    await controller.undoHideIdea();
    expect(fake.unhideIdea).toHaveBeenCalledWith("idea-1");
    expect(controller.getSnapshot().hiddenIdea).toBeUndefined();
  });

  it("marks the ideas as failed without blocking the requests", async () => {
    const controller = createRequestsController({
      port: port({ loadIdeas: vi.fn().mockRejectedValue(new Error("offline")) }),
    });
    await Promise.all([controller.refresh(), controller.refreshIdeas()]);
    expect(controller.getSnapshot()).toMatchObject({ ideasStatus: "error", status: "ready" });
  });

  describe("typed brief", () => {
    it("starts a request and shows it in the list", async () => {
      const fake = port();
      const controller = createRequestsController({ port: fake });
      await expect(controller.submitBrief({ description: "Cosy baking shows" })).resolves.toEqual({
        jobId: "job-new",
        kind: "started",
      });
      expect(fake.submitBrief).toHaveBeenCalledWith({ description: "Cosy baking shows" });
      expect(fake.loadJourneys).toHaveBeenCalled();
    });

    it.each([
      ["feature_not_configured", "ai"],
      ["grounding_not_configured", "grounding"],
    ] as const)("tells %s apart from a failure, so the draft is kept", async (type, reason) => {
      const submitBrief = vi.fn().mockRejectedValue(refusal(type, "Not set up"));
      const controller = createRequestsController({ port: port({ submitBrief }) });
      await expect(controller.submitBrief({ description: "Cosy baking shows" })).resolves.toEqual({
        kind: "unavailable",
        reason,
      });
    });

    it("words any other refusal with the server's title", async () => {
      const submitBrief = vi
        .fn()
        .mockRejectedValue(refusal("quota_exceeded", "You have too many pending requests"));
      const controller = createRequestsController({ port: port({ submitBrief }) });
      await expect(controller.submitBrief({ description: "Cosy baking shows" })).resolves.toEqual({
        kind: "failed",
        message: "You have too many pending requests",
      });
    });
  });

  describe("admin review", () => {
    const admin = () => ({ loadMe: vi.fn(async () => ({ role: "admin" }) as never) });

    it("approves, reports the new channel, and re-reads the queue", async () => {
      const fake = port(admin());
      const controller = createRequestsController({ port: fake });
      await expect(controller.approve("approval-1")).resolves.toEqual({
        channelId: "channel-1",
        kind: "done",
      });
      expect(fake.approve).toHaveBeenCalledWith("approval-1");
      expect(fake.loadApprovals).toHaveBeenCalled();
      expect(controller.getSnapshot().deciding).toEqual([]);
    });

    it("denies with a trimmed reason, or none", async () => {
      const fake = port(admin());
      const controller = createRequestsController({ port: fake });
      await controller.deny("approval-1", "  Too broad  ");
      await controller.deny("approval-1", "   ");
      expect(fake.deny).toHaveBeenNthCalledWith(1, "approval-1", "Too broad");
      expect(fake.deny).toHaveBeenNthCalledWith(2, "approval-1", undefined);
    });

    it("keeps a refused approval in the queue and says why", async () => {
      const approve = vi.fn().mockRejectedValue(new ApiError(403, { title: "Admins only" }));
      const controller = createRequestsController({ port: port({ ...admin(), approve }) });
      await expect(controller.approve("approval-1")).resolves.toEqual({
        kind: "failed",
        message: "Admins only",
      });
      expect(controller.getSnapshot()).toMatchObject({
        approvals: [approval],
        deciding: [],
        notice: "Admins only",
      });
    });

    it("says how many of a bulk approval went through", async () => {
      const approveMany = vi.fn(async () => ({ approved: 2, failed: 1 }));
      const controller = createRequestsController({ port: port({ ...admin(), approveMany }) });
      const result = await controller.approveMany(["a", "b", "c"]);
      expect(result).toEqual({
        kind: "failed",
        message: "2 approved, 1 couldn't be approved. The rest are still waiting.",
      });
    });

    it("marks only the proposals being decided as in flight", async () => {
      let settle: () => void = () => undefined;
      const approve = vi.fn(
        () => new Promise<{ channelId: string }>((resolve) => (settle = () => resolve({ channelId: "c" }))),
      );
      const controller = createRequestsController({ port: port({ ...admin(), approve }) });
      const pending = controller.approve("approval-1");
      expect(controller.getSnapshot().deciding).toEqual(["approval-1"]);
      settle();
      await pending;
      expect(controller.getSnapshot().deciding).toEqual([]);
    });
  });
});

describe("requests port", () => {
  const respond = (body: unknown, status = 200) =>
    new Response(body === undefined ? null : JSON.stringify(body), { status });

  it("reads the caller's own jobs and every title state through the authenticated fetch", async () => {
    const request = vi.fn(async (url: RequestInfo | URL) =>
      String(url).includes("/v1/titles") ? respond({ titles: [{ key: "t" }] }) : respond({ journeys: [] }),
    );
    const requests = createRequestsPort(request as never);
    await requests.loadJourneys(new AbortController().signal);
    const titles = await requests.loadTitles(new AbortController().signal);

    expect(request.mock.calls[0]?.[0]).toBe("/v1/proposal-jobs?mine=true");
    const states = request.mock.calls
      .slice(1)
      .map(([url]) => new URL(String(url), "http://x").searchParams.get("state"));
    expect(states.sort()).toEqual(["available", "downloading", "requested", "unavailable", "wanted"]);
    expect(titles).toHaveLength(5);
  });

  it("posts a request with its JSON body and the right verb for each action", async () => {
    const request = vi.fn(async () => respond({ jobId: "job-9", channelId: "c-1" }, 202));
    const requests = createRequestsPort(request as never);
    await requests.requestIdea("idea-1");
    await requests.submitBrief({ description: "Cosy baking shows" });
    await requests.approve("p-1");
    await requests.deny("p-1", "Too broad");
    await requests.hideIdea("idea-1");
    await requests.unhideIdea("idea-1");

    const calls = request.mock.calls as unknown as [string, RequestInit][];
    expect(calls.map(([url, init]) => `${init.method} ${url}`)).toEqual([
      "POST /v1/discovery/ideas/idea-1/request",
      "POST /v1/proposals",
      "POST /v1/proposals/p-1/approve",
      "POST /v1/proposals/p-1/deny",
      "PUT /v1/me/hidden-ideas/idea-1",
      "DELETE /v1/me/hidden-ideas/idea-1",
    ]);
    expect(calls[1]?.[1].body).toBe('{"description":"Cosy baking shows"}');
    expect(calls[3]?.[1].body).toBe('{"reason":"Too broad"}');
  });

  it("throws the server's problem as an ApiError", async () => {
    const request = vi.fn(async () =>
      respond({ title: "AI isn't set up", type: "feature_not_configured" }, 422),
    );
    const requests = createRequestsPort(request as never);
    await expect(requests.submitBrief({ description: "x" })).rejects.toMatchObject({
      problem: { type: "feature_not_configured" },
      status: 422,
    });
  });
});
