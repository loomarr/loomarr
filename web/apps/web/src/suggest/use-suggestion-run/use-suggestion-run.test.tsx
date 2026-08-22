import type { ProposalDTO, ProposalJobDTO } from "@loomarr/api";
import {
  getGetProposalJobMockHandler,
  getListProposalsMockHandler,
  getSubmitProposalMockHandler,
} from "@loomarr/api/msw";
import type { EventHandlers, SuggestionPhase } from "@loomarr/core/events";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
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

const queuedJob: ProposalJobDTO = {
  jobId: "job-1",
  status: "queued",
  intent: { description: "90s action movies" },
};

// The SUBMIT succeeds (200 + jobId) even when the job later fails. The job read, not the
// latency-only SSE phase, carries the authoritative failure classification and preserved Intent.
const stub = (proposals: ProposalDTO[] = [], job: ProposalJobDTO = queuedJob) => {
  server.use(
    getSubmitProposalMockHandler({ jobId: "job-1" }),
    getGetProposalJobMockHandler(job),
    getListProposalsMockHandler({ proposals }),
  );
};

describe("useSuggestionRun", () => {
  it("surfaces the authoritative no-grounded-titles failure", async () => {
    stub([], {
      ...queuedJob,
      status: "failed",
      failure: {
        code: "no_grounded_titles",
        message: "No grounded titles matched this request. Try the same request again.",
      },
    });
    const { result } = renderHook(() => useSuggestionRun(), { wrapper: makeWrapper() });

    act(() => result.current.start({ description: "90s action movies" }));
    await waitFor(() => expect(result.current.failed).toBe(true));

    expect(result.current.isRunning).toBe(false);
    expect(result.current.failure?.code).toBe("no_grounded_titles");
    expect(result.current.failure?.message).toMatch(/No grounded titles/);
    expect(result.current.proposal).toBeUndefined();
    expect(result.current.error).toBeFalsy();
  });

  it("is not `failed` when the run is still in flight", async () => {
    stub([]);
    const { result } = renderHook(() => useSuggestionRun(), { wrapper: makeWrapper() });

    act(() => result.current.start({ description: "90s action movies" }));
    await emit("job-1", "reasoning");

    expect(result.current.failed).toBe(false);
    expect(result.current.isRunning).toBe(true);
  });

  it("retries with the exact preserved Intent", async () => {
    const submissions: unknown[] = [];
    server.use(
      getSubmitProposalMockHandler(async ({ request }) => {
        submissions.push(await request.json());
        return { jobId: `job-${submissions.length}` };
      }),
      getGetProposalJobMockHandler({
        ...queuedJob,
        status: "failed",
        failure: { code: "no_grounded_titles", message: "No grounded titles matched this request." },
      }),
      getListProposalsMockHandler({ proposals: [] }),
    );
    const { result } = renderHook(() => useSuggestionRun(), { wrapper: makeWrapper() });
    const intent = { description: "Classic Simpson Episodes", era: "1989-1999", maxAcquire: 2 };

    act(() => result.current.start(intent));
    await waitFor(() => expect(result.current.failed).toBe(true));
    act(() => result.current.retry());
    await waitFor(() => expect(submissions).toHaveLength(2));

    expect(submissions).toEqual([intent, intent]);
  });
});
