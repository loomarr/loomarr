import { layoutGuide } from "@loomarr/core/guide";
import { describe, expect, it } from "vitest";

import { watchingPanelFromGuide } from "../index";

const NOW = Date.UTC(2026, 8, 28, 20, 26);
const HOUR = 3_600_000;
const FROM = Date.UTC(2026, 8, 28, 20, 0);

const layout = layoutGuide(
  {
    fromMs: FROM,
    toMs: FROM + 2 * HOUR,
    timezone: "UTC",
    channels: [1, 2, 3].map((n) => ({
      channelId: `ch-${n}`,
      name: `Channel ${n}`,
      number: n,
      pendingCount: 0,
      status: "live" as const,
      airings: [0, 1].map((j) => ({
        kind: "program" as const,
        scheduleBlockId: `ch-${n}-${j}`,
        startMs: FROM + j * HOUR,
        stopMs: FROM + (j + 1) * HOUR,
        title: `Programme ${n}-${j}`,
      })),
    })),
  },
  NOW,
);

describe("watchingPanelFromGuide", () => {
  it("reads now and next, the favourites and Previous from the guide, as web and phone both draw them", () => {
    const panel = watchingPanelFromGuide({
      channelId: "ch-1",
      favouriteIds: ["ch-3", "ch-2"],
      layout,
      nowMs: NOW,
      playableChannelIds: ["ch-1", "ch-2", "ch-3"],
      recentIds: ["ch-1", "ch-2"],
    });
    expect(panel.clockLabel).toBe("8:26 PM");
    expect(panel.schedule?.now?.title).toContain("Programme 1-0");
    // In guide order, as the Surf rail lists them.
    expect(panel.favourites.map((channel) => channel.id)).toEqual(["ch-2", "ch-3"]);
    expect(panel.previousId).toBe("ch-2");
  });

  it("has no Previous when the only recent is this channel or can't play, and no schedule without a guide", () => {
    const panel = watchingPanelFromGuide({
      channelId: "ch-1",
      favouriteIds: [],
      nowMs: NOW,
      playableChannelIds: ["ch-1"],
      recentIds: ["ch-1", "ch-2"],
    });
    expect(panel.previousId).toBeUndefined();
    expect(panel.schedule).toBeUndefined();
    expect(panel.favourites).toEqual([]);
  });
});
