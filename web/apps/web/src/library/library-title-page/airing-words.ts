import type { LibraryAiringEpisodeDTO } from "@loomarr/api/models/libraryAiringEpisodeDTO";
import { formatGuideTime } from "@loomarr/core/guide";

// An airing in plain words, on the household's wall clock as the Guide tells it.

const DAY_MS = 24 * 60 * 60 * 1000;

// The days since 1970 that a zone's calendar reads at a moment.
const dayNumber = (at: number, timeZone?: string) => {
  const date = new Intl.DateTimeFormat("en-CA", {
    timeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(at);
  return Date.parse(`${date}T00:00:00Z`) / DAY_MS;
};

const dayWords = (at: number, now: number, timeZone?: string) => {
  const away = dayNumber(at, timeZone) - dayNumber(now, timeZone);
  if (away === 0) {
    const hour = Number(
      new Intl.DateTimeFormat("en-US", { timeZone, hour: "numeric", hourCycle: "h23" }).format(at),
    );
    return hour >= 18 ? "tonight" : "today";
  }
  if (away === 1) return "tomorrow";
  return new Intl.DateTimeFormat("en-US", {
    timeZone,
    ...(away < 7 ? { weekday: "long" } : { month: "long", day: "numeric" }),
  }).format(at);
};

const airingWhen = (start: string, timeZone: string | undefined, now = Date.now()) => {
  const at = Date.parse(start);
  if (at <= now) return "On now";
  return `Next ${dayWords(at, now, timeZone)} at ${formatGuideTime(at, timeZone)}`;
};

const airingEpisode = ({ season, number, end, name }: LibraryAiringEpisodeDTO) => {
  const numbered =
    number > 0 ? (end && end > number ? `episodes ${number} and ${end}` : `episode ${number}`) : "";
  const where = [season === 0 ? "Special" : `Season ${season}`, numbered].filter(Boolean).join(", ");
  return [where, name].filter(Boolean).join(" · ");
};

export { airingEpisode, airingWhen };
