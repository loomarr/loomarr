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
  /** Its artwork, drawn by the app; absent draws the stand-in. */
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
  /** Where it sits, from its media folder's name down; absent draws no folder heading. */
  folder?: string;
  /** Its files on this machine (a series' episodes), in path order. */
  files: readonly string[];
  /** The first file's length in milliseconds; 0 until its details are read. */
  lengthMillis: number;
  /** The first file's size in bytes. */
  bytes: number;
  /**
   * A 16:9 frame of its main file, drawn by the app with an empty alt: the file name heads the
   * card. Absent draws a neutral box.
   */
  frame?: ReactNode;
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
  /** Its year, which tints the stand-in; absent reads "Year unknown". */
  year?: number;
  homeVideo?: boolean;
  /** Its artwork, drawn by the app; absent draws the stand-in. */
  poster?: ReactNode;
  /** Its files that can't be played. */
  unplayable: number;
  /** How many extra copies it has; 0 for none. */
  extraCopies: number;
  /** What its extra copies take, in bytes; 0 for none. */
  extraBytes: number;
  /** How many of those extra copies can't be played; absent where that isn't known. */
  brokenCopies?: number;
}

/** What kind of order one of TMDB's other episode orders is. */
type LibraryOrderKind = "aired" | "absolute" | "dvd" | "digital" | "story" | "production" | "tv";

/** How many episodes one season has. */
interface LibrarySeasonSize {
  /** 0 is specials. */
  season: number;
  episodes: number;
}

/** One order a show's files may follow (#2151 decision 53). */
interface LibraryOrderChoice {
  /** `catalog` is TMDB's own seasons; `order` is one of its other orders. */
  choice: "catalog" | "order";
  /** The order to send back; only with `order`. */
  orderId?: string;
  /** Only with `order`. */
  kind?: LibraryOrderKind;
  /** TMDB's name for it; only with `order`. */
  name?: string;
  /** Its layout; empty while it is being fetched. */
  seasons: readonly LibrarySeasonSize[];
  /** How many episodes it has in all. */
  episodes: number;
  /** Every season on disk ends where its season does. */
  fits: boolean;
}

/** A file of an episode that keeps its file's name and number, numbered as the file says. */
interface LibraryOrderFile {
  path: string;
  season: number;
  episode: number;
}

/** A show whose files don't say which of TMDB's orders they follow. */
interface LibraryOrderNeed {
  /** The library show's id: what an answer is for. */
  id: string;
  name: string;
  /** Its year, which tints the stand-in; absent reads "Year unknown". */
  year?: number;
  /** Its artwork, drawn by the app; absent draws the stand-in. */
  poster?: ReactNode;
  /** How far each season runs on disk, by season. */
  disk: readonly LibrarySeasonSize[];
  /** The episodes that wait on the answer, in path order. */
  files: readonly LibraryOrderFile[];
  /** TMDB's own seasons, then its other orders; empty while the show's details are fetched. */
  choices: readonly LibraryOrderChoice[];
}

/** An answer to which order a show's files follow; `files` keeps the names in the files. */
type LibraryOrderAnswer = { choice: "catalog" } | { choice: "order"; orderId: string } | { choice: "files" };

/** What a search for a title found: guesses without their posters. */
type LibraryNeedSearch = (query: string, kind: LibraryKind) => Promise<readonly LibraryNeedCandidate[]>;

interface LibraryNeedsAnswers {
  /** Says the title is externalId. Resolves once saved; a rejection puts the question back. */
  onLink: (id: string, externalId: LibraryExternalId) => Promise<void>;
  /** Says it isn't a title. Resolves once saved; a rejection puts the question back. */
  onIgnore: (id: string) => Promise<void>;
  /** Says which order a show's files follow. Resolves once saved; a rejection puts the question back. */
  onChooseOrder: (id: string, answer: LibraryOrderAnswer) => Promise<void>;
}

interface LibraryNeedsCardsProps extends LibraryNeedsAnswers {
  needs: readonly LibraryNeed[];
  orders: readonly LibraryOrderNeed[];
  problems: readonly LibraryProblem[];
  onSearch: LibraryNeedSearch;
  /** The page around the cards already says "Needs you" and how many wait. */
  hideHeading?: boolean;
  /** How often each title has been brought back; a new count asks about that title again. */
  restored?: Readonly<Record<string, number>>;
  /** Drawn once nothing is left to ask; absent says so in one line. */
  empty?: ReactNode;
  /** Ignores exactly these titles in one save; absent offers no folder answer. */
  onIgnoreFolder?: (folder: string, ids: string[]) => Promise<void>;
  /** Keeps a title with no guess as the household's own video; absent offers no such answer. */
  onKeep?: (id: string) => Promise<void>;
  /** See why: opens a title's page. */
  onOpenTitle: (id: string) => void;
  /** The last card still asking was answered, so there is no next card to focus. */
  onNoneLeft?: () => void;
}

export type {
  LibraryExternalId,
  LibraryNeed,
  LibraryNeedCandidate,
  LibraryNeedSearch,
  LibraryNeedsAnswers,
  LibraryNeedsCardsProps,
  LibraryOrderAnswer,
  LibraryOrderChoice,
  LibraryOrderFile,
  LibraryOrderKind,
  LibraryOrderNeed,
  LibraryProblem,
  LibrarySeasonSize,
};
