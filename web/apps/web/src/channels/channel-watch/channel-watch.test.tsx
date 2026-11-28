import type { ChannelTracksOutputBody } from "@loomarr/api";
import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
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
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { LivePlaybackState } from "@/components/loomarr/player/video-player";
import { channel } from "@/test/fixtures/channels";
import { server } from "@/test/msw/server";
import type { WatchPlayer, WatchTuneReport } from "../use-watch-player";
import { useWatchTune } from "../use-watch-tune";
import { watchLineupChannels } from "../watch-lineup";
import { type ChannelWatchProps, ChannelWatch as WatchSurface } from "./channel-watch";

// The player the route hands in (useWatchPlayer), as a test double: what it reports is set per test.
const hls = vi.hoisted(() => ({
  status: "playing" as "loading" | "playing" | "error",
  attemptId: 1 as number | undefined,
  paused: false,
  attach: vi.fn(() => () => undefined),
  retry: vi.fn(),
  tune: vi.fn(),
  setQuality: vi.fn(),
  liveTransport: {
    state: { mode: "live", lagSeconds: 0, viewerTimeMs: 1_000_000, noticeRevision: 0 } as LivePlaybackState,
    play: vi.fn(),
    pause: vi.fn(),
    goLive: vi.fn(),
    seek: vi.fn(),
  },
}));
vi.mock("@/diagnostics/client-reporter", () => ({ clientDiagnostics: { record: vi.fn() } }));
const staticLock = vi.hoisted(() => vi.fn());
const switchSound = vi.hoisted(() =>
  vi.fn<(video: HTMLVideoElement, options: { enabled: boolean }) => { lock: () => void } | undefined>(() => ({
    lock: staticLock,
  })),
);
vi.mock("../switch-sound", () => ({ startChannelSwitchSound: switchSound }));
// The message line: what the screen says, as the app's toaster shows it.
const toast = vi.hoisted(() => vi.fn());
vi.mock("sonner", () => ({ toast }));

const CHANNELS = [
  channel({ id: "ch-1", name: "Late Night Noir", number: 1, status: "live" }),
  channel({ id: "ch-2", name: "Toon Town", number: 2, status: "live" }),
  channel({ id: "ch-3", name: "Game Night", number: 3, status: "live" }),
];
const [noir, toons, games] = CHANNELS as [ChannelDTO, ChannelDTO, ChannelDTO];

const watchPlayer = (tuned: ChannelDTO, overrides: Partial<WatchPlayer>): WatchPlayer => ({
  attach: hls.attach,
  ...(hls.attemptId === undefined ? {} : { attemptId: hls.attemptId }),
  channel: tuned,
  ladder: { rungs: [] },
  liveTransport: hls.liveTransport,
  playbackSessionId: "playback_1",
  quality: "auto",
  retry: hls.retry,
  setQuality: hls.setQuality,
  paused: hls.paused,
  status: hls.status,
  tune: hls.tune,
  ...overrides,
});

// Where the route's player says a tune began: the route's own tune hears it.
const reports: { current: ((report: WatchTuneReport) => void) | undefined } = { current: undefined };
const back = { label: "Back", onPress: vi.fn() };
const onGuide = vi.fn();
const onTuneSettled = vi.fn();
const favourites = { ids: [] as string[] };
const setFavourite = vi.fn(async (channelId: string, favourite: boolean) => {
  favourites.ids = favourite
    ? [...favourites.ids, channelId]
    : favourites.ids.filter((id) => id !== channelId);
});

// The route in miniature: the tune lives beside the player, and the screen mounts once there is a
// channel (`mounted`), as the route's does. Reads the double when it renders.
const ChannelWatch = ({
  channel: tuned = noir,
  isAdmin = false,
  mounted = true,
  player,
}: Partial<Omit<ChannelWatchProps, "player" | "tune">> & {
  mounted?: boolean;
  player?: Partial<WatchPlayer>;
}) => {
  const tune = useWatchTune();
  reports.current = tune.onTune;
  if (!mounted) return null;
  return (
    <WatchSurface
      back={back}
      channel={tuned}
      channels={{
        lineup: {
          channels: watchLineupChannels(CHANNELS, [], Date.now()),
          favouriteIds: favourites.ids,
          lastTunedId: undefined,
          recentIds: [],
        },
        onTuneSettled,
        setFavourite,
      }}
      isAdmin={isAdmin}
      onGuide={onGuide}
      player={watchPlayer(tuned, player ?? {})}
      tune={tune}
    />
  );
};

