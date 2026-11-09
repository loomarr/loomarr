import type { ChannelTracksOutputBody } from "@loomarr/api";
import {
  getChannelPlayUrlMockHandler,
  getChannelTimelineMockHandler,
  getChannelTracksMockHandler,
} from "@loomarr/api/msw";
import { LoomarrProvider } from "@loomarr/design-system";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { toast } from "sonner";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { LivePlaybackState } from "@/components/loomarr/player/video-player";
import { channel } from "@/test/fixtures/channels";
import { server } from "@/test/msw/server";
import type { WatchPlayer } from "../use-watch-player";
import { type ChannelWatchProps, ChannelWatch as WatchSurface } from "./channel-watch";

// The player the route hands in (useWatchPlayer), as a test double: what it reports is set per test.
const hls = vi.hoisted(() => ({
  status: "playing" as "loading" | "playing" | "error",
  attach: vi.fn(() => () => undefined),
  retry: vi.fn(),
  step: vi.fn(),
  tune: vi.fn(),
  setQuality: vi.fn(),
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
    seek: vi.fn(),
  },
}));
const diagnosticsRecord = vi.hoisted(() => vi.fn());

const watchPlayer = (overrides: Partial<WatchPlayer> = {}): WatchPlayer => ({
  attach: hls.attach,
  canSurf: true,
  ladder: { rungs: [] },
  liveTransport: hls.liveTransport,
  playbackSessionId: "playback_1",
  quality: "auto",
  retry: hls.retry,
  setQuality: hls.setQuality,
  status: hls.status,
  step: hls.step,
  tune: hls.tune,
  ...overrides,
});

// Reads the double when it renders, so a rerender sees what the test has changed since.
const ChannelWatch = ({
  player,
  ...props
}: Omit<ChannelWatchProps, "player"> & { player?: Partial<WatchPlayer> }) => (
  <WatchSurface {...props} player={watchPlayer(player)} />
);

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
  // The player chrome is the shared @loomarr/ui one, which main.tsx themes app-wide.
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <LoomarrProvider theme="dark">{children}</LoomarrProvider>
    </QueryClientProvider>
  );
};

// Every test but the first-play ones is past the session's first play (the sound is on).
beforeEach(() => sessionStorage.setItem("loomarr.player.first-play", "1"));

// The tuner's wash: up while a channel is tuning in, under the chrome.
const wash = () => document.querySelector<HTMLElement>("[data-wash]");
const tuningIn = () => waitFor(() => expect(wash()).not.toBeNull());

// The Audio sheet, opened from the top row's Audio key.
const openAudio = async () => {
  await userEvent.click(await screen.findByRole("button", { name: "Audio" }));
  return screen.getByRole("dialog", { name: "Audio" });
};

// The More sheet, opened from the keys' More.
const openMore = async () => {
  await userEvent.click(await screen.findByRole("button", { name: "More" }));
  return screen.getByRole("dialog", { name: "More" });
};

