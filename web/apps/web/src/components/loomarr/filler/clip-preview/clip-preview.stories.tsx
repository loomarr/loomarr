import { taggedClip } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { pauseStoryVideo } from "@/test/story-video";
import { TINY_MP4 } from "@/test/video-fixture";
import { ClipPreview } from "./clip-preview";

// The real embedded player, not a lookalike or a second modal. Pause its inline fixture
// after metadata loads so the workshop never captures a moving playhead.
const meta = {
  title: "Filler/ClipPreview",
  component: ClipPreview,
  decorators: [widthFrame(560)],
  play: async ({ canvasElement }) => {
    await pauseStoryVideo(canvasElement);
  },
} satisfies Meta<typeof ClipPreview>;

type Story = StoryObj<typeof meta>;

const Default: Story = { args: { clip: { ...taggedClip, hash: TINY_MP4 } } };
const LongTitle: Story = {
  args: {
    clip: {
      ...taggedClip,
      hash: TINY_MP4,
      name: "Cleveland local broadcast reel, tape 1, side B, segment 4 of 9 — an unusually long clip name (1987)",
    },
  },
};

export default meta;
export { Default, LongTitle };
