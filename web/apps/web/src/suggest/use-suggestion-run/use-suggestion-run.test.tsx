import type { ProposalJourneyDTO } from "@loomarr/api";
import { getGetProposalJobMockHandler, getSubmitProposalMockHandler } from "@loomarr/api/msw";
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
    journey.failure = { code: "generation_failed", message: "Loomarr couldn't generate this channel." };
    journey.actions = ["retry"];
    await emit("job-1", "failed");

    // Without the fix this is the silent hole: isRunning goes false, no proposal, error is
    // null — the panel would fall through to a blank form. `failed` is what makes it visible.
    await waitFor(() => expect(result.current.failed).toBe(true));
    expect(result.current.isRunning).toBe(false);
    expect(result.current.proposal).toBeUndefined();
    expect(result.current.error).toBeFalsy();
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
    journey.failure = { code: "no_grounded_titles", message: "No grounded titles matched this request." };
    journey.actions = ["edit", "retry"];
    window.sessionStorage.setItem("loomarr.activeProposalJob", "job-1");

    const { result } = renderHook(() => useSuggestionRun(), { wrapper: makeWrapper() });

    await waitFor(() => expect(result.current.failed).toBe(true));
    expect(result.current.failure?.code).toBe("no_grounded_titles");
    expect(result.current.actions).toEqual(["edit", "retry"]);
  });
});
