import type { ReactNode } from "react";
import type {
  LibraryDetails,
  LibraryHealth,
  LibraryHeldRemoval,
  LibraryScan,
} from "../library-settings/library-settings.type";

type LibraryKind = "movie" | "series";

/** The tabs: All, Movies or Shows filter the grid; Collections shows the collections instead. */
type LibraryFilter = "all" | LibraryKind | "collections";

/** The wall's order: by name (A–Z), or the titles added last first (Recently added). */
type LibrarySort = "name" | "recent";

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

/** How many titles each tab holds, across the whole library rather than the search. */
interface LibraryShelfCounts {
  all: number;
  movie: number;
  series: number;
}

/** What the one line under the heading says about the library. */
interface LibraryShelfStatus {
  health: LibraryHealth;
  scan: LibraryScan;
  held: readonly LibraryHeldRemoval[];
  /** Folders in use that didn't open at their last check. */
  offline: readonly string[];
  /** Titles waiting in Needs you: the line says "N need you · Review". */
  needsYou: number;
  /** How the titles' details are getting on; absent reads as nothing waiting. */
  details?: LibraryDetails;
}

/** What only an admin sees and does on the wall: the status line, Needs you and Library settings. */
interface LibraryShelfAdmin {
  status: LibraryShelfStatus;
  /** A check of every folder is running. */
  checking: boolean;
  /** Checks every folder again: an offline folder may be back. */
  onCheckAgain: () => void;
  /** Scans every folder again: a held removal is looked at afresh. */
  onRescan: () => void;
  onOpenSettings: () => void;
  /** Review, on the status line: opens Needs you's own page. Absent on that page. */
  onOpenNeeds?: () => void;
}

interface LibraryShelfProps {
  /** Absent for a member, who browses read-only: no status line, no Needs you, no settings. */
  admin?: LibraryShelfAdmin;
  filter: LibraryFilter;
  onFilterChange: (filter: LibraryFilter) => void;
  /** The tabs' counts; absent until they arrive, when the tabs show none. */
  counts?: LibraryShelfCounts;
  sort: LibrarySort;
  onSortChange: (sort: LibrarySort) => void;
  search: string;
  onSearchChange: (search: string) => void;
  titles: readonly LibraryShelfTitle[];
  /** The first page hasn't arrived: the grid draws placeholders. */
  loading: boolean;
  onOpenTitle: (id: string) => void;
  /** The grid's last rows are on screen: load the next page, if there is one. */
  onEndReached?: () => void;
  /** Needs you's own screen draws its cards here, in place of the grid. */
  needs?: ReactNode;
  /** False on Needs you's own screen: the tabs stay, the search and the grid go. Default true. */
  showTitles?: boolean;
  /** The Collections tab's content, in place of the search and the grid while it's chosen. */
  collections?: ReactNode;
  /** The household's timezone, which times are told in; the device's while undefined. */
  timeZone?: string;
}

export type {
  LibraryFilter,
  LibraryKind,
  LibraryShelfAdmin,
  LibraryShelfCounts,
  LibraryShelfProps,
  LibraryShelfStatus,
  LibraryShelfTitle,
  LibrarySort,
};
