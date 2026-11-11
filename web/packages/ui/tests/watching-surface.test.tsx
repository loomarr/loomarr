// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import type { PlayerSnapshot } from "@loomarr/player";
import type { ReactNode } from "react";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import { WatchingSurface, type WatchingSurfaceProps } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// The package compiles without the DOM lib (it is React Native first), so the test names the little
// of the DOM it touches.
type Node = { click(): void; getAttribute(n: string): string | null; textContent: string | null };
type Host = Parameters<typeof createRoot>[0] & {
  querySelector(s: string): Node | null;
  querySelectorAll(s: string): ArrayLike<Node>;
  remove(): void;
  textContent: string | null;
};
const doc = (
  globalThis as unknown as {
    document: { body: { append(n: unknown): void }; createElement(t: string): Host };
  }
).document;

const mounted: { host: Host; unmount: () => void }[] = [];
const mount = (node: () => ReactNode) => {
  const host = doc.createElement("div");
  doc.body.append(host);
  const root = createRoot(host);
  const render = (next: () => ReactNode = node) =>
    act(() => root.render(<LoomarrProvider theme="dark">{next()}</LoomarrProvider>));
  render();
  mounted.push({ host, unmount: () => act(() => root.unmount()) });
  return { host, render };
};
afterEach(() => {
  for (const { host, unmount } of mounted.splice(0)) {
    unmount();
    host.remove();
  }
  vi.useRealTimers();
});

const button = (host: Host, name: string) =>
  Array.from(host.querySelectorAll('[role="button"]')).find(
    (el) => el.getAttribute("aria-label") === name || el.textContent === name,
  );

const seven = { id: "seven", inAppPlayable: true, name: "Science Fiction", number: 7 };
const six = { id: "six", inAppPlayable: true, name: "Nature", number: 6 };
const playing: PlayerSnapshot = {
  attemptId: 4,
  catalog: [six, seven],
  channel: seven,
  livePlayback: { lagSeconds: 0, mode: "live", noticeRevision: 0, viewerTimeMs: 30 * 60_000 },
  previousChannelId: "six",
  recentChannelIds: ["six"],
  status: "playing",
  tuneReason: "step",
};
const chrome: NonNullable<WatchingSurfaceProps["chrome"]> = {
  band: {
    channelName: "Science Fiction",
    channelNumber: "7",
    line: "S1 E4 “Orbit”",
    next: { name: "The Next Frontier", startMs: 40 * 60_000 },
    title: "The Current Frontier",
  },
  segments: [
    { chips: [], id: "a", kind: "programme", name: "The Current Frontier", startMs: 0, stopMs: 40 * 60_000 },
    {
      chips: [],
      id: "b",
      kind: "programme",
      name: "The Next Frontier",
      startMs: 40 * 60_000,
      stopMs: 90 * 60_000,
    },
  ],
  window: { endMs: 90 * 60_000, nowMs: 30 * 60_000, startMs: 15 * 60_000 },
};

const surface = (overrides: Partial<WatchingSurfaceProps> = {}) => (
  <WatchingSurface
    chrome={chrome}
    formatTime={(ms) => `t${ms}`}
    onDismissControls={vi.fn()}
    onGoLive={vi.fn()}
    onOpenGuide={vi.fn()}
    onRetry={vi.fn()}
    onTuneChannel={vi.fn()}
    playheadMs={30 * 60_000}
    player={<div data-player="one-native-player" />}
    snapshot={playing}
    {...overrides}
  />
);

