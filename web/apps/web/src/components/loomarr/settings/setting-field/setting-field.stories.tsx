import type { SettingEntry } from "@loomarr/api";
import { SettingEntryApply } from "@loomarr/api/models/settingEntryApply";
import { SettingKey } from "@loomarr/api/models/settingKey";
import { SettingKind } from "@loomarr/api/models/settingKind";
import { SettingPresentation } from "@loomarr/api/models/settingPresentation";
import { SettingUnit } from "@loomarr/api/models/settingUnit";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { SettingField } from "./setting-field";

const noop = () => {};

const base: SettingEntry = {
  key: "library.url",
  group: "connections.media_server",
  owner: "settings.connections",
  kind: "url",
  doc: "Base URL of your Emby/Jellyfin server.",
  advanced: false,
  apply: SettingEntryApply.live,
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
  decorators: [widthFrame(420)],
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
    entry: {
      ...base,
      key: "filler.transcribe.enabled",
      kind: "bool",
      doc: "Transcribe clips whose source details are sparse.",
    },
    value: "true",
  },
};

// A duration offered through the registry's presets, with Custom after them. The value matches
// a preset in the canonical form the server publishes, so the picker reads "Every 6 hours".
const DownloadSchedule: Story = {
  args: {
    entry: {
      ...base,
      key: SettingKey.FillerFetchEvery,
      group: "filler",
      owner: "filler.downloads",
      kind: SettingKind.duration,
      presentation: SettingPresentation.filler_download_schedule,
      label: "Look for new clips",
      doc: "How often Loomarr checks enabled sources for new clips.",
      presets: [
        { value: "0s", label: "Never" },
        { value: "6h0m0s", label: "Every 6 hours" },
        { value: "12h0m0s", label: "Every 12 hours" },
        { value: "24h0m0s", label: "Daily" },
        { value: "168h0m0s", label: "Weekly" },
      ],
    },
    value: "6h0m0s",
  },
};

// A whole number of seconds edited in a unit: 14400 reads as 4 hours and still saves as seconds.
const SecondsWithUnit: Story = {
  args: {
    entry: {
      ...base,
      key: SettingKey.FillerCooldownSeconds,
      group: "filler",
      owner: "filler.playback",
      kind: SettingKind.int,
      unit: SettingUnit.seconds,
      label: "Repeat spacing · same commercial",
      doc: "How long before a commercial airs again.",
    },
    value: "14400",
  },
};

// The server's zones in a native select, the viewer's device first.
const Timezone: Story = {
  args: {
    entry: {
      ...base,
      key: SettingKey.GuideTimezone,
      group: "playout",
      owner: "settings.playback",
      kind: SettingKind.string,
      presentation: SettingPresentation.timezone,
      label: "Timezone",
      doc: "Which timezone the TV guide's times are shown in.",
      presets: [
        { value: "", label: "Each viewer's device" },
        { value: "UTC", label: "UTC" },
        { value: "America/New_York", label: "America/New York" },
        { value: "Europe/London", label: "Europe/London" },
      ],
    },
    value: "America/New_York",
  },
};

const Invalid: Story = {
  args: {
    value: "not-a-url",
    result: { key: "library.url", status: "invalid", problem: "must include a scheme, e.g. http://" },
  },
};

export default meta;
export { Bool, DownloadSchedule, Enum, EnvPinned, Invalid, SecondsWithUnit, StoredSecret, Timezone, Url };
