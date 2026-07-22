import type { SettingEntry } from "@loomarr/api";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { TooltipProvider } from "@/components/ui";
import { widthFrame } from "@/test/story-utils";
import { SettingsFields } from "./settings-fields";

const noop = () => {};

// Fields carry FieldHelp (i) tooltips, which need a TooltipProvider ancestor.
const withTooltip: Decorator = (Story) => (
  <TooltipProvider>
    <Story />
  </TooltipProvider>
);

const entry = (over: Partial<SettingEntry> & Pick<SettingEntry, "key" | "kind" | "doc">): SettingEntry => ({
  group: "connections.media_server",
  advanced: false,
  secret: false,
  set: true,
  provenance: "db",
  ...over,
});

const mediaServer: SettingEntry[] = [
  entry({
    key: "library.url",
    kind: "url",
    doc: "Base URL of your Emby/Jellyfin server.",
    value: "http://emby:8096",
  }),
  entry({
    key: "library.flavor",
    kind: "enum",
    enum: ["emby", "jellyfin"],
    doc: "Which flavor to speak.",
    value: "emby",
  }),
  entry({
    key: "library.token",
    kind: "secret",
    secret: true,
    preview: "…9f3c",
    doc: "Admin API token.",
    value: "",
  }),
  entry({
    key: "season.precision",
    kind: "enum",
    enum: ["series", "season"],
    doc: "Availability granularity.",
    value: "series",
    advanced: true,
  }),
];

// A group's fields, controlled by whoever owns the save (config-design §5/§6): the wizard
// saves one group per step, a Settings page saves several blocks from one bar.
const meta = {
  title: "Loomarr/SettingsFields",
  component: SettingsFields,
  args: { entries: mediaServer, values: {}, onChange: noop },
  decorators: [withTooltip, widthFrame(460)],
} satisfies Meta<typeof SettingsFields>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

const EnvPinned: Story = {
  args: {
    entries: mediaServer.map((e) => (e.key === "library.url" ? { ...e, provenance: "env" } : e)),
  },
};

const WithAudit: Story = {
  args: {
    entries: mediaServer.map((e) =>
      e.key === "library.url" ? { ...e, updatedBy: "matt", updatedAt: "2026-07-19T06:00:00Z" } : e,
    ),
  },
};

const Invalid: Story = {
  args: {
    values: { "library.url": "emby:8096" },
    results: [{ key: "library.url", status: "invalid", problem: "must include a scheme, e.g. http://" }],
  },
};

export default meta;
export { Default, EnvPinned, Invalid, WithAudit };
