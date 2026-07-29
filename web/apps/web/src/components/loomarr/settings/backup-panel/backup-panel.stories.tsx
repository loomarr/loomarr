import type { BackupList } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { BackupPanel } from "./backup-panel";

const noop = () => {};

// Fixed timestamps rather than Date.now(): the "3d ago" column would otherwise change
// every day and churn the visual baseline for no reason.
const DAY = 86_400;
const NOW = 1_800_000_000;

const base: BackupList = {
  supported: true,
  dir: "/data/backups",
  schedule: "0 30 3 * * *",
  retain: 7,
  backups: [
    { name: "loomarr-2026-07-29-033000.db", bytes: 4_404_019, writtenAt: NOW },
    { name: "loomarr-2026-07-28-033000.db", bytes: 4_298_342, writtenAt: NOW - DAY },
    { name: "loomarr-2026-07-27-033000.db", bytes: 4_194_304, writtenAt: NOW - 2 * DAY },
  ],
};

const meta = {
  title: "Settings/BackupPanel",
  component: BackupPanel,
  args: { list: base, onBackUpNow: noop, onDownload: noop },
  decorators: [widthFrame(760)],
} satisfies Meta<typeof BackupPanel>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

// A fresh install has written none yet. The empty state points at both ways to get one,
// rather than looking like the feature failed to load.
const Empty: Story = {
  args: { list: { ...base, backups: [] } },
};

const Working: Story = {
  args: { pending: true },
};

// A write that failed says so where it happened — an unwritable backup directory is the
// common cause and the message names it.
const Failed: Story = {
  args: {
    error: "The backup could not be written. Check that the backup directory is writable.",
  },
};

// ⚠ Postgres does not get an empty table. In-app backup is SQLite-only by design, and an
// empty list would read as breakage on the one install that is correctly using pg_dump.
const Postgres: Story = {
  args: { list: { ...base, supported: false, backups: [] } },
};

// retain=0 keeps everything, so the footer must not say "keeps 0 backups".
const KeepsEverything: Story = {
  args: { list: { ...base, retain: 0 } },
};

export default meta;
export { Default, Empty, Failed, KeepsEverything, Postgres, Working };
