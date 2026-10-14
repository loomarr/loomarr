import type { GuideAiring } from "@loomarr/api/models/guideAiring";
import type { GuideChannelTimeline } from "@loomarr/api/models/guideChannelTimeline";
import { describe, expect, it } from "vitest";
import { drawerChannels } from "./drawer-channels";

const NOW = Date.UTC(2026, 8, 28, 21, 30);
const MIN = 60_000;

const airing = (
  id: string,
  startMin: number,
  stopMin: number,
  extra: Partial<GuideAiring> = {},
): GuideAiring => ({
  kind: "program",
  scheduleBlockId: id,
  startMs: NOW + startMin * MIN,
  stopMs: NOW + stopMin * MIN,
  title: id,
  ...extra,
});

const timeline = (extra: Partial<GuideChannelTimeline>): GuideChannelTimeline => ({
  airings: [],
  channelId: "c",
  name: "A channel",
  number: 1,
  pendingCount: 0,
  status: "live",
  ...extra,
});

describe("drawerChannels", () => {
  it("reads what's on now, the minutes left and the strip from just before now to two hours on", () => {
    const [first] = drawerChannels(
      [
        timeline({
          airings: [
            airing("before", -120, -60),
            airing("now", -30, 20, { series: "A sitcom", title: "The pilot" }),
            airing("break", 20, 23, { kind: "filler" }),
            airing("later", 23, 180),
          ],
        }),
      ],
      NOW,
    );
    expect(first?.now).toBe("A sitcom — “The pilot”");
    expect(first?.minutesLeft).toBe(20);
    expect(first?.blocks.map((b) => [b.key, b.label, b.minutes, b.pod])).toEqual([
      ["now", "A sitcom", 44, false],
      ["break", undefined, 3, true],
      ["later", "later", 97, false],
    ]);
  });

  it("says why a paused or unfinished channel has nothing on, in channel-number order", () => {
    const rows = drawerChannels(
      [
        timeline({ channelId: "b", number: 9, status: "building", airings: [airing("x", -5, 5)] }),
        timeline({ channelId: "p", number: 3, status: "paused", airings: [airing("y", -5, 5)] }),
      ],
      NOW,
    );
    expect(rows.map((r) => [r.id, r.offAir, r.now, r.blocks.length])).toEqual([
      ["p", "paused", undefined, 0],
      ["b", "building", undefined, 0],
    ]);
  });
});
