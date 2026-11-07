import type { ClipDTO } from "@loomarr/api";
import { getSplitFillerMockHandler } from "@loomarr/api/msw";
import type { EventHandlers } from "@loomarr/core/events";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { useClipSplit } from "./use-clip-split";

// The hook's two collaborators are the router (it navigates to the review route on success) and the
// event bus (progress arrives as frames). Both are replaced so a test can deliver a frame by hand.
const navigate = vi.fn();
let handlers: EventHandlers = {};
vi.mock("@tanstack/react-router", () => ({ useNavigate: () => navigate }));
vi.mock("@/events/events-provider", () => ({
  useLoomarrEventListener: (next: EventHandlers) => {
    handlers = next;
  },
}));

const wrapperFor = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => createElement(QueryClientProvider, { client }, children);
};

const compilation = { hash: "reel-hash", name: "Saturday reel" } as ClipDTO;

const startSplit = async () => {
  server.use(getSplitFillerMockHandler({ jobId: "job-1" }));
  const hook = renderHook(() => useClipSplit(), { wrapper: wrapperFor() });
  act(() => hook.result.current.start(compilation));
  await waitFor(() => expect(hook.result.current.job).toBeDefined());
  return hook;
};

beforeEach(() => {
  navigate.mockReset();
  handlers = {};
});

describe("useClipSplit", () => {
  it("tracks a running split under the clip's name", async () => {
    const { result } = await startSplit();

    expect(result.current.job).toEqual({
      clipHash: "reel-hash",
      clipName: "Saturday reel",
      jobId: "job-1",
      status: "running",
    });
    expect(result.current.isRunning("reel-hash")).toBe(true);
    expect(result.current.isRunning("other-hash")).toBe(false);
  });

  it("goes to the review route when the job succeeds, and forgets the job", async () => {
    const { result } = await startSplit();

    act(() => handlers.onFillerSplit?.({ jobId: "job-1", status: "success", proposalId: "prop-9" } as never));

    expect(navigate).toHaveBeenCalledWith({
      to: "/filler/splits/$proposalId",
      params: { proposalId: "prop-9" },
      search: { from: "library" },
    });
    expect(result.current.job).toBeUndefined();
  });

  it("keeps the job and its message when detection fails", async () => {
    const { result } = await startSplit();

    act(() =>
      handlers.onFillerSplit?.({ jobId: "job-1", status: "error", error: "No cuts found." } as never),
    );

    expect(result.current.job).toMatchObject({ status: "error", error: "No cuts found." });
    expect(result.current.isRunning("reel-hash")).toBe(false);
    expect(navigate).not.toHaveBeenCalled();
  });

  it("ignores frames for another job", async () => {
    const { result } = await startSplit();

    act(() => handlers.onFillerSplit?.({ jobId: "someone-elses", status: "error", error: "x" } as never));
    act(() => handlers.onFillerSplit?.({ status: "success", proposalId: "prop-9" } as never));

    expect(result.current.job?.status).toBe("running");
    expect(navigate).not.toHaveBeenCalled();
  });
});
