import type { DatabaseCheck, DatabaseStatus } from "@loomarr/api";

// The browser owns only the decisions before the process-level operation and the
// reconnect wait after it. Copy, verification, switchover and restart are one atomic
// backend operation, not client-controlled stages.
type MigrationStep = "connect" | "preflight" | "backup" | "reconnect";

interface DatabaseMigrationProps {
  status: DatabaseStatus;
  /** Current stage, or null while idle. Owned by the page so a re-render cannot reset it. */
  step: MigrationStep | null;
  onStepChange: (step: MigrationStep | null) => void;

  /** Target connection fields. Controlled so the page can persist them across stages. */
  dsn: string;
  onDsnChange: (dsn: string) => void;

  checks: DatabaseCheck[];
  preflightPassed: boolean;

  onPreflight: () => void;
  onBackup: () => void;
  onMigrate: () => void;

  /** Which action is in flight, so exactly one control shows a pending state. */
  pending?: "preflight" | "backup" | "migrate" | null;
  /** A failure from the last action, rendered in the stage that produced it. */
  error?: string | null;

  /**
   * True when DATABASE_URL is pinned by the environment. Atomic migration is unavailable:
   * a copy-only operation would leave two databases able to diverge.
   */
  envPinned?: boolean;
  className?: string;
}

export type { DatabaseMigrationProps, MigrationStep };
