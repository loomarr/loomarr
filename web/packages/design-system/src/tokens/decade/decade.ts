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

export type { DecadeFill };
export { decadeFill, decadeFillColor };
