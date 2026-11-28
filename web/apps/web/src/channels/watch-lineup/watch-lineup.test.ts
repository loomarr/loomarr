import type { GuideChannelTimeline } from "@loomarr/api/models/guideChannelTimeline";
import { describe, expect, it } from "vitest";
import { channel } from "@/test/fixtures/channels";
import { watchLineupChannels } from "./watch-lineup";

const NOW = 10 * 60 * 60_000;

const timeline = (channelId: string, airings: GuideChannelTimeline["airings"]): GuideChannelTimeline => ({
  airings,
  channelId,
  name: channelId,
  number: 0,
  pendingCount: 0,
  status: "live",
});

const airing = (title: string, startMs: number, stopMs: number, kind: "program" | "filler" = "program") =>
  ({ kind, scheduleBlockId: title, startMs, stopMs, title }) as GuideChannelTimeline["airings"][number];

describe("watchLineupChannels", () => {
  it("lists every channel with what is on now and the minutes left, from the guide", () => {
    const channels = [
      channel({ decade: 1990, id: "ch-1", name: "Noir", number: 1 }),
      channel({ id: "ch-2", name: "Toons", number: 2 }),
    ];
    const lineup = watchLineupChannels(
      channels,
      [
        timeline("ch-1", [
          airing("Old one", NOW - 60_000 * 60, NOW - 1),
          airing("Now one", NOW - 1, NOW + 12 * 60_000),
        ]),
      ],
      NOW,
    );
    expect(lineup[0]).toMatchObject({
      decade: 1990,
      minutesLeft: 12,
      name: "Noir",
      onNow: "Now one",
      playable: true,
    });
    // The guide has not read it: its name and number alone.
    expect(lineup[1]).toEqual({ id: "ch-2", name: "Toons", number: 2, playable: true });
  });

  it("reads a break as nothing on now", () => {
    const lineup = watchLineupChannels(
      [channel({ id: "ch-1", number: 1 })],
      [timeline("ch-1", [airing("Spots", NOW - 1, NOW + 60_000, "filler")])],
      NOW,
    );
    expect(lineup[0]?.onNow).toBeUndefined();
  });

  it("lists a paused channel, and one the app cannot play, as not playable", () => {
    const lineup = watchLineupChannels(
      [
        channel({ id: "ch-1", number: 1, status: "paused" }),
        channel({ id: "ch-2", inAppPlayable: false, number: 2 }),
        channel({ id: "ch-3", number: 3 }),
      ],
      [],
      NOW,
    );
    expect(lineup.map((each) => each.playable)).toEqual([false, false, true]);
  });
});
