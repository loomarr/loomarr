import { semanticColors } from "../tokens";

// A channel's era colour (#2767): the decade the API sends (`decade`, 1980 for the 1980s) as the
// theme colour its number block is filled with. Only a decade the era palette names has a colour;
// any other decade, or none, is neutral, and the caller keeps its own block.

type EraDecade = "1970" | "1980" | "1990";
type DecadeFill = `decade${EraDecade}`;

const eraDecades: readonly EraDecade[] = ["1970", "1980", "1990"];

const eraDecadeOf = (decade: number | undefined): EraDecade | undefined => {
  const key = String(decade) as EraDecade;
  return eraDecades.includes(key) ? key : undefined;
};

/** The theme colour a channel's number block takes for its decade; undefined is neutral. */
const decadeFill = (decade: number | undefined): DecadeFill | undefined => {
  const key = eraDecadeOf(decade);
  return key ? `decade${key}` : undefined;
};

/**
 * The decade's fill as a colour value, for chrome drawn the same in both themes (the player's lower
 * third) that takes inline styles rather than a theme key. The number on it is `semanticColors.decade.ink`.
 */
const decadeFillColor = (decade: number | undefined): string | undefined => {
  const key = eraDecadeOf(decade);
  return key ? semanticColors.decade[key] : undefined;
};

// The Cross's decade tints (frontend-design §2.1), for the watched channel's numeral and the By
// decade headings, and on a Library title's drawn cover. The server sends any decade from 1900 to
// 2090, or none; only 1950 to 2020 have a tint, and everything else is neutral.

type TintDecade = Exclude<keyof typeof semanticColors.decadeTint, "neutral">;
type DecadeTint = `decadeTint${TintDecade | "Neutral"}`;

const tintDecadeOf = (decade: number | undefined): TintDecade | undefined => {
  const key = String(decade);
  return key !== "neutral" && Object.hasOwn(semanticColors.decadeTint, key) ? (key as TintDecade) : undefined;
};

/** The theme colour for a channel's decade (1980 for the 1980s), which follows the theme. */
const decadeTint = (decade: number | undefined): DecadeTint =>
  `decadeTint${tintDecadeOf(decade) ?? "Neutral"}`;

/**
 * The tint as a colour value for always-dark watching chrome, which takes the dark set in both
 * themes. Anything that follows the theme uses `decadeTint`.
 */
const decadeTintColor = (decade: number | undefined): string =>
  semanticColors.decadeTint[tintDecadeOf(decade) ?? "neutral"];

type CoverWash = `coverWash${TintDecade | "Neutral"}`;

/** The theme colour a Library title's drawn cover is washed with, for the year it came out. */
const coverWash = (year: number | undefined): CoverWash =>
  `coverWash${tintDecadeOf(year && Math.floor(year / 10) * 10) ?? "Neutral"}`;

export type { CoverWash, DecadeFill, DecadeTint };
export { coverWash, decadeFill, decadeFillColor, decadeTint, decadeTintColor };
