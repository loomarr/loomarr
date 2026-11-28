import type { GuideAiringLayout } from "@loomarr/core/guide";

/** One airing as the guide read carries it. */
type StripAiring = GuideAiringLayout["source"];

/** One block of the film strip: a programme, or a run of one series. */
interface StripProgramme {
  airings: readonly StripAiring[];
  /** How many different episodes the block holds; a film or a pending slot holds one. */
  episodes: number;
  id: string;
  startMs: number;
  stopMs: number;
  type: "programme";
}

/** Commercials or dead time: drawn as a tick, never a block. */
interface StripBreak {
  id: string;
  startMs: number;
  stopMs: number;
  type: "break";
}

type StripItem = StripBreak | StripProgramme;

const isBreak = (airing: StripAiring) => airing.kind === "filler" || airing.kind === "flex";

const seriesOf = (airing: StripAiring) =>
  airing.kind === "program" ? airing.series?.trim() || undefined : undefined;

// One episode either side of a mid-roll break is the same episode.
const episodeOf = (airing: StripAiring) =>
  airing.itemId || `${airing.season ?? ""}/${airing.episode ?? ""}/${airing.title}`;

/**
 * Turn one channel's airings into the strip's items, in time order. Adjacent airings of one series
 * are a run, with or without a break between them; each break stays its own tick, including one that
 * falls inside a run. The guide read carries no episode count per run, so it is counted here, from
 * the airings the caller holds.
 */
const groupSeriesRuns = (airings: readonly StripAiring[]): StripItem[] => {
  const items: StripItem[] = [];
  let run: { airings: StripAiring[]; item: StripProgramme; series: string | undefined } | undefined;

  for (const airing of [...airings].sort((a, b) => a.startMs - b.startMs)) {
    const id = `${airing.scheduleBlockId}@${airing.startMs}`;
    if (isBreak(airing)) {
      items.push({ id, startMs: airing.startMs, stopMs: airing.stopMs, type: "break" });
      continue;
    }
    const series = seriesOf(airing);
    if (run && series && run.series === series) {
      run.airings.push(airing);
      run.item.stopMs = airing.stopMs;
      run.item.episodes = new Set(run.airings.map(episodeOf)).size;
      continue;
    }
    const grouped = [airing];
    const item: StripProgramme = {
      airings: grouped,
      episodes: 1,
      id,
      startMs: airing.startMs,
      stopMs: airing.stopMs,
      type: "programme",
    };
    items.push(item);
    run = { airings: grouped, item, series };
  }
  return items;
};

export type { StripAiring, StripBreak, StripItem, StripProgramme };
export { groupSeriesRuns };
