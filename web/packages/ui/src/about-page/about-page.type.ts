import type { ReactNode } from "react";

/** One technical fact, already in words: "Version" · "v0.9.3", "Uptime" · "6d 4h 12m". */
interface AboutDetail {
  label: string;
  value: string;
}

interface AboutPageProps {
  /** The release this install runs, as the server names it: "v0.9.3", "v0.9.3 (modified)", "dev". */
  version: string;
  /**
   * What someone quotes in a bug report, in order. Rows the server could not fill are left out by
   * the host, never sent blank.
   */
  details: AboutDetail[];
  /**
   * TMDB's own mark, from their attribution page and sized by the host (16px high). TMDB's terms
   * want it less prominent than Loomarr's branding, which the large lockup above it is.
   */
  tmdbLogo?: ReactNode;
  /**
   * This server is the demo site, so its films, cartoons and ads are the openly licensed demo pack
   * and About says where their sources are listed (#3409).
   */
  demo?: boolean;
}

export type { AboutDetail, AboutPageProps };
