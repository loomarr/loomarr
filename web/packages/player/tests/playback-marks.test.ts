import type { VideoPlayer } from "expo-video";
import { describe, expect, it, vi } from "vitest";

vi.mock("expo-crypto", () => ({ randomUUID: vi.fn() }));
vi.mock("expo-video", () => ({
  createVideoPlayer: vi.fn(),
  VideoView: vi.fn(),
}));

const { createNativePlayerTransport, createPlaybackMarks } = await import("@loomarr/player/native");

type PlayerListener = (payload: never) => void;

const nativePlayer = () => {
  const listeners = new Map<string, PlayerListener>();
  const player = {
    addListener: vi.fn((event: string, listener: PlayerListener) => {
      listeners.set(event, listener);
      return { remove: vi.fn(() => listeners.delete(event)) };
    }),
    currentLiveTimestamp: null,
    currentOffsetFromLive: null,
    pause: vi.fn(),
    play: vi.fn(),
    release: vi.fn(),
    replaceAsync: vi.fn().mockResolvedValue(undefined),
  };
  return {
    emit: (event: string, payload: unknown) => listeners.get(event)?.(payload as never),
    listens: (event: string) => listeners.has(event),
    player: player as unknown as VideoPlayer,
  };
};

const recorder = () => {
  let clock = 1_000;
  const lines: string[] = [];
  const marks = createPlaybackMarks({ enabled: true, now: () => clock, sink: (line) => lines.push(line) });
  return {
    advance: (ms: number) => {
      clock += ms;
    },
    lines,
    marks,
  };
};

const source = { uri: "https://loomarr.test/signed/live.m3u8?sig=secret" };

describe("playback marks", () => {
  it("writes one prefixed key=value line per event on the monotonic clock", () => {
    const { lines, marks } = recorder();

    marks.tune({
      attemptId: 3,
      channelId: "ch_1",
      channelNumber: 104,
      keyAtMs: 990.25,
      reason: "step",
      warm: true,
    });
    marks.held(3, "osd");

    expect(lines).toEqual([
      "LoomarrCert v=1 ev=tune t=1000 att=3 ch=ch_1 key=990.3 num=104 path=warm why=step",
      "LoomarrCert v=1 ev=held t=1000 att=3 what=osd",
    ]);
  });

  it("keeps every value one token so a line never splits or carries prose", () => {
    const { lines, marks } = recorder();

    marks.error(1, "decoder failed at https://host/path?a=b c");

    expect(lines[0]?.split(" ")).toHaveLength(6);
    expect(lines[0]).toMatch(/ cause=[\w./:-]+$/);
  });

  it("is inert when disabled and never lets a failing sink reach playback", () => {
    const sink = vi.fn();
    createPlaybackMarks({ enabled: false, sink }).tune({
      attemptId: 1,
      channelId: "ch_1",
      channelNumber: 1,
      reason: "step",
      warm: false,
    });
    expect(sink).not.toHaveBeenCalled();

    const throwing = createPlaybackMarks({
      enabled: true,
      sink: () => {
        throw new Error("console gone");
      },
    });
    expect(() => throwing.firstFrame(1)).not.toThrow();
  });
});

describe("native transport marks", () => {
  it("marks the first frame once per attempt and a stall only after it", async () => {
    const { advance, lines, marks } = recorder();
    const native = nativePlayer();
    const transport = createNativePlayerTransport(native.player, undefined, marks);

    await transport.replace(source, { attemptId: 5, signal: new AbortController().signal });
    // Start-up buffering is the tune, not a stall.
    native.emit("statusChange", { status: "loading" });
    transport.firstFrame();
    transport.firstFrame();
    native.emit("statusChange", { status: "readyToPlay" });
    native.emit("statusChange", { status: "loading" });
    advance(420);
    native.emit("statusChange", { status: "readyToPlay" });

    expect(lines.map((line) => line.replace(/ t=\S+/, ""))).toEqual([
      "LoomarrCert v=1 ev=first-frame att=5",
      "LoomarrCert v=1 ev=stall-start att=5",
      "LoomarrCert v=1 ev=stall-end att=5 dur=420 why=resumed",
    ]);
  });

  it("ends an open stall on the old attempt when the viewer surfs away", async () => {
    const { lines, marks } = recorder();
    const native = nativePlayer();
    const transport = createNativePlayerTransport(native.player, undefined, marks);

    await transport.replace(source, { attemptId: 1, signal: new AbortController().signal });
    transport.firstFrame();
    native.emit("statusChange", { status: "loading" });
    await transport.replace(source, { attemptId: 2, signal: new AbortController().signal });
    native.emit("statusChange", { status: "loading" });

    expect(lines.at(-1)).toBe("LoomarrCert v=1 ev=stall-end t=1000 att=1 dur=0 why=retuned");
    expect(lines.filter((line) => line.includes("stall-start"))).toHaveLength(1);
  });

  it("does not count buffering while the viewer has paused", async () => {
    const { lines, marks } = recorder();
    const native = nativePlayer();
    const transport = createNativePlayerTransport(native.player, undefined, marks);

    await transport.replace(source, { attemptId: 1, signal: new AbortController().signal });
    transport.firstFrame();
    transport.pause();
    native.emit("statusChange", { status: "loading" });

    expect(lines.some((line) => line.includes("stall-start"))).toBe(false);
  });

  it("reduces a native error to a cause token and reports each format change", async () => {
    const { lines, marks } = recorder();
    const native = nativePlayer();
    const transport = createNativePlayerTransport(native.player, undefined, marks);

    await transport.replace(source, { attemptId: 4, signal: new AbortController().signal });
    native.emit("videoTrackChange", {
      videoTrack: {
        bitrate: 4_000_000,
        frameRate: 29.97,
        mimeType: "video/avc",
        size: { height: 1080, width: 1920 },
      },
    });
    native.emit("statusChange", {
      error: { message: "Response code: 404 https://loomarr.test/signed/seg.m4s?sig=secret" },
      status: "error",
    });

    expect(lines.map((line) => line.replace(/ t=\S+/, ""))).toEqual([
      "LoomarrCert v=1 ev=format att=4 br=4000000 fps=30 h=1080 mime=video/avc w=1920",
      "LoomarrCert v=1 ev=error att=4 cause=http_404",
    ]);
    expect(lines.join("\n")).not.toContain("loomarr.test");
  });

  it("does not subscribe to track changes when marks are off", () => {
    const native = nativePlayer();
    createNativePlayerTransport(native.player);

    expect(native.listens("statusChange")).toBe(true);
    expect(native.listens("videoTrackChange")).toBe(false);
  });
});
