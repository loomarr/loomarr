import { getGetPlayoutStatusMockHandler } from "@loomarr/api/msw";
import { LoomarrProvider } from "@loomarr/design-system";
import type { LivePlaybackFacts } from "@loomarr/player/browser";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { PlaybackInfoSheet } from "./playback-info-sheet";

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    <LoomarrProvider theme="dark">{children}</LoomarrProvider>
  </QueryClientProvider>
);

const facts = (bufferMs: number): LivePlaybackFacts => ({
  bitrate: 6_000_000,
  bufferMs,
  codecs: "avc1.640028,mp4a.40.2",
  height: 1080,
  rung: { number: 1, of: 1 },
  url: "/v1/channels/ch-1/stream.m3u8?sig=abc",
  width: 1920,
});

const renderSheet = (isAdmin: boolean, playbackFacts: () => LivePlaybackFacts | undefined) =>
  render(
    <PlaybackInfoSheet
      auto
      channelId="ch-1"
      density="pointer"
      isAdmin={isAdmin}
      lagSeconds={0}
      playbackFacts={playbackFacts}
      sessionId="playback_1"
    />,
    { wrapper },
  );

const stubStatus = () =>
  server.use(
    getGetPlayoutStatusMockHandler({
      budget: { classes: {} } as never,
      capability: { hardware: true, maxChannels: 4 },
      channels: [
        {
          bufferedMs: 4_000,
          channelId: "ch-1",
          coldStartMs: 800,
          encoder: "nvidia",
          hardware: true,
          health: "ok",
          mode: "transcode",
          name: "Coast 90s",
          number: 7,
          reason: "Transcoding at 1.40× realtime — comfortable margin.",
          speed: 1.4,
          target: "1080p-h264-sdr",
          viewers: 1,
        },
        {
          bufferedMs: 0,
          channelId: "ch-1",
          coldStartMs: 0,
          encoder: "libx265",
          hardware: false,
          health: "stalled",
          mode: "transcode",
          name: "Coast 90s",
          number: 7,
          reason: "The premium encode.",
          speed: 0.5,
          target: "4k-hevc-sdr",
          viewers: 1,
        },
      ],
      gpu: {} as never,
      running: true,
    }),
  );

describe("PlaybackInfoSheet", () => {
  afterEach(() => vi.useRealTimers());

  it("reads the player again each second while open", () => {
    vi.useFakeTimers();
    let buffer = 4_000;
    renderSheet(false, () => facts(buffer));
    const stream = screen.getByRole("list", { name: "Stream" });
    expect(within(stream).getByText("Auto · 1080p (rung 1 of 1)")).toBeInTheDocument();
    expect(within(stream).getByText("H.264 High · AAC-LC")).toBeInTheDocument();
    expect(within(stream).getByText("4.0 s")).toBeInTheDocument();

    buffer = 9_500;
    act(() => vi.advanceTimersByTime(1_000));
    expect(within(stream).getByText("9.5 s")).toBeInTheDocument();
  });

  it("shows a member no server side and no stream URL", () => {
    renderSheet(false, () => facts(4_000));
    expect(screen.queryByRole("list", { name: "Admin" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Copy stream URL/ })).not.toBeInTheDocument();
  });

  it("shows an admin the browser encode in the server's words, and copies the absolute stream URL", async () => {
    stubStatus();
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
    renderSheet(true, () => facts(4_000));

    const admin = screen.getByRole("list", { name: "Admin" });
    expect(await within(admin).findByText("nvidia · hardware")).toBeInTheDocument();
    expect(
      within(admin).getByText("Transcoding at 1.40× realtime — comfortable margin."),
    ).toBeInTheDocument();
    // The rung playing is the baseline, so the row is the baseline's, not the 4K encode's.
    expect(within(admin).queryByText("The premium encode.")).not.toBeInTheDocument();
    expect(within(admin).getByText("playback_1")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /Copy stream URL/ }));
    expect(writeText).toHaveBeenCalledExactlyOnceWith(
      new URL("/v1/channels/ch-1/stream.m3u8?sig=abc", window.location.href).href,
    );
    expect(await screen.findByRole("button", { name: /Copied/ })).toBeInTheDocument();
  });

  it("reads the premium encode's row while the 4K rung plays", async () => {
    stubStatus();
    renderSheet(true, () => ({ ...facts(4_000), height: 2160, width: 3840 }));

    const admin = screen.getByRole("list", { name: "Admin" });
    expect(await within(admin).findByText("The premium encode.")).toBeInTheDocument();
    expect(within(admin).getByText("libx265 · software")).toBeInTheDocument();
  });

  it("offers no copy before the player has a source", () => {
    renderSheet(true, () => undefined);
    expect(screen.queryByRole("button", { name: /Copy stream URL/ })).not.toBeInTheDocument();
  });
});