const makeWrapper = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <LoomarrProvider theme="dark">{children}</LoomarrProvider>
    </QueryClientProvider>
  );
};

const stubTracks = (tracks: Partial<ChannelTracksOutputBody> = {}) => {
  let probed = false;
  server.use(
    getChannelTimelineMockHandler({ serverNowMs: 1_000_000, airings: [] }),
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
    getChannelSubtitlesMockHandler({ cues: [], pending: false, tracks: [] }),
  );
  return { wasProbed: () => probed };
};

const mount = (props: Parameters<typeof ChannelWatch>[0] = {}) => {
  const view = render(<ChannelWatch {...props} />, { wrapper: makeWrapper() });
  return {
    ...view,
    again: (next: Parameters<typeof ChannelWatch>[0] = {}) => view.rerender(<ChannelWatch {...next} />),
  };
};
const press = (key: string) => fireEvent.keyDown(document.body, { key });
const menu = (name: string) => screen.getByRole("dialog", { name });
const numeral = () => screen.getByTestId("lower-third-numeral").textContent;
const openMore = async () => {
  await userEvent.click(await screen.findByRole("button", { name: "More" }));
  return menu("More");
};
const report = (
  attemptId: number,
  channelId: string,
  warm: boolean,
  reason: WatchTuneReport["reason"] = "channel",
) => act(() => reports.current?.({ attemptId, channelId, reason, warm }));

beforeEach(() => {
  sessionStorage.setItem("loomarr.player.first-play", "1");
  localStorage.setItem("loomarr.player.first-run-hint", "seen");
  hls.status = "playing";
  hls.attemptId = 1;
  hls.paused = false;
  favourites.ids = [];
  hls.liveTransport.state = { mode: "live", lagSeconds: 0, viewerTimeMs: 1_000_000, noticeRevision: 0 };
  stubTracks();
});
afterEach(() => {
  localStorage.clear();
  vi.clearAllMocks();
});

describe("ChannelWatch: the screen's requests go to the player", () => {
  it("flips with the arrows at once, showing the lower third and tuning through the player", () => {
    mount();
    press("ArrowDown");
    expect(hls.tune).toHaveBeenCalledWith("ch-2");
    expect(numeral()).toBe("2");
    expect(screen.queryByRole("listbox", { name: "Channels" })).toBeNull();
  });

  it("pauses and resumes with Space, and End goes live", () => {
    mount();
    press(" ");
    expect(hls.liveTransport.pause).toHaveBeenCalledOnce();
    press(" ");
    expect(hls.liveTransport.play).toHaveBeenCalledOnce();
    press(" ");
    press("End");
    expect(hls.liveTransport.goLive).toHaveBeenCalledOnce();
  });

  it("mutes the player's own element with M, and the quiet row says so", async () => {
    mount();
    const video = document.querySelector("video") as HTMLVideoElement;
    await screen.findByRole("button", { name: "Mute (M)" });
    press("m");
    expect(await screen.findByRole("button", { name: "Unmute (M)" })).toBeInTheDocument();
    expect(video.muted).toBe(true);
  });

  it("opens the Guide from G, and leaves by the named back control", async () => {
    mount();
    press("g");
    expect(onGuide).toHaveBeenCalledOnce();
    await userEvent.click(screen.getByRole("button", { name: "Back" }));
    expect(back.onPress).toHaveBeenCalledOnce();
  });

  it("tunes typed digits on Enter", () => {
    mount();
    press("3");
    press("Enter");
    expect(hls.tune).toHaveBeenCalledWith("ch-3");
  });

  it("moves through the live window with the arrows across, never forward of live", () => {
    hls.liveTransport.state = { mode: "paused", lagSeconds: 60, viewerTimeMs: 1_000_000, noticeRevision: 0 };
    const { again } = mount();
    press("ArrowLeft");
    expect(hls.liveTransport.seek).toHaveBeenCalledWith(990_000);
    hls.liveTransport.state = { mode: "live", lagSeconds: 0, viewerTimeMs: 1_000_000, noticeRevision: 0 };
    again();
    press("ArrowRight");
    expect(hls.liveTransport.seek).toHaveBeenCalledOnce();
  });
});

