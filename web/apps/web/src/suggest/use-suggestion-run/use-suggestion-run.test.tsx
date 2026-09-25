import type { Intent, ProposalJourneyDTO } from "@loomarr/api";
import {
  getGetProposalJobMockHandler,
  getReviseProposalJobMockHandler,
  getSubmitProposalMockHandler,
} from "@loomarr/api/msw";
import type { EventHandlers, SuggestionPhase } from "@loomarr/core/events";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { useSuggestionRun } from "./use-suggestion-run";

// Capture the handlers useSuggestionRun registers so a test can drive the SSE phases the
// hook otherwise only receives live. The real provider is an EventSource jsdom cannot run;
// this stands in for exactly the one seam under test — the phase fan-out — without it.
let handlers: EventHandlers | undefined;
vi.mock("@/events/events-provider", () => ({
  useLoomarrEventListener: (h: EventHandlers) => {
    handlers = h;
  },
}));

const emit = (jobId: string, phase: SuggestionPhase) =>
  act(() => handlers?.onSuggestion?.({ jobId, phase, round: 0 }));

const makeWrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

// The SUBMIT succeeds (200 + jobId) even when the job later fails, so the proposals list
// stays empty and no `error` is ever set — the failure arrives only as a `failed` phase.
const stub = () => {
  const journey: ProposalJourneyDTO = {
    version: 1,
    jobId: "job-1",
    milestone: "generating",
    intent: { description: "90s action movies" },
    attempts: [{ version: 1, number: 1, status: "running", startedAt: "2026-08-22T12:00:00Z" }],
    actions: ["wait"],
    createdAt: "2026-08-22T12:00:00Z",
    updatedAt: "2026-08-22T12:00:00Z",
  };
  server.use(
    getSubmitProposalMockHandler({ jobId: "job-1" }),
    getGetProposalJobMockHandler(() => journey),
  );
  return journey;
};

