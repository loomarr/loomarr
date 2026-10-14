import type { DatabaseStatus } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { DatabaseMigration } from "./database-migration";

const noop = () => {};

const base: DatabaseStatus = {
  backend: "sqlite",
  canMigrate: true,
  phase: "idle",
  tables: [],
  parity: "unknown",
};

// One story per state that changes what the operator can DO — which, for a stepper, is the
// only useful axis.
const meta = {
  title: "Settings/DatabaseMigration",
  component: DatabaseMigration,
  args: {
    status: base,
    step: null,
    onStepChange: noop,
    dsn: "",
    onDsnChange: noop,
    checks: [],
    preflightPassed: false,
    onPreflight: noop,
    onBackup: noop,
    onMigrate: noop,
  },
  decorators: [widthFrame(760)],
} satisfies Meta<typeof DatabaseMigration>;

type Story = StoryObj<typeof meta>;

const Idle: Story = {};

// A failing check with its detail visible — "preflight failed" alone would send the
// operator to debug the wrong thing.
const PreflightFailed: Story = {
  args: {
    step: "preflight",
    checks: [
      { name: "Reachable", detail: "connected in 24ms", ok: true },
      { name: "Version", detail: "PostgreSQL 16.2 — needs 13 or newer", ok: true },
      {
        name: "Target is empty",
        detail: "10 table(s) already present (channels, clips, jobs, +7 more)",
        ok: false,
      },
    ],
    preflightPassed: false,
  },
};

// The gate: Migrate is disabled because no backup exists yet.
const BackupRequired: Story = {
  args: { step: "backup" },
};

const Reconnecting: Story = {
  args: {
    step: "reconnect",
    status: { ...base, phase: "migrating" },
  },
};

// The failure that matters most: what was NOT lost.
const Failed: Story = {
  args: {
    step: null,
    status: {
      ...base,
      phase: "failed",
      error:
        "Row counts did not match. Loomarr restarted on SQLite; clear the PostgreSQL target before retrying.",
    },
    error:
      "Row counts did not match. Loomarr restarted on SQLite; clear the PostgreSQL target before retrying.",
  },
};

// An env pin wins at boot, so the stepper collapses to an explanation.
const EnvPinned: Story = {
  args: { envPinned: true },
};

// Already migrated: an answered question, not an absent feature.
const AlreadyPostgres: Story = {
  args: { status: { ...base, backend: "postgres", canMigrate: false } },
};

export default meta;
export { AlreadyPostgres, BackupRequired, EnvPinned, Failed, Idle, PreflightFailed, Reconnecting };
