import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { AboutPanel } from "./about-panel";

const meta = {
  title: "Settings/AboutPanel",
  component: AboutPanel,
  args: {
    version: {
      version: "v0.9.3",
      commit: "9871fef2ac10",
      builtAt: "2026-07-22T18:04:00Z",
      ready: true,
    },
    backend: "sqlite",
  },
  decorators: [widthFrame(760)],
} satisfies Meta<typeof AboutPanel>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

// A source build carries no linker stamps: version falls back to "dev" and the optional
// rows are simply absent rather than rendering as blanks.
const SourceBuild: Story = {
  args: { version: { version: "dev", ready: true }, backend: undefined },
};

// Built from a working tree with uncommitted changes — worth surfacing, since "v0.9.3"
// and "v0.9.3 plus local edits" are different things when diagnosing a problem.
const Dirty: Story = {
  args: {
    version: {
      version: "v0.9.3",
      commit: "9871fef2ac10",
      builtAt: "2026-07-22T18:04:00Z",
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
      ready: false,
      detail: "media server unreachable at http://emby.lan:8096",
    },
  },
};

export default meta;
export { Default, Dirty, NotReady, SourceBuild };