describe("useSuggestionRun", () => {
  beforeEach(() => window.sessionStorage.clear());

  it("surfaces a terminal `failed` phase as run.failed (not a silent empty state)", async () => {
    const journey = stub();
    const { result } = renderHook(() => useSuggestionRun(), { wrapper: makeWrapper() });

    act(() => result.current.start({ description: "90s action movies" }));
    await waitFor(() => expect(result.current.isRunning).toBe(true));

    // The job errors mid-flight; the backend emits `failed` over the stream.
    journey.milestone = "failed";
    journey.failure = {
      code: "generation_failed",
      message: "Loomarr couldn't generate this channel.",
      reason: "provider_unavailable",
      recoveryAction: "retry_later",
      guidance: "Try again later.",
    };
    journey.actions = ["retry"];
    await emit("job-1", "failed");

    // Without the fix this is the silent hole: isRunning goes false, no proposal, error is
    // null — the panel would fall through to a blank form. `failed` is what makes it visible.
    await waitFor(() => expect(result.current.failed).toBe(true));
    expect(result.current.isRunning).toBe(false);
    expect(result.current.proposal).toBeUndefined();
    expect(result.current.error).toBeFalsy();
  });

  it("re-reads the Journey on a frame, so a new stage or pick shows before the 2 s poll", async () => {
    const journey = stub();
    journey.progress = { stage: "reading", terms: [], picks: [], target: 8, startedAt: "2026-08-22T12:00:00Z" };
    const { result } = renderHook(() => useSuggestionRun(), { wrapper: makeWrapper() });
    act(() => result.current.start({ description: "90s action movies" }));
    await waitFor(() => expect(result.current.progress?.stage).toBe("reading"));

    journey.progress = {
      stage: "choosing",
      terms: ["heist"],
      picks: [{ key: "movie:tmdb:1", mediaType: "movie", name: "Speed", year: 1994, inLibrary: true }],
      target: 8,
      startedAt: "2026-08-22T12:00:00Z",
    };
    await emit("job-1", "reasoning");

    // waitFor's default timeout is 1 s: only the frame's invalidation can satisfy this in time.
    await waitFor(() => expect(result.current.progress?.picks.map((p) => p.name)).toEqual(["Speed"]));
    expect(result.current.progress?.stage).toBe("choosing");
  });

  it("is not `failed` when the run is still in flight", async () => {
    stub();
    const { result } = renderHook(() => useSuggestionRun(), { wrapper: makeWrapper() });

    act(() => result.current.start({ description: "90s action movies" }));
    await emit("job-1", "reasoning");

    expect(result.current.failed).toBe(false);
    expect(result.current.isRunning).toBe(true);
  });

  it("restores the active Job from authoritative state after a reload", async () => {
    const journey = stub();
    journey.milestone = "failed";
    journey.failure = {
      code: "no_grounded_titles",
      message: "No grounded titles matched this request.",
      reason: "no_catalog_match",
      recoveryAction: "broaden_request",
      guidance: "Broaden the request.",
    };
    journey.actions = ["edit", "retry"];
    window.sessionStorage.setItem("loomarr.activeProposalJob", "job-1");

    const { result } = renderHook(() => useSuggestionRun(), { wrapper: makeWrapper() });

    await waitFor(() => expect(result.current.failed).toBe(true));
    expect(result.current.failure?.code).toBe("no_grounded_titles");
    expect(result.current.actions).toEqual(["edit", "retry"]);
  });

  it("keeps a deep-linked Job resumable after its URL handoff is removed", async () => {
    stub();

    renderHook(() => useSuggestionRun("job-1"), { wrapper: makeWrapper() });

    await waitFor(() => expect(window.sessionStorage.getItem("loomarr.activeProposalJob")).toBe("job-1"));
  });

  it("retains the complete authorized intent when editing a failed request", async () => {
    const journey = stub();
    journey.intent = {
      description: "90s action movies",
      era: "1990s",
      mustInclude: ["Heat"],
      runtimeTargetMin: 180,
    };
    journey.milestone = "failed";
    journey.failure = {
      code: "no_grounded_titles",
      message: "No grounded titles matched this request.",
      reason: "no_catalog_match",
      recoveryAction: "broaden_request",
      guidance: "Broaden the request.",
    };
    journey.actions = ["edit"];
    window.sessionStorage.setItem("loomarr.activeProposalJob", "job-1");

    const { result } = renderHook(() => useSuggestionRun(), { wrapper: makeWrapper() });
    await waitFor(() => expect(result.current.failed).toBe(true));

    act(() => result.current.reset(true));
    expect(result.current.intent).toEqual(journey.intent);
    expect(result.current.failed).toBe(false);
  });

  it("revises the current Job while preserving its review through a failed replacement", async () => {
    const revisions: unknown[] = [];
    const journey: ProposalJourneyDTO = {
      version: 1,
      jobId: "job-1",
      milestone: "awaiting_approval",
      intent: {
        description: "90s action movies",
        era: "1990s",
        mustInclude: ["Heat"],
        runtimeTargetMin: 180,
      },
      attempts: [
        {
          version: 1,
          number: 1,
          status: "succeeded",
          startedAt: "2026-08-22T12:00:00Z",
          completedAt: "2026-08-22T12:00:01Z",
        },
      ],
      proposal: {
        id: "proposal-1",
        status: "submitted",
        proposal: {
          intent: { description: "90s action movies" },
          channelName: "Friday Night Action",
          lineup: [{ name: "Heat", mediaType: "movie", tmdbId: 949, year: 1995, inLibrary: true }],
          acquisitions: [],
          alternates: [],
          scores: {
            version: 1,
            themeFit: 1,
            availabilityRatio: 1,
            eraBalance: 1,
            theme: {
              status: "supported",
              basis: "qualifiers",
              assessedItems: 1,
              unknownItems: 0,
              qualifiers: [{ term: "action", supportedItems: 1 }],
            },
            era: { status: "supported", assessedItems: 1, matchingItems: 1, unknownItems: 0 },
          },
          trace: { version: 1, surfacedTotal: 1, recordedTotal: 1, truncated: false, candidates: [] },
        },
      },
      actions: ["review", "edit"],
      createdAt: "2026-08-22T12:00:00Z",
      updatedAt: "2026-08-22T12:00:01Z",
    };
    server.use(
      getGetProposalJobMockHandler(() => journey),
      getReviseProposalJobMockHandler(async ({ request }) => {
        const revised = (await request.json()) as Intent;
        revisions.push(revised);
        journey.intent = revised;
        journey.milestone = "generating";
        journey.attempts.push({
          version: 1,
          number: 2,
          status: "running",
          startedAt: "2026-08-22T12:01:00Z",
        });
        journey.actions = ["wait"];
        return { jobId: "job-1" };
      }),
    );
    const { result } = renderHook(() => useSuggestionRun("job-1"), { wrapper: makeWrapper() });
    await waitFor(() => expect(result.current.proposal?.id).toBe("proposal-1"));

    const revisedIntent = {
      ...journey.intent,
      description: "90s action movies with more sci-fi variety",
    };
    act(() => result.current.revise(revisedIntent));

    await waitFor(() => expect(revisions).toEqual([revisedIntent]));
    expect(result.current.jobId).toBe("job-1");
    expect(result.current.proposal?.id).toBe("proposal-1");
    expect(result.current.isRunning).toBe(true);

    // A failed replacement is not a failed Journey while the previous
    // submitted Proposal remains available. The server projects the fallback
    // as awaiting approval and carries the failed Attempt as local guidance.
    journey.milestone = "awaiting_approval";
    journey.failure = {
      code: "generation_failed",
      message: "Loomarr couldn't update these suggestions.",
      reason: "provider_unavailable",
      recoveryAction: "retry_later",
      guidance: "Try again later.",
    };
    journey.attempts[1] = {
      version: 1,
      number: 2,
      status: "failed",
      startedAt: "2026-08-22T12:01:00Z",
      completedAt: "2026-08-22T12:01:01Z",
    };
    journey.actions = ["review", "edit", "retry"];
    await emit("job-1", "failed");

    await waitFor(() => expect(result.current.failure?.reason).toBe("provider_unavailable"));
    expect(result.current.failed).toBe(false);
    expect(result.current.proposal?.id).toBe("proposal-1");
    expect(result.current.isRunning).toBe(false);
    expect(result.current.error).toBeFalsy();
  });
});
