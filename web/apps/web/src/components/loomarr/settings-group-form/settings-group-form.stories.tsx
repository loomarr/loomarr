import type { SettingEntry } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { SettingsGroupForm } from "./settings-group-form";

const noop = () => {};

const entry = (over: Partial<SettingEntry> & Pick<SettingEntry, "key" | "kind" | "doc">): SettingEntry => ({
  group: "connections.media_server",
  advanced: false,
  secret: false,
  set: true,
  provenance: "db",
  ...over,
});

// Entries seed the form — a setting's resolved `value` is its starting value. A stored
// secret's value is empty by contract (§4 never echoes it); its masked preview is shown.
const mediaServer: SettingEntry[] = [
  entry({
    key: "library.url",
    kind: "url",
    doc: "Base URL of your Emby/Jellyfin server.",
    value: "http://emby.local:8096",
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

// The one settings form (config-design §6): the wizard renders a group per step and
// Settings a group per page. Idle · testing · a failed check that never blames.
const meta = {
  title: "Loomarr/SettingsGroupForm",
  component: SettingsGroupForm,
  args: { entries: mediaServer, onSave: noop, onTest: noop },
  decorators: [widthFrame(460)],
} satisfies Meta<typeof SettingsGroupForm>;

type Story = StoryObj<typeof meta>;

const Idle: Story = {};
const Testing: Story = { args: { testing: true } };
const TestPassed: Story = { args: { testOk: true } };
const TestFailed: Story = {
  args: { testOk: false, testHint: "Emby refused the token — check it's an admin API key." },
};
const Saving: Story = { args: { saving: true } };
const Invalid: Story = {
  args: {
    entries: mediaServer.map((e) => (e.key === "library.url" ? { ...e, value: "emby.local" } : e)),
    results: [{ key: "library.url", status: "invalid", problem: "must include a scheme, e.g. http://" }],
  },
};

export default meta;
export { Idle, Invalid, Saving, TestFailed, Testing, TestPassed };
