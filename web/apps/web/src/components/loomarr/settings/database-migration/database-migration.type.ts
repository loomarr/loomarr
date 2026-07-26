import type { DatabaseCheck, DatabaseStatus } from "@loomarr/api";

// The six stages, in order (§18, V11 — and the v2 mock's `order` array). `idle` is not a
// stage: it is the state before the operator has started, where the stepper is collapsed
// to its opening pitch.
type MigrationStep = "connect" | "preflight" | "backup" | "migrate" | "verify" | "restart";

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
  onSwitchover: () => void;

  /** Which action is in flight, so exactly one control shows a pending state. */
  pending?: "preflight" | "backup" | "migrate" | "switchover" | null;
  /** A failure from the last action, rendered in the stage that produced it. */
  error?: string | null;

  /**
   * True when DATABASE_URL is pinned by the environment. The migration is then a
   * copy-only operation: Loomarr can move the data but cannot record the switch, because
   * an env pin always wins at boot (the server refuses the switchover for the same
   * reason). The mock models this as `dbPinned` and collapses the stepper.
   */
  envPinned?: boolean;
  className?: string;
}

export type { DatabaseMigrationProps, MigrationStep };
