import { QueryClient } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { invalidateByPrefix, openEventStream, resyncEventBackedQueries } from "./events";

afterEach(() => vi.unstubAllGlobals());

describe("invalidateByPrefix", () => {
  it("invalidates only queries whose URL key matches the prefix", async () => {
    const qc = new QueryClient();
    // Seed three cached queries keyed like orval's fetch client ([url, params]).
    await qc.prefetchQuery({ queryKey: ["/v1/titles", { state: "wanted" }], queryFn: () => "t" });
    await qc.prefetchQuery({ queryKey: ["/v1/channels"], queryFn: () => "c" });

    invalidateByPrefix(qc, "/v1/titles");

    const titles = qc.getQueryState(["/v1/titles", { state: "wanted" }]);
    const channels = qc.getQueryState(["/v1/channels"]);
    expect(titles?.isInvalidated).toBe(true);
    expect(channels?.isInvalidated).toBe(false);
  });
});

describe("SSE reconnect recovery", () => {
  it("invalidates every GET-backed event view", async () => {
    const qc = new QueryClient();
    for (const key of ["/v1/dashboard/summary", "/v1/channels", "/v1/activity", "/v1/users"]) {
      await qc.prefetchQuery({ queryKey: [key], queryFn: () => key });
    }

    resyncEventBackedQueries(qc);

    expect(qc.getQueryState(["/v1/dashboard/summary"])?.isInvalidated).toBe(true);
    expect(qc.getQueryState(["/v1/channels"])?.isInvalidated).toBe(true);
    expect(qc.getQueryState(["/v1/activity"])?.isInvalidated).toBe(true);
    expect(qc.getQueryState(["/v1/users"])?.isInvalidated).toBe(false);
  });

  it("runs the recovery hook on initial open and reconnect", () => {
    const listeners = new Map<string, Array<() => void>>();
    class CaptureEventSource {
      addEventListener(type: string, cb: () => void): void {
        listeners.set(type, [...(listeners.get(type) ?? []), cb]);
      }
      close(): void {}
    }
    vi.stubGlobal("EventSource", CaptureEventSource);
    const onOpen = vi.fn();

    openEventStream({}, "/v1/events", { onOpen });
    listeners.get("open")?.[0]?.();
    listeners.get("open")?.[0]?.();

    expect(onOpen).toHaveBeenCalledTimes(2);
  });
});
