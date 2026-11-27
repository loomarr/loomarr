import type { FillerAcquisitionRunDTO } from "@loomarr/api/models/fillerAcquisitionRunDTO";
import { getGetFillerAcquisitionMockHandler, getQueueFillerSourceItemMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { beforeEach, describe, expect, it } from "vitest";
import { LoomarrEventsProvider } from "@/events/events-provider";
import { FakeEventSource } from "@/test/fake-event-source";
import { server } from "@/test/msw/server";
import { QUEUED_CLIP_JOBS_KEY, readQueuedClipJobs, useQueuedClipJobs } from "./use-queued-clip-jobs";

// The client is built once per hook, outside the wrapper: a client created inside it would be rebuilt
// on every render and drop the state under test.
const wrapperFor = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client }, createElement(LoomarrEventsProvider, null, children));
};

const run = (status: "queued" | "running" | "success" | "error"): FillerAcquisitionRunDTO => ({
  id: "acq-1",
  trigger: "source",
  sourceId: "archive:classic",
  status,
  requested: 1,
  fetched: 0,
  skipped: 0,
  failed: 0,
  empty: 0,
  startedAt: "2026-09-13T14:00:00Z",
  updatedAt: "2026-09-13T14:00:00Z",
  outcome: {
    enrolled: 0,
    preparing: 0,
    needsDecision: 0,
    ready: 0,
    complete: 0,
    rejected: 0,
    dismissed: 0,
  },
  artifacts: { staged: 0, published: 0, consumed: 0, repair: 0 },
});

beforeEach(() => sessionStorage.clear());

describe("readQueuedClipJobs", () => {
  it("keeps only well-formed entries and only the newest 50", () => {
    const jobs = Array.from({ length: 60 }, (_, i) => ({
      sourceID: "archive:classic",
      clipID: `clip-${i}`,
      jobID: `job-${i}`,
    }));
    sessionStorage.setItem(
      QUEUED_CLIP_JOBS_KEY,
      JSON.stringify([{ sourceID: "archive:classic", clipID: 3 }, null, ...jobs]),
    );

    const read = readQueuedClipJobs();

    expect(read).toHaveLength(50);
    expect(read[0]?.clipID).toBe("clip-10");
    expect(read.at(-1)?.clipID).toBe("clip-59");
  });

  it("treats unreadable storage as no jobs", () => {
    sessionStorage.setItem(QUEUED_CLIP_JOBS_KEY, "{not json");
    expect(readQueuedClipJobs()).toEqual([]);
    sessionStorage.setItem(QUEUED_CLIP_JOBS_KEY, JSON.stringify({ not: "a list" }));
    expect(readQueuedClipJobs()).toEqual([]);
  });
});

describe("useQueuedClipJobs", () => {
  it("queues a clip under its source and remembers the job for a remount", async () => {
    const sent: { sourceId: string; body: unknown }[] = [];
    server.use(
      getQueueFillerSourceItemMockHandler(async ({ request, params }) => {
        sent.push({ sourceId: String(params.id), body: await request.json() });
        return { jobId: "acq-1" };
      }),
      getGetFillerAcquisitionMockHandler(run("running")),
    );
    const { result } = renderHook(() => useQueuedClipJobs("archive:classic"), { wrapper: wrapperFor() });
    expect(result.current.queueStatus).toEqual({});

    act(() => result.current.queue({ id: "clip-a", url: "https://archive.org/details/clip-a" }));

    await waitFor(() => expect(result.current.queueStatus).toEqual({ "clip-a": "running" }));
    expect(sent).toEqual([
      {
        sourceId: "archive:classic",
        body: { remoteId: "clip-a", url: "https://archive.org/details/clip-a" },
      },
    ]);
    expect(readQueuedClipJobs()).toEqual([{ sourceID: "archive:classic", clipID: "clip-a", jobID: "acq-1" }]);
  });

  // The backend stores the run's new state, then publishes its frame; the frame is the cue to read
  // that run again, and a frame for another run reads nothing.
  it("reads a queued run again when its frame arrives, and only then", async () => {
    sessionStorage.setItem(
      QUEUED_CLIP_JOBS_KEY,
      JSON.stringify([{ sourceID: "archive:classic", clipID: "clip-a", jobID: "acq-1" }]),
    );
    let current = run("running");
    let reads = 0;
    server.use(
      getGetFillerAcquisitionMockHandler(() => {
        reads++;
        return current;
      }),
    );
    const { result } = renderHook(() => useQueuedClipJobs("archive:classic"), { wrapper: wrapperFor() });
    await waitFor(() => expect(result.current.queueStatus).toEqual({ "clip-a": "running" }));

    act(() => FakeEventSource.broadcast("filler_ingest", { jobId: "acq-other", status: "starting" }));
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(reads).toBe(1);

    current = run("success");
    act(() => FakeEventSource.broadcast("filler_ingest", { jobId: "acq-1", status: "success" }));
    await waitFor(() => expect(result.current.queueStatus).toEqual({ "clip-a": "success" }));
  });

  it("reports only the jobs of the source it was given", () => {
    sessionStorage.setItem(
      QUEUED_CLIP_JOBS_KEY,
      JSON.stringify([{ sourceID: "archive:other", clipID: "clip-z", jobID: "acq-z" }]),
    );
    const { result } = renderHook(() => useQueuedClipJobs("archive:classic"), { wrapper: wrapperFor() });
    expect(result.current.queueStatus).toEqual({});
  });
});
