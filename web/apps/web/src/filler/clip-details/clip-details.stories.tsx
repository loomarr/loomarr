import { fillerRefinementKnownClip, fillerRefinementUnknownClip } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { widthFrame } from "@/test/story-utils";
import { pauseStoryVideo } from "@/test/story-video";
import { TINY_MP4 } from "@/test/video-fixture";
import { ClipDetails } from "./clip-details";

const meta = {
  title: "Filler/ClipDetails",
  component: ClipDetails,
  decorators: [widthFrame(560)],
  play: async ({ canvasElement }) => pauseStoryVideo(canvasElement),
} satisfies Meta<typeof ClipDetails>;
type Story = StoryObj<typeof meta>;

const Known: Story = {
  args: { clip: { ...fillerRefinementKnownClip, hash: TINY_MP4 }, onEdit: () => {} },
};
const Unknown: Story = {
  args: { clip: { ...fillerRefinementUnknownClip, hash: TINY_MP4 } },
};
const Preparing: Story = {
  args: { clip: { ...fillerRefinementUnknownClip, held: true } },
  play: async ({ canvasElement }) => {
    await expect(canvasElement.querySelector("video")).toBeNull();
  },
};

export default meta;
export { Known, Preparing, Unknown };
