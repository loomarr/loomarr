import { bumperClip, podClips } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { PodTimeline } from "./pod-timeline";

// A commercial break made legible (§3, §10): matched · fallback-widened · bumper-card-only.
const meta = {
  title: "Loomarr/PodTimeline",
  component: PodTimeline,
  args: { era: 1990, audience: "kids" },
  decorators: [widthFrame(480)],
} satisfies Meta<typeof PodTimeline>;

type Story = StoryObj<typeof meta>;

const Matched: Story = { args: { clips: podClips, match: "matched" } };
const FallbackWidened: Story = { args: { clips: podClips, match: "fallback-widened" } };
const BumperCardOnly: Story = { args: { clips: [bumperClip], match: "bumper-card-only" } };

export default meta;
export { BumperCardOnly, FallbackWidened, Matched };
