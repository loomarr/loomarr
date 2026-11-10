import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { ServicesPanel } from "./services-panel";

const loomarr = { name: "loomarr", ok: true, target: "v0.9.3 · darwin/arm64 · sqlite schema 21" };

const meta = {
  title: "Dashboard/ServicesPanel",
  component: ServicesPanel,
  args: {
    view: {
      loomarr,
      rows: [
        { name: "media_server", ok: true, target: "http://emby.lan:8096" },
        { name: "requester", ok: true, target: "http://seerr.lan:5055" },
        { name: "tunarr", ok: true, target: "http://tunarr.lan:8000" },
        { name: "llm", ok: true, target: "http://ollama.lan:11434" },
        { name: "tmdb", ok: true },
      ],
    },
    onFix: () => {},
  },
  decorators: [widthFrame(760)],
} satisfies Meta<typeof ServicesPanel>;

type Story = StoryObj<typeof meta>;

const AllHealthy: Story = {};

// A failing row owes the operator three things: what broke, where it was pointed, and a way
// to go fix it.
const OneFailing: Story = {
  args: {
    view: {
      loomarr,
      rows: [
        { name: "media_server", ok: true, target: "http://emby.lan:8096" },
        {
          name: "requester",
          ok: false,
          target: "http://seerr.lan:5055",
          hint: "connection refused — is Seerr running?",
          settingsGroup: "requester",
        },
        { name: "tunarr", ok: true, target: "http://tunarr.lan:8000" },
      ],
    },
  },
};

// An unconfigured integration has no target. "not configured" beats an empty cell, which
// reads as a rendering bug rather than an answer.
const Unconfigured: Story = {
  args: {
    view: { loomarr, rows: [{ name: "filler", ok: false, notSetUp: true, settingsGroup: "filler" }] },
  },
};

// A fresh install (#3403): the optional services nobody has set up read "Not set up" with their
// plain hint, the same word Connections uses. Only required services would be red.
const FreshInstall: Story = {
  args: {
    view: {
      loomarr,
      rows: [
        { name: "media_server", ok: true, target: "http://emby.lan:8096" },
        {
          name: "requester",
          ok: false,
          hint: "set the Seerr URL",
          notSetUp: true,
          settingsGroup: "requester",
        },
        { name: "tunarr", ok: false, hint: "set the Tunarr URL", notSetUp: true, settingsGroup: "tunarr" },
        {
          name: "llm",
          ok: false,
          hint: "set the Ollama URL, or configure a hosted provider in AI settings",
          notSetUp: true,
          settingsGroup: "ai",
        },
        {
          name: "tunarr_library",
          ok: false,
          hint: "set the Tunarr URL",
          notSetUp: true,
          settingsGroup: "tunarr",
        },
        { name: "tmdb", ok: true },
      ],
    },
  },
};

const Refreshing: Story = {
  args: { refreshing: true },
};

export default meta;
export { AllHealthy, FreshInstall, OneFailing, Refreshing, Unconfigured };
