import { aiTaggedClip, taggedClip, untaggedClip } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { ClipCard } from "./clip-card";

const noop = () => {};

// A filler clip (§3, §10): tagged (pod-ready) · untagged (caution + Tag) · ai-suggested
// (the `suggest` marker + a one-click confirm; the human still gates the AI's guess).
const meta = {
  title: "Loomarr/ClipCard",
  component: ClipCard,
  decorators: [widthFrame(280)],
} satisfies Meta<typeof ClipCard>;

type Story = StoryObj<typeof meta>;

const Tagged: Story = { args: { clip: taggedClip } };
const Untagged: Story = { args: { clip: untaggedClip, onTag: noop } };
const AiSuggestedTags: Story = { args: { clip: aiTaggedClip, onConfirmTags: noop } };

export default meta;
export { AiSuggestedTags, Tagged, Untagged };
