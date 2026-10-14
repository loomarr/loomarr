import type { CycleSummaryDTO } from "@loomarr/api/models/cycleSummaryDTO";

// What one full pass through a channel's titles looks like — straight from the scheduler's
// cycle preview (CycleSummaryDTO), plus the order the scheduler actually resolved. Nothing here
// is hand-counted: the numbers are whatever the backend says the channel would air.
interface PlayOrderCycle extends CycleSummaryDTO {
  ordering?: string;
}

const HOUR_MS = 3_600_000;
const DAY_MS = 24 * HOUR_MS;

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

// "226 episodes", "12 movies", "40 episodes and 3 movies". Shows are named only when more than
// one feeds the channel — for a single show the count alone is the whole story.
const describeTitles = ({ episodes, movies, series }: CycleSummaryDTO): string => {
  const parts: string[] = [];
  if (episodes > 0) parts.push(plural(episodes, "episode", "episodes"));
  if (movies > 0) parts.push(plural(movies, "movie", "movies"));
  const titles = parts.join(" and ");
  return series > 1 ? `${titles} from ${series} shows` : titles;
};

// "about 3½ days": days to the nearest half under ten days, whole days then weeks beyond; hours
// below a day. Commercial breaks are not counted — "about" carries that.
const describeSpan = (runtimeMs: number): string => {
  if (runtimeMs < DAY_MS) {
    return `${plural(Math.max(1, Math.round(runtimeMs / HOUR_MS)), "hour", "hours")}`;
  }
  const days = runtimeMs / DAY_MS;
  if (days < 10) {
    const halves = Math.round(days * 2) / 2;
    const whole = Math.trunc(halves);
    const text = halves === whole ? `${whole}` : whole === 0 ? "½" : `${whole}½`;
    return `${text} ${halves > 1 ? "days" : "day"}`;
  }
  if (days < 28) return `${Math.round(days)} days`;
  return plural(Math.round(days / 7), "week", "weeks");
};

// Option wording, with no numbers: shown while the cycle loads and for a channel with nothing
// to count. "Syndication" keeps its name (the control names it) but is always explained.
const PLAY_ORDER_SUMMARY: Record<string, string> = {
  sequential: "Plays the list in order, then starts over.",
  shuffle: "Plays everything in a random mix instead of episode order.",
  syndication:
    "Deals the list like a deck of cards: everything plays once before anything repeats, then it is dealt again.",
};

// The consequence of the effective play order for THIS channel. `ordering` is the mode the
// scheduler resolved (the policy's choice, or what "use channel strategy" falls back to); a mode
// with no description here (time slots) yields nothing rather than a guess.
const describePlayOrder = (ordering: string, cycle?: CycleSummaryDTO): string | undefined => {
  const summary = PLAY_ORDER_SUMMARY[ordering];
  if (!summary) return undefined;
  if (!cycle || cycle.episodes + cycle.movies === 0) return summary;

  const titles = describeTitles(cycle);
  const span = describeSpan(cycle.runtimeMs);
  switch (ordering) {
    case "sequential":
      return `${titles}, ${cycle.series === 1 && cycle.movies === 0 ? "start to finish" : "one after another in order"}, then it starts over — about every ${span}.`;
    case "shuffle":
      return `${titles}, in a random mix instead of episode order. One full pass is about ${span}.`;
    default:
      return `${titles}, dealt like a deck of cards: everything plays once before anything repeats, then it is dealt again. One full pass is about ${span}.`;
  }
};

export type { PlayOrderCycle };
export { describePlayOrder, describeSpan };
