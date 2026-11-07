import {
  getAddFavouriteChannelMockHandler,
  getChannelGuideMockHandler,
  getMyChannelsMockHandler,
  getRecordChannelTuneMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { useWatchChannels } from "./use-watch-channels";

const timeline = (id: string, number: number) => ({
  airings: [],
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

const stubGuide = () =>
  server.use(
    getChannelGuideMockHandler({ channels: [timeline("ch-1", 1), timeline("ch-2", 2)], fromMs: 0, toMs: 0 }),
    getMyChannelsMockHandler(mine(["ch-2"], ["ch-1", "ch-2"])),
  );

describe("useWatchChannels", () => {
  it("lists the guide's channels with the viewer's favourites and recents", async () => {
    stubGuide();
    const { result } = renderHook(() => useWatchChannels("ch-1"), { wrapper });
    await waitFor(() => expect(result.current.drawer.channels).toHaveLength(2));
    await waitFor(() => expect(result.current.drawer.favourites).toEqual(["ch-2"]));
    expect(result.current.drawer.recent).toEqual(["ch-1", "ch-2"]);
    // Previous is the newest recent that isn't the tuned channel.
    expect(result.current.phone?.previousId).toBe("ch-2");
  });

  it("has no phone panel before a channel is tuned", async () => {
    stubGuide();
    const { result } = renderHook(() => useWatchChannels(undefined), { wrapper });
    await waitFor(() => expect(result.current.drawer.channels).toHaveLength(2));
    expect(result.current.phone).toBeUndefined();
  });

  it("takes a star's answer as the viewer's lists", async () => {
    stubGuide();
    server.use(getAddFavouriteChannelMockHandler(mine(["ch-2", "ch-1"], ["ch-1", "ch-2"])));
    const { result } = renderHook(() => useWatchChannels("ch-1"), { wrapper });
    await waitFor(() => expect(result.current.drawer.favourites).toEqual(["ch-2"]));
    act(() => result.current.drawer.onFavourite("ch-1", true));
    await waitFor(() => expect(result.current.drawer.favourites).toEqual(["ch-2", "ch-1"]));
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
    const { result } = renderHook(() => useWatchChannels("ch-2"), { wrapper });
    await waitFor(() => expect(result.current.drawer.recent).toEqual(["ch-1", "ch-2"]));
    act(() => result.current.onTuneSettled("ch-2"));
    await waitFor(() => expect(result.current.drawer.recent).toEqual(["ch-2", "ch-1"]));
    expect(recorded).toBe("ch-2");
  });
});
