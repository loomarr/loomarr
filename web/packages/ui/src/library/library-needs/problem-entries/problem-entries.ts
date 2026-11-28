import { formatBytes } from "@loomarr/core/format";

import type { LibraryProblem } from "../library-needs.type";
import type { ProblemEntry, ProblemReason } from "./problem-entries.type";

const reasonsFor = (p: LibraryProblem): ProblemReason[] => {
  const out: ProblemReason[] = [];
  if (p.unplayable > 0) {
    const movie = p.kind === "movie";
    const one = movie || p.unplayable === 1;
    const episodes = p.unplayable === 1 ? "1 episode" : `${p.unplayable} episodes`;
    out.push({
      key: "unplayable",
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
  if (p.extraCopies > 0) {
    const one = p.extraCopies === 1;
    const size = formatBytes(p.extraBytes);
    out.push({
      key: "extra",
      headline: `${p.name} has ${one ? "an extra copy" : `${p.extraCopies} extra copies`}`,
      reason: one
        ? `The extra copy is taking ${size} and never plays. Removing it gets the space back.`
        : `${p.extraCopies} extra copies are taking ${size} and never play. Removing them gets the space back.`,
    });
  }
  return out;
};

// One entry per title, in the order given; a title with nothing wrong has none.
const problemEntries = (problems: readonly LibraryProblem[]): ProblemEntry[] =>
  problems.flatMap((p) => {
    const reasons = reasonsFor(p);
    return reasons.length > 0 ? [{ id: p.id, name: p.name, poster: p.poster, reasons }] : [];
  });

export { problemEntries };
