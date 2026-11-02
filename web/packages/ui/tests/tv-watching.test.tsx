// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import type { PlayerSnapshot } from "@loomarr/player";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { TvWatching, type TvWatchingProps } from "../index";

(
  globalThis as typeof globalThis & {
    IS_REACT_ACT_ENVIRONMENT: boolean;
  }
).IS_REACT_ACT_ENVIRONMENT = true;

const channel = { id: "seven", inAppPlayable: true, name: "Science Fiction", number: 7 };
const playing: PlayerSnapshot = {
  attemptId: 4,
  catalog: [channel],
  channel,
  livePlayback: { lagSeconds: 0, mode: "live", noticeRevision: 0, viewerTimeMs: 1_777_777_777_000 },
  recentChannelIds: [],
  status: "playing",
  tuneReason: "step",
};

const chrome: NonNullable<TvWatchingProps["chrome"]> = {
  band: { channelName: "Science Fiction", channelNumber: "7", programmeLine: "The Current Frontier" },
  formatTime: () => "9:00 PM",
  transport: {
    density: "tv",
    live: { lagSeconds: 0, mode: "live" },
    muted: false,
    onChannels: vi.fn(),
    onGoLive: vi.fn(),
    onGuide: vi.fn(),
    onSeekBack: vi.fn(),
    onSeekForward: vi.fn(),
    onToggleMute: vi.fn(),
    onTogglePlay: vi.fn(),
    onVolume: vi.fn(),
    playing: true,
    pointer: false,
    volume: 1,
  },
};

const props = (snapshot: PlayerSnapshot, overrides: Partial<TvWatchingProps> = {}): TvWatchingProps => ({
  chrome,
  chromeVisible: true,
  controlsShown: false,
  loading: false,
  onBarePress: vi.fn(),
  onChangeServer: vi.fn(),
  onLeave: vi.fn(),
  onRetry: vi.fn(),
  player: <div data-player="one-native-player" />,
  snapshot,
  ...overrides,
});

const render = (snapshot: PlayerSnapshot, overrides: Partial<TvWatchingProps> = {}) =>
  renderToStaticMarkup(
    <LoomarrProvider>
      <TvWatching {...props(snapshot, overrides)} />
    </LoomarrProvider>,
  );

const createTestContainer = () =>
  (
    globalThis as unknown as {
      document: { createElement: (tagName: string) => Parameters<typeof createRoot>[0] };
    }
  ).document.createElement("div");

describe("TvWatching", () => {
  // #2088 decision 15: over a bare picture OK belongs to the remote map, not to a hidden control.
  it("leaves only the bare-picture layer to focus while the controls are hidden", () => {
    const bare = render(playing);
    expect(bare).toContain('aria-label="Show the controls"');
    expect(bare).not.toContain('aria-label="Pause"');
    expect(bare).not.toContain("The Current Frontier");

    const shown = render(playing, { controlsShown: true });
    expect(shown).not.toContain('aria-label="Show the controls"');
    expect(shown).toContain('aria-label="Pause"');
    expect(shown).toContain("The Current Frontier");
    // The ways to the Guide and Surf that OK and Left no longer are.
    expect(shown).toContain('aria-label="Guide"');
    expect(shown).toContain('aria-label="Channels"');
    // Back leaves Watching on a TV; there is no ✕.
    expect(shown).not.toContain('aria-label="Leave Watch"');
  });

  it("hands OK over the bare picture to the app", () => {
    const onBarePress = vi.fn();
    const container = createTestContainer();
    const root = createRoot(container);
    act(() =>
      root.render(
        <LoomarrProvider>
          <TvWatching {...props(playing, { onBarePress })} />
        </LoomarrProvider>,
      ),
    );
    const layer = (
      container as unknown as { querySelector: (s: string) => { click: () => void } | null }
    ).querySelector('[aria-label="Show the controls"]');
    act(() => layer?.click());
    expect(onBarePress).toHaveBeenCalledTimes(1);
    act(() => root.unmount());
  });

  it("keeps one player mounted while a journey hides all of Watching's chrome", () => {
    const output = render(playing, { chromeVisible: false, controlsShown: true });
    expect(output.match(/data-player="one-native-player"/g)).toHaveLength(1);
    expect(output).not.toContain('aria-label="Pause"');
    expect(output).not.toContain('aria-label="Show the controls"');
  });

  it("holds the controls back while a tune is still starting", () => {
    expect(render({ ...playing, status: "tuning" }, { controlsShown: true })).not.toContain(
      'aria-label="Pause"',
    );
  });

  it("offers Retry and Change server when the catalog fails, focused on Retry", () => {
    const output = render(
      { catalog: [], recentChannelIds: [], status: "empty" },
      { chrome: undefined, loadError: "This Loomarr isn't answering." },
    );
    expect(output).toContain("Can&#x27;t load channels");
    expect(output).toContain("This Loomarr isn&#x27;t answering.");
    expect(output).toContain(">Retry<");
    expect(output).toContain(">Change server<");
    expect(output).not.toContain("No playable channels");
    expect(output).not.toContain('aria-label="Show the controls"');
  });

  it("covers a channel that fails with Retry channel and the way to the Guide", () => {
    const output = render({ ...playing, error: "The stream stopped.", status: "failed" });
    expect(output).toContain("Can&#x27;t play this channel");
    expect(output).toContain("CH 7 · Science Fiction");
    expect(output).toContain(">Retry channel<");
    expect(output).toContain(">Guide<");
    expect(output).not.toContain('aria-label="Show the controls"');
  });

  it("says why there is no picture, and nothing until the catalog has answered", () => {
    expect(render({ catalog: [], recentChannelIds: [], status: "empty" }, { chrome: undefined })).toContain(
      "No playable channels are on this Loomarr yet.",
    );
    expect(
      render({ catalog: [channel], recentChannelIds: [], status: "idle" }, { chrome: undefined }),
    ).toContain("Choose a channel from the Guide or Surf.");
    const loading = render(
      { catalog: [], recentChannelIds: [], status: "empty" },
      { chrome: undefined, loading: true },
    );
    expect(loading).toContain("Loading channels");
    expect(loading).not.toContain("No playable channels");
  });

  it("draws the typed digits and a live auto-tune countdown (#1659 map 5a)", () => {
    vi.useFakeTimers();
    vi.setSystemTime(1_000);
    const container = createTestContainer();
    const textContent = () => Reflect.get(container, "textContent") as string;
    const root = createRoot(container);
    act(() =>
      root.render(
        <LoomarrProvider>
          <TvWatching
            {...props(playing, { numberEntry: { channelName: "Nature", digits: "21", expiresAtMs: 2_200 } })}
          />
        </LoomarrProvider>,
      ),
    );

    expect(textContent()).toContain("2 1 _");
    expect(textContent()).toContain("Nature");
    expect(textContent()).toContain("auto-tunes in 1.2 s");

    act(() => vi.advanceTimersByTime(700));
    expect(textContent()).toContain("auto-tunes in 0.5 s");

    act(() => root.unmount());
    vi.useRealTimers();
  });

  it("omits the countdown readout once digit entry has no expiry", () => {
    const output = render(playing, { numberEntry: { digits: "7" } });
    expect(output).toContain("7 _");
    expect(output).not.toContain("auto-tunes in");
  });
});
