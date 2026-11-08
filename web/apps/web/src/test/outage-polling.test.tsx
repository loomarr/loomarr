import * as channelsApi from "@loomarr/api/endpoints/channels";
import { QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useWatchChannels } from "@/channels/use-watch-channels";
import { server } from "@/test/msw/server";

// #2723: through a run of 503s the Watch and Guide reads must back off, not re-poll about once a
// second. The same reads Watch mounts, behind the client's own defaults.
const probe = (tunedId: string) => {
  const channels = channelsApi.useListChannels({ query: { staleTime: 30_000 } });
  const nowNext = channelsApi.useChannelsNowNext({ query: { staleTime: 15_000 } });
  const timeline = channelsApi.useChannelTimeline(tunedId, undefined, {
    query: { refetchInterval: 30_000, retry: false },
  });
  const around = useWatchChannels(tunedId);
  return { around, channels, nowNext, timeline };
};

describe("an outage", () => {
  const requests: string[] = [];

  beforeEach(() => {
    vi.useFakeTimers({
      shouldAdvanceTime: false,
      toFake: ["setTimeout", "clearTimeout", "setInterval", "clearInterval", "Date"],
    });
    requests.length = 0;
    server.use(
      http.get("*/v1/*", ({ request }) => {
        requests.push(new URL(request.url).pathname);
        return HttpResponse.json({ title: "Service Unavailable" }, { status: 503 });
      }),
    );
  });
  afterEach(() => vi.useRealTimers());

  it("asks each Watch endpoint at most 6 times in its first minute", async () => {
    const { createQueryClient } = await import("@/lib/query-client");
    const client = createQueryClient();
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    renderHook(() => probe("ch-1"), { wrapper });

    for (let second = 0; second < 60; second += 1) {
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1_000);
      });
    }

    const perEndpoint = new Map<string, number>();
    for (const path of requests) perEndpoint.set(path, (perEndpoint.get(path) ?? 0) + 1);
    // The reads did fail and ask again: this is not passing against nothing.
    expect(perEndpoint.size).toBeGreaterThan(0);
    expect(Math.max(...perEndpoint.values())).toBeLessThanOrEqual(6);
  });
});
