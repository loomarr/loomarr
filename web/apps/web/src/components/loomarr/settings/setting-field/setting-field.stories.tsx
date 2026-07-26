import type { SettingEntry } from "@loomarr/api";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { TooltipProvider } from "@/components/ui";
import { widthFrame } from "@/test/story-utils";
import { SettingField } from "./setting-field";

const noop = () => {};

// The field's doc is a FieldHelp (i) tooltip now, which needs a TooltipProvider ancestor
// (mounted at the app root; supplied here for isolation).
const withTooltip: Decorator = (Story) => (
  <TooltipProvider>
    <Story />
  </TooltipProvider>
);

const base: SettingEntry = {
  key: "library.url",
  group: "connections.media_server",
  kind: "url",
  doc: "Base URL of your Emby/Jellyfin server.",
  advanced: false,
  secret: false,
  set: true,
  provenance: "db",
};

// One registry key as a control (config-design §2/§3): the kinds that look different,
// plus the two provenance/secret states an operator actually has to reason about.
const meta = {
  title: "Settings/SettingField",
  component: SettingField,
  args: { entry: base, value: "http://emby.local:8096", onChange: noop },
  decorators: [withTooltip, widthFrame(420)],
} satisfies Meta<typeof SettingField>;

type Story = StoryObj<typeof meta>;

const Url: Story = {};

const EnvPinned: Story = {
  args: { entry: { ...base, provenance: "env" } },
};

const StoredSecret: Story = {
  args: {
    entry: {
      ...base,
      key: "seerr.api_key",
      kind: "secret",
      secret: true,
      preview: "…a1b2",
      doc: "Seerr API key.",
    },
    value: "",
  },
};

const Enum: Story = {
  args: {
    entry: {
      ...base,
      key: "library.flavor",
      kind: "enum",
      enum: ["emby", "jellyfin"],
      doc: "Which media server flavor to speak.",
    },
    value: "jellyfin",
  },
};

const Bool: Story = {
  args: {
    entry: { ...base, key: "filler.ai_tagging", kind: "bool", doc: "AI-tag untagged commercials." },
    value: "true",
  },
};

const Invalid: Story = {
  args: {
    value: "not-a-url",
    result: { key: "library.url", status: "invalid", problem: "must include a scheme, e.g. http://" },
  },
};

export default meta;
export { Bool, Enum, EnvPinned, Invalid, StoredSecret, Url };
