import { formatBytes } from "@loomarr/core/format";

import type { LibraryProblem } from "../library-needs.type";
import type { ProblemEntry } from "./problem-entries.type";

const problemEntries = (problems: readonly LibraryProblem[]): ProblemEntry[] =>
  problems.flatMap((p) => {
    const out: ProblemEntry[] = [];
    const base = { id: p.id, name: p.name, poster: p.poster };
    if (p.unplayable > 0) {
      const movie = p.kind === "movie";
      const one = movie || p.unplayable === 1;
      const episodes = p.unplayable === 1 ? "1 episode" : `${p.unplayable} episodes`;
      out.push({
        ...base,
        key: `${p.id}:unplayable`,
        line: movie ? `${p.name} — can't be played` : `${p.name} — ${episodes} can't be played`,
        headline: movie
          ? `${p.name} can't be played`
          : p.unplayable === 1
            ? `${p.name} has an episode that can't be played`
            : `${p.name} has ${episodes} that can't be played`,
        reason: one
          ? "The file may be damaged. Loomarr skips it until it's fixed."
          : "The files may be damaged. Loomarr skips them until they're fixed.",
      });
    }
    if (p.extraBytes > 0) {
      const size = formatBytes(p.extraBytes);
      out.push({
        ...base,
        key: `${p.id}:extra`,
        line: `${p.name} — an extra copy is taking ${size}`,
        headline: `${p.name} has an extra copy`,
        reason: `The extra copy is taking ${size} and never plays. Removing it gets the space back.`,
      });
    }
    return out;
  });

export { problemEntries };
