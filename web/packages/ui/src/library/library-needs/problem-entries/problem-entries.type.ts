import type { ReactNode } from "react";

// A title that needs a look, as Needs you words it (decision 24, the mock's word sheet): the card's
// headline and its one reason. A title with both kinds of problem is two entries; See why opens its
// page.
interface ProblemEntry {
  key: string;
  id: string;
  name: string;
  poster?: ReactNode;
  headline: string;
  reason: string;
}

export type { ProblemEntry };
