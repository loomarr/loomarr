import type { LibraryAiringDTO } from "@loomarr/api/models/libraryAiringDTO";
import type { LibraryAiringEpisodeDTO } from "@loomarr/api/models/libraryAiringEpisodeDTO";
import { formatGuideTime } from "@loomarr/core/guide";

// An airing in plain words, on the household's wall clock as the Guide tells it. Days, times and
// dates share the Guide's locale, so one page never mixes two.

const LOCALE = "en-US";
const DAY_MS = 24 * 60 * 60 * 1000;

// The days since 1970 that a zone's calendar reads at a moment.
const dayNumber = (at: number, timeZone?: string) => {
  // en-CA writes a date as YYYY-MM-DD, which is read here and never shown.
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
      new Intl.DateTimeFormat(LOCALE, { timeZone, hour: "numeric", hourCycle: "h23" }).format(at),
    );
    return hour >= 18 ? "tonight" : "today";
  }
  if (away === 1) return "tomorrow";
  return new Intl.DateTimeFormat(LOCALE, {
    timeZone,
    ...(away < 7 ? { weekday: "long" } : { month: "long", day: "numeric" }),
  }).format(at);
};

// When a channel shows the title; undefined for an airing that has ended.
const airingWhen = (
  { start, end }: Pick<LibraryAiringDTO, "start" | "end">,
  timeZone: string | undefined,
  now = Date.now(),
) => {
  if (Date.parse(end) <= now) return undefined;
  const at = Date.parse(start);
  if (at <= now) return "On now";
  return `Next ${dayWords(at, now, timeZone)} at ${formatGuideTime(at, timeZone)}`;
};

// The next moment an airing's words change: its start, then its end. A past one means the server
// has a later airing to tell.
const airingsChangeAt = (airings: readonly LibraryAiringDTO[], now = Date.now()) => {
  const moments = airings.map((a) => {
    const start = Date.parse(a.start);
    return start > now ? start : Date.parse(a.end);
  });
  return moments.length > 0 ? Math.min(...moments) : undefined;
};

// "March 14, 1979", read as the calendar date it names rather than a moment in the viewer's zone.
const dateWords = (date: string) =>
  new Intl.DateTimeFormat(LOCALE, { timeZone: "UTC", dateStyle: "long" }).format(Date.parse(date));

const airingEpisode = ({ season, number, end, name }: LibraryAiringEpisodeDTO) => {
  const numbered =
    number > 0 ? (end && end > number ? `episodes ${number} and ${end}` : `episode ${number}`) : "";
  const where = [season === 0 ? "Special" : `Season ${season}`, numbered].filter(Boolean).join(", ");
  return [where, name].filter(Boolean).join(" · ");
};

export { airingEpisode, airingsChangeAt, airingWhen, dateWords };
