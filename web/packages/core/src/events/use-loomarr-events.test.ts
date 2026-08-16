import { QueryClient } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useLoomarrEvents } from "./events";

const hook = vi.hoisted(() => ({ queryClient: undefined as QueryClient | undefined }));

vi.mock("@tanstack/react-query", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-query")>()),
  useQueryClient: () => hook.queryClient,
}));

vi.mock("react", async (importOriginal) => ({
  ...(await importOriginal<typeof import("react")>()),
  useEffect: (effect: () => void) => effect(),
  useRef: <T>(current: T) => ({ current }),
}));

class MockEventSource {
  static last: MockEventSource | undefined;
  private readonly handlers = new Map<string, (event: MessageEvent) => void>();

  constructor() {
    MockEventSource.last = this;
  }

  addEventListener(type: string, handler: (event: MessageEvent) => void) {
    this.handlers.set(type, handler);
  }

  close() {}

  emit(type: string, data: unknown) {
    this.handlers.get(type)?.({ data: JSON.stringify(data) } as MessageEvent);
  }
}

beforeEach(() => {
  hook.queryClient = new QueryClient();
  vi.stubGlobal("EventSource", MockEventSource);
});

afterEach(() => vi.unstubAllGlobals());

describe("useLoomarrEvents", () => {
  it.each(["title", "suggestion"])("invalidates Proposal Job list and detail reads on a %s frame", (type) => {
    const client = hook.queryClient as QueryClient;
    const listKey = ["/v1/proposal-jobs", { mine: true }];
    const detailKey = ["/v1/proposal-jobs/job-1"];
    const unrelatedKey = ["/v1/channels"];
    client.setQueryData(listKey, { proposalJobs: [] });
    client.setQueryData(detailKey, { jobId: "job-1" });
    client.setQueryData(unrelatedKey, { channels: [] });

    useLoomarrEvents();
    MockEventSource.last?.emit(type, { jobId: "job-1" });

    expect(client.getQueryState(listKey)?.isInvalidated).toBe(true);
    expect(client.getQueryState(detailKey)?.isInvalidated).toBe(true);
    expect(client.getQueryState(unrelatedKey)?.isInvalidated).toBe(false);
  });
});
