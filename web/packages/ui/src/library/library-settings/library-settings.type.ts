import type { ReactNode } from "react";

/** An app that names library folders: the media server, Sonarr or Radarr. */
type LibraryAuthority = "emby" | "jellyfin" | "sonarr" | "radarr";

/**
 * Whether any folder is in use, whether every folder in use opened at its last check, and whether
 * one of them held nothing (folder_empty: usually a drive or share that isn't connected).
 */
type LibraryHealth = "needs_folders" | "folder_offline" | "folder_empty" | "ok";

/** One media folder on this machine. */
interface LibraryFolder {
  path: string;
  /** The maintainer chose it. */
  inUse: boolean;
  /** Loomarr opened it at the last check. */
  readable: boolean;
  /** Loomarr opened it and found nothing in it at the last check. */
  empty?: boolean;
  /** The last check, ISO; absent when never checked. */
  checkedAt?: string;
  addedByHand: boolean;
  /** Every app that names this folder, and what it calls it. */
  foundIn: readonly { authority: LibraryAuthority; path: string }[];
}

/** One row of the path map: an app's folder and the folder here it maps to. */
interface LibraryPathMapping {
  authority: LibraryAuthority;
  authorityPath: string;
  /** "" when no folder here was found for it. */
  localPath: string;
  /** Set by the maintainer; looking for folders again leaves it alone. */
  byHand: boolean;
}

/** What the scanner is doing now: the earliest of an import's three steps still running. */
interface LibraryScan {
  /** False when reading media folders is turned off. */
  enabled: boolean;
  /** walking looks through the folders; reading reads each new file; lookingUp looks up details. */
  step: "idle" | "walking" | "reading" | "lookingUp";
  /** The step's number, 1 to 3; 0 when idle. */
  stepNumber: number;
  done: number;
  /** 0 while the step is still counting. */
  total: number;
  /** The folder, file or title the step is on; absent when unknown. */
  current?: string;
  /** Seconds the step has left; absent until it can be told. */
  etaSeconds?: number;
  /** The step waits while someone is watching. */
  paused: boolean;
  /** An hourly pass with nothing to add: one small line, not the progress block. */
  quiet: boolean;
}

/** How the titles' details are getting on. */
interface LibraryDetails {
  /** Movies and series whose details haven't been looked up yet. */
  waiting: number;
  /** TMDB didn't answer the last lookup. */
  unreachable: boolean;
  /** When looking up is tried again, ISO; absent when nothing waits on a retry. */
  retryAt?: string;
}

/** A title that became ready while the import runs (decision 29). */
interface LibraryReadyTitle {
  id: string;
  name: string;
  /** Its poster; absent draws a plain tile. */
  poster?: ReactNode;
}

/** Removals a scan found in one folder and is holding for the maintainer. */
interface LibraryHeldRemoval {
  path: string;
  movies: number;
  episodes: number;
  other: number;
}

/** Whether an arr tells Loomarr about new downloads. */
interface LibraryHook {
  app: "sonarr" | "radarr";
  state: "registered" | "not_configured" | "no_address" | "failed";
}

interface LibrarySettingsProps {
  health: LibraryHealth;
  folders: readonly LibraryFolder[];
  pathMap: readonly LibraryPathMapping[];
  scan: LibraryScan;
  /** The latest few titles that became ready, last first, shown while the import runs. */
  readySoFar?: readonly LibraryReadyTitle[];
  held: readonly LibraryHeldRemoval[];
  hooks: readonly LibraryHook[];
  /** How the titles' details are getting on; absent reads as nothing waiting. */
  details?: LibraryDetails;
  /** A check of every folder is running. */
  checking: boolean;
  onCheckAgain: () => void;
  /** Scans every folder again ("Scan now"); a held removal is looked at afresh. */
  onRescan: () => void;
  /** "Refresh everything": every title's details are read and fetched again. Asked first. */
  onRefreshEverything: () => void;
  /** The maintainer says the held files are gone. */
  onConfirmRemoval: (path: string) => void;
  onUseFolder: (path: string, inUse: boolean) => void;
  /** Resolves to a sentence saying why the folder was refused, or undefined when it was added. */
  onAddFolder: (path: string) => Promise<string | undefined>;
  /** An empty localPath hands the row back to Loomarr. */
  onMapFolder: (mapping: { authority: LibraryAuthority; authorityPath: string; localPath: string }) => void;
  /** Injected for stories and tests; defaults to the current time. */
  now?: number;
}

/** The setup wizard's Library step: the folders, each with Use this folder, and Check again. */
type LibraryWizardStepProps = Pick<
  LibrarySettingsProps,
  | "folders"
  | "pathMap"
  | "scan"
  | "readySoFar"
  | "checking"
  | "onCheckAgain"
  | "onUseFolder"
  | "onAddFolder"
  | "now"
> & {
  /** A Check again finished a moment ago. */
  checkedJustNow?: boolean;
};

export type {
  LibraryAuthority,
  LibraryDetails,
  LibraryFolder,
  LibraryHealth,
  LibraryHeldRemoval,
  LibraryHook,
  LibraryPathMapping,
  LibraryReadyTitle,
  LibraryScan,
  LibrarySettingsProps,
  LibraryWizardStepProps,
};
