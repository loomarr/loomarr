import type { ReactNode } from "react";
import type { LibraryNeedSearch } from "../library-needs/library-needs.type";
import type { LibraryKind } from "../library-shelf/library-shelf.type";

/** One of a movie's files. */
interface LibraryTitleFile {
  path: string;
  /** Bytes. */
  size: number;
}

interface LibraryTitleEpisode {
  id: string;
  name: string;
  number: number;
  /** The last episode a multi-episode file holds. */
  end?: number;
  ready: boolean;
}

interface LibraryTitleSeason {
  /** 0 is the specials. */
  number: number;
  episodes: readonly LibraryTitleEpisode[];
}

/** A movie or a series, as its page shows it. */
interface LibraryTitleDetail {
  kind: LibraryKind;
  name: string;
  year?: number;
  /** Its artwork, drawn by the app; absent draws the name on the placeholder. */
  poster?: ReactNode;
  overview?: string;
  genres: readonly string[];
  /** The certification, such as TV-14. */
  rating?: string;
  runtimeMillis?: number;
  /** A movie's files; empty for a series. */
  files: readonly LibraryTitleFile[];
  /** A series' episodes by season; empty for a movie. */
  seasons: readonly LibraryTitleSeason[];
  match: LibraryTitleMatch;
}

/**
 * What decides which title this is, for "Wrong title?" (decision 26): the app that manages it, the
 * file itself (an id in its name, or an NFO), the household's answer, or nothing yet.
 */
interface LibraryTitleMatch {
  by: "app" | "file" | "you" | "none";
  /** The app's name, such as Sonarr, when by is app. */
  app?: string;
}

interface LibraryTitleProps {
  title: LibraryTitleDetail;
  onBack: () => void;
  /** Update details: settles once the details are fetched; rejects when they couldn't be. */
  onUpdateDetails: () => Promise<void>;
  /** Check files again: settles once the files are queued to be read; rejects when they couldn't be. */
  onCheckFiles: () => Promise<void>;
  /** Opens the title's page in the app that decides; absent when it has none. */
  onOpenApp?: () => void;
  /** Wrong title? when the household's answer, or nothing, decides: searches for the title. */
  onSearch: LibraryNeedSearch;
  /** Says which title it is; rejects when the answer couldn't be saved. */
  onLink: (tmdbId: string) => Promise<void>;
  /** Forget my answer: the title goes back to needing the household. */
  onForget: () => Promise<void>;
}

export type {
  LibraryTitleDetail,
  LibraryTitleEpisode,
  LibraryTitleFile,
  LibraryTitleMatch,
  LibraryTitleProps,
  LibraryTitleSeason,
};