describe("ChannelWatch: a tune from outside, and no echo", () => {
  it("tells the screen of a channel change it did not ask for", () => {
    const { again } = mount();
    again({ channel: games });
    expect(numeral()).toBe("3");
    expect(hls.tune).not.toHaveBeenCalled();
  });

  it("does not tell back a tune the screen asked for, even one the player reaches late", () => {
    const { again } = mount();
    press("ArrowDown");
    press("ArrowDown");
    expect(hls.tune.mock.calls.map(([id]) => id)).toEqual(["ch-2", "ch-3"]);
    // The player lands on the first request after the second was asked for.
    again({ channel: toons });
    expect(numeral()).toBe("3");
    again({ channel: games });
    expect(numeral()).toBe("3");
    // Caught up, a later change from outside is told again.
    again({ channel: noir });
    expect(numeral()).toBe("1");
  });
});

describe("ChannelWatch: the tune follows the player", () => {
  it("draws a short burst for a flip to a warmed channel, which clears by itself", async () => {
    mount();
    report(2, "ch-2", true);
    expect(screen.getByTestId("tune-static-burst")).toBeInTheDocument();
    await waitFor(() => expect(screen.getByTestId("tune-static-none")).toBeInTheDocument());
  });

  it("holds static for a channel that must start, with Tuning in, until its picture plays", () => {
    hls.status = "playing";
    const { again } = mount();
    report(2, "ch-2", false);
    hls.status = "loading";
    hls.attemptId = 2;
    again({ channel: toons });
    expect(screen.getByTestId("tune-static-cold")).toBeInTheDocument();
    expect(screen.getByText(/^Tuning in…/)).toBeInTheDocument();
    // There is nothing to pause while tuning in.
    press(" ");
    expect(hls.liveTransport.pause).not.toHaveBeenCalled();
    hls.status = "playing";
    again({ channel: toons });
    expect(screen.getByTestId("tune-static-none")).toBeInTheDocument();
  });

  it("treats the page's first tune as tuning in, even to a warmed channel", () => {
    hls.status = "loading";
    mount();
    report(1, "ch-1", true);
    expect(screen.getByTestId("tune-static-cold")).toBeInTheDocument();
  });

  it("ends a tune-in that fails, and shows the stopped cover with Retry and the way out", async () => {
    const { again } = mount();
    report(2, "ch-2", false);
    hls.status = "error";
    hls.attemptId = 2;
    again({ channel: toons, player: { failureCause: "stream" } });
    expect(screen.getByTestId("tune-static-none")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /retry/i }));
    expect(hls.retry).toHaveBeenCalledOnce();
  });

  it("covers a channel that is off air, and probes nothing there", async () => {
    const { wasProbed } = stubTracks();
    mount({ channel: channel({ id: "ch-1", number: 1, status: "paused" }) });
    expect(await screen.findAllByText(/off air/i)).not.toHaveLength(0);
    expect(wasProbed()).toBe(false);
  });

  it("starts the channel-change sound once a picture has played, and cuts it when the next one plays", async () => {
    const { again } = mount();
    hls.status = "loading";
    again({ channel: toons });
    await waitFor(() =>
      expect(switchSound).toHaveBeenCalledWith(expect.any(HTMLVideoElement), { enabled: true }),
    );
    hls.status = "playing";
    again({ channel: toons });
    await waitFor(() => expect(staticLock).toHaveBeenCalledOnce());
  });

  it("stays silent on the page's first tune", () => {
    hls.status = "loading";
    mount();
    expect(switchSound).not.toHaveBeenCalled();
  });
});

describe("ChannelWatch: the first-run hint", () => {
  it("shows until the first channel change and is remembered for the next visit", () => {
    localStorage.removeItem("loomarr.player.first-run-hint");
    mount();
    expect(screen.getByText(/Up and Down change channel/)).toBeInTheDocument();
    press("ArrowDown");
    expect(screen.queryByText(/Up and Down change channel/)).toBeNull();
    expect(localStorage.getItem("loomarr.player.first-run-hint")).toBe("seen");
  });

  it("is not shown again once remembered", () => {
    mount();
    expect(screen.queryByText(/Up and Down change channel/)).toBeNull();
  });
});

