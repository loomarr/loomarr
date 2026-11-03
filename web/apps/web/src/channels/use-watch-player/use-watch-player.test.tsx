import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import type { BrowserPlayer, BrowserPlayerOptions } from "@loomarr/player/browser";
import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const browser = vi.hoisted(() => ({
  controls: {
    goLive: vi.fn(),
    pause: vi.fn(),
    play: vi.fn(),
    previous: vi.fn(),
    retry: vi.fn(),
    step: vi.fn(),
    tuneChannel: vi.fn(),
    tuneNumber: vi.fn(),
  },
  options: undefined as BrowserPlayerOptions | undefined,
  snapshot: { catalog: [], recentChannelIds: [], status: "empty" } as BrowserPlayer["snapshot"],
}));

vi.mock("@loomarr/player/browser", () => ({
  useBrowserPlayer: (options: BrowserPlayerOptions): BrowserPlayer => {
    browser.options = options;
    return {
      attach: vi.fn(),
      controls: browser.controls,
      playbackSessionId: "web_session",
      snapshot: browser.snapshot,
    };
  },
}));
vi.mock("@/diagnostics/client-reporter", () => ({ clientDiagnostics: { record: vi.fn() } }));
vi.mock("../use-hls-player/device-profile", () => ({
  deviceProfile: () => ({ audio: [], video: [], video10bit: false }),
}));

import { useWatchPlayer } from "./use-watch-player";

const channel = (id: string, number: number, extra: Partial<ChannelDTO> = {}) =>
  ({ id, inAppPlayable: true, name: `Channel ${number}`, number, status: "live", ...extra }) as ChannelDTO;
const ten = channel("ten", 10);
const twenty = channel("twenty", 20);
const thirty = channel("thirty", 30);
const lineup = [ten, twenty, thirty];

const snapshot = (
  status: BrowserPlayer["snapshot"]["status"],
  tuned?: ChannelDTO,
  extra: Partial<BrowserPlayer["snapshot"]> = {},
): BrowserPlayer["snapshot"] => ({
  catalog: lineup,
  ...(tuned ? { attemptId: 1, channel: tuned } : {}),
  recentChannelIds: [],
  status,
  ...extra,
});

const render = (initial: { channelId: string; channels?: ChannelDTO[] }) => {
  const onNavigate = vi.fn();
  const view = renderHook(
    (props: { channelId: string; channels?: ChannelDTO[] }) =>
      useWatchPlayer({ channelId: props.channelId, channels: props.channels, nowNext: [], onNavigate }),
    { initialProps: initial },
  );
  return { onNavigate, ...view };
};

