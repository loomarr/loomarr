import type { GrowthCounts, GrowthJoinedTitle, GrowthStill } from "./channel-growth-section.type";

// Plain words for the channel page: the household reads these, so none of the backend's own terms
// (line, batch, fit) appear in them.

const STILL_WORDS: Record<Exclude<GrowthStill, "">, string> = {
  by_hand: "Made by hand, so it doesn't add shows by itself.",
  fixed: "Nothing in your library fits its picks yet.",
  kept: "Kept as it is, so nothing joins by itself.",
  not_active: "Not on air, so it isn't adding anything.",
};

const stillWords = (still: GrowthStill): string => (still === "" ? "" : STILL_WORDS[still]);

const count = (n: number, singular: string, plural = `${singular}s`) => `${n} ${n === 1 ? singular : plural}`;

const joinParts = (parts: readonly string[]): string =>
  parts.length < 2 ? (parts[0] ?? "") : `${parts.slice(0, -1).join(", ")} and ${parts[parts.length - 1]}`;

const isEmpty = (c: GrowthCounts) => c.episodes + c.shows + c.films === 0;

/** What the channel lacks for a week without repeats, or that it has enough; "" when there is no target. */
const shortfallWords = (target: GrowthCounts, short: GrowthCounts): string => {
  if (isEmpty(target)) return "";
  if (isEmpty(short)) return "It has enough for a week without repeats.";
  const parts = [
    short.episodes > 0 ? `about ${count(short.episodes, "more episode")}` : "",
    short.shows > 0 ? count(short.shows, "more show") : "",
    short.films > 0 ? count(short.films, "more film") : "",
  ].filter(Boolean);
  return `Needs ${joinParts(parts)} for a week without repeats.`;
};

const waitingWords = (waiting: number): string =>
  waiting <= 0
    ? ""
    : `${count(waiting, "title is", "titles are")} waiting for the AI check before ${waiting === 1 ? "it" : "they"} can join.`;

// "growth-2026-11-18" is the night it joined; anything else reads as an earlier night.
const nightOf = (batch: string): string => /\d{4}-\d{2}-\d{2}$/.exec(batch)?.[0] ?? "";

const nightLabel = (night: string): string => {
  const date = new Date(`${night}T00:00:00Z`);
  if (!night || Number.isNaN(date.getTime())) return "Earlier";
  const day = date.toLocaleDateString("en-US", {
    day: "numeric",
    month: "long",
    timeZone: "UTC",
    year: "numeric",
  });
  return `Added on ${day}`;
};

interface JoinedNight {
  label: string;
  titles: GrowthJoinedTitle[];
}

/** The joined titles grouped by the night they joined, newest night first. */
const groupByNight = (joined: readonly GrowthJoinedTitle[]): JoinedNight[] => {
  const nights = new Map<string, GrowthJoinedTitle[]>();
  for (const title of joined) {
    const night = nightOf(title.batch);
    nights.set(night, [...(nights.get(night) ?? []), title]);
  }
  return [...nights.entries()]
    .sort(([a], [b]) => (a < b ? 1 : a > b ? -1 : 0))
    .map(([night, titles]) => ({ label: nightLabel(night), titles }));
};

export type { JoinedNight };
export { groupByNight, shortfallWords, stillWords, waitingWords };
