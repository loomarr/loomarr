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

// A tagged clip is still editable: §10's likely error is a trailer scanned as a
// commercial, which arrives fully tagged and therefore wrong-but-"complete". Gating the
// edit on `!tagged` left exactly that clip uncorrectable.
const TaggedEditable: Story = { args: { clip: taggedClip, onTag: noop } };
const Untagged: Story = { args: { clip: untaggedClip, onTag: noop } };
const AiSuggestedTags: Story = { args: { clip: aiTaggedClip, onConfirmTags: noop } };

// The admin view on the Filler catalog: edit the tags AND pin the clip straight into a
// channel's filler (P3 cohesion) — the two actions sit together in the card's action row.
const AdminActions: Story = { args: { clip: taggedClip, onTag: noop, onPin: noop } };

export default meta;
export { AdminActions, AiSuggestedTags, Tagged, TaggedEditable, Untagged };
