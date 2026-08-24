import type { ChannelTracksOutputBody } from "@loomarr/api";
import {
  getChannelPlayUrlMockHandler,
  getChannelTimelineMockHandler,
  getChannelTracksMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { toast } from "sonner";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui";
import type { LivePlaybackState } from "@/components/ui/video-player";
import { channel } from "@/test/fixtures/channels";
import { server } from "@/test/msw/server";
import { ChannelWatch } from "./channel-watch";

const hls = vi.hoisted(() => ({
  status: "playing",
  attach: vi.fn(() => () => undefined),
  liveTransport: {
    state: {
      mode: "live",
      lagSeconds: 0,
      viewerTimeMs: 1_000_000,
      noticeRevision: 0,
    } as LivePlaybackState,
    play: vi.fn(),
    pause: vi.fn(),
    goLive: vi.fn(),
  },
}));

vi.mock("../use-hls-player", () => ({
  useHlsPlayer: () => ({
    status: hls.status,
    playbackSessionId: "playback_1",
    attach: hls.attach,
    liveTransport: hls.liveTransport,
  }),
}));
vi.mock("@/diagnostics/client-reporter", () => ({ clientDiagnostics: { record: vi.fn() } }));

const makeWrapper = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <TooltipProvider>{children}</TooltipProvider>
    </QueryClientProvider>
  );
};

// stubTracks makes GET /v1/channels/:id/tracks return the given media tracks, and
// reports whether it was asked at all — the last test's whole claim is that it was NOT.
//
// ⚠ The stub this replaced ended in a catch-all `jsonResponse(200, {})`, so any other request the
// player made was answered with an empty object and nothing said so. It also matched on the
// substring "/tracks", which would have accepted that path under any resource.
const stubTracks = (tracks: Partial<ChannelTracksOutputBody> = {}) => {
  let probed = false;
  server.use(
    // ⚠ The player also reads the channel timeline; the OLD catch-all answered it with `{}`, so
    // the strip rendered against an empty object and nothing said so. The guard named it.
    getChannelTimelineMockHandler({ airings: [] }),
    // ⚠ And the play-url mint. THREE requests this component makes were answered by the old
    // `json({})` catch-all — timeline, play-url and any other — so three code paths ran against
    // an empty object with nothing to say so.
    getChannelPlayUrlMockHandler({
      url: "http://localhost/hls/master.m3u8",
      relativeUrl: "/hls/master.m3u8",
      expiresAt: "2026-08-09T23:59:59Z",
    }),
    getChannelTracksMockHandler(() => {
      probed = true;
      return { audio: tracks.audio ?? [], subtitles: tracks.subtitles ?? [] };
    }),
  );
  return { wasProbed: () => probed };
};

// ⚠ `as ChannelDTO` is GONE. The cast silenced eleven missing required fields — it is the exact
// escape hatch the shared `channel()` fixture exists to remove, and a component reading
// `pendingCount` off this object would have seen undefined where the server always sends a number.
const live = channel({ id: "ch-1", name: "Late Night Noir", number: 42, status: "live" });

