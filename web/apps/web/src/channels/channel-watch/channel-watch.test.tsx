import type { ChannelTracksOutputBody } from "@loomarr/api";
import {
  getChannelPlayUrlMockHandler,
  getChannelTimelineMockHandler,
  getChannelTracksMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor, within } from "@testing-library/react";
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
  onManifest: undefined as (() => void) | undefined,
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
const diagnosticsRecord = vi.hoisted(() => vi.fn());

vi.mock("../use-hls-player", () => ({
  useHlsPlayer: (_channelId: string, _attempt?: unknown, onManifest?: () => void) => {
    hls.onManifest = onManifest;
    return {
      status: hls.status,
      playbackSessionId: "playback_1",
      attach: hls.attach,
      liveTransport: hls.liveTransport,
    };
  },
}));
vi.mock("@/diagnostics/client-reporter", () => ({ clientDiagnostics: { record: diagnosticsRecord } }));
const staticLock = vi.hoisted(() => vi.fn());
const switchSound = vi.hoisted(() =>
  vi.fn<(video: HTMLVideoElement, options: { enabled: boolean }) => { lock: () => void } | undefined>(() => ({
    lock: staticLock,
  })),
);
vi.mock("../switch-sound", () => ({ startChannelSwitchSound: switchSound }));

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
    getChannelTimelineMockHandler({ serverNowMs: 1_000_000, airings: [] }),
    // ⚠ And the play-url mint. THREE requests this component makes were answered by the old
    // `json({})` catch-all — timeline, play-url and any other — so three code paths ran against
    // an empty object with nothing to say so.
    getChannelPlayUrlMockHandler({
      url: "http://localhost/hls/master.m3u8",
      relativeUrl: "/hls/master.m3u8",
      stillUrl: "http://localhost/still.jpg",
      relativeStillUrl: "/still.jpg",
      expiresAt: "2026-08-09T23:59:59Z",
      serverTimeMs: Date.UTC(2026, 7, 9, 23, 54, 59),
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
    diagnosticsRecord.mockReset();
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
        tuner={{ canSurf: true, ready, step, tune: vi.fn(), retry: vi.fn() }}
      />,
      { wrapper: makeWrapper() },
    );
    await startWatching();
    // A playing player alone is not the signal: neighbours warm when the target's manifest arrives.
    expect(ready).not.toHaveBeenCalled();
    act(() => hls.onManifest?.());
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
        serverNowMs: 1_060_000,
        airings: [
          {
            kind: "program",
            scheduleBlockId: "block_paused",
            title: "Paused Programme",
            startMs: 1_000_000,
            stopMs: 1_090_000,
          },
        ],
      }),
    );

    render(<ChannelWatch channel={live} isAdmin onSavePolicy={vi.fn()} />, { wrapper: makeWrapper() });

    expect(await screen.findByText("0:30")).toBeInTheDocument();
    expect(screen.getByText("1m left")).toBeInTheDocument();
    await waitFor(() =>
      expect(diagnosticsRecord).toHaveBeenCalledWith(
        expect.objectContaining({
          event: "player.schedule_block_changed",
          playbackSessionId: "playback_1",
          channelId: "ch-1",
          scheduleBlockId: "block_paused",
          viewerTimeMs: 1_030_000,
        }),
      ),
    );
    expect(diagnosticsRecord).toHaveBeenCalledWith(
      expect.objectContaining({ event: "player.playhead_drift", driftMs: expect.any(Number) }),
    );
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

