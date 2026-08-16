import type { BackupList } from "@loomarr/api/models/backupList";

interface BackupPanelProps {
  /** The server's whole view: the files on disk plus the policy in force. */
  list: BackupList;
  /** Write a backup now — the same work the scheduled job does. */
  onBackUpNow: () => void;
  /**
   * Download one backup by filename. The panel never builds the URL itself: the name is
   * a server-validated identifier, and the page owns how a download is started.
   */
  onDownload: (name: string) => void;
  /** True while "Back up now" is in flight, so the button reads as busy. */
  pending?: boolean;
  /** A failure from the last action, rendered where it happened. */
  error?: string | null;
  className?: string;
}

export type { BackupPanelProps };