describe("ChannelWatch pickers", () => {
  beforeEach(() => {
    hls.status = "playing";
    hls.liveTransport.state = {
      mode: "live",
      lagSeconds: 0,
      viewerTimeMs: 1_000_000,
      noticeRevision: 0,
    };
  });
  // The audio control lives IN the player's bar (V47), so the player must be running
  // before they render.
  //
  // ⚠ **No click any more: Watch tunes in on mount (§9.1 V54).** This used to press the
  // "Watch live" poster, which no longer exists for a playing channel — the poster is now reserved
  // for paused/off-air, where there genuinely is nothing to play. Asserting the player is present
  // instead of clicking to summon it keeps the test on the behaviour rather than on the affordance
  // that used to precede it.
  const startWatching = async () => {
    expect(await screen.findByRole("button", { name: "Audio" })).toBeInTheDocument();
  };

  it("does not probe the network-mounted source until the first frame is playing", async () => {
    const { wasProbed } = stubTracks();
    hls.status = "loading";

    const { rerender } = render(<ChannelWatch channel={live} isAdmin onSavePolicy={vi.fn()} />, {
      wrapper: makeWrapper(),
    });

    expect(await screen.findByText("Tuning in…")).toBeInTheDocument();
    expect(wasProbed()).toBe(false);
    hls.status = "playing";
    rerender(<ChannelWatch channel={live} isAdmin onSavePolicy={vi.fn()} />);
    await waitFor(() => expect(wasProbed()).toBe(true));
  });

  it("builds the Audio menu from the AIRING media's tracks, not a hardcoded list", async () => {
    // The airing programme carries English + Russian audio — so those, and only those (plus Auto),
    // are the choices. A hardcoded list would show French/Spanish/Japanese, which this asserts absent.
    stubTracks({
      audio: [
        { index: 0, language: "eng" },
        { index: 1, language: "rus" },
      ],
    });

    render(<ChannelWatch channel={live} isAdmin onSavePolicy={vi.fn()} />, { wrapper: makeWrapper() });
    await startWatching();

    // Open the Audio menu (an icon button in the player bar). Its items are menuitemcheckboxes.
    await userEvent.click(await screen.findByRole("button", { name: "Audio" }));
    expect(await screen.findByRole("menuitemcheckbox", { name: /English/ })).toBeInTheDocument();
    expect(screen.getByRole("menuitemcheckbox", { name: /Russian/ })).toBeInTheDocument();
    expect(screen.getByRole("menuitemcheckbox", { name: /Auto/ })).toBeInTheDocument();
    // Nothing the media does not carry.
    expect(
      screen.queryByRole("menuitemcheckbox", { name: /French|Spanish|Japanese/ }),
    ).not.toBeInTheDocument();
  });

  it("does not fetch tracks for a paused channel (nothing airing to probe)", async () => {
    const { wasProbed } = stubTracks();

    render(<ChannelWatch channel={channel({ status: "paused" })} isAdmin onSavePolicy={vi.fn()} />, {
      wrapper: makeWrapper(),
    });

    await screen.findByText(/off air/i);
    // ⚠ The handler simply never fires. The old form asked whether any recorded url CONTAINED
    // "/tracks" — true only of the spelling the test itself chose. And if a paused channel ever
    // did fetch something unmodelled, the unhandled-request guard now fails this test by name
    // rather than a catch-all answering it.
    expect(wasProbed()).toBe(false);
  });

  it("renders accessible Channel Up/Down controls that share the tuner step action", async () => {
    stubTracks();
    const step = vi.fn();
    const ready = vi.fn();
    render(
      <ChannelWatch
        channel={live}
        isAdmin
        onSavePolicy={vi.fn()}
        tuner={{ canSurf: true, ready, step, retry: vi.fn() }}
      />,
      { wrapper: makeWrapper() },
    );
    await startWatching();
    expect(ready).toHaveBeenCalledWith("ch-1");

    await userEvent.click(screen.getByRole("button", { name: "Channel up" }));
    await userEvent.click(screen.getByRole("button", { name: "Channel down" }));
    expect(step.mock.calls.map(([direction]) => direction)).toEqual([1, -1]);
  });

  it("keeps programme context on the viewer's paused broadcast time", async () => {
    stubTracks();
    hls.liveTransport.state = {
      mode: "paused",
      lagSeconds: 30,
      viewerTimeMs: 1_030_000,
      noticeRevision: 0,
    };
    server.use(
      getChannelTimelineMockHandler({
        airings: [{ kind: "program", title: "Paused Programme", startMs: 1_000_000, stopMs: 1_090_000 }],
      }),
    );

    render(<ChannelWatch channel={live} isAdmin onSavePolicy={vi.fn()} />, { wrapper: makeWrapper() });

    expect(await screen.findByText("0:30")).toBeInTheDocument();
    expect(screen.getByText("1m left")).toBeInTheDocument();
  });

  it("explains when an expired paused point returns the viewer live", async () => {
    stubTracks();
    const notice = vi.spyOn(toast, "info");
    const { rerender } = render(<ChannelWatch channel={live} isAdmin onSavePolicy={vi.fn()} />, {
      wrapper: makeWrapper(),
    });
    hls.liveTransport.state = { ...hls.liveTransport.state, noticeRevision: 1 };

    rerender(<ChannelWatch channel={live} isAdmin onSavePolicy={vi.fn()} />);

    await waitFor(() =>
      expect(notice).toHaveBeenCalledWith("That paused point is no longer available, so you're back live."),
    );
  });
});

describe("ChannelWatch — Open in media server hand-off", () => {
  beforeEach(() => {
    hls.status = "playing";
  });

  it("opens the media server's URL in a new tab (a real hand-off, not a toast)", async () => {
    stubTracks();
    const open = vi.spyOn(window, "open").mockReturnValue(null);
    render(
      <ChannelWatch
        channel={live}
        isAdmin
        onSavePolicy={vi.fn()}
        mediaServerName="Emby"
        mediaServerUrl="http://emby.home:8096"
      />,
      { wrapper: makeWrapper() },
    );

    await userEvent.click(await screen.findByRole("button", { name: "Open in Emby" }));

    expect(open).toHaveBeenCalledWith("http://emby.home:8096", "_blank", "noopener,noreferrer");
    open.mockRestore();
  });

  it("hides the button when no media-server URL is configured (no dead affordance)", async () => {
    stubTracks();
    render(<ChannelWatch channel={live} isAdmin onSavePolicy={vi.fn()} />, { wrapper: makeWrapper() });

    await screen.findByRole("button", { name: "Audio" });
    expect(screen.queryByRole("button", { name: /Open in/ })).not.toBeInTheDocument();
  });
});