// The switch readout (#1620 B4). The wash drains the picture under the loader only when there IS one:
// the previous channel's held frame. A cold start has nothing to drain.
describe("ChannelWatch switch readout", () => {
  const next = channel({ id: "ch-2", name: "Saturday Cartoons", number: 12, status: "live" });
  const tunerFor = (requestedChannel?: typeof next) => ({
    canSurf: true,
    requestedChannel,
    ready: vi.fn(),
    step: vi.fn(),
    tune: vi.fn(),
    retry: vi.fn(),
  });
  const wash = () => document.querySelector<HTMLElement>("[data-wash]");
  // Every VISIBLE place the channel is named: not the decorative readout, not the SR-only OSD.
  const visibleNames = (name: string) =>
    screen
      .queryAllByText(name)
      .filter((el) => !el.closest("[aria-hidden='true']") && !el.closest("[role='status']"));
  const switchTo = async (target: typeof next) => {
    hls.status = "playing";
    const view = render(
      <ChannelWatch channel={live} isAdmin={false} onSavePolicy={vi.fn()} tuner={tunerFor()} />,
      {
        wrapper: makeWrapper(),
      },
    );
    await screen.findByRole("button", { name: "Audio" });
    hls.status = "loading";
    view.rerender(
      <ChannelWatch channel={target} isAdmin={false} onSavePolicy={vi.fn()} tuner={tunerFor(target)} />,
    );
    await waitFor(() => expect(wash()).not.toBeNull());
    return view;
  };

  beforeEach(() => {
    stubTracks();
    switchSound.mockClear();
    staticLock.mockClear();
    localStorage.clear();
  });

  it("hides the player's channel title while the readout names the channel, and restores it with the picture", async () => {
    const view = await switchTo(next);
    expect(visibleNames("Saturday Cartoons")).toEqual([]);

    hls.status = "playing";
    view.rerender(<ChannelWatch channel={next} isAdmin={false} onSavePolicy={vi.fn()} tuner={tunerFor()} />);
    await waitFor(() => expect(visibleNames("Saturday Cartoons")).toHaveLength(1));
  });

  it("starts the dial set once when a switch begins, on the player's own element", async () => {
    await switchTo(next);
    expect(switchSound).toHaveBeenCalledTimes(1);
    expect(switchSound).toHaveBeenCalledWith(expect.any(HTMLVideoElement), { enabled: true });
  });

  it("mutes the programme under the static, then cuts the static and restores it when the picture locks", async () => {
    const view = await switchTo(next);
    const video = document.querySelector("video") as HTMLVideoElement;
    expect(video.muted).toBe(true);
    expect(staticLock).not.toHaveBeenCalled();

    hls.status = "playing";
    view.rerender(<ChannelWatch channel={next} isAdmin={false} onSavePolicy={vi.fn()} tuner={tunerFor()} />);
    await waitFor(() => expect(staticLock).toHaveBeenCalledTimes(1));
    expect(video.muted).toBe(false);
  });

  it("leaves the programme alone when the sound is off", async () => {
    localStorage.setItem("loomarr.player.channel-change-sound", "off");
    switchSound.mockReturnValueOnce(undefined);
    await switchTo(next);
    expect((document.querySelector("video") as HTMLVideoElement).muted).toBe(false);
  });

  it("stays silent on a cold start", async () => {
    hls.status = "loading";
    render(<ChannelWatch channel={next} isAdmin onSavePolicy={vi.fn()} tuner={tunerFor(next)} />, {
      wrapper: makeWrapper(),
    });
    await screen.findByText("Tuning in…");
    expect(switchSound).not.toHaveBeenCalled();
  });

  it("lets any viewer turn the sound off from the Audio menu, and remembers it", async () => {
    hls.status = "playing";
    render(<ChannelWatch channel={live} isAdmin={false} onSavePolicy={vi.fn()} tuner={tunerFor()} />, {
      wrapper: makeWrapper(),
    });
    await userEvent.click(await screen.findByRole("button", { name: "Audio" }));
    const toggle = await screen.findByRole("menuitemcheckbox", { name: "Channel-change sound" });
    expect(toggle).toHaveAttribute("aria-checked", "true");
    expect(toggle).not.toHaveAttribute("aria-disabled", "true");
    await userEvent.click(toggle);
    expect(localStorage.getItem("loomarr.player.channel-change-sound")).toBe("off");
  });

  it("passes the preference through when the viewer has turned the sound off", async () => {
    localStorage.setItem("loomarr.player.channel-change-sound", "off");
    await switchTo(next);
    expect(switchSound).toHaveBeenCalledWith(expect.any(HTMLVideoElement), { enabled: false });
  });

  it("drains the held frame when switching from a channel that played", async () => {
    hls.status = "playing";
    const { rerender } = render(
      <ChannelWatch channel={live} isAdmin onSavePolicy={vi.fn()} tuner={tunerFor()} />,
      { wrapper: makeWrapper() },
    );
    await screen.findByRole("button", { name: "Audio" });

    hls.status = "loading";
    rerender(<ChannelWatch channel={next} isAdmin onSavePolicy={vi.fn()} tuner={tunerFor(next)} />);

    await waitFor(() => expect(wash()).toHaveAttribute("data-wash", "draining"));
  });

  it("does not drain on a cold start, where no frame is held", async () => {
    hls.status = "loading";
    render(<ChannelWatch channel={next} isAdmin onSavePolicy={vi.fn()} tuner={tunerFor(next)} />, {
      wrapper: makeWrapper(),
    });

    await screen.findByText("Tuning in…");
    expect(wash()).toHaveAttribute("data-wash", "rest");
  });

  it("names the channel in the readout and keeps the OSD for screen readers only", async () => {
    hls.status = "loading";
    render(<ChannelWatch channel={next} isAdmin onSavePolicy={vi.fn()} tuner={tunerFor(next)} />, {
      wrapper: makeWrapper(),
    });

    const osd = await screen.findByRole("status");
    expect(osd).toHaveTextContent("CH 12");
    // The wash's centred channel line is the visible readout; the card would repeat it on screen.
    expect(osd).toHaveClass("sr-only");
    expect(wash()?.closest("[aria-hidden]")).toHaveTextContent("CH 12Saturday Cartoons");
  });
});

