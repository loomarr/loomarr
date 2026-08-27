import { LoomarrProvider } from "@loomarr/design-system";
import type { PlayerSnapshot } from "@loomarr/player";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { WatchingSurface } from "../index";

const playing: PlayerSnapshot = {
  attemptId: 4,
  catalog: [],
  channel: { id: "seven", inAppPlayable: true, name: "Science Fiction", number: 7 },
  livePlayback: { lagSeconds: 0, mode: "live", noticeRevision: 0, viewerTimeMs: 1_777_777_777_000 },
  overlayVisible: true,
  previousChannelId: "six",
  recentChannelIds: ["six"],
  status: "playing",
  tuneReason: "step",
};

const renderSurface = (
  snapshot: PlayerSnapshot,
  loadError?: string,
  chromeVisible = true,
  schedule?: Parameters<typeof WatchingSurface>[0]["schedule"],
  density: Parameters<typeof WatchingSurface>[0]["density"] = "tv",
) =>
  renderToStaticMarkup(
    <LoomarrProvider>
      <WatchingSurface
        chromeVisible={chromeVisible}
        density={density}
        loadError={loadError}
        onChannelDown={vi.fn()}
        onChannelUp={vi.fn()}
        onDismissControls={vi.fn()}
        onGoLive={vi.fn()}
        onOpenGuide={vi.fn()}
        onOpenSurf={vi.fn()}
        onPause={vi.fn()}
        onPlay={vi.fn()}
        onPrevious={vi.fn()}
        onRetry={vi.fn()}
        onShowControls={vi.fn()}
        player={<div data-player="one-native-player" />}
        schedule={schedule}
        snapshot={snapshot}
      />
    </LoomarrProvider>,
  );

describe("WatchingSurface", () => {
  it("keeps one player mounted behind Compose-parity TV chrome and quiet remote hints", () => {
    const output = renderSurface(playing);
    expect(output).toContain("one-native-player");
    expect(output).toContain("Science Fiction");
    expect(output).toContain("Up/Down tune");
    expect(output).toContain("Left Surf");
    expect(output).toContain("0–9 jump");
    expect(output).toContain("OK Guide");
    expect(output).not.toContain("Playback controls");
    expect(output).not.toContain("Previous");
    expect(output).not.toContain("Channel −");
    expect(output).not.toContain("Channel +");
  });

  it("keeps explicit playback and tune actions in the touch adapter", () => {
    const output = renderSurface(playing, undefined, true, undefined, "touch");
    expect(output).toContain("one-native-player");
    expect(output).toContain("Previous");
    expect(output).toContain("Channel −");
    expect(output).toContain("Guide");
    expect(output).toContain("Surf");
    expect(output).toContain("Channel +");
    expect(output).toContain("Pause");
    expect(output).toContain("Live");
    expect(output).not.toContain("Go Live");
  });

  it("shows delayed playback lag and offers the explicit live-edge action", () => {
    const output = renderSurface({
      ...playing,
      livePlayback: {
        lagSeconds: 83,
        mode: "paused",
        noticeRevision: 0,
        viewerTimeMs: 1_777_777_777_000,
      },
      status: "paused",
    });

    expect(output).toContain("Paused · 1:23 behind");
    expect(output).toContain("Go Live");
    expect(output).toContain("Play");
  });

  it("explains when an expired paused point safely returns to live", () => {
    const output = renderSurface({
      ...playing,
      livePlayback: { ...playing.livePlayback!, noticeRevision: 1 },
    });

    expect(output).toContain("Paused position expired. Returned to live.");
  });

  it("renders tuning and recoverable transport failure without replacing Channel identity", () => {
    expect(renderSurface({ ...playing, status: "tuning" })).toContain("Tuning…");
    const failed = renderSurface({ ...playing, error: "Decoder failed", status: "failed" });
    expect(failed).toContain("Science Fiction");
    expect(failed).toContain("Decoder failed");
    expect(failed).toContain("Retry");
  });

  it("renders authoritative now, next, and live progress in playback chrome", () => {
    const output = renderSurface(playing, undefined, true, {
      next: { timeLabel: "9:30 PM", title: "The Next Frontier" },
      now: {
        badge: { label: "On now", tone: "live" },
        progressPercent: 42,
        timeLabel: "9:00 PM–9:30 PM",
        title: "The Current Frontier",
      },
    });

    expect(output).toContain("The Current Frontier");
    expect(output).toContain("Next 9:30 PM · The Next Frontier");
    expect(output).toContain('aria-valuenow="42"');
  });

  it("states authoritative catalog failure and dead air separately", () => {
    const empty: PlayerSnapshot = {
      catalog: [],
      overlayVisible: true,
      recentChannelIds: [],
      status: "empty",
    };
    const deadAir = renderSurface(empty);
    expect(deadAir).toContain("No playable channels");
    expect(deadAir).not.toContain("Retry");
    const failedLoad = renderSurface(empty, "Couldn't load channels.");
    expect(failedLoad).toContain("Couldn&#x27;t load channels.");
    expect(failedLoad).toContain("Retry");
  });

  it("keeps the native player mounted without leaking Watching chrome into another journey", () => {
    const output = renderSurface(playing, undefined, false);
    expect(output).toContain("one-native-player");
    expect(output).not.toContain("Playback controls");
    expect(output).not.toContain("Science Fiction");
    expect(output).not.toContain("Show playback controls");
  });
});
