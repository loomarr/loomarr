import type { ChannelSubtitlesOutputBody, ChannelTracksOutputBody } from "@loomarr/api";
import {
  getChannelPlayUrlMockHandler,
  getChannelSubtitlesMockHandler,
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
import { TooltipProvider } from "@/components/ui";
import type { LivePlaybackState } from "@/components/ui/video-player";
import { channel } from "@/test/fixtures/channels";
import { server } from "@/test/msw/server";
import { ChannelWatch } from "./channel-watch";

const hls = vi.hoisted(() => ({
  status: "playing",
  onManifest: undefined as (() => void) | undefined,
  ladder: { rungs: [] } as { rungs: { height: number; index: number }[]; height?: number },
  quality: undefined as unknown,
  subtitleCues: undefined as { cues: { text: string }[]; label: string; language: string } | undefined,
  alternateAudio: false as boolean | undefined,
  attach: vi.fn(() => () => undefined),
  playbackFacts: () => ({ bufferMs: 8_000, height: 1080, url: "/v1/stream/ch-1.m3u8", width: 1920 }),
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

vi.mock("../use-hls-player", () => ({
  useHlsPlayer: (
    _channelId: string,
    _attempt?: unknown,
    onManifest?: () => void,
    quality?: unknown,
    subtitleCues?: typeof hls.subtitleCues,
    alternateAudio?: boolean,
  ) => {
    hls.onManifest = onManifest;
    hls.quality = quality;
    hls.subtitleCues = subtitleCues;
    hls.alternateAudio = alternateAudio;
    return {
      status: hls.status,
      playbackSessionId: "playback_1",
      attach: hls.attach,
      liveTransport: hls.liveTransport,
      ladder: hls.ladder,
      playbackFacts: hls.playbackFacts,
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
  // The player chrome is the shared @loomarr/ui one, which main.tsx themes app-wide.
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <LoomarrProvider theme="dark">
        <TooltipProvider>{children}</TooltipProvider>
      </LoomarrProvider>
    </QueryClientProvider>
  );
};

// The tuner's wash: up while a channel is tuning in, under the chrome.
const wash = () => document.querySelector<HTMLElement>("[data-wash]");
const tuningIn = () => waitFor(() => expect(wash()).not.toBeNull());

// The Settings sheet, opened from the transport's ⚙.
const openSettings = async () => {
  await userEvent.click(await screen.findByRole("button", { name: "Settings" }));
  return screen.getByRole("dialog", { name: "Settings" });
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
      return {
        audio: tracks.audio ?? [],
        subtitles: tracks.subtitles ?? [],
        defaultAudio: tracks.defaultAudio ?? 0,
        alternateAudio: tracks.alternateAudio,
      };
    }),
    // The subtitles sidecar: no programme subtitled unless a test says so.
    getChannelSubtitlesMockHandler({ airings: [] }),
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
  // Watch tunes in on mount (§9.1 V54): the chrome's transport is there once the player is.
  const startWatching = async () => {
    expect(await screen.findByRole("button", { name: "Settings" })).toBeInTheDocument();
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

  it("lists Settings' audio from the AIRING media's tracks, not a hardcoded list", async () => {
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

    const settings = await openSettings();
    const languages = within(settings).getByRole("list", { name: "Audio language" });
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

  it("offers the programme's second language as a choice, and plays the Alternate rendition once chosen", async () => {
    // The stream's Default carries English and its Alternate Spanish (#2088 decision 6): the viewer
    // picks between the two, and the choice switches the player to the master's Alternate.
    localStorage.removeItem("loomarr.player.audio");
    stubTracks({
      audio: [
        { index: 0, language: "eng", languageName: "English" },
        { index: 1, language: "spa", languageName: "Español" },
      ],
      defaultAudio: 0,
      alternateAudio: 1,
    });

    render(<ChannelWatch channel={live} isAdmin onLeave={vi.fn()} />, { wrapper: makeWrapper() });
    await startWatching();

    const settings = await openSettings();
    const audio = await within(settings).findByRole("radiogroup", { name: "Audio" });
    expect(within(audio).getByRole("radio", { name: "English · stereo" })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    expect(hls.alternateAudio).toBe(false);

    await userEvent.click(within(audio).getByRole("radio", { name: "Español · stereo" }));
    await waitFor(() => expect(hls.alternateAudio).toBe(true));
    expect(localStorage.getItem("loomarr.player.audio")).toBe(JSON.stringify("spa"));
    localStorage.removeItem("loomarr.player.audio");
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

  it("steps channels from the arrow keys through the tuner, once its manifest has warmed the neighbours", async () => {
    stubTracks();
    const step = vi.fn();
    const ready = vi.fn();
    render(
      <ChannelWatch
        channel={live}
        isAdmin
        onLeave={vi.fn()}
        tuner={{ canSurf: true, ready, step, tune: vi.fn(), retry: vi.fn() }}
      />,
      { wrapper: makeWrapper() },
    );
    await startWatching();
    // A playing player alone is not the signal: neighbours warm when the target's manifest arrives.
    expect(ready).not.toHaveBeenCalled();
    act(() => hls.onManifest?.());
    expect(ready).toHaveBeenCalledWith("ch-1");

    // The live player takes focus on mount, so the arrows reach it without a click.
    await userEvent.keyboard("{ArrowUp}{ArrowDown}");
    expect(step.mock.calls.map(([direction]) => direction)).toEqual([1, -1]);
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

    it("steps 10 s back from ⟲ and forward from ⟳ once behind", async () => {
      stubTracks();
      hls.liveTransport.state = {
        mode: "behind",
        lagSeconds: 60,
        viewerTimeMs: 1_000_000,
        noticeRevision: 0,
      };
      render(<ChannelWatch channel={live} isAdmin onLeave={vi.fn()} />, { wrapper: makeWrapper() });

      await userEvent.click(await screen.findByRole("button", { name: "Back 10 seconds" }));
      await userEvent.click(screen.getByRole("button", { name: "Forward 10 seconds" }));
      expect(hls.liveTransport.seek.mock.calls.map(([, at]) => at)).toEqual([990_000, 1_010_000]);
    });

    it("offers no forward from live", async () => {
      stubTracks();
      render(<ChannelWatch channel={live} isAdmin onLeave={vi.fn()} />, { wrapper: makeWrapper() });
      expect(await screen.findByRole("button", { name: "Forward 10 seconds" })).toHaveAttribute(
        "aria-disabled",
        "true",
      );
    });

    it("starts over at the programme's start while the window still holds it", async () => {
      stubTracks();
      server.use(programmeFrom(500_000));
      render(<ChannelWatch channel={live} isAdmin onLeave={vi.fn()} />, { wrapper: makeWrapper() });

      await userEvent.click(await screen.findByRole("button", { name: "Start over" }));
      expect(hls.liveTransport.seek).toHaveBeenCalledWith(expect.any(HTMLVideoElement), 500_000);
    });

    it("has no Start over for a programme that began before the window", async () => {
      stubTracks();
      // Live is 1_060_000; the window reaches back fifteen minutes, to 160_000.
      server.use(programmeFrom(100_000));
      render(<ChannelWatch channel={live} isAdmin onLeave={vi.fn()} />, { wrapper: makeWrapper() });

      expect(await screen.findByText("Airing Programme")).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Start over" })).not.toBeInTheDocument();
    });
  });

  it("goes live from the chrome when the viewer is behind", async () => {
    stubTracks();
    hls.liveTransport.state = { mode: "behind", lagSeconds: 23, viewerTimeMs: 977_000, noticeRevision: 0 };
    render(<ChannelWatch channel={live} isAdmin onLeave={vi.fn()} />, { wrapper: makeWrapper() });

    expect(await screen.findByText("23s behind")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Go live" }));
    expect(hls.liveTransport.goLive).toHaveBeenCalledWith(expect.any(HTMLVideoElement));
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

    // The lower third reads the programme under the paused playhead, and how far behind live it is.
    expect(await screen.findByText("Paused Programme")).toBeInTheDocument();
    expect(screen.getByText(/m in · 30s behind$/)).toBeInTheDocument();
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

// Full-bleed (#2088): the route has no app nav, so the player's ✕ is the only way out, and the
// channel's audio is set on its info page, not here.
describe("ChannelWatch — full-bleed route", () => {
  beforeEach(() => {
    hls.status = "playing";
  });

  it("leaves from the player's ✕", async () => {
    stubTracks();
    const onLeave = vi.fn();
    render(<ChannelWatch channel={live} isAdmin onLeave={onLeave} />, { wrapper: makeWrapper() });

    await userEvent.click(await screen.findByRole("button", { name: "Leave Watch" }));
    expect(onLeave).toHaveBeenCalledOnce();
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

    const settings = await openSettings();
    // A list to read, not a choice to make: nothing in it is a control.
    expect(within(settings).getByRole("list", { name: "Audio language" })).toBeInTheDocument();
    expect(within(settings).queryByRole("radio")).not.toBeInTheDocument();
    expect(within(settings).getByText(/An admin sets it on the channel’s page/)).toBeInTheDocument();
  });

  // Subtitles (#2088 step 7): the sidecar lists the tracks of the programme at the playhead (the
  // mock's viewer time is 1,000,000), offered in Settings, on the CC tool and on C; the viewer's
  // choice is remembered, asked of the sidecar, and its cues go to the player.
  describe("subtitles", () => {
    // Named in themselves by the server, as every client shows them.
    const english = { language: "eng", languageName: "English", title: "" };
    const spanish = { language: "spa", languageName: "Español", title: "" };
    const asked: URLSearchParams[] = [];
    // One programme around the playhead carrying English and Spanish; with a pick, that track's cue.
    const stubSubtitles = (body?: ChannelSubtitlesOutputBody) => {
      stubTracks();
      server.use(
        getChannelSubtitlesMockHandler(({ request }) => {
          const params = new URL(request.url).searchParams;
          asked.push(params);
          const language = params.get("language");
          return (
            body ?? {
              airings: [
                {
                  startMs: 900_000,
                  stopMs: 2_000_000,
                  tracks: [english, spanish],
                  cues: language ? [{ startMs: 1_000_500, endMs: 1_002_000, text: `${language} cue` }] : [],
                },
              ],
            }
          );
        }),
      );
    };
    beforeEach(() => {
      localStorage.clear();
      asked.length = 0;
    });

    it("offers Off and each track in Settings; a pick's cues are played and the pick remembered", async () => {
      stubSubtitles();
      render(<ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} />, { wrapper: makeWrapper() });

      const group = within(await openSettings()).getByRole("radiogroup", { name: "Subtitles" });
      expect(
        within(group)
          .getAllByRole("radio")
          .map((o) => o.getAttribute("aria-label")),
      ).toEqual(["Off", "English", "Español"]);
      expect(within(group).getByRole("radio", { name: "Off" })).toBeChecked();
      expect(hls.subtitleCues).toBeUndefined();
      // Off asks for the tracks alone: no extraction for a viewer not reading them.
      expect(asked.at(-1)?.has("language")).toBe(false);

      await userEvent.click(within(group).getByRole("radio", { name: "Español" }));
      await waitFor(() => expect(hls.subtitleCues?.cues.map((c) => c.text)).toEqual(["spa cue"]));
      expect(hls.subtitleCues?.language).toBe("spa");
      expect(asked.at(-1)?.get("language")).toBe("spa");
      expect(JSON.parse(localStorage.getItem("loomarr.player.subtitles") ?? "")).toEqual(spanish);
    });

    it("steps from the track shown to the next with the CC tool", async () => {
      localStorage.setItem("loomarr.player.subtitles", JSON.stringify(english));
      stubSubtitles();
      render(<ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} />, { wrapper: makeWrapper() });

      await userEvent.click(await screen.findByRole("button", { name: "Next subtitles" }));
      await waitFor(() => expect(hls.subtitleCues?.cues.map((c) => c.text)).toEqual(["spa cue"]));
    });

    it("steps from the last track to off with C", async () => {
      localStorage.setItem("loomarr.player.subtitles", JSON.stringify(spanish));
      stubSubtitles();
      render(<ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} />, { wrapper: makeWrapper() });
      await waitFor(() => expect(hls.subtitleCues?.language).toBe("spa"));

      act(() => (screen.getByRole("button", { name: "Settings" }) as HTMLElement).focus());
      await userEvent.keyboard("c");
      await waitFor(() => expect(hls.subtitleCues).toBeUndefined());
      expect(localStorage.getItem("loomarr.player.subtitles")).toBe('"off"');
    });

    it("has no Subtitles, CC or C row where the programme at the playhead carries no track", async () => {
      // The subtitled programme has ended before the playhead; the one airing carries none.
      localStorage.setItem("loomarr.player.subtitles", JSON.stringify(english));
      stubSubtitles({
        airings: [
          { startMs: 0, stopMs: 999_000, tracks: [english], cues: [] },
          { startMs: 999_000, stopMs: 2_000_000, tracks: [], cues: [] },
        ],
      });
      render(<ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} />, { wrapper: makeWrapper() });

      await waitFor(() => expect(asked.length).toBeGreaterThan(0));
      expect(
        within(await openSettings()).queryByRole("radiogroup", { name: "Subtitles" }),
      ).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: /subtitles/i })).not.toBeInTheDocument();
      expect(hls.subtitleCues).toBeUndefined();
    });
  });

  // Quality (#2088 decision 7): Auto naming the rung on air, then the channel's own rungs; the
  // viewer's choice goes to the player and is remembered.
  describe("quality", () => {
    beforeEach(() => {
      localStorage.clear();
      hls.ladder = {
        height: 2160,
        rungs: [
          { height: 1080, index: 0 },
          { height: 2160, index: 1 },
        ],
      };
    });
    afterEach(() => {
      hls.ladder = { rungs: [] };
    });

    it("offers Auto, naming the rung on air, and the channel's rungs; a pick is played and remembered", async () => {
      stubTracks();
      render(<ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} />, { wrapper: makeWrapper() });

      const quality = within(await openSettings()).getByRole("radiogroup", { name: "Quality" });
      const options = within(quality).getAllByRole("radio");
      expect(options.map((o) => o.getAttribute("aria-label"))).toEqual(["Auto · 4K", "1080p", "4K"]);
      expect(within(quality).getByRole("radio", { name: "Auto · 4K" })).toBeChecked();
      expect(hls.quality).toBe("auto");

      await userEvent.click(within(quality).getByRole("radio", { name: "1080p" }));
      expect(hls.quality).toBe(1080);
      expect(within(quality).getByRole("radio", { name: "1080p" })).toBeChecked();
      expect(localStorage.getItem("loomarr.player.quality")).toBe("1080");
    });

    it("shows Auto chosen when the remembered rung isn't offered here, because Auto plays", async () => {
      localStorage.setItem("loomarr.player.quality", "720");
      stubTracks();
      render(<ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} />, { wrapper: makeWrapper() });

      const quality = within(await openSettings()).getByRole("radiogroup", { name: "Quality" });
      expect(within(quality).getByRole("radio", { name: "Auto · 4K" })).toBeChecked();
    });

    it("has no Quality while the player offers no ladder", async () => {
      hls.ladder = { rungs: [] };
      stubTracks();
      render(<ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} />, { wrapper: makeWrapper() });

      expect(
        within(await openSettings()).queryByRole("radiogroup", { name: "Quality" }),
      ).not.toBeInTheDocument();
    });
  });

  it("closes Settings from its ✕ and from Escape, handing focus back to ⚙", async () => {
    stubTracks();
    render(<ChannelWatch channel={live} isAdmin onLeave={vi.fn()} />, { wrapper: makeWrapper() });

    await openSettings();
    await userEvent.click(screen.getByRole("button", { name: "Close Settings" }));
    expect(screen.queryByRole("dialog", { name: "Settings" })).not.toBeInTheDocument();

    await openSettings();
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("dialog", { name: "Settings" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Settings" })).toHaveFocus();
  });

  // Playback info (#2088 decision 13): from Settings, Settings' ⚙ stays lit, and closing goes back to ⚙.
  it("opens Playback info from Settings, and closing it hands focus back to ⚙", async () => {
    stubTracks();
    render(<ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} />, { wrapper: makeWrapper() });

    await userEvent.click(within(await openSettings()).getByRole("button", { name: "Playback info" }));
    const sheet = screen.getByRole("dialog", { name: "Playback info" });
    expect(within(sheet).getByText("1920×1080")).toBeInTheDocument();
    expect(within(sheet).getByText("8.0 s")).toBeInTheDocument();
    // A member reads no server side.
    expect(within(sheet).queryByText("Session")).not.toBeInTheDocument();

    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("dialog", { name: "Playback info" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Settings" })).toHaveFocus();
  });

  // The key map (#2088 decision 11): Esc closes a sheet first and leaves only from the bare picture;
  // `?` lists the keys this Watch answers.
  it("leaves from Escape when no sheet is open", async () => {
    stubTracks();
    const onLeave = vi.fn();
    render(<ChannelWatch channel={live} isAdmin={false} onLeave={onLeave} />, { wrapper: makeWrapper() });

    await openSettings();
    await userEvent.keyboard("{Escape}");
    expect(onLeave).not.toHaveBeenCalled();
    await userEvent.keyboard("{Escape}");
    expect(onLeave).toHaveBeenCalledOnce();
  });

  it("lists the keys from ?, naming only what this Watch answers", async () => {
    stubTracks();
    render(<ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} />, { wrapper: makeWrapper() });

    // Without a tuner the player takes no focus on mount, so put it on the frame's own control.
    act(() => screen.getByRole("button", { name: "Settings" }).focus());
    await userEvent.keyboard("?");
    const sheet = screen.getByRole("dialog", { name: "Keyboard shortcuts" });
    expect(within(sheet).getByText("Mute")).toBeInTheDocument();
    expect(within(sheet).getByText("Fullscreen")).toBeInTheDocument();
    // No tuner, no programme, no picture-in-picture in jsdom: none of their rows.
    expect(within(sheet).queryByText(/Channel up/)).not.toBeInTheDocument();
    expect(within(sheet).queryByText("Info")).not.toBeInTheDocument();
    expect(within(sheet).queryByText("Picture-in-picture")).not.toBeInTheDocument();

    await userEvent.keyboard("?");
    expect(screen.queryByRole("dialog", { name: "Keyboard shortcuts" })).not.toBeInTheDocument();
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

    it("opens Info from ⓘ and from I, reading the programme and what is up next", async () => {
      stubTracks();
      withProgramme();
      render(<ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} />, { wrapper: makeWrapper() });

      await userEvent.click(await screen.findByRole("button", { name: "Info" }));
      const info = screen.getByRole("dialog", { name: "Info" });
      expect(within(info).getByText("A storm cuts the island off.")).toBeInTheDocument();
      expect(within(info).getByText("1994 · TV-PG")).toBeInTheDocument();
      expect(within(info).getByText(/^Up next · .+ · Next Programme$/)).toBeInTheDocument();

      await userEvent.keyboard("{Escape}");
      expect(screen.queryByRole("dialog", { name: "Info" })).not.toBeInTheDocument();
      await userEvent.keyboard("i");
      expect(screen.getByRole("dialog", { name: "Info" })).toBeInTheDocument();
    });

    it("has no Info while nothing is known to be airing", async () => {
      stubTracks();
      render(<ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} />, { wrapper: makeWrapper() });

      expect(await screen.findByRole("button", { name: "Settings" })).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Info" })).not.toBeInTheDocument();
    });

    it("closes Info by itself after eight seconds while the channel plays", async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true });
      stubTracks();
      withProgramme();
      render(<ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} />, { wrapper: makeWrapper() });

      await userEvent.click(await screen.findByRole("button", { name: "Info" }));
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

      await userEvent.click(await screen.findByRole("button", { name: "Info" }));
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
      await screen.findByRole("button", { name: "Settings" });
      fireEvent.play(document.querySelector("video") as HTMLVideoElement);

      expect(await screen.findByText("CH 7 · Coast 90s · 1990s · Portland, ME")).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Pause" })).not.toBeInTheDocument();
      act(() => vi.advanceTimersByTime(5_500));
      expect(screen.queryByText("CH 7 · Coast 90s · 1990s · Portland, ME")).not.toBeInTheDocument();
    });
  });

  it("covers the picture when the stream stops, with Retry and the way out", async () => {
    stubTracks();
    hls.status = "error";
    const retry = vi.fn();
    const onLeave = vi.fn();
    render(
      <ChannelWatch
        channel={live}
        isAdmin
        onLeave={onLeave}
        tuner={{ canSurf: true, ready: vi.fn(), step: vi.fn(), tune: vi.fn(), retry }}
      />,
      { wrapper: makeWrapper() },
    );

    const cover = await screen.findByRole("alert");
    expect(cover).toHaveTextContent("The stream stopped.");
    expect(cover).toHaveTextContent("CH 42 · Late Night Noir");
    // The cover takes the chrome's place: no playbar or transport over a stream that stopped.
    expect(screen.queryByRole("button", { name: "Settings" })).not.toBeInTheDocument();
    await userEvent.click(within(cover).getByRole("button", { name: "Retry channel" }));
    expect(retry).toHaveBeenCalledOnce();
    await userEvent.click(within(cover).getByRole("button", { name: "Leave" }));
    expect(onLeave).toHaveBeenCalledOnce();
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
    const view = render(
      <ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} tuner={tunerFor()} />,
      {
        wrapper: makeWrapper(),
      },
    );
    await screen.findByRole("button", { name: "Settings" });
    hls.status = "loading";
    view.rerender(
      <ChannelWatch channel={target} isAdmin={false} onLeave={vi.fn()} tuner={tunerFor(target)} />,
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
    view.rerender(<ChannelWatch channel={next} isAdmin={false} onLeave={vi.fn()} tuner={tunerFor()} />);
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
    view.rerender(<ChannelWatch channel={next} isAdmin={false} onLeave={vi.fn()} tuner={tunerFor()} />);
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
    render(<ChannelWatch channel={next} isAdmin onLeave={vi.fn()} tuner={tunerFor(next)} />, {
      wrapper: makeWrapper(),
    });
    await tuningIn();
    expect(switchSound).not.toHaveBeenCalled();
  });

  it("lets any viewer turn the sound off from Settings, and remembers it", async () => {
    hls.status = "playing";
    render(<ChannelWatch channel={live} isAdmin={false} onLeave={vi.fn()} tuner={tunerFor()} />, {
      wrapper: makeWrapper(),
    });
    const settings = await openSettings();
    const toggle = within(settings).getByRole("switch", { name: "Channel-change sound" });
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
    const { rerender } = render(
      <ChannelWatch channel={live} isAdmin onLeave={vi.fn()} tuner={tunerFor()} />,
      { wrapper: makeWrapper() },
    );
    await screen.findByRole("button", { name: "Settings" });

    hls.status = "loading";
    rerender(<ChannelWatch channel={next} isAdmin onLeave={vi.fn()} tuner={tunerFor(next)} />);

    await waitFor(() => expect(wash()).toHaveAttribute("data-wash", "draining"));
  });

  it("does not drain on a cold start, where no frame is held", async () => {
    hls.status = "loading";
    render(<ChannelWatch channel={next} isAdmin onLeave={vi.fn()} tuner={tunerFor(next)} />, {
      wrapper: makeWrapper(),
    });

    await tuningIn();
    expect(wash()).toHaveAttribute("data-wash", "rest");
  });

  it("names the channel in the readout and keeps the OSD for screen readers only", async () => {
    hls.status = "loading";
    render(<ChannelWatch channel={next} isAdmin onLeave={vi.fn()} tuner={tunerFor(next)} />, {
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
    await screen.findByRole("button", { name: "Settings" });
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
    await screen.findByRole("button", { name: "Settings" });
    await userEvent.keyboard("7{Enter}");
    expect(tune).toHaveBeenCalledWith("ch-2");

    await userEvent.keyboard("9{Enter}");
    expect(await screen.findByText("NO SUCH CHANNEL")).toBeInTheDocument();
    expect(tune).toHaveBeenCalledTimes(1);
  });
});

// A portrait phone (§9, mock "portrait"): the picture bare, the chrome's planes in a panel under it.
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

  const renderPhone = (favourites: string[] = []) => {
    stubTracks();
    const step = vi.fn();
    const tune = vi.fn();
    const onFavourite = vi.fn();
    render(
      <ChannelWatch
        channel={live}
        isAdmin={false}
        onLeave={vi.fn()}
        tuner={{ canSurf: true, ready: vi.fn(), step, tune, retry: vi.fn() }}
        drawer={{ channels: [], favourites, recent: [], nowPercent: 0, onFavourite }}
        previousId="ch-9"
      />,
      { wrapper: makeWrapper() },
    );
    return { onFavourite, step, tune };
  };

  // The drawer's star is not on a phone, so the panel's is the only way to fill Favorites there.
  it("pins and unpins the tuned channel from the panel's star", async () => {
    const { onFavourite } = renderPhone();
    await userEvent.click(await screen.findByRole("button", { name: "Add to Favorites" }));
    expect(onFavourite).toHaveBeenCalledWith("ch-1", true);
  });

  it("shows the tuned channel as pinned when it is a favourite", async () => {
    const { onFavourite } = renderPhone(["ch-1"]);
    await userEvent.click(await screen.findByRole("button", { name: "Remove from Favorites" }));
    expect(onFavourite).toHaveBeenCalledWith("ch-1", false);
  });

  it("draws the panel under the picture, not the overlay's volume and window tools", async () => {
    renderPhone();
    const picture = document.querySelector("video")?.parentElement as HTMLElement;
    const play = await screen.findByRole("button", { name: "Play" });
    expect(picture).not.toContainElement(play);
    for (const name of ["Leave Watch", "Previous", "Channel up", "Channel down", "Settings"])
      expect(screen.getByRole("button", { name })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Mute" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Channels" })).not.toBeInTheDocument();
  });

  it("draws the station-ID card on the picture as a tune settles", async () => {
    renderPhone();
    const picture = document.querySelector("video")?.parentElement as HTMLElement;
    await screen.findByRole("button", { name: "Play" });
    fireEvent.play(document.querySelector("video") as HTMLVideoElement);
    expect(await within(picture).findByText(`CH ${live.number} · ${live.name}`)).toBeInTheDocument();
  });

  it("opens a sheet in the panel's place and gives the panel back when it closes", async () => {
    renderPhone();
    await userEvent.click(await screen.findByRole("button", { name: "Settings" }));
    expect(screen.getByRole("dialog", { name: "Settings" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Leave Watch" })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Close Settings" }));
    expect(screen.getByRole("button", { name: "Leave Watch" })).toBeInTheDocument();
  });

  it("steps and returns through the tuner, and plays through the live transport", async () => {
    const { step, tune } = renderPhone();
    await userEvent.click(await screen.findByRole("button", { name: "Channel up" }));
    await userEvent.click(screen.getByRole("button", { name: "Channel down" }));
    await userEvent.click(screen.getByRole("button", { name: "Previous" }));
    // jsdom's <video> never leaves `paused`, so the disc offers Play.
    await userEvent.click(screen.getByRole("button", { name: "Play" }));
    expect(step.mock.calls.map(([direction]) => direction)).toEqual([1, -1]);
    expect(tune).toHaveBeenCalledWith("ch-9");
    expect(hls.liveTransport.play).toHaveBeenCalledWith(expect.any(HTMLVideoElement));
  });
});
