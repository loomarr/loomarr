import { taggedClip } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { waitFor } from "storybook/test";
import { widthFrame } from "@/test/story-utils";
import { TINY_MP4 } from "@/test/video-fixture";
import { ClipPreview } from "./clip-player";

// The real embedded player, not a lookalike or a second modal. Pause its inline fixture
// after metadata loads so the workshop never captures a moving playhead.
const meta = {
  title: "Filler/ClipPreview",
  component: ClipPreview,
  decorators: [widthFrame(560)],
  play: async ({ canvasElement }) => {
    const video = canvasElement.querySelector("video");
    if (!video) throw new Error("The exact clip preview must mount a video");
    await waitFor(() => {
      if (video.readyState < 2) throw new Error("Waiting for the offline clip fixture");
    });
    video.pause();
    video.currentTime = 0;
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
