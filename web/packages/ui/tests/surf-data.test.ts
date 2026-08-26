import { layoutGuide } from "@loomarr/core/guide";
import { describe, expect, it } from "vitest";
import { restoreSurfSelection, surfGroupsFromGuide, watchingScheduleFromGuide } from "../index";

const layout = layoutGuide(
  {
    channels: [
      {
        airings: [
          {
            kind: "program",
            scheduleBlockId: "now",
            startMs: 1_000,
            stopMs: 3_000,
            thumbUrl: "/v1/images/now.jpg",
            title: "Now",
          },
          { kind: "program", scheduleBlockId: "next", startMs: 3_000, stopMs: 5_000, title: "Next" },
        ],
        channelId: "seven",
        logo: "https://logos.example/seven.png",
        name: "Science Fiction",
        number: 7,
        pendingCount: 0,
        status: "live",
      },
      {
        airings: [],
        channelId: "blocked",
        name: "Not playable",
        number: 8,
        pendingCount: 0,
        status: "live",
      },
    ],
    fromMs: 1_000,
    timezone: "America/New_York",
    toMs: 5_000,
  },
  2_000,
);

describe("Surf data", () => {
  it("uses authoritative now/next identity and only server-declared playable channels", () => {
    const groups = surfGroupsFromGuide(layout, ["seven"], ["blocked", "seven"], 2_000);

    expect(groups[0]).toEqual({ channels: [], kind: "favourites", label: "Favourites" });
    expect(groups[1]?.channels.map(({ id }) => id)).toEqual(["seven"]);
    expect(groups[2]?.channels).toHaveLength(1);
    expect(groups[2]?.channels[0]).toMatchObject({
      channelName: "Science Fiction",
      channelLogoUri: "https://logos.example/seven.png",
      next: { title: "Next" },
      now: { artworkUri: "/v1/images/now.jpg", progressPercent: 50, title: "Now" },
    });
  });

  it("restores by group, then channel identity, then first available row", () => {
    const groups = surfGroupsFromGuide(layout, ["seven"], ["seven"], 2_000);
    expect(restoreSurfSelection(groups, { channelId: "seven", group: "recent" })).toEqual({
      channelId: "seven",
      group: "recent",
    });
    expect(restoreSurfSelection(groups.slice(2), { channelId: "seven", group: "recent" })).toEqual({
      channelId: "seven",
      group: "all",
    });
  });

  it("maps the tuned channel into the Watching now/next schedule", () => {
    expect(watchingScheduleFromGuide(layout, "seven", 2_000)).toMatchObject({
      next: { title: "Next" },
      now: { artworkUri: "/v1/images/now.jpg", progressPercent: 50, title: "Now" },
    });
    expect(watchingScheduleFromGuide(layout, "missing", 2_000)).toBeUndefined();
  });
});
