import type { ReactNode } from "react";
import type { LibraryKind } from "../library-shelf/library-shelf.type";

/** A title a search found for one that needs the household, and which of its facts agree. */
interface LibraryNeedCandidate {
  tmdbId: string;
  kind: LibraryKind;
  name: string;
  /** Absent when unknown. */
  year?: number;
  overview?: string;
  /** Its artwork, drawn by the app; absent draws the name on the placeholder. */
  poster?: ReactNode;
  nameMatches: boolean;
  yearMatches: boolean;
  /** Movies only: its runtime and the file's length agree. */
  lengthMatches: boolean;
}

/** A title Loomarr can't tell what it is. */
interface LibraryNeed {
  /** The library title's id: what an answer is for. */
  id: string;
  kind: LibraryKind;
  /** Its files on this machine (a series' episodes), in path order. */
  files: readonly string[];
  /** The one guess good enough to ask about: its name agrees. Absent asks the household to find it. */
  best?: LibraryNeedCandidate;
  /** The other guesses, best first, shown before anything is typed into the search. */
  others: readonly LibraryNeedCandidate[];
}

/** What a search for a title found: guesses without their posters. */
type LibraryNeedSearch = (query: string, kind: LibraryKind) => Promise<readonly LibraryNeedCandidate[]>;

interface LibraryNeedsAnswers {
  /** Says the title is tmdbId. Resolves once saved; a rejection puts the question back. */
  onLink: (id: string, tmdbId: string) => Promise<void>;
  /** Says it isn't a title. Resolves once saved; a rejection puts the question back. */
  onIgnore: (id: string) => Promise<void>;
}

interface LibraryNeedsStripProps extends Pick<LibraryNeedsAnswers, "onLink"> {
  needs: readonly LibraryNeed[];
  /** Opens Needs you, where a different title can be picked. */
  onOpenNeeds: () => void;
}

interface LibraryNeedsCardsProps extends LibraryNeedsAnswers {
  needs: readonly LibraryNeed[];
  onSearch: LibraryNeedSearch;
}

export type {
  LibraryNeed,
  LibraryNeedCandidate,
  LibraryNeedSearch,
  LibraryNeedsAnswers,
  LibraryNeedsCardsProps,
  LibraryNeedsStripProps,
};
