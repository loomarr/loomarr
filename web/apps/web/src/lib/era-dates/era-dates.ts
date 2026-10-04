import type { DateScope } from "@loomarr/api/models/dateScope";
import type { Range } from "@loomarr/api/models/range";

// An era is a shortcut for programming dates, not a second date scope (#1877): era R is the
// same range on movie release, series premiere and episode airing. The backend stores and
// returns only `dates`, so the era a scope holds is read back from era-shaped dates.

const sameRanges = (a: Range[] = [], b: Range[] = []): boolean =>
  a.length === b.length && a.every((r, i) => r.from === b[i]?.from && r.to === b[i]?.to);

// eraOf returns the single range when all three axes hold the same one range; otherwise the
// scope is not expressible as an era and there is none.
const eraOf = (dates: DateScope | null | undefined): Range | undefined => {
  const movie = dates?.movieRelease ?? [];
  if (
    movie.length !== 1 ||
    !sameRanges(movie, dates?.seriesPremiere) ||
    !sameRanges(movie, dates?.seriesAiring)
  ) {
    return undefined;
  }
  return movie[0];
};

// eraDates is the date scope an era stands for. A range open at both ends constrains nothing.
const eraDates = (era: Range | undefined): DateScope | undefined => {
  if (!era || (!era.from && !era.to)) return undefined;
  return { movieRelease: [era], seriesPremiere: [era], seriesAiring: [era] };
};

export { eraDates, eraOf };
