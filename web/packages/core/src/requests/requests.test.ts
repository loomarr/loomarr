import type { ProposalDTO } from "@loomarr/api/models/proposalDTO";
import type { ProposalJourneyDTO } from "@loomarr/api/models/proposalJourneyDTO";
import type { PullDTO } from "@loomarr/api/models/pullDTO";
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
const pull = { id: "pull-1", status: "pending", title: "Static stingers" } as unknown as PullDTO;

const refusal = (type: string, title: string) => new ApiError(422, { title, type });

const port = (over: Partial<RequestsPort> = {}): RequestsPort => ({
  approve: vi.fn(async () => ({ channelId: "channel-1" })),
  approveFillerPulls: vi.fn(async (ids: readonly string[]) => ({
    approved: ids.length,
    results: ids.map((id) => ({ id, ok: true })),
  })),
  approveMany: vi.fn(async (ids: readonly string[]) => ({
    approved: ids.length,
    results: ids.map((id) => ({ id, ok: true })),
  })),
  deny: vi.fn(async () => undefined),
  dismissFillerPull: vi.fn(async () => undefined),
  hideIdea: vi.fn(async () => undefined),
  loadApprovals: vi.fn(async () => [approval]),
  loadFillerPulls: vi.fn(async () => [pull]),
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
    expect(fake.loadFillerPulls).not.toHaveBeenCalled();
    expect(snapshot.fillerPulls).toEqual([]);
    expect(requestsNeedsYouCount(snapshot)).toBe(1);
  });

  it("gives an admin the pending approvals and filler pulls, counted with failed requests", async () => {
    const controller = createRequestsController({
      port: port({ loadMe: vi.fn(async () => ({ role: "admin" }) as never) }),
    });
    await controller.refresh();
    expect(controller.getSnapshot().approvals).toEqual([approval]);
    expect(controller.getSnapshot().fillerPulls).toEqual([pull]);
    expect(requestsNeedsYouCount(controller.getSnapshot())).toBe(3);
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
      expect(fake.approve).toHaveBeenCalledWith("approval-1", undefined);
      expect(fake.loadApprovals).toHaveBeenCalled();
      expect(controller.getSnapshot().deciding).toEqual([]);
    });

    it("sends an edit on the same approve call, never as a separate save", async () => {
      const fake = port(admin());
      const controller = createRequestsController({ port: fake });
      await controller.approve("approval-1", { drop: ["movie:tmdb:90009001"], note: "Skip the first" });
      expect(fake.approve).toHaveBeenCalledWith("approval-1", {
        drop: ["movie:tmdb:90009001"],
        note: "Skip the first",
      });
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

    const partial = {
      approved: 2,
      results: [
        { id: "a", ok: true },
        { id: "b", ok: true },
        { error: "Already decided by another admin", id: "c", ok: false },
      ],
    };

    it("returns every id's outcome from a bulk approval, so one refusal is a row, not a notice", async () => {
      const approveMany = vi.fn(async () => partial);
      const controller = createRequestsController({ port: port({ ...admin(), approveMany }) });
      await expect(controller.approveMany(["a", "b", "c"])).resolves.toEqual({ kind: "done", ...partial });
      expect(controller.getSnapshot().notice).toBeUndefined();
      expect(controller.getSnapshot().deciding).toEqual([]);
    });

    it("keeps filler pulls in their own group: approving them never touches the proposal endpoint", async () => {
      const fake = port({ ...admin(), approveFillerPulls: vi.fn(async () => partial) });
      const controller = createRequestsController({ port: fake });
      await expect(controller.approveFillerPulls(["a", "b", "c"])).resolves.toEqual({
        kind: "done",
        ...partial,
      });
      expect(fake.approveMany).not.toHaveBeenCalled();
      expect(fake.loadFillerPulls).toHaveBeenCalled();
    });

    it("says why when a bulk approval can't run at all", async () => {
      const approveMany = vi.fn().mockRejectedValue(new ApiError(403, { title: "Admins only" }));
      const controller = createRequestsController({ port: port({ ...admin(), approveMany }) });
      await expect(controller.approveMany(["a"])).resolves.toEqual({
        kind: "failed",
        message: "Admins only",
      });
      expect(controller.getSnapshot().notice).toBe("Admins only");
    });

    it("approves one filler pull through the bulk method and surfaces a refusal's sentence", async () => {
      const approveFillerPulls = vi.fn(async () => ({
        approved: 0,
        results: [{ error: "Already decided", id: "pull-1", ok: false }],
      }));
      const controller = createRequestsController({ port: port({ ...admin(), approveFillerPulls }) });
      await expect(controller.approveFillerPull("pull-1")).resolves.toEqual({
        kind: "failed",
        message: "Already decided",
      });
      expect(approveFillerPulls).toHaveBeenCalledWith(["pull-1"]);
      expect(controller.getSnapshot().notice).toBe("Already decided");
    });

    it("dismisses a filler pull and re-reads the group", async () => {
      const fake = port(admin());
      const controller = createRequestsController({ port: fake });
      await expect(controller.dismissFillerPull("pull-1")).resolves.toEqual({ kind: "done" });
      expect(fake.dismissFillerPull).toHaveBeenCalledWith("pull-1");
      expect(fake.loadFillerPulls).toHaveBeenCalled();
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

  it("approves filler pulls one at a time and reports a refused pull without stopping the rest", async () => {
    const request = vi.fn(async (url: RequestInfo | URL) =>
      String(url).includes("/pull-2/")
        ? respond({ title: "Already decided by another admin" }, 409)
        : respond({ id: "ok" }),
    );
    const requests = createRequestsPort(request as never);
    const outcome = await requests.approveFillerPulls(["pull-1", "pull-2", "pull-3"]);

    const calls = request.mock.calls as unknown as [string, RequestInit][];
    expect(calls.map(([url, init]) => `${init.method} ${url}`)).toEqual([
      "POST /v1/filler/pulls/pull-1/approve",
      "POST /v1/filler/pulls/pull-2/approve",
      "POST /v1/filler/pulls/pull-3/approve",
    ]);
    expect(outcome).toEqual({
      approved: 2,
      results: [
        { id: "pull-1", ok: true },
        { error: "Already decided by another admin", id: "pull-2", ok: false },
        { id: "pull-3", ok: true },
      ],
    });
  });

  it("reads pending pulls, dismisses one, and carries an approval's edit in its body", async () => {
    const request = vi.fn(async () => respond({ channelId: "c-1", pulls: [] }));
    const requests = createRequestsPort(request as never);
    await requests.loadFillerPulls(new AbortController().signal);
    await requests.dismissFillerPull("pull-1");
    await requests.approve("p-1", { drop: ["movie:tmdb:90009001"] });

    const calls = request.mock.calls as unknown as [string, RequestInit][];
    expect(calls.map(([url, init]) => `${init.method} ${url}`)).toEqual([
      "GET /v1/filler/pulls?status=pending",
      "POST /v1/filler/pulls/pull-1/dismiss",
      "POST /v1/proposals/p-1/approve",
    ]);
    expect(calls[2]?.[1].body).toBe('{"drop":["movie:tmdb:90009001"]}');
  });

  it("maps a bulk approval's results to {id, ok, error}", async () => {
    const request = vi.fn(async () =>
      respond({
        approved: 1,
        results: [
          { channelId: "c-1", enqueued: 3, id: "a", ok: true },
          { error: "Already decided", id: "b", ok: false },
        ],
      }),
    );
    const outcome = await createRequestsPort(request as never).approveMany(["a", "b"]);
    expect(outcome).toEqual({
      approved: 1,
      results: [
        { id: "a", ok: true },
        { error: "Already decided", id: "b", ok: false },
      ],
    });
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