describe("ChannelWatch channels drawer and direct tune (#1659 W1)", () => {
  const rows = [
    { id: "ch-1", number: 42, name: "Late Night Noir", now: "A detective film", minutesLeft: 20, blocks: [] },
    { id: "ch-2", number: 7, name: "Saturday Cartoons", now: "A cartoon", minutesLeft: 11, blocks: [] },
  ];
  const renderWatch = (tune = vi.fn(), onTuneSettled = vi.fn(), onFavourite = vi.fn()) => {
    hls.status = "playing";
    render(
      <ChannelWatch
        channel={live}
        isAdmin={false}
        onSavePolicy={vi.fn()}
        tuner={{ canSurf: true, ready: vi.fn(), step: vi.fn(), tune, retry: vi.fn() }}
        drawer={{ channels: rows, favourites: ["ch-2"], recent: [], nowPercent: 17, onFavourite }}
        onTuneSettled={onTuneSettled}
      />,
      { wrapper: makeWrapper() },
    );
    return { tune, onTuneSettled, onFavourite };
  };

  it("records the tune once its first frame plays", async () => {
    stubTracks();
    const { onTuneSettled } = renderWatch();
    await screen.findByRole("button", { name: "Audio" });
    expect(onTuneSettled).toHaveBeenCalledTimes(1);
    expect(onTuneSettled).toHaveBeenCalledWith("ch-1");
  });

  it("opens the drawer from its button, lists favourites first, tunes a row and stars from beside it", async () => {
    stubTracks();
    const { tune, onFavourite } = renderWatch();
    await userEvent.click(await screen.findByRole("button", { name: "Channels" }));
    const favourites = screen.getByRole("region", { name: "FAVORITES" });
    expect(favourites).toHaveTextContent("Saturday Cartoons");
    expect(screen.getByRole("searchbox", { name: "Find a channel or show" })).toHaveFocus();

    await userEvent.click(within(favourites).getByRole("button", { name: /^07 Saturday Cartoons/ }));
    expect(tune).toHaveBeenCalledWith("ch-2");

    const allChannels = screen.getByRole("region", { name: "ALL CHANNELS" });
    await userEvent.click(within(allChannels).getByRole("button", { name: "Favorite Late Night Noir" }));
    expect(onFavourite).toHaveBeenCalledWith("ch-1", true);
  });

  it("finds by name without the player taking the typed keys, and Escape hands focus back", async () => {
    stubTracks();
    renderWatch();
    const opener = await screen.findByRole("button", { name: "Channels" });
    await userEvent.click(opener);
    // "m" mutes and "k" pauses on the player; in the find box they are letters.
    await userEvent.type(screen.getByRole("searchbox"), "mk");
    expect(screen.getByRole("searchbox")).toHaveValue("mk");
    expect(screen.getByText("MATCHES")).toBeInTheDocument();
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("searchbox")).not.toBeInTheDocument();
    expect(opener).toHaveFocus();
  });

  it("tunes a typed channel number, and names a number no channel has", async () => {
    stubTracks();
    const { tune } = renderWatch();
    await screen.findByRole("button", { name: "Audio" });
    await userEvent.keyboard("7{Enter}");
    expect(tune).toHaveBeenCalledWith("ch-2");

    await userEvent.keyboard("9{Enter}");
    expect(await screen.findByText("NO SUCH CHANNEL")).toBeInTheDocument();
    expect(tune).toHaveBeenCalledTimes(1);
  });
});
