import {
  getChannelGuideMockHandler,
  getMyChannelsMockHandler,
  getRecordChannelTuneMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { channel } from "@/test/fixtures/channels";
import { server } from "@/test/msw/server";
import { useWatchChannels } from "./use-watch-channels";

const NOW = Date.now();

const timeline = (id: string, number: number, title?: string) => ({
  airings: title
    ? [{ kind: "program" as const, scheduleBlockId: id, startMs: NOW - 60_000, stopMs: NOW + 600_000, title }]
    : [],
  channelId: id,
  name: `Channel ${id}`,
  number,
  pendingCount: 0,
  status: "live" as const,
});

const mine = (favourites: string[], recent: string[]) => ({
  favourites: favourites.map((channelId) => ({ channelId, addedAt: "2026-11-06T20:00:00Z" })),
  recent: recent.map((channelId) => ({ channelId, tunedAt: "2026-11-06T20:00:00Z" })),
});

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    {children}
  </QueryClientProvider>
);

const CHANNELS = [channel({ id: "ch-1", number: 1 }), channel({ id: "ch-2", number: 2 })];

const stubGuide = () =>
  server.use(
    getChannelGuideMockHandler({
      channels: [timeline("ch-1", 1, "Night Shift"), timeline("ch-2", 2)],
      fromMs: 0,
      toMs: 0,
    }),
    getMyChannelsMockHandler(mine(["ch-2"], ["ch-1", "ch-2"])),
  );

describe("useWatchChannels", () => {
  it("lists every channel with what is on now, and the viewer's favourites and recents", async () => {
    stubGuide();
    const { result } = renderHook(() => useWatchChannels(CHANNELS), { wrapper });
    expect(result.current.lineup.channels.map((each) => each.id)).toEqual(["ch-1", "ch-2"]);
    await waitFor(() => expect(result.current.lineup.channels[0]?.onNow).toBe("Night Shift"));
    await waitFor(() => expect(result.current.lineup.favouriteIds).toEqual(["ch-2"]));
    expect(result.current.lineup.recentIds).toEqual(["ch-1", "ch-2"]);
    // The landing is the channel last tuned: the newest recent.
    expect(result.current.lineup.lastTunedId).toBe("ch-1");
  });

  it("lists nothing before the channels are read", () => {
    stubGuide();
    const { result } = renderHook(() => useWatchChannels(undefined), { wrapper });
    expect(result.current.lineup.channels).toEqual([]);
  });

  it("records a settled tune and takes the answer as the recents", async () => {
    stubGuide();
    let recorded: string | undefined;
    server.use(
      getRecordChannelTuneMockHandler(({ params }) => {
        recorded = String(params.channelId);
        return mine(["ch-2"], ["ch-2", "ch-1"]);
      }),
    );
    const { result } = renderHook(() => useWatchChannels(CHANNELS), { wrapper });
    await waitFor(() => expect(result.current.lineup.recentIds).toEqual(["ch-1", "ch-2"]));
    act(() => result.current.onTuneSettled("ch-2"));
    await waitFor(() => expect(result.current.lineup.recentIds).toEqual(["ch-2", "ch-1"]));
    expect(recorded).toBe("ch-2");
  });
});
