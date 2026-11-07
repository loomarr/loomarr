import type { ReactNode } from "react";
import type {
  LibraryHealth,
  LibraryHeldRemoval,
  LibraryScan,
} from "../library-settings/library-settings.type";

type LibraryKind = "movie" | "series";

/** The grid's filter: All, Movies or Shows. */
type LibraryFilter = "all" | LibraryKind;

/** One title on the shelf. */
interface LibraryShelfTitle {
  id: string;
  kind: LibraryKind;
  name: string;
  /** Absent when unknown. */
  year?: number;
  /** False while some of its files' details aren't read yet: it shows as Getting ready. */
  ready: boolean;
  /** Its artwork, drawn by the app; absent draws the name on the placeholder. */
  poster?: ReactNode;
  /** It needs a look: a file can't be played, or an extra copy takes space. */
  attention?: boolean;
}

/** What the one line under the heading says about the library. */
interface LibraryShelfStatus {
  health: LibraryHealth;
  scan: LibraryScan;
  held: readonly LibraryHeldRemoval[];
  /** Folders in use that didn't open at their last check. */
  offline: readonly string[];
}

interface LibraryShelfProps {
  status: LibraryShelfStatus;
  /** A check of every folder is running. */
  checking: boolean;
  /** Checks every folder again: an offline folder may be back. */
  onCheckAgain: () => void;
  /** Scans every folder again: a held removal is looked at afresh. */
  onRescan: () => void;
  onOpenSettings: () => void;
  filter: LibraryFilter;
  onFilterChange: (filter: LibraryFilter) => void;
  search: string;
  onSearchChange: (search: string) => void;
  titles: readonly LibraryShelfTitle[];
  /** The first page hasn't arrived: the grid draws nothing yet. */
  loading: boolean;
  onOpenTitle: (id: string) => void;
  /** Needs you, under the status line: the strip, or on its own screen the cards. */
  needs?: ReactNode;
  /** False on Needs you's own screen: the tabs stay, the search and the grid go. Default true. */
  showTitles?: boolean;
}

export type { LibraryFilter, LibraryKind, LibraryShelfProps, LibraryShelfStatus, LibraryShelfTitle };