// A sheet reached through More: its row, then the sheet it opens.
const openFromMore = async (row: string) => {
  const more = await openMore();
  await userEvent.click(within(more).getByRole("button", { name: row }));
  return screen.getByRole("dialog", { name: row });
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
  // Watch tunes in on mount (§9.1 V54): the chrome's keys are there once the player is.
  const startWatching = async () => {
    expect(await screen.findByRole("button", { name: "More" })).toBeInTheDocument();
  };

  it("does not probe the network-mounted source until the first frame is playing", async () => {
    const { wasProbed } = stubTracks();
    hls.status = "loading";

    const { rerender } = render(<ChannelWatch channel={live} isAdmin onLeave={vi.fn()} />, {
      wrapper: makeWrapper(),
    });

    await tuningIn();
    expect(wasProbed()).toBe(false);
    hls.status = "playing";
    rerender(<ChannelWatch channel={live} isAdmin onLeave={vi.fn()} />);
    await waitFor(() => expect(wasProbed()).toBe(true));
  });

  it("lists the Audio sheet's tracks from the AIRING media's tracks, not a hardcoded list", async () => {
    // The airing programme carries English + Russian audio — so those, and only those (plus Auto),
    // are the choices. A hardcoded list would show French/Spanish/Japanese, which this asserts absent.
    stubTracks({
      audio: [
        { index: 0, language: "eng" },
        { index: 1, language: "rus" },
      ],
    });

    render(<ChannelWatch channel={live} isAdmin onLeave={vi.fn()} />, { wrapper: makeWrapper() });
    await startWatching();

    const audioSheet = await openAudio();
    const languages = within(audioSheet).getByRole("list", { name: "Audio language" });
    await waitFor(() => expect(within(languages).getAllByRole("listitem")).toHaveLength(3));
    const items = within(languages)
      .getAllByRole("listitem")
      .map((item) => item.textContent);
    expect(items).toEqual([
      expect.stringMatching(/Auto/),
      expect.stringMatching(/English/),
      expect.stringMatching(/Russian/),
    ]);
    // The channel's own choice (Auto, unset) is the one marked.
    expect(within(languages).getAllByRole("listitem")[0]).toHaveAttribute("aria-current", "true");
  });

  it("does not fetch tracks for a paused channel (nothing airing to probe)", async () => {
    const { wasProbed } = stubTracks();

    render(<ChannelWatch channel={channel({ status: "paused" })} isAdmin onLeave={vi.fn()} />, {
      wrapper: makeWrapper(),
    });

    await screen.findByText(/off air/i);
    // ⚠ The handler simply never fires. The old form asked whether any recorded url CONTAINED
    // "/tracks" — true only of the spelling the test itself chose. And if a paused channel ever
    // did fetch something unmodelled, the unhandled-request guard now fails this test by name
    // rather than a catch-all answering it.
    expect(wasProbed()).toBe(false);
  });

  it("steps channels from the arrow keys through the player", async () => {
    stubTracks();
    hls.step.mockClear();
    render(<ChannelWatch channel={live} isAdmin onLeave={vi.fn()} />, { wrapper: makeWrapper() });
    await startWatching();

    // The live player takes focus on mount, so the arrows reach it without a click.
    await userEvent.keyboard("{ArrowUp}{ArrowDown}");
    expect(hls.step.mock.calls.map(([direction]) => direction)).toEqual([1, -1]);
  });

  // Seeking the window (#2088 decision 2). The timeline's clock is the server's: live is 1_060_000.
  describe("seeking the window", () => {
    const programmeFrom = (startMs: number) =>
      getChannelTimelineMockHandler({
        serverNowMs: 1_060_000,
        airings: [
          { kind: "program", scheduleBlockId: "now", title: "Airing Programme", startMs, stopMs: 4_000_000 },
        ],
      });

    beforeEach(() => hls.liveTransport.seek.mockClear());

    it("starts over, from More, at the programme's start while the window still holds it", async () => {
      stubTracks();
      server.use(programmeFrom(500_000));
      render(<ChannelWatch channel={live} isAdmin onLeave={vi.fn()} />, { wrapper: makeWrapper() });

      await screen.findByText("Airing Programme");
      const more = await openMore();
      await userEvent.click(within(more).getByRole("button", { name: "Start over" }));
      expect(hls.liveTransport.seek).toHaveBeenCalledWith(500_000);
    });

    it("has no Start over for a programme that began before the window", async () => {
      stubTracks();
      // Live is 1_060_000; the window reaches back fifteen minutes, to 160_000.
      server.use(programmeFrom(100_000));
      render(<ChannelWatch channel={live} isAdmin onLeave={vi.fn()} />, { wrapper: makeWrapper() });

      expect(await screen.findByText("Airing Programme")).toBeInTheDocument();
      const more = await openMore();
      expect(within(more).queryByRole("button", { name: "Start over" })).not.toBeInTheDocument();
    });
  });

  it("goes live from the band when the viewer is behind, and from More", async () => {
    stubTracks();
    hls.liveTransport.state = { mode: "behind", lagSeconds: 23, viewerTimeMs: 977_000, noticeRevision: 0 };
    render(<ChannelWatch channel={live} isAdmin onLeave={vi.fn()} />, { wrapper: makeWrapper() });

    await userEvent.click(await screen.findByRole("button", { name: "Go live" }));
    expect(hls.liveTransport.goLive).toHaveBeenCalledOnce();
    const more = await openMore();
    await userEvent.click(within(more).getByRole("button", { name: "Go live" }));
    expect(hls.liveTransport.goLive).toHaveBeenCalledTimes(2);
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

    render(<ChannelWatch channel={live} isAdmin onLeave={vi.fn()} />, { wrapper: makeWrapper() });

    // The band reads the programme under the paused playhead, and how far behind live it is.
    expect(await screen.findByText("Paused Programme")).toBeInTheDocument();
    expect(screen.getByText("30s behind")).toBeInTheDocument();
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
    const { rerender } = render(<ChannelWatch channel={live} isAdmin onLeave={vi.fn()} />, {
      wrapper: makeWrapper(),
    });
    hls.liveTransport.state = { ...hls.liveTransport.state, noticeRevision: 1 };

    rerender(<ChannelWatch channel={live} isAdmin onLeave={vi.fn()} />);

    await waitFor(() =>
      expect(notice).toHaveBeenCalledWith("That paused point is no longer available, so you're back live."),
    );
  });
});