describe("ChannelWatch: the shipped menus in the screen's slot", () => {
  it("lists Audio's tracks from the airing media, and keeps the Channel-change sound switch", async () => {
    stubTracks({
      audio: [
        { index: 0, language: "eng" },
        { index: 1, language: "rus" },
      ],
    });
    mount();
    await userEvent.click(await screen.findByRole("button", { name: "Audio (A)" }));
    const languages = within(menu("Audio")).getByRole("list", { name: "Audio language" });
    await waitFor(() => expect(within(languages).getAllByRole("listitem")).toHaveLength(3));
    const toggle = within(menu("Audio")).getByRole("switch", { name: "Channel-change sound" });
    await userEvent.click(toggle);
    expect(localStorage.getItem("loomarr.player.channel-change-sound")).toBe("off");
  });

  it("lists the programme's subtitles and draws the chosen track's line", async () => {
    server.use(
      getChannelSubtitlesMockHandler(({ request }) => {
        const track = new URL(request.url).searchParams.get("track");
        return {
          cues: track ? [{ endMs: 1_002_000, startMs: 999_000, text: `Line in ${track}` }] : [],
          pending: false,
          tracks: [{ forced: false, hearingImpaired: false, key: "en", language: "en", title: "" }],
        };
      }),
    );
    mount();
    press("c");
    const choices = within(menu("Subtitles")).getByRole("radiogroup", { name: "Subtitles" });
    await userEvent.click(await within(choices).findByRole("radio", { name: "English" }));
    expect(await screen.findByText("Line in en")).toBeInTheDocument();
    expect(localStorage.getItem("loomarr.player.subtitles")).toBe("en");
  });

  it("keeps More's rows but Channels and Info, with Picture and Playback details as its pages", async () => {
    const ladder = {
      height: 2160,
      rungs: [
        { height: 1080, index: 0 },
        { height: 2160, index: 1 },
      ],
    };
    mount({ player: { ladder } });
    const more = await openMore();
    expect(within(more).queryByRole("button", { name: "Channels" })).toBeNull();
    expect(within(more).queryByRole("button", { name: "Info" })).toBeNull();
    await userEvent.click(within(more).getByRole("button", { name: /^Picture(?! in picture)/ }));
    const quality = within(menu("Picture")).getByRole("radiogroup", { name: "Picture quality" });
    await userEvent.click(within(quality).getByRole("radio", { name: "1080p" }));
    expect(hls.setQuality).toHaveBeenCalledWith(1080);
    await userEvent.click(screen.getByRole("button", { name: "Back from Picture" }));
    await userEvent.click(within(menu("More")).getByRole("button", { name: "Playback details" }));
    expect(menu("Playback details")).toBeInTheDocument();
  });

  it("offers Go live from More while behind live, and Last channel once one was left", async () => {
    // Behind live by a seek the screen did not ask for: the player's own word says so.
    hls.liveTransport.state = { mode: "behind", lagSeconds: 60, viewerTimeMs: 1_000_000, noticeRevision: 0 };
    mount();
    await userEvent.click(within(await openMore()).getByRole("button", { name: /^Go live/ }));
    expect(hls.liveTransport.goLive).toHaveBeenCalledOnce();
    press("ArrowDown");
    const more = await openMore();
    await userEvent.click(within(more).getByRole("button", { name: /^Last channel/ }));
    expect(hls.tune).toHaveBeenLastCalledWith("ch-1");
  });

  it("goes live with End after seeks it did not ask for", () => {
    const { again } = mount();
    press("End");
    expect(hls.liveTransport.goLive).not.toHaveBeenCalled();
    hls.liveTransport.state = { mode: "behind", lagSeconds: 30, viewerTimeMs: 970_000, noticeRevision: 0 };
    again();
    press("End");
    expect(hls.liveTransport.goLive).toHaveBeenCalledOnce();
  });

  it("goes live from More after a Start over three seconds into the programme", async () => {
    const { again } = mount();
    hls.liveTransport.state = { mode: "behind", lagSeconds: 3, viewerTimeMs: 997_000, noticeRevision: 0 };
    again();
    await userEvent.click(within(await openMore()).getByRole("button", { name: /^Go live/ }));
    expect(hls.liveTransport.goLive).toHaveBeenCalledOnce();
  });

  it("goes live with End after a Start over three seconds into the programme", () => {
    const { again } = mount();
    hls.liveTransport.state = { mode: "behind", lagSeconds: 3, viewerTimeMs: 997_000, noticeRevision: 0 };
    again();
    press("End");
    expect(hls.liveTransport.goLive).toHaveBeenCalledOnce();
  });

  it("takes a pause from outside the screen, so Space resumes it", () => {
    const { again } = mount();
    hls.paused = true;
    again();
    press(" ");
    expect(hls.liveTransport.play).toHaveBeenCalledOnce();
    expect(hls.liveTransport.pause).not.toHaveBeenCalled();
  });

  it("stars and unstars the watched channel from More, and says so", async () => {
    const { again } = mount();
    await userEvent.click(within(await openMore()).getByRole("button", { name: "Add to Favourites" }));
    expect(setFavourite).toHaveBeenLastCalledWith("ch-1", true);
    await waitFor(() => expect(toast).toHaveBeenLastCalledWith("1 Late Night Noir added to Favourites"));
    again();
    await userEvent.click(within(await openMore()).getByRole("button", { name: "Remove from Favourites" }));
    expect(setFavourite).toHaveBeenLastCalledWith("ch-1", false);
    await waitFor(() => expect(toast).toHaveBeenLastCalledWith("1 Late Night Noir removed from Favourites"));
  });

  it("says a star the server refused, and changes nothing", async () => {
    setFavourite.mockRejectedValueOnce(new Error("refused"));
    const { again } = mount();
    await userEvent.click(within(await openMore()).getByRole("button", { name: "Add to Favourites" }));
    await waitFor(() =>
      expect(toast).toHaveBeenLastCalledWith("1 Late Night Noir could not be added to Favourites"),
    );
    again();
    expect(within(await openMore()).getByRole("button", { name: "Add to Favourites" })).toBeInTheDocument();
  });

  it("shows the first tune of a visit as tuning in when the screen mounts after the player began it", () => {
    hls.status = "loading";
    const { again } = mount({ mounted: false });
    // The player tunes the URL's channel before the page has a channel to draw.
    report(1, "ch-1", true);
    again({ mounted: true });
    expect(screen.getByTestId("tune-static-cold")).toBeInTheDocument();
    expect(screen.getByText(/^Tuning in…/)).toBeInTheDocument();
    press(" ");
    expect(hls.liveTransport.pause).not.toHaveBeenCalled();
    hls.status = "playing";
    again();
    expect(screen.getByTestId("tune-static-none")).toBeInTheDocument();
    press(" ");
    expect(hls.liveTransport.pause).toHaveBeenCalledOnce();
  });

  it("shows a reconnect to the channel on screen as reconnecting, with no static", () => {
    const { again } = mount();
    report(1, "ch-1", true);
    report(2, "ch-1", false, "retry");
    hls.status = "loading";
    hls.attemptId = 2;
    again();
    expect(screen.getByText("Reconnecting")).toBeInTheDocument();
    expect(screen.queryByTestId("tune-static-cold")).toBeNull();
  });

  it("offers Play when the browser refuses to start the picture by itself", async () => {
    hls.status = "loading";
    mount();
    const video = document.querySelector("video") as HTMLVideoElement;
    act(() => {
      video.dispatchEvent(new Event("pause"));
    });
    await userEvent.click(screen.getByRole("button", { name: "Play" }));
    expect(hls.liveTransport.play).toHaveBeenCalledOnce();
    act(() => {
      video.dispatchEvent(new Event("playing"));
    });
    expect(screen.queryByText("Ready to play")).toBeNull();
  });

  it("names full screen for the browser's real state, entering on F and leaving by the browser's own", () => {
    const request = vi.fn(async () => undefined);
    Object.defineProperty(document.documentElement, "requestFullscreen", {
      configurable: true,
      value: request,
    });
    mount();
    expect(screen.getByRole("button", { name: "Full screen (F)" })).toBeInTheDocument();
    press("f");
    expect(request).toHaveBeenCalledOnce();
    Object.defineProperty(document, "fullscreenElement", { configurable: true, value: document.body });
    act(() => {
      document.dispatchEvent(new Event("fullscreenchange"));
    });
    expect(screen.getByRole("button", { name: "Leave full screen (F)" })).toBeInTheDocument();
    // The browser's own Esc leaves it.
    Object.defineProperty(document, "fullscreenElement", { configurable: true, value: null });
    act(() => {
      document.dispatchEvent(new Event("fullscreenchange"));
    });
    expect(screen.getByRole("button", { name: "Full screen (F)" })).toBeInTheDocument();
    Reflect.deleteProperty(document, "fullscreenElement");
    Reflect.deleteProperty(document.documentElement, "requestFullscreen");
  });

  it("says in the message line why a key did nothing", () => {
    mount();
    press("l");
    expect(toast).toHaveBeenCalledWith("There is no last channel yet");
  });

  it("follows the player's mute through its chrome, from M and back", async () => {
    mount();
    press("m");
    expect(await screen.findByRole("button", { name: "Unmute (M)" })).toBeInTheDocument();
    press("m");
    expect(await screen.findByRole("button", { name: "Mute (M)" })).toBeInTheDocument();
    expect((document.querySelector("video") as HTMLVideoElement).muted).toBe(false);
  });

  describe("seeking the window", () => {
    const programmeFrom = (startMs: number) =>
      getChannelTimelineMockHandler({
        serverNowMs: 1_060_000,
        airings: [
          { kind: "program", scheduleBlockId: "now", startMs, stopMs: 4_000_000, title: "Airing Programme" },
        ],
      });

    it("starts over, from More, at the programme's start while the window still holds it", async () => {
      server.use(programmeFrom(500_000));
      mount();
      await screen.findByText("Airing Programme");
      await userEvent.click(within(await openMore()).getByRole("button", { name: /^Start over/ }));
      expect(hls.liveTransport.seek).toHaveBeenCalledWith(500_000);
    });

    it("has no Start over for a programme that began before the window", async () => {
      // Live is 1_060_000; the window reaches back fifteen minutes, to 160_000.
      server.use(programmeFrom(100_000));
      mount();
      await screen.findByText("Airing Programme");
      expect(within(await openMore()).queryByRole("button", { name: /^Start over/ })).toBeNull();
    });
  });

  it("offers Picture in picture only where the browser has it", async () => {
    // jsdom has no picture in picture.
    mount();
    expect(within(await openMore()).queryByRole("button", { name: "Picture in picture" })).toBeNull();
  });

  it("copies the playback details rows as text", async () => {
    const user = userEvent.setup();
    mount();
    await user.click(await screen.findByRole("button", { name: "More" }));
    await user.click(within(menu("More")).getByRole("button", { name: "Playback details" }));
    await user.click(within(menu("Playback details")).getByRole("button", { name: "Copy details" }));
    expect(await navigator.clipboard.readText()).toMatch(/Behind live: /);
  });

  it("shows the channel's audio read-only, even to an admin", async () => {
    stubTracks({ audio: [{ index: 0, language: "eng" }] });
    mount({ isAdmin: true });
    press("a");
    const audio = menu("Audio");
    expect(within(audio).getByRole("list", { name: "Audio language" })).toBeInTheDocument();
    expect(within(audio).queryByRole("radio")).toBeNull();
    expect(within(audio).getByText(/An admin sets it on the channel’s page/)).toBeInTheDocument();
  });

  it("asks the browser for full screen on a double click on the picture", () => {
    const request = vi.fn(async () => undefined);
    Object.defineProperty(document.documentElement, "requestFullscreen", {
      configurable: true,
      value: request,
    });
    mount();
    fireEvent.doubleClick(screen.getByTestId("watch-picture").firstElementChild as HTMLElement);
    expect(request).toHaveBeenCalledOnce();
    Reflect.deleteProperty(document.documentElement, "requestFullscreen");
  });

  it("opens the shortcuts sheet from ?", () => {
    mount();
    press("?");
    expect(within(menu("Keyboard shortcuts")).getByText("Change channel")).toBeInTheDocument();
  });
});

describe("ChannelWatch: the first play of the session", () => {
  it("starts muted with the unmute pill, and one press turns the sound on", async () => {
    sessionStorage.removeItem("loomarr.player.first-play");
    mount();
    const pill = await screen.findByRole("button", { name: "Tap to unmute" });
    expect((document.querySelector("video") as HTMLVideoElement).muted).toBe(true);
    await userEvent.click(pill);
    expect(screen.queryByRole("button", { name: "Tap to unmute" })).toBeNull();
    expect((document.querySelector("video") as HTMLVideoElement).muted).toBe(false);
  });

  it("records the tune once its first frame plays", () => {
    mount();
    expect(onTuneSettled).toHaveBeenCalledWith("ch-1");
  });
});
