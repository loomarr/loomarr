// @vitest-environment jsdom

import type { PlayerTransportEvent } from "@loomarr/player";
import { type BrowserTransportOptions, createBrowserTransport } from "@loomarr/player/browser";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import MockHls, { hlsMock as hls } from "./hls.mock";

vi.mock("hls.js", () => ({ default: MockHls }));

// The transport drives hls.js over Media Source Extensions, which jsdom cannot run. The element is a
// fake with a real listener bus, and hls.js is the mock beside this file; what is tested is the
// transport's own behaviour: the order it attaches and loads in, what it hands the controller, and
// how the live clock answers pause, resume, skew and stalls.

type Mocked = ReturnType<typeof vi.fn>;

interface FakeHls {
  attachMedia: Mocked;
  config: { autoStartLoad?: boolean; enableWorker?: boolean };
  destroy: Mocked;
  levels: { attrs: { CODECS?: string }; height: number; videoRange: string }[];
  liveSyncPosition: number | null;
  loadLevel: number;
  loadSource: Mocked;
  on: Mocked;
  playingDate: Date | null;
  recoverMediaError: Mocked;
  startLevel: number;
  startLoad: Mocked;
  stopLoad: Mocked;
  transferMedia: Mocked;
}

const instances = () => hls.instances as FakeHls[];

const handler = (instance: FakeHls, event: string) =>
  instance.on.mock.calls.filter((call: unknown[]) => call[0] === event).at(-1)?.[1] as (
    ...args: unknown[]
  ) => void;

const createVideo = (canPlay = "", overrides: Record<string, unknown> = {}) => {
  const bus = new Map<string, { callback: EventListener; once: boolean }[]>();
  const video = {
    addEventListener: vi.fn((type: string, callback: EventListener, options?: { once?: boolean }) => {
      bus.set(type, [...(bus.get(type) ?? []), { callback, once: options?.once === true }]);
    }),
    buffered: { end: vi.fn(), length: 0, start: vi.fn() },
    canPlayType: () => canPlay,
    currentTime: 0,
    dataset: {},
    load: vi.fn(),
    pause: vi.fn(),
    play: vi.fn().mockResolvedValue(undefined),
    poster: "",
    querySelectorAll: vi.fn().mockReturnValue([]),
    readyState: 0,
    removeAttribute: vi.fn(),
    removeEventListener: vi.fn((type: string, callback: EventListener) => {
      bus.set(
        type,
        (bus.get(type) ?? []).filter((entry) => entry.callback !== callback),
      );
    }),
    seekable: { end: vi.fn(), length: 0, start: vi.fn() },
    videoHeight: 0,
    videoWidth: 0,
    ...overrides,
  };
  const fire = (type: string) => {
    for (const entry of [...(bus.get(type) ?? [])]) {
      if (entry.once)
        bus.set(
          type,
          (bus.get(type) ?? []).filter((other) => other !== entry),
        );
      entry.callback(new Event(type));
    }
  };
  const listening = (type: string) => (bus.get(type) ?? []).length;
  return { fire, listening, video: video as unknown as HTMLVideoElement };
};

/** A video whose frame callbacks the test delivers itself. */
const createFramedVideo = (canPlay = "", overrides: Record<string, unknown> = {}) => {
  const frames: Array<() => void> = [];
  const created = createVideo(canPlay, {
    cancelVideoFrameCallback: vi.fn(),
    requestVideoFrameCallback: vi.fn((callback: () => void) => {
      frames.push(callback);
      return frames.length;
    }),
    ...overrides,
  });
  // The newest request is the one that belongs to the source being opened.
  return { ...created, deliverFrame: () => frames.pop()?.() };
};

const masterUrl = (id: string) => `/v1/playout/hls/${id}/master.m3u8`;

const harness = (video: HTMLVideoElement, options: Partial<BrowserTransportOptions> = {}) => {
  const events: PlayerTransportEvent[] = [];
  const phases: Array<[number, string]> = [];
  const recordDiagnostic = vi.fn();
  let channel = "ch-1";
  let attempt = 0;
  const transport = createBrowserTransport({
    channelId: () => channel,
    onPhase: (attemptId, phase) => phases.push([attemptId, phase]),
    recordDiagnostic,
    ...options,
  });
  transport.subscribe((event) => events.push(event));
  const release = transport.attach(video);
  /** Starts a tune. Its promise settles at the first manifest, so tests don't await it. */
  const open = (id: string, extra: { serverTimeMs?: number; signal?: AbortSignal } = {}) => {
    channel = id;
    attempt += 1;
    const done = transport.replace(
      { serverTimeMs: extra.serverTimeMs, uri: masterUrl(id) },
      { attemptId: attempt, signal: extra.signal ?? new AbortController().signal },
    );
    return { attemptId: attempt, done };
  };
  const live = () => events.filter((event) => event.type === "live-state").at(-1);
  return { events, live, open, phases, recordDiagnostic, release, transport };
};

const transferable = (buffer: Record<string, unknown>, readyState = "open") => ({
  media: undefined,
  mediaSource: { readyState },
  tracks: { video: { buffer } },
});

const loaded = async (index: number, id: string) => {
  await vi.waitFor(() => expect(instances()[index]?.loadSource).toHaveBeenCalledWith(masterUrl(id)));
  return instances()[index] as FakeHls;
};

