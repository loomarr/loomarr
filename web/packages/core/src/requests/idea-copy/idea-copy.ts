import type { ChannelIdeaDTO } from "@loomarr/api/models/channelIdeaDTO";
import { ChannelIdeaDTOFacet } from "@loomarr/api/models/channelIdeaDTOFacet";
import { ChannelIdeaReasonDTOKind } from "@loomarr/api/models/channelIdeaReasonDTOKind";

// The words on a channel idea card, worded from the server's typed facts. They live here so Web's
// Home and the phone's Requests can never describe the same idea differently.

const DAY_MS = 24 * 60 * 60 * 1000;
// Holidays become ideas at most six weeks out (the API's horizon), so six covers every count.
const WEEKS = ["", "a week", "two weeks", "three weeks", "four weeks", "five weeks", "six weeks"];

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

// What the idea's titles are, as its name says it: movies, series, or titles for a mix.
const kind = (idea: ChannelIdeaDTO, n: number) => {
  if (idea.series === 0) return n === 1 ? "movie" : "movies";
  if (idea.movies === 0) return "series";
  return n === 1 ? "title" : "titles";
};

// "90s" in the last century, "2010s" in this one: the same words the idea's name uses.
const decadeWords = (value: string) => {
  const year = Number(value);
  return year >= 1900 && year < 2000 ? `${String(year % 100).padStart(2, "0")}s` : `${value}s`;
};

// "Halloween is five weeks away" (the mock).
const holidayReason = (label: string, startsAtMs: number | undefined, nowMs: number) => {
  if (startsAtMs === undefined || startsAtMs <= nowMs) return `${label} is on now`;
  const days = Math.ceil((startsAtMs - nowMs) / DAY_MS);
  if (days === 1) return `${label} is tomorrow`;
  if (days < 7) return `${label} is ${days} days away`;
  return `${label} is ${WEEKS[Math.min(Math.floor(days / 7), WEEKS.length - 1)]} away`;
};

// The small line above the name, worded here from the server's typed reason (Q-H3). "14 comedy
// movies, none on a channel yet" (the mock); with nothing on the air every title is unaired, so
// it counts the library instead: "9 comedy series in your library".
const ideaReason = (idea: ChannelIdeaDTO, empty: boolean, nowMs: number) => {
  const { reason } = idea;
  if (reason.kind === ChannelIdeaReasonDTOKind.holiday) {
    return holidayReason(reason.holidayLabel ?? idea.name, reason.startsAtMs, nowMs);
  }
  const noun = kind(idea, reason.count);
  const what =
    idea.facet === ChannelIdeaDTOFacet.genre
      ? `${reason.count} ${idea.value.toLowerCase()} ${noun}`
      : idea.facet === ChannelIdeaDTOFacet.decade
        ? `${reason.count} ${noun} from the ${decadeWords(idea.value)}`
        : `${reason.count} ${noun}`;
  return empty ? `${what} in your library` : `${what}, none on a channel yet`;
};

// "18 movies · 16 in your library, 2 to download" (the mock).
const ideaCount = (idea: ChannelIdeaDTO) =>
  [idea.movies > 0 && plural(idea.movies, "movie", "movies"), idea.series > 0 && `${idea.series} series`]
    .filter(Boolean)
    .join(" and ");

const ideaAvailability = (idea: ChannelIdeaDTO) =>
  idea.toDownload === 0
    ? "all in your library"
    : `${idea.inLibrary} in your library, ${idea.toDownload} to download`;

export { ideaAvailability, ideaCount, ideaReason };