// Full-bleed (#2088): the route has no app nav, so Leave is the only way out, and the channel's
// audio is set on its info page, not here.
describe("ChannelWatch — full-bleed route", () => {
  beforeEach(() => {
    hls.status = "playing";
  });

  it("leaves from the top row's Leave, and from Esc once no sheet is open", async () => {
    stubTracks();
    const onLeave = vi.fn();
    render(<ChannelWatch channel={live} isAdmin onLeave={onLeave} />, { wrapper: makeWrapper() });

    await userEvent.click(await screen.findByRole("button", { name: "Leave" }));
    expect(onLeave).toHaveBeenCalledOnce();

    // Esc closes the top layer first: with a sheet open it does not leave.
    await openAudio();
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("dialog", { name: "Audio" })).not.toBeInTheDocument();
    expect(onLeave).toHaveBeenCalledOnce();
    await userEvent.keyboard("{Escape}");
    expect(onLeave).toHaveBeenCalledTimes(2);
  });

  it("flips channels from the rocker's keys", async () => {
    stubTracks();
    hls.step.mockClear();
    render(<ChannelWatch channel={live} isAdmin onLeave={vi.fn()} />, { wrapper: makeWrapper() });

    await userEvent.click(await screen.findByRole("button", { name: "Channel up" }));
    await userEvent.click(screen.getByRole("button", { name: "Channel down" }));
    expect(hls.step.mock.calls.map(([direction]) => direction)).toEqual([1, -1]);
  });

  it("has no Last key until there is a channel to return to, then tunes it from the key and from L", async () => {
    stubTracks();
    const rows = [
      { id: "ch-1", number: 42, name: "Late Night Noir", now: "A film", minutesLeft: 20, blocks: [] },
      { id: "ch-9", number: 9, name: "Movie Matinee", now: "A film", minutesLeft: 11, blocks: [] },
    ];
    const tune = vi.fn();
    const renderWith = (previousId?: string) =>
      render(
        <ChannelWatch
          channel={live}
          isAdmin={false}
          onLeave={vi.fn()}
          player={{ tune }}
          channels={{
            drawer: { channels: rows, favourites: [], recent: [], nowPercent: 0, onFavourite: vi.fn() },
            phone: { favourites: [], previousId },
            onTuneSettled: vi.fn(),
          }}
        />,
        { wrapper: makeWrapper() },
      );
    const first = renderWith();
    await screen.findByRole("button", { name: "More" });
    expect(screen.queryByRole("button", { name: /^Last channel/ })).not.toBeInTheDocument();
    first.unmount();

    renderWith("ch-9");
    await userEvent.click(await screen.findByRole("button", { name: "Last channel, 9" }));
    expect(tune).toHaveBeenCalledWith("ch-9");
    await userEvent.keyboard("l");
    expect(tune).toHaveBeenCalledTimes(2);
    // L is the last channel when there is one: it does not also go live.
    expect(hls.liveTransport.goLive).not.toHaveBeenCalled();
  });

  it("still offers the way out on an off-air channel, where no player is drawn", async () => {
    stubTracks();
    const onLeave = vi.fn();
    render(<ChannelWatch channel={channel({ status: "paused" })} isAdmin={false} onLeave={onLeave} />, {
      wrapper: makeWrapper(),
    });

    expect(await screen.findByText(/is off air$/)).toBeInTheDocument();
    expect(screen.getByText("This channel is paused, so there's nothing to watch.")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Leave" }));
    expect(onLeave).toHaveBeenCalledOnce();
  });

  it("shows the channel's audio read-only, even to an admin", async () => {
    stubTracks({ audio: [{ index: 0, language: "eng" }] });
    render(<ChannelWatch channel={live} isAdmin onLeave={vi.fn()} />, { wrapper: makeWrapper() });

    const audioSheet = await openAudio();
    // A list to read, not a choice to make: nothing in it is a control.
    expect(within(audioSheet).getByRole("list", { name: "Audio language" })).toBeInTheDocument();
    expect(within(audioSheet).queryByRole("radio")).not.toBeInTheDocument();
    expect(within(audioSheet).getByText(/An admin sets it on the channel’s page/)).toBeInTheDocument();
  });

  // Quality (#2088 decision 7): Auto naming the rung on air, then the channel's own rungs; the
  // viewer's choice goes to the player.
  describe("quality", () => {
    const ladder = {
      height: 2160,
      rungs: [
        { height: 1080, index: 0 },
        { height: 2160, index: 1 },
      ],
    };

    it("offers Auto, naming the rung on air, and the channel's rungs; a pick goes to the player", async () => {
      stubTracks();
      render(<ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} player={{ ladder }} />, {
        wrapper: makeWrapper(),
      });

      const quality = within(await openFromMore("Picture")).getByRole("radiogroup", {
        name: "Picture quality",
      });
      const options = within(quality).getAllByRole("radio");
      expect(options.map((o) => o.getAttribute("aria-label"))).toEqual(["Auto · 4K", "1080p", "4K"]);
      expect(within(quality).getByRole("radio", { name: "Auto · 4K" })).toBeChecked();

      await userEvent.click(within(quality).getByRole("radio", { name: "1080p" }));
      expect(hls.setQuality).toHaveBeenCalledWith(1080);
    });

    it("shows the remembered rung chosen when the channel offers it", async () => {
      stubTracks();
      render(
        <ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} player={{ ladder, quality: 1080 }} />,
        { wrapper: makeWrapper() },
      );

      const quality = within(await openFromMore("Picture")).getByRole("radiogroup", {
        name: "Picture quality",
      });
      expect(within(quality).getByRole("radio", { name: "1080p" })).toBeChecked();
    });

    it("shows Auto chosen when the remembered rung isn't offered here, because Auto plays", async () => {
      stubTracks();
      render(
        <ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} player={{ ladder, quality: 720 }} />,
        { wrapper: makeWrapper() },
      );

      const quality = within(await openFromMore("Picture")).getByRole("radiogroup", {
        name: "Picture quality",
      });
      expect(within(quality).getByRole("radio", { name: "Auto · 4K" })).toBeChecked();
    });

    it("has no Picture while the player offers no ladder", async () => {
      stubTracks();
      render(<ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} />, { wrapper: makeWrapper() });

      const more = await openMore();
      expect(within(more).queryByRole("button", { name: "Picture" })).not.toBeInTheDocument();
    });
  });

  it("closes a sheet from its close key and from Escape, handing focus back to what opened it", async () => {
    stubTracks();
    render(<ChannelWatch channel={live} isAdmin onLeave={vi.fn()} />, { wrapper: makeWrapper() });

    await openAudio();
    await userEvent.click(screen.getByRole("button", { name: "Close Audio" }));
    expect(screen.queryByRole("dialog", { name: "Audio" })).not.toBeInTheDocument();

    await openAudio();
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("dialog", { name: "Audio" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Audio" })).toHaveFocus();
  });

  it("keeps the keys up with a sheet open, and names the channel in the sheet's header", async () => {
    stubTracks();
    render(<ChannelWatch channel={live} isAdmin onLeave={vi.fn()} />, { wrapper: makeWrapper() });

    const more = await openMore();
    expect(screen.getByRole("button", { name: /^(Play|Pause)$/ })).toBeInTheDocument();
    expect(within(more).getByText("Late Night Noir")).toBeInTheDocument();
  });

  // Info (#2088 decision 9): the programme on air and the next one, from ⓘ or I.
  describe("Info and the station-ID card", () => {
    const withProgramme = () =>
      server.use(
        getChannelTimelineMockHandler({
          serverNowMs: 1_060_000,
          airings: [
            {
              kind: "program",
              scheduleBlockId: "now",
              title: "Airing Programme",
              description: "A storm cuts the island off.",
              year: 1994,
              rating: "TV-PG",
              startMs: 1_000_000,
              stopMs: 4_000_000,
            },
            {
              kind: "program",
              scheduleBlockId: "next",
              title: "Next Programme",
              startMs: 4_000_000,
              stopMs: 5_000_000,
            },
          ],
        }),
      );

    afterEach(() => vi.useRealTimers());

    it("opens Info from More, reading the programme and what is up next", async () => {
      stubTracks();
      withProgramme();
      render(<ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} />, { wrapper: makeWrapper() });

      await screen.findByText("Airing Programme");
      await openFromMore("Info");
      const info = screen.getByRole("dialog", { name: "Info" });
      expect(within(info).getByText("A storm cuts the island off.")).toBeInTheDocument();
      expect(within(info).getByText("1994 · TV-PG")).toBeInTheDocument();
      expect(within(info).getByText(/^Up next · .+ · Next Programme$/)).toBeInTheDocument();

      await userEvent.keyboard("{Escape}");
      expect(screen.queryByRole("dialog", { name: "Info" })).not.toBeInTheDocument();
    });

    it("has no Info while nothing is known to be airing", async () => {
      stubTracks();
      render(<ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} />, { wrapper: makeWrapper() });

      const more = await openMore();
      expect(within(more).queryByRole("button", { name: "Info" })).not.toBeInTheDocument();
    });

    it("closes Info by itself after eight seconds while the channel plays", async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      stubTracks();
      withProgramme();
      render(<ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} />, { wrapper: makeWrapper() });

      await screen.findByText("Airing Programme");
      await openFromMore("Info");
      act(() => vi.advanceTimersByTime(7_000));
      expect(screen.getByRole("dialog", { name: "Info" })).toBeInTheDocument();
      act(() => vi.advanceTimersByTime(1_500));
      expect(screen.queryByRole("dialog", { name: "Info" })).not.toBeInTheDocument();
    });

    it("keeps Info open while the viewer is paused", async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      stubTracks();
      withProgramme();
      hls.liveTransport.state = {
        mode: "paused",
        lagSeconds: 30,
        viewerTimeMs: 1_030_000,
        noticeRevision: 0,
      };
      render(<ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} />, { wrapper: makeWrapper() });

      await screen.findByText("Airing Programme");
      await openFromMore("Info");
      act(() => vi.advanceTimersByTime(20_000));
      expect(screen.getByRole("dialog", { name: "Info" })).toBeInTheDocument();
    });

    it("names the station's era and place on the card a settled tune shows, for five seconds", async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      stubTracks();
      const coast = channel({
        id: "ch-1",
        name: "Coast 90s",
        number: 7,
        status: "live",
        policy: {
          filler: { era: { from: 1990, to: 1999 }, geography: { country: "US", place: "Portland, ME" } },
        },
      });
      render(<ChannelWatch channel={coast} isAdmin={false} onLeave={vi.fn()} />, { wrapper: makeWrapper() });
      // The picture plays: the card clears the controls, which only a playing player may hide.
      await screen.findByRole("button", { name: "More" });
      fireEvent.play(document.querySelector("video") as HTMLVideoElement);

      expect(await screen.findByText("Coast 90s · 1990s · Portland, ME")).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Pause" })).not.toBeInTheDocument();
      act(() => vi.advanceTimersByTime(5_500));
      expect(screen.queryByText("Coast 90s · 1990s · Portland, ME")).not.toBeInTheDocument();
    });
  });

  // Playback details (§9.5): the one place technical terms are allowed, with only what the browser knows.
  describe("Playback details", () => {
    it("opens from More and from I as a panel, shows the real rows, and closes on Esc before it leaves", async () => {
      stubTracks();
      const onLeave = vi.fn();
      render(
        <ChannelWatch
          channel={live}
          isAdmin={false}
          onLeave={onLeave}
          player={{
            ladder: {
              height: 1080,
              onAir: {
                bitrate: 4_600_000,
                codecs: "avc1.640029,mp4a.40.2",
                frameRate: 29.97,
                height: 1080,
                videoRange: "SDR",
              },
              rungs: [],
            },
          }}
        />,
        { wrapper: makeWrapper() },
      );
      await userEvent.click(within(await openMore()).getByRole("button", { name: "Playback details" }));
      const panel = screen.getByRole("dialog", { name: "Playback details" });
      for (const text of [
        "Bitrate",
        "4.6 Mbps",
        "avc1.640029,mp4a.40.2",
        "None (SDR)",
        "Behind live",
        "Server",
      ])
        expect(within(panel).getByText(text)).toBeInTheDocument();
      expect(screen.queryByRole("dialog", { name: "More" })).not.toBeInTheDocument();

      await userEvent.keyboard("{Escape}");
      expect(screen.queryByRole("dialog", { name: "Playback details" })).not.toBeInTheDocument();
      expect(onLeave).not.toHaveBeenCalled();
      await userEvent.keyboard("i");
      expect(screen.getByRole("dialog", { name: "Playback details" })).toBeInTheDocument();
      await userEvent.click(
        within(screen.getByRole("dialog", { name: "Playback details" })).getByRole("button", {
          name: "Close Playback details",
        }),
      );
      expect(screen.queryByRole("dialog", { name: "Playback details" })).not.toBeInTheDocument();
    });

    it("copies the rows as text", async () => {
      stubTracks();
      const user = userEvent.setup();
      render(<ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} />, { wrapper: makeWrapper() });
      await user.click(within(await openMore()).getByRole("button", { name: "Playback details" }));
      await user.click(screen.getByRole("button", { name: "Copy details" }));
      // user-event stands in for the clipboard; the rows are on it as text.
      expect(await navigator.clipboard.readText()).toMatch(/Behind live: \d+\.\d s\nServer: /);
    });

    it("offers Picture in picture only where the browser has it", async () => {
      stubTracks();
      render(<ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} />, { wrapper: makeWrapper() });
      expect(
        within(await openMore()).queryByRole("button", { name: "Picture in picture" }),
      ).not.toBeInTheDocument();
    });
  });

  it("covers the picture when the stream stops, with Retry and the way out", async () => {
    stubTracks();
    hls.status = "error";
    const retry = vi.fn();
    const onLeave = vi.fn();
    render(<ChannelWatch channel={live} isAdmin onLeave={onLeave} player={{ retry }} />, {
      wrapper: makeWrapper(),
    });

    const cover = await screen.findByRole("alert");
    expect(cover).toHaveTextContent("The stream stopped.");
    expect(cover).toHaveTextContent("CH 42 · Late Night Noir");
    // The cover takes the chrome's place: no band or keys over a stream that stopped.
    expect(screen.queryByRole("button", { name: "More" })).not.toBeInTheDocument();
    await userEvent.click(within(cover).getByRole("button", { name: "Retry channel" }));
    expect(retry).toHaveBeenCalledOnce();
    await userEvent.click(within(cover).getByRole("button", { name: "Leave" }));
    expect(onLeave).toHaveBeenCalledOnce();
  });

  it.each([
    ["server", "Loomarr's server isn't answering"],
    ["channel", "This channel couldn't start"],
    ["stream", "The stream stopped."],
  ] as const)("names the %s as the cause on the stopped cover (#2722)", async (failureCause, title) => {
    stubTracks();
    hls.status = "error";
    render(<ChannelWatch channel={live} isAdmin onLeave={vi.fn()} player={{ failureCause }} />, {
      wrapper: makeWrapper(),
    });

    expect(await screen.findByRole("alert")).toHaveTextContent(title);
  });
});

