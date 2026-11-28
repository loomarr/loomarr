import type { ReactNode } from "react";

/** A poster in a collection's pile: its artwork, or its stand-in. */
interface LibraryCollectionPoster {
  /** The title's id, which picks the stand-in's colour; absent picks it from the name. */
  id?: string;
  name: string;
  /** Drawn by the app; absent draws the stand-in. */
  poster?: ReactNode;
}

/** A channel that uses a collection. */
interface LibraryCollectionChannel {
  name: string;
  number: number;
}

/** One of the household's own collections, as its card shows it. */
interface LibraryCollectionCard {
  id: string;
  name: string;
  /** How many of its titles are in the library. */
  titles: number;
  /** Its first titles, front first: the card piles up to three. */
  pile: readonly LibraryCollectionPoster[];
  /** The name of the media server it was brought over from, as said ("Emby"); absent when made in Loomarr. */
  importedFrom?: string;
  usedBy: readonly LibraryCollectionChannel[];
}

/** A ready-made set (a TMDB collection) the library has a movie of. */
interface LibrarySetCard {
  id: string;
  name: string;
  /** How many of its movies the library has, of all of them. */
  have: number;
  total: number;
  pile: readonly LibraryCollectionPoster[];
}

/** What an admin does with the collections. */
interface LibraryCollectionsManage {
  onNew: () => void;
  onEdit: (id: string) => void;
  /** Make my own copy. Resolves once copied; a rejection says it couldn't. */
  onCopy: (id: string) => Promise<void>;
}

interface LibraryCollectionsProps {
  collections: readonly LibraryCollectionCard[];
  sets: readonly LibrarySetCard[];
  /** Absent for a member, who reads the collections only. */
  manage?: LibraryCollectionsManage;
}

export type {
  LibraryCollectionCard,
  LibraryCollectionChannel,
  LibraryCollectionPoster,
  LibraryCollectionsProps,
  LibrarySetCard,
};
