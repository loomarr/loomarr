import type { ReactNode } from "react";
import type { LibraryExternalId, LibraryNeedSearch } from "../library-needs/library-needs.type";
import type { LibraryKind } from "../library-shelf/library-shelf.type";

/** One of a movie's files, with what its header says once read. */
interface LibraryTitleFile {
  path: string;
  /** Bytes. */
  size: number;
  /** How long it plays, measured from the file. */
  lengthMillis?: number;
  /** Its first video stream. */
  picture?: { width: number; height: number; hdr: boolean };
  /** The channels of the sound that plays. */
  audioChannels?: number;
  /** Each subtitle stream; a language is an ISO 639 code, absent when the file doesn't say. */
  subtitles: readonly { language?: string }[];
  /** A copy that never plays, because another of the title's files does. */
  extra: boolean;
}

/** Where a title's details came from, each as a name to show ("TMDB", "Sonarr"). */
interface LibraryTitleDetailsFrom {
  description?: string;
  rating?: string;
  /** Where most of a series' episode names came from. */
  episodeNames?: string;
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
  /** Its files that can't be played: their details couldn't be read. */
  unplayable: number;
  match: LibraryTitleMatch;
  detailsFrom: LibraryTitleDetailsFrom;
}

/**
 * What decides which title this is, for "Wrong title?" (decision 26): the app that manages it, the
 * file itself (an id in its name, or an NFO), the household's answer, a TMDB search that agreed on
 * its name, year and length (decision 56), or nothing yet.
 */
interface LibraryTitleMatch {
  by: "app" | "file" | "you" | "search" | "none";
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
  onLink: (externalId: LibraryExternalId) => Promise<void>;
  /** Forget my answer: the title goes back to needing the household. */
  onForget: () => Promise<void>;
  /** Remove extra copy: moves the file at path to the trash; rejects when it couldn't. */
  onRemoveExtra: (path: string) => Promise<void>;
}

export type {
  LibraryTitleDetail,
  LibraryTitleEpisode,
  LibraryTitleFile,
  LibraryTitleMatch,
  LibraryTitleProps,
  LibraryTitleSeason,
};