// The switch readout (#1620 B4). The wash drains the picture under the loader only when there IS one:
// the previous channel's held frame. A cold start has nothing to drain.
describe("ChannelWatch switch readout", () => {
  const next = channel({ id: "ch-2", name: "Saturday Cartoons", number: 12, status: "live" });
  // Every VISIBLE place the channel is named: not the decorative readout, not the SR-only OSD, not
  // the chrome while it is hidden under the wash.
  const visibleNames = (name: string) =>
    screen
      .queryAllByText(
        (_, el) => el?.childNodes[0]?.nodeType === Node.TEXT_NODE && (el.textContent ?? "").includes(name),
      )
      .filter((el) => !el.closest("[aria-hidden='true']") && !el.closest("[role='status']"));
  const switchTo = async (target: typeof next) => {
    hls.status = "playing";
    const view = render(<ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} />, {
      wrapper: makeWrapper(),
    });
    await screen.findByRole("button", { name: "More" });
    hls.status = "loading";
    view.rerender(<ChannelWatch channel={target} isAdmin={false} onLeave={vi.fn()} />);
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
    view.rerender(<ChannelWatch channel={next} isAdmin={false} onLeave={vi.fn()} />);
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
    view.rerender(<ChannelWatch channel={next} isAdmin={false} onLeave={vi.fn()} />);
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
    render(<ChannelWatch channel={next} isAdmin onLeave={vi.fn()} />, {
      wrapper: makeWrapper(),
    });
    await tuningIn();
    expect(switchSound).not.toHaveBeenCalled();
  });

  it("lets any viewer turn the sound off from the Audio sheet, and remembers it", async () => {
    hls.status = "playing";
    render(<ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} />, {
      wrapper: makeWrapper(),
    });
    const audioSheet = await openAudio();
    const toggle = within(audioSheet).getByRole("switch", { name: "Channel-change sound" });
    expect(toggle).toBeChecked();
    expect(toggle).toBeEnabled();
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
    const { rerender } = render(<ChannelWatch channel={live} isAdmin onLeave={vi.fn()} />, {
      wrapper: makeWrapper(),
    });
    await screen.findByRole("button", { name: "More" });

    hls.status = "loading";
    rerender(<ChannelWatch channel={next} isAdmin onLeave={vi.fn()} />);

    await waitFor(() => expect(wash()).toHaveAttribute("data-wash", "draining"));
  });

  it("does not drain on a cold start, where no frame is held", async () => {
    hls.status = "loading";
    render(<ChannelWatch channel={next} isAdmin onLeave={vi.fn()} />, {
      wrapper: makeWrapper(),
    });

    await tuningIn();
    expect(wash()).toHaveAttribute("data-wash", "rest");
  });

  it("names the channel in the readout and keeps the OSD for screen readers only", async () => {
    hls.status = "loading";
    render(<ChannelWatch channel={next} isAdmin onLeave={vi.fn()} />, {
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
        onLeave={vi.fn()}
        player={{ tune }}
        channels={{
          drawer: { channels: rows, favourites: ["ch-2"], recent: [], nowPercent: 17, onFavourite },
          onTuneSettled,
        }}
      />,
      { wrapper: makeWrapper() },
    );
    return { tune, onTuneSettled, onFavourite };
  };

  it("records the tune once its first frame plays", async () => {
    stubTracks();
    const { onTuneSettled } = renderWatch();
    await screen.findByRole("button", { name: "More" });
    expect(onTuneSettled).toHaveBeenCalledTimes(1);
    expect(onTuneSettled).toHaveBeenCalledWith("ch-1");
  });

  it("opens the drawer from its button, lists favourites first, tunes a row and stars from beside it", async () => {
    stubTracks();
    const { tune, onFavourite } = renderWatch();
    await userEvent.click(within(await openMore()).getByRole("button", { name: "Channels" }));
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
    const opener = await screen.findByRole("button", { name: "More" });
    await userEvent.click(opener);
    await userEvent.click(
      within(screen.getByRole("dialog", { name: "More" })).getByRole("button", { name: "Channels" }),
    );
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
    await screen.findByRole("button", { name: "More" });
    await userEvent.keyboard("7{Enter}");
    expect(tune).toHaveBeenCalledWith("ch-2");

    await userEvent.keyboard("9{Enter}");
    expect(await screen.findByText("No such channel")).toBeInTheDocument();
    expect(tune).toHaveBeenCalledTimes(1);
  });
});

