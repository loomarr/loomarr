import { describe, expect, it } from "vitest";
import type { WatchPlayer } from "../../use-watch-player";
import { qualityOptionsFor } from "./quality-options";

const ladder = (rungs: number[], height?: number) =>
  ({
    height,
    onAir: height,
    rungs: rungs.map((rung) => ({ height: rung })),
  }) as unknown as WatchPlayer["ladder"];

describe("qualityOptionsFor", () => {
  it("offers nothing while the channel's rungs are not known", () => {
    expect(qualityOptionsFor({ ladder: ladder([]), quality: "auto" })).toBeUndefined();
  });

  it("puts Auto first, naming the rung on air, and has it chosen on auto", () => {
    expect(qualityOptionsFor({ ladder: ladder([1080, 720], 1080), quality: "auto" })).toEqual([
      { label: "Auto · 1080p", selected: true, value: "auto" },
      { label: "1080p", selected: false, value: "1080" },
      { label: "720p", selected: false, value: "720" },
    ]);
  });

  it("calls the premium rung 4K and plain Auto when nothing is on air yet", () => {
    const options = qualityOptionsFor({ ladder: ladder([2160, 1080]), quality: "auto" });
    expect(options?.map((o) => o.label)).toEqual(["Auto", "4K", "1080p"]);
  });

  it("chooses a pinned rung the channel offers, and not Auto", () => {
    const options = qualityOptionsFor({ ladder: ladder([1080, 720], 1080), quality: 720 });
    expect(options?.filter((o) => o.selected).map((o) => o.value)).toEqual(["720"]);
  });

  it("shows Auto chosen when the remembered rung is one this channel doesn't offer", () => {
    const options = qualityOptionsFor({ ladder: ladder([1080, 720], 1080), quality: 480 });
    expect(options?.filter((o) => o.selected).map((o) => o.value)).toEqual(["auto"]);
  });
});
