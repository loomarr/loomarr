import type { ReactNode } from "react";

import type { LibraryKind } from "../../library-shelf/library-shelf.type";

// One thing wrong with a title (decision 24, the mock's word sheet): a headline and its one reason.
interface ProblemReason {
  key: string;
  headline: string;
  reason: string;
}

// A title that needs a look, with everything wrong with it; See why opens its page.
interface ProblemEntry {
  id: string;
  name: string;
  // What the stand-in cover says and is tinted by.
  year?: number;
  kind?: LibraryKind;
  homeVideo?: boolean;
  poster?: ReactNode;
  reasons: readonly ProblemReason[];
}

export type { ProblemEntry, ProblemReason };
