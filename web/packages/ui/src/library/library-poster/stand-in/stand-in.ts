// What a drawn cover knows of its title (frontend-design §2.1). Its foot says the year alone; the
// caption under the poster says the kind, in the Library's own words.
interface CoverFacts {
  year?: number;
  kind?: "movie" | "series";
  homeVideo?: boolean;
}

export type { CoverFacts };
