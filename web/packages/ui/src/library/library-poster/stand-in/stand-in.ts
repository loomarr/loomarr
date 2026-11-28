// What the foot of a drawn cover says (frontend-design §2.1): the year, and the kind where a movie
// is not what it is.

interface CoverFacts {
  year?: number;
  kind?: "movie" | "series";
  homeVideo?: boolean;
}

const coverYear = (year: number | undefined) => (year ? String(year) : "Year unknown");

const coverKind = ({ kind, homeVideo }: CoverFacts) => {
  if (homeVideo) return "Home video";
  return kind === "series" ? "Series" : "";
};

export type { CoverFacts };
export { coverKind, coverYear };
