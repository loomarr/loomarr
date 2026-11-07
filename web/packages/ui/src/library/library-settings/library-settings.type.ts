/** An app that names library folders: the media server, Sonarr or Radarr. */
type LibraryAuthority = "emby" | "jellyfin" | "sonarr" | "radarr";

/** Whether any folder is in use, and whether every folder in use opened at its last check. */
type LibraryHealth = "needs_folders" | "folder_offline" | "ok";

/** One media folder on this machine. */
interface LibraryFolder {
  path: string;
  /** The maintainer chose it. */
  inUse: boolean;
  /** Loomarr opened it at the last check. */
  readable: boolean;
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

interface LibrarySettingsProps {
  health: LibraryHealth;
  folders: readonly LibraryFolder[];
  pathMap: readonly LibraryPathMapping[];
  /** A check of every folder is running. */
  checking: boolean;
  onCheckAgain: () => void;
  onUseFolder: (path: string, inUse: boolean) => void;
  /** Resolves to a sentence saying why the folder was refused, or undefined when it was added. */
  onAddFolder: (path: string) => Promise<string | undefined>;
  /** An empty localPath hands the row back to Loomarr. */
  onMapFolder: (mapping: { authority: LibraryAuthority; authorityPath: string; localPath: string }) => void;
  /** Injected for stories and tests; defaults to the current time. */
  now?: number;
}

export type { LibraryAuthority, LibraryFolder, LibraryHealth, LibraryPathMapping, LibrarySettingsProps };