describe("browser transport", () => {
  beforeEach(() => {
    hls.supported = false;
    hls.instances.length = 0;
    vi.spyOn(navigator, "userAgent", "get").mockReturnValue("Mozilla/5.0 Firefox/142.0");
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  describe("opening a source", () => {
    it("loads the source on hls.js and settles once its first manifest has arrived", async () => {
      hls.supported = true;
      const { video } = createVideo();
      const { open, phases } = harness(video);

      const { attemptId, done } = open("ch-1");
      const controller = await loaded(0, "ch-1");
      // Loading the source is not the manifest arriving: neighbours must not warm yet.
      expect(phases).toEqual([]);

      handler(controller, "manifestParsed")();
      await done;
      expect(phases).toEqual([[attemptId, "manifest"]]);
    });

    it("waits for the page's video when asked to open a source before one is attached", async () => {
      hls.supported = true;
      const { video } = createVideo();
      const transport = createBrowserTransport({ channelId: () => "ch-1", recordDiagnostic: vi.fn() });
      const opening = transport.replace(
        { uri: masterUrl("ch-1") },
        { attemptId: 1, signal: new AbortController().signal },
      );
      await Promise.resolve();
      expect(hls.instances).toHaveLength(0);

      transport.attach(video);

      await loaded(0, "ch-1");
      handler(instances()[0] as FakeHls, "manifestParsed")();
      await opening;
    });

    it("abandons a source whose attempt was aborted before the video arrived", async () => {
      hls.supported = true;
      const { video } = createVideo();
      const transport = createBrowserTransport({ channelId: () => "ch-1", recordDiagnostic: vi.fn() });
      const abort = new AbortController();
      const opening = transport.replace({ uri: masterUrl("ch-1") }, { attemptId: 1, signal: abort.signal });
      abort.abort();
      await opening;

      transport.attach(video);
      await Promise.resolve();
      expect(hls.instances).toHaveLength(0);
    });

    it("prefers native HLS on Apple WebKit even when hls.js MSE is available", async () => {
      vi.spyOn(navigator, "userAgent", "get").mockReturnValue(
        "Mozilla/5.0 AppleWebKit/605.1.15 Version/18.6 Safari/605.1.15",
      );
      hls.supported = true;
      const { video } = createVideo("probably");
      const { open } = harness(video);

      open("ch-1");

      await vi.waitFor(() => expect(video.play).toHaveBeenCalledOnce());
      expect(video.src).toBe(masterUrl("ch-1"));
      expect(hls.instances).toHaveLength(0);
    });

    it("reports the first manifest on native HLS at loadedmetadata", async () => {
      const { fire, video } = createVideo("application/vnd.apple.mpegurl");
      const { open, phases } = harness(video);

      const { attemptId, done } = open("ch-1");
      await vi.waitFor(() => expect(video.play).toHaveBeenCalledOnce());
      expect(phases).toEqual([]);

      fire("loadedmetadata");
      await done;
      expect(phases).toEqual([[attemptId, "manifest"]]);
    });

    it("pins the baseline of a 4K channel when the browser cannot decode the premium", async () => {
      // #1512 G10: playing the premium variant starts its 4K encode on the server, so a browser that
      // cannot decode HEVC (jsdom has no MediaSource at all) must never request it.
      hls.supported = true;
      const { video } = createVideo();
      const { open } = harness(video);
      open("ch-1");
      const active = await loaded(0, "ch-1");
      active.levels = [
        { attrs: { CODECS: "hvc1.2.4.L150.90,mp4a.40.2" }, height: 2160, videoRange: "PQ" },
        { attrs: { CODECS: "avc1.640028,mp4a.40.2" }, height: 1080, videoRange: "SDR" },
      ];

      handler(active, "manifestParsed")();

      expect(active.loadLevel).toBe(1);
      expect(active.startLevel).toBe(1);
    });

    it("says plainly that the browser can play neither MSE nor native HLS", async () => {
      const { video } = createVideo("");
      const { open, recordDiagnostic } = harness(video);

      await expect(open("ch-1").done).rejects.toThrow(/can't play live channels/i);
      expect(recordDiagnostic).toHaveBeenCalledWith(
        expect.objectContaining({ errorCode: "unsupported", event: "player.media_error", fatal: true }),
      );
    });

    it("names the channel on the element before any decoded-frame observer is armed", async () => {
      hls.supported = true;
      const { video } = createFramedVideo();
      const { open } = harness(video);
      open("ch-9");
      await loaded(0, "ch-9");
      expect(video.dataset.playbackChannel).toBe("ch-9");
    });
  });

  describe("the first frame", () => {
    it("keeps the controller's tuning picture until a decoded frame, then clears the held poster", async () => {
      const { deliverFrame, fire, video } = createFramedVideo("application/vnd.apple.mpegurl", {
        poster: "data:image/png;base64,held",
      });
      const { events, open } = harness(video);

      const { attemptId } = open("ch-1");
      await vi.waitFor(() => expect(video.play).toHaveBeenCalledOnce());
      expect(video.requestVideoFrameCallback).not.toHaveBeenCalled();

      fire("loadedmetadata");
      expect(video.requestVideoFrameCallback).not.toHaveBeenCalled();
      fire("loadeddata");
      // The element playing is not a picture: `playing` before the first frame is not announced.
      fire("playing");
      expect(events.map((event) => event.type)).not.toContain("playing");
      expect(events.map((event) => event.type)).not.toContain("first-frame");

      deliverFrame();
      expect(events).toContainEqual({ attemptId, type: "first-frame" });
      expect(video.removeAttribute).toHaveBeenCalledWith("poster");
    });

    it("announces playing once the first frame has shown", async () => {
      hls.supported = true;
      const { deliverFrame, fire, video } = createFramedVideo();
      const { events, open } = harness(video);
      const { attemptId } = open("ch-1");
      await loaded(0, "ch-1");

      deliverFrame();
      fire("playing");

      expect(events.filter((event) => event.type === "playing")).toEqual([{ attemptId, type: "playing" }]);
    });

    it("holds the outgoing picture as a poster until the replacement has frame data", async () => {
      hls.supported = true;
      const { fire, video } = createVideo("", { readyState: 4, videoHeight: 4, videoWidth: 8 });
      const draw = vi.fn();
      const realCreate = document.createElement.bind(document);
      vi.spyOn(document, "createElement").mockImplementation((tag: string) =>
        tag === "canvas"
          ? ({
              getContext: () => ({ drawImage: draw }),
              toDataURL: () => "data:image/png;base64,held",
            } as never)
          : realCreate(tag),
      );
      const { open } = harness(video);

      open("ch-1");
      await loaded(0, "ch-1");
      expect(video.poster).toBe("data:image/png;base64,held");
      expect(draw).toHaveBeenCalledOnce();

      video.poster = "data:image/png;base64,held";
      fire("loadeddata");
      expect(video.removeAttribute).toHaveBeenCalledWith("poster");
    });
  });

  describe("replacing a source on the persistent element", () => {
    it("keeps one active controller and a fresh one-source standby across repeated tunes", async () => {
      hls.supported = true;
      const { deliverFrame, video } = createFramedVideo();
      const { open, release } = harness(video);

      open("ch-1");
      const first = await loaded(0, "ch-1");
      expect(hls.instances).toHaveLength(1);

      handler(first, "fragBuffered")();
      deliverFrame();
      await vi.waitFor(() => expect(hls.instances).toHaveLength(2));
      const second = instances()[1] as FakeHls;
      expect(second.loadSource).not.toHaveBeenCalled();

      open("ch-2");
      await loaded(1, "ch-2");
      expect(video.dataset.playbackChannel).toBe("ch-2");
      expect(first.stopLoad).toHaveBeenCalled();
      expect(first.transferMedia).toHaveBeenCalledOnce();
      expect(first.destroy).not.toHaveBeenCalled();
      expect(second.attachMedia).toHaveBeenLastCalledWith(video);

      handler(second, "fragBuffered")();
      deliverFrame();
      await vi.waitFor(() => expect(first.destroy).toHaveBeenCalledOnce());
      await vi.waitFor(() => expect(hls.instances).toHaveLength(3));
      const third = instances()[2] as FakeHls;
      expect(third.loadSource).not.toHaveBeenCalled();
      expect(instances().filter((instance) => instance.destroy.mock.calls.length === 0)).toHaveLength(2);

      open("ch-3");
      await loaded(2, "ch-3");
      expect(hls.instances).toHaveLength(3);

      release();
      expect(second.destroy).toHaveBeenCalledOnce();
      expect(third.destroy).toHaveBeenCalledOnce();
    });

    it("reuses compatible open SourceBuffers across channel tunes", async () => {
      hls.supported = true;
      const setTime = vi.fn();
      const { deliverFrame, fire, video } = createFramedVideo();
      Object.defineProperty(video, "currentTime", { configurable: true, get: () => 0.5, set: setTime });
      const { open } = harness(video);
      open("ch-1");
      const controller = await loaded(0, "ch-1");
      vi.mocked(video.requestVideoFrameCallback).mockClear();
      vi.mocked(video.play).mockClear();

      let finishRemoval: () => void = () => undefined;
      const remove = vi.fn();
      const transferred = transferable({
        addEventListener: vi.fn((event: string, callback: () => void) => {
          if (event === "updateend") finishRemoval = callback;
        }),
        buffered: { end: () => 1, length: 1, start: () => 0 },
        remove,
        removeEventListener: vi.fn(),
        updating: false,
      });
      controller.transferMedia.mockReturnValue(transferred);
      open("ch-2");

      await vi.waitFor(() => expect(video.pause).toHaveBeenCalledOnce());
      // The exact range end is not presentable in WebKit. Seek to the final frame interval instead,
      // and do not start removal until the media element acknowledges that in-range seek.
      expect(setTime).toHaveBeenLastCalledWith(0.95);
      expect(remove).not.toHaveBeenCalled();
      fire("seeked");
      await vi.waitFor(() => expect(remove).toHaveBeenCalledWith(0, 1));
      expect(hls.instances).toHaveLength(2);
      expect(controller.destroy).not.toHaveBeenCalled();
      expect(video.play).not.toHaveBeenCalled();
      const replacement = instances()[1] as FakeHls;
      expect(replacement.attachMedia).not.toHaveBeenCalled();
      expect(replacement.config.autoStartLoad).toBe(false);
      // Baseline HLS carries MPEG-TS, so transmuxing must stay off the UI thread. hls.js shares and
      // reference-counts its worker across source-scoped controllers; the bounded active/standby
      // pool must preserve that production invariant while disabling auto-load.
      expect(replacement.config.enableWorker).toBe(true);
      expect(replacement.loadSource).not.toHaveBeenCalled();
      expect(replacement.startLoad).not.toHaveBeenCalled();

      finishRemoval();
      await vi.waitFor(() => expect(setTime).toHaveBeenCalledWith(0));
      await vi.waitFor(() => expect(replacement.attachMedia).toHaveBeenLastCalledWith(transferred));
      await vi.waitFor(() => expect(video.requestVideoFrameCallback).toHaveBeenCalledOnce());
      await vi.waitFor(() => expect(replacement.loadSource).toHaveBeenCalledWith(masterUrl("ch-2")));
      await vi.waitFor(() => expect(replacement.startLoad).toHaveBeenCalledOnce());
      const order = (mock: Mocked) => mock.mock.invocationCallOrder.at(-1) ?? 0;
      expect(order(replacement.attachMedia)).toBeLessThan(order(replacement.loadSource));
      expect(order(replacement.loadSource)).toBeLessThan(order(replacement.startLoad));
      expect(order(vi.mocked(video.requestVideoFrameCallback))).toBeLessThan(order(replacement.startLoad));
      // A cached WebKit append can decode before MANIFEST_PARSED is delivered. The target must have
      // a pending playback join before media loading starts so its first decoded frame cannot surface
      // paused; later manifest/fragment/loadeddata joins still cover WebKit's queued load resets.
      await vi.waitFor(() => expect(video.play).toHaveBeenCalledOnce());
      expect(order(replacement.loadSource)).toBeLessThan(order(vi.mocked(video.play)));
      expect(order(vi.mocked(video.play))).toBeLessThan(order(replacement.startLoad));

      vi.mocked(video.play).mockClear();
      fire("loadedmetadata");
      expect(video.play).toHaveBeenCalledOnce();
      vi.mocked(video.play).mockClear();
      handler(replacement, "manifestParsed")();
      expect(setTime).toHaveBeenLastCalledWith(98);
      await vi.waitFor(() => expect(video.play).toHaveBeenCalledOnce());

      setTime.mockClear();
      handler(replacement, "levelUpdated")();
      expect(setTime).toHaveBeenLastCalledWith(98);

      deliverFrame();
      setTime.mockClear();
      handler(replacement, "levelUpdated")();
      expect(setTime).not.toHaveBeenCalled();

      vi.mocked(video.play).mockClear();
      handler(replacement, "fragBuffered")();
      expect(video.play).toHaveBeenCalledOnce();

      vi.mocked(video.play).mockClear();
      fire("loadeddata");
      expect(video.play).toHaveBeenCalledOnce();
      fire("loadeddata");
      expect(video.play).toHaveBeenCalledOnce();

      vi.mocked(video.play).mockClear();
      fire("canplay");
      expect(video.play).toHaveBeenCalledOnce();
    });

    it("does not wait for a seek event when playback is already at the buffered edge", async () => {
      hls.supported = true;
      let mediaTime = 4.041666;
      let seeking = false;
      const { video } = createFramedVideo();
      Object.defineProperty(video, "currentTime", {
        configurable: true,
        get: () => mediaTime,
        // Chromium, Firefox, and WebKit all clamp this fixture's exact half-open range end to the last
        // decoded timestamp and can remain in `seeking` without ever dispatching `seeked`.
        set: () => {
          mediaTime = 4.083332;
          seeking = true;
        },
      });
      Object.defineProperty(video, "seeking", { configurable: true, get: () => seeking });
      const { open } = harness(video);
      open("ch-3");
      const first = await loaded(0, "ch-3");

      let finishRemoval: () => void = () => undefined;
      const remove = vi.fn();
      first.transferMedia.mockReturnValue(
        transferable({
          addEventListener: vi.fn((event: string, callback: () => void) => {
            if (event === "updateend") finishRemoval = callback;
          }),
          buffered: { end: () => 4.083333333333333, length: 1, start: () => 0 },
          remove,
          removeEventListener: vi.fn(),
          updating: false,
        }),
      );

      open("ch-17");

      // Browser media clocks round the same frame edge slightly differently from TimeRanges. Setting
      // the range end clamps to an effectively identical decoded timestamp without starting a seek,
      // so the handoff must proceed instead of waiting forever before HLS attachment.
      await vi.waitFor(() => expect(remove).toHaveBeenCalledWith(0, 4.083333333333333));
      expect(video.seeking).toBe(false);
      finishRemoval();
      await loaded(1, "ch-17");
    });

    it("uses the bounded fresh-MSE branch for an open WebKit handoff", async () => {
      hls.supported = true;
      vi.spyOn(navigator, "userAgent", "get").mockReturnValue(
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Version/18.0 Safari/605.1.15",
      );
      const { listening, video } = createFramedVideo();
      video.src = "blob:http://localhost/webkit-open-source";
      Object.defineProperty(video, "currentSrc", { configurable: true, get: () => video.src });
      Object.defineProperty(video, "currentTime", { configurable: true, get: () => 0.5, set: vi.fn() });
      const revokeObjectURL = vi.spyOn(URL, "revokeObjectURL");
      const { open } = harness(video);
      open("ch-1");
      const controller = await loaded(0, "ch-1");
      const remove = vi.fn();
      controller.transferMedia.mockReturnValue(
        transferable({
          addEventListener: vi.fn(),
          buffered: { end: () => 4, length: 1, start: () => 0 },
          remove,
          removeEventListener: vi.fn(),
          updating: false,
        }),
      );

      open("ch-2");

      await vi.waitFor(() => expect(hls.instances).toHaveLength(2));
      const replacement = instances()[1] as FakeHls;
      await vi.waitFor(() => expect(replacement.attachMedia).toHaveBeenCalledWith(video));
      await vi.waitFor(() => expect(replacement.loadSource).toHaveBeenCalledWith(masterUrl("ch-2")));
      expect(replacement.startLoad).toHaveBeenCalledOnce();
      expect(replacement.loadSource.mock.invocationCallOrder.at(-1)).toBeLessThan(
        replacement.attachMedia.mock.invocationCallOrder.at(-1) ?? 0,
      );
      expect(video.pause).not.toHaveBeenCalled();
      expect(listening("seeked")).toBe(0);
      expect(remove).not.toHaveBeenCalled();
      expect(video.load).not.toHaveBeenCalled();
      expect(revokeObjectURL).toHaveBeenCalledWith("blob:http://localhost/webkit-open-source");
    });

    it("lets the tuning overlay paint before it holds the old frame and binds the new source", async () => {
      hls.supported = true;
      const frames: FrameRequestCallback[] = [];
      vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => frames.push(callback));
      const { video } = createVideo();
      const { open } = harness(video);
      open("ch-1");
      await vi.waitFor(() => expect(frames).toHaveLength(1));
      frames.shift()?.(0);
      frames.shift()?.(0);
      await loaded(0, "ch-1");

      open("ch-2");
      await vi.waitFor(() => expect(frames).toHaveLength(1));
      expect(hls.instances).toHaveLength(1);

      frames.shift()?.(16);
      await Promise.resolve();
      expect(hls.instances).toHaveLength(1);
      frames.shift()?.(32);
      await vi.waitFor(() => expect(hls.instances).toHaveLength(2));
    });

    it("replaces an ended MediaSource without waiting on its decoded range", async () => {
      hls.supported = true;
      const requestAnimationFrame = vi.fn();
      vi.stubGlobal("requestAnimationFrame", requestAnimationFrame);
      const { video } = createVideo();
      video.src = "blob:http://localhost/ended-source";
      Object.defineProperty(video, "currentSrc", { configurable: true, get: () => video.src });
      const source = { remove: vi.fn(), src: video.src };
      vi.mocked(video.querySelectorAll).mockReturnValue([source] as unknown as NodeListOf<HTMLSourceElement>);
      const revokeObjectURL = vi.spyOn(URL, "revokeObjectURL");
      const { open } = harness(video);
      open("ch-1");
      const controller = await loaded(0, "ch-1");
      const remove = vi.fn();
      controller.transferMedia.mockReturnValue(
        transferable(
          {
            addEventListener: vi.fn(),
            buffered: { end: () => 4, length: 1, start: () => 0 },
            remove,
            removeEventListener: vi.fn(),
            updating: false,
          },
          "ended",
        ),
      );
      const framesBefore = requestAnimationFrame.mock.calls.length;

      open("ch-2");

      await vi.waitFor(() => expect(hls.instances).toHaveLength(2));
      const replacement = instances()[1] as FakeHls;
      await vi.waitFor(() => expect(replacement.attachMedia).toHaveBeenCalledWith(video));
      await vi.waitFor(() => expect(replacement.loadSource).toHaveBeenCalledWith(masterUrl("ch-2")));
      expect(replacement.startLoad).toHaveBeenCalledOnce();
      // The one frame asked for is the overlay's paint before the swap; the handoff waits on none.
      expect(requestAnimationFrame.mock.calls.length - framesBefore).toBe(1);
      expect(remove).not.toHaveBeenCalled();
      expect(video.removeAttribute).toHaveBeenCalledWith("src");
      expect(source.remove).toHaveBeenCalledOnce();
      expect(video.load).toHaveBeenCalledOnce();
      expect(revokeObjectURL).toHaveBeenCalledWith("blob:http://localhost/ended-source");
      expect(vi.mocked(video.load).mock.invocationCallOrder.at(-1)).toBeLessThan(
        replacement.attachMedia.mock.invocationCallOrder.at(-1) ?? 0,
      );
    });

    it("does not attach or load a superseded replacement after its transferred clear finishes", async () => {
      hls.supported = true;
      let mediaTime = 0.5;
      const { fire, video } = createVideo();
      Object.defineProperty(video, "currentTime", {
        configurable: true,
        get: () => mediaTime,
        set: (value: number) => {
          mediaTime = value;
        },
      });
      const { open, release } = harness(video);
      open("ch-1");
      const first = await loaded(0, "ch-1");
      let finishRemoval: () => void = () => undefined;
      first.transferMedia.mockReturnValue(
        transferable({
          addEventListener: vi.fn((event: string, callback: () => void) => {
            if (event === "updateend") finishRemoval = callback;
          }),
          buffered: { end: () => 1, length: 1, start: () => 0 },
          remove: vi.fn(),
          removeEventListener: vi.fn(),
          updating: false,
        }),
      );

      open("ch-2");
      await vi.waitFor(() => expect(video.pause).toHaveBeenCalled());
      fire("seeked");
      await vi.waitFor(() => expect(hls.instances).toHaveLength(2));
      const superseded = instances()[1] as FakeHls;

      open("ch-3");
      await vi.waitFor(() => expect(hls.instances).toHaveLength(3));
      const current = instances()[2] as FakeHls;
      await vi.waitFor(() => expect(current.loadSource).toHaveBeenCalledWith(masterUrl("ch-3")));
      video.currentTime = 7;

      finishRemoval();
      await new Promise((resolve) => setTimeout(resolve, 0));
      expect(hls.instances).toHaveLength(3);
      expect(current.attachMedia).toHaveBeenLastCalledWith(video);
      expect(first.destroy).toHaveBeenCalledOnce();
      expect(superseded.destroy).not.toHaveBeenCalled();
      expect(superseded.loadSource).not.toHaveBeenCalled();
      expect(superseded.attachMedia).not.toHaveBeenCalled();
      expect(superseded.startLoad).not.toHaveBeenCalled();
      expect(video.currentTime).toBe(7);

      release();
      expect(current.destroy).toHaveBeenCalled();
      expect(superseded.destroy).toHaveBeenCalled();
    });

    it("never loads a source whose attempt was aborted while the player library was loading", async () => {
      hls.supported = true;
      const { video } = createVideo();
      const { open } = harness(video);
      const abort = new AbortController();

      const { done } = open("ch-1", { signal: abort.signal });
      abort.abort();
      await done;

      expect(
        hls.instances.every((instance) => (instance as FakeHls).loadSource.mock.calls.length === 0),
      ).toBe(true);
    });
  });

  describe("pause, resume and the live edge", () => {
    const seekable = (video: HTMLVideoElement, range: () => [number, number]) =>
      Object.defineProperty(video, "seekable", {
        configurable: true,
        value: { end: () => range()[1], length: 1, start: () => range()[0] },
      });

    it("resumes the exact paused point and can return to the live edge", async () => {
      hls.supported = true;
      vi.spyOn(Date, "now").mockReturnValue(1_000_000);
      const { video } = createVideo("", { currentTime: 44 });
      seekable(video, () => [0, 100]);
      const { events, live, open, transport } = harness(video);
      open("ch-1");
      const controller = await loaded(0, "ch-1");

      transport.pause();
      expect(video.pause).toHaveBeenCalledOnce();
      expect(events.map((event) => event.type)).toContain("paused");
      expect(live()).toMatchObject({ state: { mode: "paused" } });

      vi.mocked(Date.now).mockReturnValue(1_015_000);
      await transport.play();
      expect(controller.startLoad).toHaveBeenCalledWith(44);
      expect(video.currentTime).toBe(44);
      expect(live()).toMatchObject({ state: { lagSeconds: 15, mode: "behind" } });

      await transport.goLive();
      expect(controller.startLoad).toHaveBeenCalledWith(-1);
      expect(video.currentTime).toBe(98);
      expect(live()).toMatchObject({ state: { lagSeconds: 0, mode: "live" } });
    });

    it("stays paused for as long as the viewer waits, keeping the shared window leased", async () => {
      hls.supported = true;
      vi.useFakeTimers({ toFake: ["Date", "setInterval", "clearInterval"] });
      vi.setSystemTime(new Date("2026-09-21T20:00:00Z"));
      const fetched = vi.fn().mockResolvedValue({ arrayBuffer: () => Promise.resolve(new ArrayBuffer(0)) });
      vi.stubGlobal("fetch", fetched);
      const { video } = createVideo("", { currentTime: 44 });
      seekable(video, () => [0, 100]);
      const { events, live, open, transport } = harness(video);
      open("ch-1");
      await loaded(0, "ch-1");

      transport.pause();
      vi.mocked(video.play).mockClear();
      vi.advanceTimersByTime(20 * 60_000);

      // Past the 15-minute window the paused point is gone, but nothing moves until the viewer resumes.
      expect(live()).toMatchObject({ state: { mode: "paused", noticeRevision: 0 } });
      expect(video.currentTime).toBe(44);
      expect(video.play).not.toHaveBeenCalled();
      expect(events.filter((event) => event.type === "paused")).toHaveLength(1);
      expect(fetched).toHaveBeenCalled();
      expect(fetched).toHaveBeenCalledWith(masterUrl("ch-1"), { cache: "no-store" });

      await transport.play();
      expect(live()).toMatchObject({ state: { mode: "live", noticeRevision: 1 } });
    });

    it("keeps an intentional pause while the active controller buffers another fragment", async () => {
      hls.supported = true;
      const { video } = createVideo();
      const { live, open, transport } = harness(video);
      open("ch-1");
      const controller = await loaded(0, "ch-1");

      transport.pause();
      vi.mocked(video.play).mockClear();
      handler(controller, "fragBuffered")();

      expect(video.play).not.toHaveBeenCalled();
      expect(live()).toMatchObject({ state: { mode: "paused" } });
    });

    it("returns live and raises a notice when the paused point has left the shared window", async () => {
      hls.supported = true;
      const { video } = createVideo("", { currentTime: 44 });
      let seekableStart = 0;
      seekable(video, () => [seekableStart, 100]);
      const { live, open, transport } = harness(video);
      open("ch-1");
      await loaded(0, "ch-1");

      transport.pause();
      seekableStart = 50;
      await transport.play();

      expect(live()).toMatchObject({ state: { mode: "live", noticeRevision: 1 } });
    });

    it("returns live when the window moves past a viewer playing behind it", async () => {
      hls.supported = true;
      const { video } = createVideo("", { currentTime: 44 });
      seekable(video, () => [0, 100]);
      const { live, open, transport } = harness(video);
      open("ch-1");
      const controller = await loaded(0, "ch-1");
      transport.pause();
      await transport.play();
      controller.startLoad.mockClear();

      handler(controller, "error")("error", { details: "fragLoadError", fatal: true, type: "networkError" });

      expect(controller.startLoad).toHaveBeenCalledWith(-1);
      expect(live()).toMatchObject({ state: { mode: "live", noticeRevision: 1 } });
    });
  });

  // The live window (#2088 decision 2): a wall-clock target maps to media time from the frame on
  // screen, which aired at 992 000 and sits at media 44 s; the live edge is media 98 s.
  describe("seeking within the window", () => {
    const seekable = (video: HTMLVideoElement, range: () => [number, number]) =>
      Object.defineProperty(video, "seekable", {
        configurable: true,
        value: { end: () => range()[1], length: 1, start: () => range()[0] },
      });

    const watching = async () => {
      hls.supported = true;
      vi.spyOn(Date, "now").mockReturnValue(1_000_000);
      const created = createVideo("", { currentTime: 44 });
      seekable(created.video, () => [0, 100]);
      const harnessed = harness(created.video);
      harnessed.open("ch-1");
      const controller = await loaded(0, "ch-1");
      controller.playingDate = new Date(992_000);
      created.fire("timeupdate");
      // From here the element's own media time carries the mapping, as it does between dated frames.
      controller.playingDate = null;
      return { ...created, ...harnessed, controller };
    };

    it("plays on from the target, behind live by its distance from now", async () => {
      const { controller, live, transport, video } = await watching();

      await transport.seek?.(982_000);

      expect(controller.startLoad).toHaveBeenCalledWith(34);
      expect(video.currentTime).toBe(34);
      expect(live()).toMatchObject({ state: { lagSeconds: 18, mode: "behind", viewerTimeMs: 982_000 } });
    });

    it("keeps a paused viewer paused at the target, and resumes from it", async () => {
      const { controller, live, transport, video } = await watching();
      transport.pause();
      vi.mocked(video.play).mockClear();

      await transport.seek?.(982_000);

      expect(video.currentTime).toBe(34);
      expect(video.play).not.toHaveBeenCalled();
      expect(live()).toMatchObject({ state: { mode: "paused", viewerTimeMs: 982_000 } });

      controller.startLoad.mockClear();
      await transport.play();
      expect(controller.startLoad).toHaveBeenCalledWith(34);
      expect(live()).toMatchObject({ state: { lagSeconds: 18, mode: "behind" } });
    });

    it("goes live when the target reaches the live edge", async () => {
      const { controller, live, transport, video } = await watching();
      await transport.seek?.(982_000);
      controller.startLoad.mockClear();

      await transport.seek?.(1_000_000);

      expect(controller.startLoad).toHaveBeenCalledWith(-1);
      expect(video.currentTime).toBe(98);
      expect(live()).toMatchObject({ state: { lagSeconds: 0, mode: "live" } });
    });
  });

  describe("the live clock", () => {
    it("anchors live programme time to the frame being shown instead of the wall-clock live edge", async () => {
      hls.supported = true;
      vi.spyOn(Date, "now").mockReturnValue(1_000_000);
      const { fire, video } = createVideo("", { currentTime: 44 });
      const { live, open } = harness(video);
      open("ch-1");
      const controller = await loaded(0, "ch-1");
      // Two HLS segments of safety buffer put the decoded frame eight seconds behind real time.
      // At an Airing boundary those eight seconds are the difference between "commercial" and the
      // following programme, so the Watch chrome must describe the frame, not Date.now().
      controller.playingDate = new Date(992_000);

      fire("timeupdate");

      expect(live()).toMatchObject({ state: { mode: "live", viewerTimeMs: 992_000 } });
    });

    it("keeps the playhead on a stalled decoded frame when HLS temporarily loses its date", async () => {
      hls.supported = true;
      vi.spyOn(Date, "now").mockReturnValue(1_000_000);
      const { fire, video } = createVideo("", { currentTime: 44 });
      const { live, open } = harness(video);
      open("ch-1");
      const controller = await loaded(0, "ch-1");

      controller.playingDate = new Date(992_000);
      fire("timeupdate");
      expect(live()).toMatchObject({ state: { viewerTimeMs: 992_000 } });

      // During the reproduced boundary stall hls.js briefly reports no playingDate. The wall clock
      // crosses into filler, but the media element is still showing the final episode frame.
      controller.playingDate = null;
      vi.mocked(Date.now).mockReturnValue(1_012_000);
      fire("timeupdate");
      expect(live()).toMatchObject({ state: { viewerTimeMs: 992_000 } });

      // Once decoded media advances, the retained mapping advances with it even before hls.js
      // republishes program-date-time metadata.
      video.currentTime = 45;
      fire("timeupdate");
      expect(live()).toMatchObject({ state: { viewerTimeMs: 993_000 } });
    });

    it("measures the live edge on the server's clock when the device's is wrong", async () => {
      hls.supported = true;
      const device = new Date("2026-09-21T20:00:00Z");
      vi.useFakeTimers({ toFake: ["Date", "setInterval", "clearInterval"] });
      vi.setSystemTime(device);
      const serverTimeMs = device.getTime() - 2 * 3_600_000;
      const { fire, video } = createVideo("", { currentTime: 44 });
      const { live, open, transport } = harness(video);
      open("ch-1", { serverTimeMs });
      const controller = await loaded(0, "ch-1");

      expect(live()).toMatchObject({ state: { lagSeconds: 0, mode: "live", viewerTimeMs: serverTimeMs } });
      vi.advanceTimersByTime(90_000);
      fire("timeupdate");
      const edge = live()?.type === "live-state" ? live()?.state.viewerTimeMs : undefined;
      expect(edge).toBeGreaterThanOrEqual(serverTimeMs + 90_000);
      expect(edge).toBeLessThan(serverTimeMs + 91_000);

      // Paused on a frame 12 s behind the server's edge, the lag is the server's 12 s, not two hours.
      controller.playingDate = new Date(serverTimeMs + 78_000);
      transport.pause();
      expect(live()).toMatchObject({ state: { lagSeconds: 12, mode: "paused" } });
    });
  });

  describe("a stall the viewer didn't cause", () => {
    /** A live, framed attempt whose stream freezes for `stallSeconds` and resumes that far behind. */
    const stallAndRecover = async (stallSeconds: number, beforeRecovery?: (pause: () => void) => void) => {
      hls.supported = true;
      vi.useFakeTimers({ toFake: ["Date", "setInterval", "clearInterval"] });
      vi.setSystemTime(new Date("2026-09-21T20:00:00Z"));
      const { deliverFrame, fire, video } = createFramedVideo();
      const harnessed = harness(video);
      harnessed.open("ch-1");
      const controller = await loaded(0, "ch-1");
      deliverFrame();
      controller.startLoad.mockClear();

      fire("waiting");
      vi.advanceTimersByTime(stallSeconds * 1_000);
      beforeRecovery?.(() => {
        harnessed.transport.pause();
        void harnessed.transport.play();
      });
      controller.playingDate = new Date(Date.now() - stallSeconds * 1_000);
      fire("playing");
      return { ...harnessed, controller, video };
    };

    it("tells the controller once, and only after the attempt's first frame", async () => {
      hls.supported = true;
      const { deliverFrame, fire, video } = createFramedVideo();
      const { events, open } = harness(video);
      const { attemptId } = open("ch-1");
      await loaded(0, "ch-1");

      fire("waiting"); // the start's own buffering is not a stall
      deliverFrame();
      fire("waiting");
      fire("waiting");

      expect(events.filter((event) => event.type === "stalled")).toEqual([{ attemptId, type: "stalled" }]);
    });

    it("rejoins the live edge when the stall left a live viewer more than 30 s behind", async () => {
      const { controller, live, video } = await stallAndRecover(45);

      expect(controller.startLoad).toHaveBeenCalledWith(-1);
      expect(video.currentTime).toBe(98);
      expect(live()).toMatchObject({ state: { lagSeconds: 0, mode: "live" } });
    });

    it("keeps the position after a short stall", async () => {
      const { controller, video } = await stallAndRecover(20);

      expect(controller.startLoad).not.toHaveBeenCalled();
      expect(video.currentTime).toBe(0);
    });

    it("leaves a viewer who paused where they chose to be, even through a stall", async () => {
      const { controller } = await stallAndRecover(45, (pauseAndPlay) => pauseAndPlay());

      expect(controller.startLoad).not.toHaveBeenCalledWith(-1);
    });
  });

  describe("a fatal playlist refusal", () => {
    const manifestFailure = (code: number, body: string) => ({
      details: "manifestLoadError",
      fatal: true,
      response: { code, data: body },
      type: "networkError",
    });
    const opened = async () => {
      hls.supported = true;
      const { video } = createVideo();
      const harnessed = harness(video);
      const attempt = harnessed.open("ch-1");
      const controller = await loaded(0, "ch-1");
      controller.startLoad.mockClear(); // the initial load is not a retry
      const onError = handler(controller, "error");
      return { ...harnessed, ...attempt, controller, onError };
    };

    it("reports the server's specific reason as a refusal instead of looping", async () => {
      const { attemptId, controller, done, events, onError } = await opened();

      onError(
        "error",
        manifestFailure(
          502,
          JSON.stringify({ detail: "Loomarr could not reach its own program source.", title: "Bad Gateway" }),
        ),
      );

      expect(events).toContainEqual({
        attemptId,
        error: "Loomarr could not reach its own program source.",
        startStatus: 502,
        type: "error",
      });
      expect(controller.startLoad).not.toHaveBeenCalled();
      expect(controller.destroy).toHaveBeenCalled();
      await done;
    });

    it("falls back to a generic sentence when a 5xx carries no problem body", async () => {
      const { events, onError } = await opened();

      onError("error", manifestFailure(502, "<html>bad gateway</html>"));

      expect(events).toContainEqual(
        expect.objectContaining({
          error: expect.stringMatching(/couldn't start this channel/i),
          type: "error",
        }),
      );
    });

    it("keeps retrying a 404: the ordinary warm-up race", async () => {
      const { controller, events, onError } = await opened();

      onError("error", manifestFailure(404, JSON.stringify({ detail: "not yet" })));

      expect(events.map((event) => event.type)).not.toContain("error");
      expect(controller.startLoad).toHaveBeenCalled();
      expect(controller.destroy).not.toHaveBeenCalled();
    });

    it("reports an unrecoverable stream error for the controller's retry budget", async () => {
      const { attemptId, events, onError } = await opened();

      onError("error", { fatal: true, type: "otherError" });

      expect(events).toContainEqual({
        attemptId,
        error: "The stream stopped. Try again in a moment.",
        type: "error",
      });
    });

    it("recovers a media error in place", async () => {
      const { controller, events, onError } = await opened();

      onError("error", { fatal: true, type: "mediaError" });

      expect(controller.recoverMediaError).toHaveBeenCalled();
      expect(events.map((event) => event.type)).not.toContain("error");
    });
  });

  describe("release and disposal", () => {
    it("frees both controllers and ignores a source that arrives later", async () => {
      hls.supported = true;
      const { video } = createFramedVideo();
      const { open, release } = harness(video);
      open("ch-1");
      const first = await loaded(0, "ch-1");

      release();

      expect(first.destroy).toHaveBeenCalledOnce();
    });

    it("stops publishing after dispose", async () => {
      hls.supported = true;
      const { fire, video } = createVideo();
      const { events, open, transport } = harness(video);
      open("ch-1");
      await loaded(0, "ch-1");
      transport.dispose();
      const seen = events.length;

      fire("timeupdate");

      expect(events).toHaveLength(seen);
    });
  });
});