describe("useWatchPlayer", () => {
  beforeEach(() => {
    for (const control of Object.values(browser.controls)) control.mockReset();
    browser.snapshot = { catalog: [], recentChannelIds: [], status: "empty" };
  });
  afterEach(() => vi.restoreAllMocks());

  describe("the URL's channel", () => {
    it("is tuned once the playable catalog is known, and not before", () => {
      const { rerender } = render({ channelId: "twenty", channels: lineup });
      expect(browser.controls.tuneChannel).not.toHaveBeenCalled();

      browser.snapshot = snapshot("idle");
      rerender({ channelId: "twenty", channels: lineup });

      expect(browser.controls.tuneChannel).toHaveBeenCalledExactlyOnceWith("twenty");
    });

    it("is tuned when the viewer navigates to another channel", () => {
      browser.snapshot = snapshot("playing", ten);
      const { rerender } = render({ channelId: "ten", channels: lineup });
      browser.controls.tuneChannel.mockClear();

      rerender({ channelId: "thirty", channels: lineup });

      expect(browser.controls.tuneChannel).toHaveBeenCalledExactlyOnceWith("thirty");
    });
  });

  describe("a channel the controller tuned", () => {
    it("moves the URL to it, once", () => {
      browser.snapshot = snapshot("playing", ten);
      const { onNavigate, rerender } = render({ channelId: "ten", channels: lineup });
      expect(onNavigate).not.toHaveBeenCalled();

      browser.snapshot = snapshot("tuning", twenty);
      rerender({ channelId: "ten", channels: lineup });
      rerender({ channelId: "ten", channels: lineup });

      expect(onNavigate).toHaveBeenCalledExactlyOnceWith("twenty");
    });

    it("is not tuned a second time when the URL catches up", () => {
      browser.snapshot = snapshot("playing", ten);
      const { rerender } = render({ channelId: "ten", channels: lineup });
      browser.snapshot = snapshot("tuning", twenty);
      rerender({ channelId: "ten", channels: lineup });
      browser.controls.tuneChannel.mockClear();

      rerender({ channelId: "twenty", channels: lineup });

      expect(browser.controls.tuneChannel).not.toHaveBeenCalled();
    });

    it("keeps the latest request when an older navigation commits while the viewer surfs on", () => {
      browser.snapshot = snapshot("playing", ten);
      const { onNavigate, rerender } = render({ channelId: "ten", channels: lineup });
      browser.snapshot = snapshot("tuning", twenty);
      rerender({ channelId: "ten", channels: lineup });
      browser.snapshot = snapshot("tuning", thirty);
      rerender({ channelId: "ten", channels: lineup });
      expect(onNavigate.mock.calls).toEqual([["twenty"], ["thirty"]]);
      browser.controls.tuneChannel.mockClear();

      // The router commits the first navigation after the second tune began.
      rerender({ channelId: "twenty", channels: lineup });
      expect(browser.controls.tuneChannel).not.toHaveBeenCalled();
      rerender({ channelId: "thirty", channels: lineup });
      expect(browser.controls.tuneChannel).not.toHaveBeenCalled();
    });

    it("ends on the channel the viewer surfed back to", () => {
      browser.snapshot = snapshot("playing", ten);
      const { onNavigate, rerender } = render({ channelId: "ten", channels: lineup });
      browser.snapshot = snapshot("tuning", twenty);
      rerender({ channelId: "ten", channels: lineup });
      browser.snapshot = snapshot("tuning", ten);
      rerender({ channelId: "ten", channels: lineup });

      expect(onNavigate.mock.calls).toEqual([["twenty"], ["ten"]]);
    });

    it("tunes an external navigation to a channel it also just navigated away from", () => {
      browser.snapshot = snapshot("playing", ten);
      const { rerender } = render({ channelId: "ten", channels: lineup });
      browser.snapshot = snapshot("tuning", twenty);
      rerender({ channelId: "ten", channels: lineup });
      rerender({ channelId: "twenty", channels: lineup });
      browser.controls.tuneChannel.mockClear();

      rerender({ channelId: "ten", channels: lineup });

      expect(browser.controls.tuneChannel).toHaveBeenCalledExactlyOnceWith("ten");
    });
  });

  describe("a channel the app can't play", () => {
    const offline = channel("offline", 40, { inAppPlayable: false, status: "paused" });
    const withOffline = [...lineup, offline];

    it("parks the player on it and shows that channel, not the one that was playing", () => {
      browser.snapshot = snapshot("playing", ten);
      const { rerender, result } = render({ channelId: "ten", channels: withOffline });
      browser.controls.tuneChannel.mockClear();

      rerender({ channelId: "offline", channels: withOffline });

      expect(browser.controls.pause).toHaveBeenCalledOnce();
      expect(browser.controls.tuneChannel).not.toHaveBeenCalled();
      expect(result.current.channel).toEqual(offline);
    });

    it("restarts the channel the player was parked on when the viewer comes back to it", () => {
      browser.snapshot = snapshot("playing", ten);
      const { rerender } = render({ channelId: "ten", channels: withOffline });
      rerender({ channelId: "offline", channels: withOffline });
      browser.controls.tuneChannel.mockClear();

      // Its stream was released with the picture, so tuning the same channel again must start it.
      rerender({ channelId: "ten", channels: withOffline });

      expect(browser.controls.retry).toHaveBeenCalledOnce();
      expect(browser.controls.tuneChannel).not.toHaveBeenCalled();
    });

    it("tunes another channel normally after leaving it", () => {
      browser.snapshot = snapshot("playing", ten);
      const { rerender } = render({ channelId: "ten", channels: withOffline });
      rerender({ channelId: "offline", channels: withOffline });

      rerender({ channelId: "thirty", channels: withOffline });

      expect(browser.controls.tuneChannel).toHaveBeenLastCalledWith("thirty");
      expect(browser.controls.retry).not.toHaveBeenCalled();
    });

    it("is a place to go from the drawer: it navigates without a tune", () => {
      browser.snapshot = snapshot("playing", ten);
      const { onNavigate, result } = render({ channelId: "ten", channels: withOffline });
      browser.controls.tuneChannel.mockClear();

      act(() => result.current.tune("offline"));

      expect(onNavigate).toHaveBeenCalledExactlyOnceWith("offline");
      expect(browser.controls.tuneChannel).not.toHaveBeenCalled();
    });
  });

  describe("what the surface shows", () => {
    it.each([
      ["tuning", "loading"],
      ["idle", "loading"],
      ["empty", "loading"],
      ["playing", "playing"],
      ["paused", "playing"],
      ["failed", "error"],
    ] as const)("reads a %s controller as %s", (status, expected) => {
      browser.snapshot = snapshot(status, ten, status === "failed" ? { error: "It stopped." } : {});
      const { result } = render({ channelId: "ten", channels: lineup });

      expect(result.current.status).toBe(expected);
      expect(result.current.error).toBe(status === "failed" ? "It stopped." : undefined);
    });

    it("says a channel the app can't play can't be played, once the lineup is known", () => {
      const offline = channel("offline", 40, { inAppPlayable: false });
      browser.snapshot = snapshot("idle");
      const { result } = render({ channelId: "offline", channels: [...lineup, offline] });

      expect(result.current.status).toBe("error");
      expect(result.current.error).toMatch(/can't be played/i);
      expect(browser.controls.tuneChannel).not.toHaveBeenCalledWith("offline");
    });

    it("keeps loading while the lineup is unknown", () => {
      const { result } = render({ channelId: "ten" });
      expect(result.current.status).toBe("loading");
    });

    it("reports the tuned channel from the lineup, and that the viewer can surf with more than one", () => {
      browser.snapshot = snapshot("tuning", twenty);
      const { result } = render({ channelId: "ten", channels: lineup });

      expect(result.current.channel).toEqual(twenty);
      expect(result.current.canSurf).toBe(true);
    });

    it("cannot surf a lineup of one", () => {
      browser.snapshot = snapshot("playing", ten, { catalog: [ten] });
      const { result } = render({ channelId: "ten", channels: [ten] });
      expect(result.current.canSurf).toBe(false);
    });

    it("offers the live position, defaulting to live", () => {
      browser.snapshot = snapshot("playing", ten, {
        livePlayback: { lagSeconds: 12, mode: "behind", noticeRevision: 2, viewerTimeMs: 5 },
      });
      const { result } = render({ channelId: "ten", channels: lineup });

      expect(result.current.liveTransport.state).toEqual({
        lagSeconds: 12,
        mode: "behind",
        noticeRevision: 2,
        viewerTimeMs: 5,
      });
    });

    it("hands the play, pause and live controls to the shared controller", async () => {
      browser.snapshot = snapshot("playing", ten);
      const { result } = render({ channelId: "ten", channels: lineup });

      await act(async () => {
        await result.current.liveTransport.play();
        result.current.liveTransport.pause();
        await result.current.liveTransport.goLive();
        result.current.step(1);
        result.current.tune("thirty");
        result.current.retry();
      });

      expect(browser.controls.play).toHaveBeenCalledOnce();
      expect(browser.controls.pause).toHaveBeenCalledOnce();
      expect(browser.controls.goLive).toHaveBeenCalledOnce();
      expect(browser.controls.step).toHaveBeenCalledWith(1);
      expect(browser.controls.tuneChannel).toHaveBeenCalledWith("thirty");
      expect(browser.controls.retry).toHaveBeenCalledOnce();
    });
  });

  describe("the tuning picture", () => {
    afterEach(() => vi.unstubAllGlobals());

    const stubImage = () => {
      const images: { onload: (() => void) | null; src: string }[] = [];
      vi.stubGlobal(
        "Image",
        class {
          onload: (() => void) | null = null;
          src = "";
          constructor() {
            images.push(this);
          }
        },
      );
      return images;
    };

    it("shows the still only once it has decoded, and only while tuning", () => {
      const images = stubImage();
      browser.snapshot = snapshot("tuning", twenty, { stillUri: "/still/twenty" });
      const { rerender, result } = render({ channelId: "ten", channels: lineup });

      expect(result.current.stillURL).toBeUndefined();
      expect(images.map((image) => image.src)).toEqual(["/still/twenty"]);

      act(() => images[0]?.onload?.());
      expect(result.current.stillURL).toBe("/still/twenty");

      browser.snapshot = snapshot("playing", twenty, { stillUri: "/still/twenty" });
      rerender({ channelId: "ten", channels: lineup });
      expect(result.current.stillURL).toBeUndefined();
    });
  });
});