describe("WatchingSurface", () => {
  it("wears the J band over the one mounted player, with a television's keys", () => {
    const { host } = mount(() => surface());
    expect(host.querySelector('[data-player="one-native-player"]')).not.toBeNull();
    expect(host.textContent).toContain("Science Fiction");
    expect(host.textContent).toContain("The Current Frontier");
    expect(button(host, "Last channel, 6")).toBeDefined();
    // The remote has Play/Pause, Channel up and down, and Volume; Back leaves.
    for (const name of ["Pause", "Play", "Channel up", "Channel down", "Mute", "Leave"])
      expect(button(host, name), name).toBeUndefined();
  });

  it("tunes the last channel from its key", () => {
    const onTuneChannel = vi.fn();
    const { host } = mount(() => surface({ onTuneChannel }));
    act(() => button(host, "Last channel, 6")?.click());
    expect(onTuneChannel).toHaveBeenCalledWith("six");
  });

  it("opens the Guide from the bare picture, which holds focus only while the controls are hidden", () => {
    const onOpenGuide = vi.fn();
    const { host, render } = mount(() => surface({ controlsVisible: false, onOpenGuide }));
    const picture = () => button(host, "Open programme guide");
    expect(picture()).toBeDefined();
    expect(host.textContent).not.toContain("The Current Frontier");
    act(() => picture()?.click());
    expect(onOpenGuide).toHaveBeenCalledOnce();

    // Up, the strip's block on air takes focus, and its OK opens the Guide too.
    render(() => surface({ controlsVisible: true, onOpenGuide }));
    expect(picture()).toBeUndefined();
  });

  it("keeps the picture's focus while the controls have nothing to focus", () => {
    const { host } = mount(() =>
      surface({
        chrome: { ...chrome, segments: [] },
        snapshot: { ...playing, previousChannelId: undefined },
      }),
    );
    expect(button(host, "Open programme guide")).toBeDefined();
  });

  it("clears the controls five seconds after activity, despite live programme updates", () => {
    vi.useFakeTimers();
    const first = vi.fn();
    const later = vi.fn();
    const { render } = mount(() => surface({ controlsActivityKey: 0, onDismissControls: first }));
    act(() => vi.advanceTimersByTime(4_999));
    render(() => surface({ controlsActivityKey: 0, onDismissControls: later, playheadMs: 31 * 60_000 }));
    act(() => vi.advanceTimersByTime(1));
    expect(first).not.toHaveBeenCalled();
    expect(later).toHaveBeenCalledOnce();
  });

  it("starts a fresh five seconds only when handled activity changes", () => {
    vi.useFakeTimers();
    const dismiss = vi.fn();
    const { render } = mount(() => surface({ controlsActivityKey: 0, onDismissControls: dismiss }));
    act(() => vi.advanceTimersByTime(4_999));
    render(() => surface({ controlsActivityKey: 1, onDismissControls: dismiss }));
    act(() => vi.advanceTimersByTime(1));
    expect(dismiss).not.toHaveBeenCalled();
    act(() => vi.advanceTimersByTime(4_999));
    expect(dismiss).toHaveBeenCalledOnce();
  });

  it("covers a stream that stopped with its words and a focused Retry, and no way out: Back leaves", () => {
    const onRetry = vi.fn();
    const { host } = mount(() =>
      surface({
        onRetry,
        snapshot: { ...playing, error: "The stream could not be decoded.", status: "failed" },
      }),
    );
    expect(host.querySelector('[role="alert"]')?.textContent).toContain("The stream could not be decoded.");
    expect(host.textContent).toContain("CH 7 · Science Fiction");
    expect(button(host, "Open programme guide")).toBeUndefined();
    act(() => button(host, "Retry channel")?.click());
    expect(onRetry).toHaveBeenCalledOnce();
    expect(button(host, "Leave")).toBeUndefined();
  });

  it("offers Change server only when the channels themselves did not load", () => {
    const empty: PlayerSnapshot = { catalog: [], recentChannelIds: [], status: "empty" };
    const failedLoad = mount(() =>
      surface({
        chrome: undefined,
        loadError: "Could not load channels.",
        onChangeServer: vi.fn(),
        snapshot: empty,
      }),
    ).host;
    expect(failedLoad.textContent).toContain("Could not load channels.");
    expect(button(failedLoad, "Try again")).toBeDefined();
    expect(button(failedLoad, "Change server")).toBeDefined();

    const failedPlayback = mount(() =>
      surface({
        onChangeServer: vi.fn(),
        snapshot: { ...playing, error: "Decode error.", status: "failed" },
      }),
    ).host;
    expect(button(failedPlayback, "Change server")).toBeUndefined();
  });

  it("says an empty catalog plainly, and not before the catalog request resolves", () => {
    const empty: PlayerSnapshot = { catalog: [], recentChannelIds: [], status: "empty" };
    expect(mount(() => surface({ chrome: undefined, snapshot: empty })).host.textContent).toContain(
      "No playable channels",
    );
    const loading = mount(() => surface({ chrome: undefined, loading: true, snapshot: empty })).host;
    expect(loading.querySelector('[aria-label="Loading channels"]')).not.toBeNull();
    expect(loading.textContent).not.toContain("No playable channels");
    expect(button(loading, "Open programme guide")).toBeUndefined();
  });

  it("keeps playback mounted while a journey hides all of Watching's chrome", () => {
    const { host } = mount(() => surface({ chromeVisible: false }));
    expect(host.querySelector('[data-player="one-native-player"]')).not.toBeNull();
    expect(host.textContent).not.toContain("Science Fiction");
    expect(button(host, "Open programme guide")).toBeUndefined();
  });

  it("draws the typed digits, the channel they reach and a live auto-tune countdown (#1659 map 5a)", () => {
    vi.useFakeTimers();
    vi.setSystemTime(1_000);
    const { host } = mount(() =>
      surface({ numberEntry: { channelName: "Nature", digits: "21", expiresAtMs: 2_200 } }),
    );
    expect(host.textContent).toContain("21");
    expect(host.textContent).toContain("Nature");
    expect(host.textContent).toContain("auto-tunes in 1.2 s");
    act(() => vi.advanceTimersByTime(700));
    expect(host.textContent).toContain("auto-tunes in 0.5 s");
  });

  it("reads Enter channel until the digits reach one, and no countdown without an expiry", () => {
    const { host } = mount(() => surface({ numberEntry: { digits: "7" } }));
    expect(host.textContent).toContain("Enter channel");
    expect(host.textContent).not.toContain("auto-tunes in");
  });

  it("says so when a paused point expired and playback went back to live", () => {
    const { host } = mount(() =>
      surface({
        snapshot: {
          ...playing,
          livePlayback: { lagSeconds: 0, mode: "live", noticeRevision: 1, viewerTimeMs: 30 * 60_000 },
        },
      }),
    );
    expect(host.textContent).toContain("you're back live");
  });

  it("offers Go live in the band while behind", () => {
    const onGoLive = vi.fn();
    const { host } = mount(() =>
      surface({
        onGoLive,
        snapshot: {
          ...playing,
          livePlayback: { lagSeconds: 83, mode: "paused", noticeRevision: 0, viewerTimeMs: 29 * 60_000 },
          status: "paused",
        },
      }),
    );
    act(() => button(host, "Go live")?.click());
    expect(onGoLive).toHaveBeenCalledOnce();
  });
});
