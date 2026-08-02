import { fillerSources } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { FillerSources } from "./filler-sources";

const noop = () => {};

// ⚠ The rows live in @loomarr/fixtures, not here (frontend-design §5.1b). They are typed
// against the orval-generated DTO, so a contract change breaks the typecheck in ONE place
// rather than at every story that happened to hand-roll the same shape.
const SOURCES = fillerSources;

const meta = {
  title: "Filler/FillerSources",
  component: FillerSources,
  args: { sources: SOURCES, total: 418, onFetch: noop },
  decorators: [widthFrame(760)],
} satisfies Meta<typeof FillerSources>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

// A fresh install: the folder row is present but unconfigured. Hiding it would leave "why is
// my catalog empty" unanswered — the operator would see two sources and assume filler works.
const NothingConfigured: Story = {
  args: {
    sources: SOURCES.map((s) => ({ ...s, count: 0, configured: false, fetchable: false })),
    total: 0,
  },
};

const Fetching: Story = {
  args: { fetching: "folder" },
};

const FetchFailed: Story = {
  args: { error: "Couldn't reach the media server to re-scan." },
};

export default meta;
export { Default, FetchFailed, Fetching, NothingConfigured };
