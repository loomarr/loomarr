import { fallbackCardEntry, podEntries } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { PodTimeline } from "./pod-timeline";

// A commercial break made legible (§3, §10). The chip names how far down the fallback
// ladder assembly went — the answer to "why are my commercials wrong".
const meta = {
  title: "Filler/PodTimeline",
  component: PodTimeline,
  args: { era: 1990, audience: "kids" },
  decorators: [widthFrame(480)],
} satisfies Meta<typeof PodTimeline>;

type Story = StoryObj<typeof meta>;

// The quiet case: era + audience both matched, so no chip.
const Exact: Story = { args: { entries: podEntries, matchLevel: "exact" } };

const EraWidened: Story = { args: { entries: podEntries, matchLevel: "widened" } };

const EraIgnored: Story = { args: { entries: podEntries, matchLevel: "audience" } };

// Nothing matched — the embedded card stands in rather than dead air (§10).
const BumperCardOnly: Story = {
  args: { entries: [fallbackCardEntry], matchLevel: "bumper_card" },
};

export default meta;
export { BumperCardOnly, EraIgnored, EraWidened, Exact };
