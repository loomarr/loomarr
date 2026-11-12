// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import type { PlayerSnapshot } from "@loomarr/player";
import type { ReactNode } from "react";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import { WatchingSurface, type WatchingSurfaceProps } from "../index";

// Records which pressable asks for the TV's focus as it renders; react-native-web drops the prop.
const preferredFocus = vi.hoisted(() => ({ labels: [] as string[] }));
vi.mock("react-native", async (importOriginal) => {
  const original = await importOriginal<typeof import("react-native")>();
  const { createElement } = await import("react");
  const Pressable = (props: Parameters<typeof original.Pressable>[0]) => {
    if (props.hasTVPreferredFocus && props.accessibilityLabel)
      preferredFocus.labels.push(props.accessibilityLabel);
    return createElement(original.Pressable, props);
  };
  return { ...original, Pressable };
});

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
    onLayer={vi.fn()}
    onOpenGuide={vi.fn()}
    onOpenSurf={vi.fn()}
    onRetry={vi.fn()}
    onSeek={vi.fn()}
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

  it("puts the channel cards in the band's place when Up or Down flips, then brings the band back", () => {
    vi.useFakeTimers();
    const surfCards = [
      { id: "six", name: "Nature", nowPlaying: "A river film", number: "6" },
      { id: "seven", name: "Science Fiction", nowPlaying: "The Current Frontier", number: "7" },
    ];
    // The surface mounts on a channel a flip already tuned: that is not a flip to show.
    const { host, render } = mount(() => surface({ surfCards }));
    expect(host.querySelector('[aria-label="Channels"]')).toBeNull();

    // Down (a handled key, so the controls are up) tunes six at once; the cards ride along, six lit,
    // and the readout names it.
    render(() => surface({ snapshot: { ...playing, channel: six }, surfCards }));
    expect(host.querySelector('[data-testid="channel-card-lit"]')?.textContent).toBe("6NatureWatching now");
    expect(host.querySelector('[role="status"]')?.textContent).toBe("6Nature");
    expect(host.querySelector('[data-testid="band-channel-block"]')).toBeNull();

    act(() => vi.advanceTimersByTime(1_200));
    expect(host.querySelector('[aria-label="Channels"]')).toBeNull();
    expect(host.querySelector('[data-testid="band-channel-block"]')).not.toBeNull();
  });

  it("keeps the band through a tune that is not a flip", () => {
    const surfCards = [{ id: "six", name: "Nature", number: "6" }];
    const { host, render } = mount(() => surface({ surfCards }));
    render(() => surface({ snapshot: { ...playing, channel: six, tuneReason: "channel" }, surfCards }));
    expect(host.querySelector('[aria-label="Channels"]')).toBeNull();
  });

  it("tunes the last channel from its key", () => {
    const onTuneChannel = vi.fn();
    const { host } = mount(() => surface({ onTuneChannel }));
    act(() => button(host, "Last channel, 6")?.click());
    expect(onTuneChannel).toHaveBeenCalledWith("six");
  });

  // More (§9.5, variant J): a menu in the cluster, over the keys; the app owns the open layer.
  it("opens More from its key, and closes it from the same key", () => {
    const onLayer = vi.fn();
    const { host, render } = mount(() => surface({ onLayer }));
    act(() => button(host, "More")?.click());
    expect(onLayer).toHaveBeenLastCalledWith("more");
    render(() => surface({ layer: "more", onLayer }));
    expect(host.querySelector('[role="dialog"][aria-label="More"]')).not.toBeNull();
    act(() => button(host, "More")?.click());
    expect(onLayer).toHaveBeenLastCalledWith(undefined);
  });

  it("lists Channels and Info in More, live; Start over and Go live join when behind", () => {
    const onLayer = vi.fn();
    const onOpenSurf = vi.fn();
    const onSeek = vi.fn();
    const { host, render } = mount(() => surface({ layer: "more", onLayer, onOpenSurf }));
    expect(button(host, "Start over")).toBeUndefined();
    act(() => button(host, "Channels")?.click());
    expect(onLayer).toHaveBeenLastCalledWith(undefined);
    expect(onOpenSurf).toHaveBeenCalledOnce();
    act(() => button(host, "Info")?.click());
    expect(onLayer).toHaveBeenLastCalledWith("info");

    render(() =>
      surface({
        chrome: { ...chrome, startOverMs: 0 },
        layer: "more",
        onLayer,
        onSeek,
        snapshot: {
          ...playing,
          livePlayback: { lagSeconds: 90, mode: "behind", noticeRevision: 0, viewerTimeMs: 28 * 60_000 },
        },
      }),
    );
    expect(host.textContent).toContain("Go live");
    // Named by what it shows: "Start over" and the programme.
    const startOver = Array.from(host.querySelectorAll('[role="button"]')).find((el) =>
      el.textContent?.startsWith("Start over"),
    );
    act(() => startOver?.click());
    expect(onSeek).toHaveBeenCalledWith(0);
  });

  // Audio (§9.5): a television's own key, opening the channel's language read-only in the cluster.
  it("opens Audio from its key, lists the channel's language, and closes from the same key", () => {
    const onLayer = vi.fn();
    const { host, render } = mount(() => surface({ onLayer }));
    act(() => button(host, "Audio")?.click());
    expect(onLayer).toHaveBeenLastCalledWith("audio");
    render(() =>
      surface({
        audioOptions: [
          { label: "Auto (default)", selected: false, value: "auto" },
          { label: "English", selected: true, value: "eng" },
        ],
        layer: "audio",
        onLayer,
      }),
    );
    const menu = '[role="dialog"][aria-label="Audio"]';
    expect(host.querySelector(menu)?.textContent).toContain("English");
    expect(host.querySelector(menu)?.textContent).toContain("An admin sets it on the channel’s page.");
    // Read-only: nothing in it to press, and no channel-change sound (the web player's own).
    const pressable = ["button", "radio", "switch"].map((role) => `${menu} [role="${role}"]`).join(", ");
    expect(host.querySelector(pressable)).toBeNull();
    expect(host.querySelector(`${menu} [aria-current="true"]`)?.textContent).toBe("English");
    act(() => button(host, "Audio")?.click());
    expect(onLayer).toHaveBeenLastCalledWith(undefined);
  });

  // Subtitles (§9.5): a television's own key, a menu of choices, and the line on the picture.
  const subtitles = (choose = vi.fn(), captions: readonly string[] = ["I thought you said tonight"]) => ({
    captions,
    choose,
    on: true,
    options: [
      { label: "Off", selected: false, value: "off" },
      { label: "English", selected: true, value: "en" },
    ],
  });

  it("draws no Subtitles key until the client has subtitles", () => {
    const { host } = mount(() => surface());
    expect(button(host, "Subtitles")).toBeUndefined();
  });

  it("opens Subtitles from its key, chooses a track from its rows, and draws the line", () => {
    const onLayer = vi.fn();
    const choose = vi.fn();
    const { host, render } = mount(() => surface({ onLayer, subtitles: subtitles(choose) }));
    act(() => button(host, "Subtitles")?.click());
    expect(onLayer).toHaveBeenLastCalledWith("subtitles");
    render(() => surface({ layer: "subtitles", onLayer, subtitles: subtitles(choose) }));
    const menu = '[role="dialog"][aria-label="Subtitles"]';
    const rows = Array.from(host.querySelectorAll(`${menu} [role="radio"]`));
    expect(rows.map((row) => [row.textContent, row.getAttribute("aria-checked")])).toEqual([
      ["Off", "false"],
      ["English", "true"],
    ]);
    act(() => rows[0]?.click());
    expect(choose).toHaveBeenCalledWith("off");
    expect(host.querySelector('[data-testid="player-caption"]')?.textContent).toBe(
      "I thought you said tonight",
    );
  });

  it("keeps drawing the line when the controls are down", () => {
    const { host } = mount(() => surface({ controlsVisible: false, subtitles: subtitles() }));
    expect(host.querySelector('[role="dialog"]')).toBeNull();
    expect(host.querySelector('[data-testid="player-caption"]')?.textContent).toBe(
      "I thought you said tonight",
    );
  });

  it("hands the remote's focus back to the key whose menu Back closed", () => {
    for (const [layer, key] of [
      ["subtitles", "Subtitles"],
      ["more", "More"],
    ] as const) {
      const { render } = mount(() => surface({ layer, subtitles: subtitles() }));
      preferredFocus.labels.length = 0;
      render(() => surface({ subtitles: subtitles() }));
      expect(preferredFocus.labels).toContain(key);
    }
  });

  it("leaves focus alone on keys whose menus never opened", () => {
    preferredFocus.labels.length = 0;
    mount(() => surface({ subtitles: subtitles() }));
    expect(preferredFocus.labels.filter((l) => ["Audio", "More", "Subtitles"].includes(l))).toEqual([]);
  });

  it("keeps the controls up while Subtitles waits for a choice", () => {
    vi.useFakeTimers();
    const dismiss = vi.fn();
    mount(() => surface({ layer: "subtitles", onDismissControls: dismiss, subtitles: subtitles() }));
    act(() => vi.advanceTimersByTime(10_000));
    expect(dismiss).not.toHaveBeenCalled();
  });

  it("keeps the controls up while Audio is open", () => {
    vi.useFakeTimers();
    const dismiss = vi.fn();
    mount(() => surface({ layer: "audio", onDismissControls: dismiss }));
    act(() => vi.advanceTimersByTime(10_000));
    expect(dismiss).not.toHaveBeenCalled();
  });

  it("keeps the controls up while More waits for a choice", () => {
    vi.useFakeTimers();
    const dismiss = vi.fn();
    mount(() => surface({ layer: "more", onDismissControls: dismiss }));
    act(() => vi.advanceTimersByTime(10_000));
    expect(dismiss).not.toHaveBeenCalled();
  });

  it("opens the opened Info as the on-air block's card, in full", () => {
    const { host } = mount(() =>
      surface({
        chrome: {
          ...chrome,
          segments: chrome.segments.map((s) =>
            s.id === "a" ? { ...s, synopsis: "A crew far from home." } : s,
          ),
        },
        layer: "info",
      }),
    );
    expect(host.textContent).toContain("A crew far from home.");
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
