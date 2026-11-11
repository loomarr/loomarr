import { QueryClient } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  browserEventStreamFactory,
  invalidateAfterReconnect,
  invalidateByPrefix,
  invalidateForFrame,
  invalidateSettings,
  openEventStream,
  openFrameStream,
} from "./events";
import type { EventStreamMessage } from "./events.type";

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

describe("settings invalidation", () => {
  // The queries a saved setting can change. LLM status is the one the save bar used to skip.
  const derived = [
    ["/v1/settings"],
    ["/v1/setup/status"],
    ["/v1/system/llm"],
    ["/v1/system/llm/discover"],
    ["/v1/filler/readiness"],
  ];

  it("maps the settings frame to every settings-derived query and nothing else", async () => {
    const qc = new QueryClient();
    for (const key of derived) await qc.prefetchQuery({ queryKey: key, queryFn: () => "x" });
    await qc.prefetchQuery({ queryKey: ["/v1/channels"], queryFn: () => "c" });

    invalidateForFrame(qc, "settings", {});

    for (const key of derived) expect(qc.getQueryState(key)?.isInvalidated, key[0]).toBe(true);
    expect(qc.getQueryState(["/v1/channels"])?.isInvalidated).toBe(false);
  });

  it("gives the save paths the same mapping through invalidateSettings", async () => {
    const qc = new QueryClient();
    await qc.prefetchQuery({ queryKey: ["/v1/system/llm"], queryFn: () => "x" });

    invalidateSettings(qc);

    expect(qc.getQueryState(["/v1/system/llm"])?.isInvalidated).toBe(true);
  });
});

describe("openEventStream", () => {
  it("uses an injected platform stream and ignores malformed latency frames", () => {
    const listeners = new Map<string, (event: EventStreamMessage) => void>();
    const close = vi.fn();
    const onChannel = vi.fn();
    const stop = openEventStream({ onChannel }, "http://loomarr.test/v1/events", (url) => {
      expect(url).toBe("http://loomarr.test/v1/events");
      return {
        addEventListener: (type, listener) => listeners.set(type, listener),
        close,
      };
    });

    listeners.get("channel")?.({ data: '{"channelId":"seven","status":"live"}' });
    listeners.get("channel")?.({ data: "not-json" });
    expect(onChannel).toHaveBeenCalledOnce();
    expect(onChannel).toHaveBeenCalledWith({ channelId: "seven", status: "live" });

    stop();
    expect(close).toHaveBeenCalledOnce();
  });
});

describe("a stream that comes back", () => {
  it("tells of a reconnect on every open after the first", () => {
    const listeners = new Map<string, (event: EventStreamMessage) => void>();
    const onReconnect = vi.fn();
    const stop = openFrameStream(
      () => {},
      "http://loomarr.test/v1/events",
      () => ({ addEventListener: (type, listener) => listeners.set(type, listener), close: () => {} }),
      onReconnect,
    );

    listeners.get("open")?.({ data: "" });
    expect(onReconnect).not.toHaveBeenCalled();
    listeners.get("open")?.({ data: "" });
    listeners.get("open")?.({ data: "" });
    expect(onReconnect).toHaveBeenCalledTimes(2);
    stop();
  });

  it("refetches every read a frame feeds, whatever the frame would have said", async () => {
    const qc = new QueryClient();
    const fed = [
      ["/v1/library/status"],
      ["/v1/library/items", { limit: 60 }],
      ["/v1/channels"],
      ["/v1/jobs"],
    ];
    for (const key of fed) await qc.prefetchQuery({ queryKey: key, queryFn: () => "x" });
    await qc.prefetchQuery({ queryKey: ["/v1/auth/me"], queryFn: () => "me" });

    await invalidateAfterReconnect(qc);

    // The library status is fed only by an idle scan frame; a reconnect refetches it regardless.
    for (const key of fed) expect(qc.getQueryState(key)?.isInvalidated, String(key[0])).toBe(true);
    expect(qc.getQueryState(["/v1/auth/me"])?.isInvalidated).toBe(false);
  });
});

describe("the browser stream", () => {
  class FakeSource {
    static readonly CLOSED = 2;
    static made: FakeSource[] = [];
    readyState = 1;
    private readonly listeners = new Map<string, ((event: unknown) => void)[]>();
    constructor(readonly url: string) {
      FakeSource.made.push(this);
    }
    addEventListener(type: string, listener: (event: unknown) => void) {
      this.listeners.set(type, [...(this.listeners.get(type) ?? []), listener]);
    }
    emit(type: string, event: unknown = {}) {
      for (const listener of this.listeners.get(type) ?? []) listener(event);
    }
    close() {
      this.readyState = FakeSource.CLOSED;
    }
  }

  beforeEach(() => {
    FakeSource.made = [];
    vi.useFakeTimers();
    vi.stubGlobal("EventSource", FakeSource);
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("opens a new source, with its listeners, in place of one a refused response closed", () => {
    const onChannel = vi.fn();
    const stream = browserEventStreamFactory("/v1/events");
    stream.addEventListener("channel", onChannel);

    // A dropped connection: the browser retries this one itself, so no second source is made.
    FakeSource.made[0]?.emit("error");
    vi.advanceTimersByTime(10_000);
    expect(FakeSource.made).toHaveLength(1);

    // A refused response: the source is closed for good.
    const first = FakeSource.made[0];
    if (first) first.readyState = FakeSource.CLOSED;
    first?.emit("error");
    vi.advanceTimersByTime(10_000);
    expect(FakeSource.made).toHaveLength(2);
    FakeSource.made[1]?.emit("channel", { data: "{}" });
    expect(onChannel).toHaveBeenCalledOnce();
    stream.close();
  });

  it("opens no new source once it is closed", () => {
    const stream = browserEventStreamFactory("/v1/events");
    const first = FakeSource.made[0];
    if (first) first.readyState = FakeSource.CLOSED;
    first?.emit("error");
    stream.close();
    vi.advanceTimersByTime(10_000);
    expect(FakeSource.made).toHaveLength(1);
  });
});
