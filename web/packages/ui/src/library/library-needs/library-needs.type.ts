import type { ReactNode } from "react";
import type { LibraryKind } from "../library-shelf/library-shelf.type";

/** A title by one catalog's id; the app passes it back as it came. */
interface LibraryExternalId {
  namespace: string;
  value: string;
}

/** A title a search found for one that needs the household, and which of its facts agree. */
interface LibraryNeedCandidate {
  externalId: LibraryExternalId;
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
  /** Series only: every season and episode on disk fits its seasons. */
  episodesMatches: boolean;
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

/** A title that needs a look (decision 24): files that can't be played, or extra copies. */
interface LibraryProblem {
  /** The library title's id: See why opens its page. */
  id: string;
  kind: LibraryKind;
  name: string;
  /** Its artwork, drawn by the app; absent draws the name on the placeholder. */
  poster?: ReactNode;
  /** Its files that can't be played. */
  unplayable: number;
  /** What its extra copies take, in bytes; 0 for none. */
  extraBytes: number;
}

/** What a search for a title found: guesses without their posters. */
type LibraryNeedSearch = (query: string, kind: LibraryKind) => Promise<readonly LibraryNeedCandidate[]>;

interface LibraryNeedsAnswers {
  /** Says the title is externalId. Resolves once saved; a rejection puts the question back. */
  onLink: (id: string, externalId: LibraryExternalId) => Promise<void>;
  /** Says it isn't a title. Resolves once saved; a rejection puts the question back. */
  onIgnore: (id: string) => Promise<void>;
}

interface LibraryNeedsStripProps extends Pick<LibraryNeedsAnswers, "onLink"> {
  needs: readonly LibraryNeed[];
  problems: readonly LibraryProblem[];
  /** Opens Needs you, where a different title can be picked. */
  onOpenNeeds: () => void;
  /** See why: opens a title's page. */
  onOpenTitle: (id: string) => void;
}

interface LibraryNeedsCardsProps extends LibraryNeedsAnswers {
  needs: readonly LibraryNeed[];
  problems: readonly LibraryProblem[];
  onSearch: LibraryNeedSearch;
  /** See why: opens a title's page. */
  onOpenTitle: (id: string) => void;
}

export type {
  LibraryExternalId,
  LibraryNeed,
  LibraryNeedCandidate,
  LibraryNeedSearch,
  LibraryNeedsAnswers,
  LibraryNeedsCardsProps,
  LibraryNeedsStripProps,
  LibraryProblem,
};
