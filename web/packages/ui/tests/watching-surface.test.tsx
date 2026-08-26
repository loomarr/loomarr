import { LoomarrProvider } from "@loomarr/design-system";
import type { PlayerSnapshot } from "@loomarr/player";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { WatchingSurface } from "../index";

const playing: PlayerSnapshot = {
  attemptId: 4,
  catalog: [],
  channel: { id: "seven", inAppPlayable: true, name: "Science Fiction", number: 7 },
  overlayVisible: true,
  previousChannelId: "six",
  recentChannelIds: ["six"],
  status: "playing",
  tuneReason: "step",
};

const renderSurface = (snapshot: PlayerSnapshot, loadError?: string, chromeVisible = true) =>
  renderToStaticMarkup(
    <LoomarrProvider>
      <WatchingSurface
        chromeVisible={chromeVisible}
        density="tv"
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
        snapshot={snapshot}
      />
    </LoomarrProvider>,
  );

describe("WatchingSurface", () => {
  it("keeps one player mounted behind Channel identity and every explicit tune action", () => {
    const output = renderSurface(playing);
    expect(output).toContain("one-native-player");
    expect(output).toContain("Science Fiction");
    expect(output).toContain("Previous");
    expect(output).toContain("Channel −");
    expect(output).toContain("Guide");
    expect(output).toContain("Surf");
    expect(output).toContain("Channel +");
    expect(output).toContain("Pause");
    expect(output).toContain("Go Live");
  });

  it("renders tuning and recoverable transport failure without replacing Channel identity", () => {
    expect(renderSurface({ ...playing, status: "tuning" })).toContain("Tuning…");
    const failed = renderSurface({ ...playing, error: "Decoder failed", status: "failed" });
    expect(failed).toContain("Science Fiction");
    expect(failed).toContain("Decoder failed");
    expect(failed).toContain("Retry");
  });

  it("states authoritative catalog failure and dead air separately", () => {
    const empty: PlayerSnapshot = {
      catalog: [],
      overlayVisible: true,
      recentChannelIds: [],
      status: "empty",
    };
    expect(renderSurface(empty)).toContain("No playable channels");
    expect(renderSurface(empty, "Couldn't load channels.")).toContain("Couldn&#x27;t load channels.");
  });

  it("keeps the native player mounted without leaking Watching chrome into another journey", () => {
    const output = renderSurface(playing, undefined, false);
    expect(output).toContain("one-native-player");
    expect(output).not.toContain("Playback controls");
    expect(output).not.toContain("Science Fiction");
    expect(output).not.toContain("Show playback controls");
  });
});
