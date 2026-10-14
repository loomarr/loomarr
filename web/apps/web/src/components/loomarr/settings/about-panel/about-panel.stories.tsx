import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { AboutPanel } from "./about-panel";

// Fixed instants, and a fixed `now` — an uptime read off the real clock would churn the
// visual baseline on every run.
const NOW = Date.parse("2026-07-29T12:00:00Z");
const STARTED = "2026-07-23T07:48:00Z"; // 6d 4h 12m before NOW, matching the mock

const meta = {
  title: "Settings/AboutPanel",
  component: AboutPanel,
  args: {
    version: {
      version: "v0.9.3",
      commit: "9871fef2ac10",
      builtAt: "2026-07-22T18:04:00Z",
      goVersion: "go1.23.4",
      platform: "linux/amd64",
      startedAt: STARTED,
      backend: "sqlite",
      schemaVersion: 20,
      ready: true,
    },
    now: NOW,
  },
  decorators: [widthFrame(760)],
} satisfies Meta<typeof AboutPanel>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

// A source build carries no linker stamps and a store-less boot no schema: version falls
// back to "dev" and the optional rows are simply absent rather than rendering as blanks.
const SourceBuild: Story = {
  args: { version: { version: "dev", ready: true } },
};

// Built from a working tree with uncommitted changes — worth surfacing, since "v0.9.3"
// and "v0.9.3 plus local edits" are different things when diagnosing a problem.
const Dirty: Story = {
  args: {
    version: {
      version: "v0.9.3",
      commit: "9871fef2ac10",
      builtAt: "2026-07-22T18:04:00Z",
      goVersion: "go1.23.4",
      platform: "linux/amd64",
      startedAt: STARTED,
      backend: "sqlite",
      schemaVersion: 20,
      dirty: true,
      ready: true,
    },
  },
};

// Not ready reports WHY, since that detail is the whole reason to look at this page.
const NotReady: Story = {
  args: {
    version: {
      version: "v0.9.3",
      goVersion: "go1.23.4",
      platform: "linux/amd64",
      startedAt: STARTED,
      backend: "postgres",
      schemaVersion: 20,
      ready: false,
      detail: "media server unreachable at http://emby.lan:8096",
    },
  },
};

// The state an operator sees right after a restart — "just started", never "0m", which
// reads like a broken value.
const JustRestarted: Story = {
  args: {
    version: {
      version: "v0.9.3",
      goVersion: "go1.23.4",
      platform: "linux/amd64",
      startedAt: "2026-07-29T11:59:40Z",
      backend: "sqlite",
      schemaVersion: 20,
      ready: true,
    },
  },
};

export default meta;
export { Default, Dirty, JustRestarted, NotReady, SourceBuild };