// A portrait phone (§9.7, #2775, pick A): the picture pinned under the top row, the band and keys
// below it in the black space, nothing over the picture.
describe("ChannelWatch at phone width", () => {
  beforeEach(() => {
    hls.status = "playing";
    hls.liveTransport.state = { mode: "live", lagSeconds: 0, viewerTimeMs: 1_000_000, noticeRevision: 0 };
    // Below the tablet breakpoint: `(min-width: 48rem)` does not match.
    vi.stubGlobal("matchMedia", (query: string) => ({
      matches: false,
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }));
    return () => vi.unstubAllGlobals();
  });

  const renderPhone = () => {
    stubTracks();
    const step = vi.fn();
    const tune = vi.fn();
    render(
      <ChannelWatch
        channel={live}
        isAdmin={false}
        onLeave={vi.fn()}
        player={{ step, tune }}
        channels={{
          drawer: { channels: [], favourites: [], recent: [], nowPercent: 0, onFavourite: vi.fn() },
          phone: { favourites: [], previousId: "ch-9", clockLabel: "8:26 PM" },
          onTuneSettled: vi.fn(),
        }}
      />,
      { wrapper: makeWrapper() },
    );
    return { step, tune };
  };

  it("pins the picture under the top row, with the band and keys below it", async () => {
    renderPhone();
    expect(await screen.findByRole("button", { name: "Leave" })).toBeInTheDocument();
    const video = document.querySelector("video") as HTMLVideoElement;
    expect(video.style.position).toBe("absolute");
    expect(video.style.top).toBe("80px");
    expect(video.style.objectPosition).toBe("center top");
    for (const name of ["Channel up", "Channel down", /^(Play|Pause)$/, "More", "Audio"])
      expect(screen.getByRole("button", { name })).toBeInTheDocument();
  });

  it("never hides the controls: nothing covers the picture, so there is nothing to uncover", async () => {
    renderPhone();
    await screen.findByRole("button", { name: "More" });
    expect(screen.getByRole("button", { name: "More" }).closest("[aria-hidden=true]")).toBeNull();
  });

  it("steps through the tuner, pauses through the player, and returns from More", async () => {
    const { step, tune } = renderPhone();
    await userEvent.click(await screen.findByRole("button", { name: "Channel up" }));
    await userEvent.click(screen.getByRole("button", { name: "Channel down" }));
    expect(step.mock.calls.map(([direction]) => direction)).toEqual([1, -1]);
    expect(tune).not.toHaveBeenCalled();
  });
});

