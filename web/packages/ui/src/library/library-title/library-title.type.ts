import type { ReactNode } from "react";
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
}

interface LibraryTitleProps {
  title: LibraryTitleDetail;
  onBack: () => void;
}

export type {
  LibraryTitleDetail,
  LibraryTitleEpisode,
  LibraryTitleFile,
  LibraryTitleProps,
  LibraryTitleSeason,
};
