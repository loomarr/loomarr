import type { SettingEntry } from "@loomarr/api";
import { SettingEntryApply } from "@loomarr/api/models/settingEntryApply";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { TooltipProvider } from "@/components/ui";
import { widthFrame } from "@/test/story-utils";
import { AllSettingsTable } from "./all-settings-table";

const noop = () => {};

// Rows render a SettingField, whose doc is a FieldHelp (i) tooltip — same TooltipProvider
// requirement as the field's own stories.
const withTooltip: Decorator = (Story) => (
  <TooltipProvider>
    <Story />
  </TooltipProvider>
);

const entry = (over: Partial<SettingEntry> & Pick<SettingEntry, "key">): SettingEntry => ({
  group: "system.jobs",
  kind: "int",
  doc: "",
  advanced: false,
  apply: SettingEntryApply.live,
  secret: false,
  set: true,
  provenance: "db",
  ...over,
});

// One row per state that changes how the row READS, which is what this table is for: the
// three provenance chips, the ADV marker, and a stored secret (whose value must stay masked
// even here — the escape hatch is not an exemption from §4).
const entries: SettingEntry[] = [
  entry({
    key: "library.url",
    group: "connections.media_server",
    kind: "url",
    value: "http://emby.local:8096",
  }),
  entry({ key: "job.workers", value: "2", provenance: "env" }),
  entry({
    key: "job.reconcile_interval",
    kind: "duration",
    value: "15m",
    provenance: "default",
    advanced: true,
  }),
  entry({
    key: "tmdb.api_key",
    group: "connections.tmdb",
    kind: "secret",
    secret: true,
    preview: "••••ab12",
  }),
];

const meta = {
  title: "Settings/AllSettingsTable",
  component: AllSettingsTable,
  args: { entries, query: "", onQueryChange: noop, values: {}, onEdit: noop },
  decorators: [withTooltip, widthFrame(880)],
} satisfies Meta<typeof AllSettingsTable>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

// Searching by GROUP, not key — the case the gate calls out and the one a key-only search
// would miss. "jobs" matches neither `library.url` nor `tmdb.api_key` by name.
const Filtered: Story = {
  args: { query: "jobs" },
};

// The empty state exists so a zero-match query cannot be mistaken for a page that failed to
// load — the two look identical without it.
const NoMatch: Story = {
  args: { query: "zzz" },
};

export default meta;
export { Default, Filtered, NoMatch };
