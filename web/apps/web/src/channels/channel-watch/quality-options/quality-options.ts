import type { WatchPlayer } from "../../use-watch-player";

// A rung as Settings names it: the premium by its class, the rest by lines.
const rungLabel = (height: number) => (height >= 2160 ? "4K" : `${height}p`);

// Quality (#2088 decision 7): Auto, naming the rung on air, then the channel's rungs. A remembered
// rung this channel doesn't offer shows Auto chosen, because Auto is what plays.
const qualityOptionsFor = ({ ladder, quality }: Pick<WatchPlayer, "ladder" | "quality">) => {
  const { rungs, height: onAir } = ladder;
  const pinned = quality !== "auto" && rungs.some((rung) => rung.height === quality);
  return rungs.length > 0
    ? [
        { label: onAir ? `Auto · ${rungLabel(onAir)}` : "Auto", selected: !pinned, value: "auto" },
        ...rungs.map((rung) => ({
          label: rungLabel(rung.height),
          selected: pinned && rung.height === quality,
          value: String(rung.height),
        })),
      ]
    : undefined;
};

export { qualityOptionsFor };