// The session's first play is muted, with a pill to undo it (§9.3).
describe("ChannelWatch first play of the session", () => {
  beforeEach(() => {
    hls.status = "playing";
    sessionStorage.removeItem("loomarr.player.first-play");
  });

  it("starts muted with a Tap to unmute pill, and one tap turns the sound on", async () => {
    stubTracks();
    render(<ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} />, { wrapper: makeWrapper() });

    const pill = await screen.findByRole("button", { name: "Tap to unmute" });
    expect((document.querySelector("video") as HTMLVideoElement).muted).toBe(true);
    await userEvent.click(pill);
    expect(screen.queryByRole("button", { name: "Tap to unmute" })).not.toBeInTheDocument();
    expect((document.querySelector("video") as HTMLVideoElement).muted).toBe(false);
  });

  it("is only the first play: a second Watch in the session starts with the sound on", async () => {
    stubTracks();
    const first = render(<ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} />, {
      wrapper: makeWrapper(),
    });
    await screen.findByRole("button", { name: "Tap to unmute" });
    first.unmount();

    render(<ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} />, { wrapper: makeWrapper() });
    await screen.findByRole("button", { name: "More" });
    expect(screen.queryByRole("button", { name: "Tap to unmute" })).not.toBeInTheDocument();
    expect((document.querySelector("video") as HTMLVideoElement).muted).toBe(false);
  });

  it("drops the pill when the sound comes on by the Mute key too", async () => {
    stubTracks();
    render(<ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} />, { wrapper: makeWrapper() });
    await screen.findByRole("button", { name: "Tap to unmute" });
    await userEvent.click(screen.getByRole("button", { name: "Unmute" }));
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Tap to unmute" })).not.toBeInTheDocument(),
    );
  });
});
