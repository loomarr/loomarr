import { aiTaggedClip, suggestedEraClip, taggedClip, thumbnailedClip, untaggedClip } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { ClipCard } from "./clip-card";

const noop = () => {};

// A filler clip (§3, §10): tagged (pod-ready) · untagged (caution + Tag) · ai-suggested
// (the `suggest` marker + a one-click confirm; the human still gates the AI's guess).
const meta = {
  title: "Filler/ClipCard",
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

// The extracted frame (V17b), served by V30. ⚠ Only clips that HAVE one render it — the
// stories above are deliberately frameless, because on a Tunarr-backed install (or one where
// ffmpeg never ran) that is the whole catalog, and it must not look broken.
const WithThumbnail: Story = { args: { clip: thumbnailedClip, onTag: noop, onPin: noop } };

// An ungrounded AI era guess (§10 V34): the "?" badge is a question, not a tag — a member
// sees only the badge, an admin gets the one-click confirm beside it.
const SuggestedEra: Story = { args: { clip: suggestedEraClip } };
const SuggestedEraAdmin: Story = { args: { clip: suggestedEraClip, onConfirmEra: noop, onTag: noop } };

// The compilation-split entry point (§10 V34), and its in-flight state while detection runs.
const SplitAction: Story = { args: { clip: taggedClip, onTag: noop, onSplit: noop } };
const SplitPending: Story = { args: { clip: taggedClip, onSplit: noop, splitPending: true } };

export default meta;
export {
  AdminActions,
  AiSuggestedTags,
  SplitAction,
  SplitPending,
  SuggestedEra,
  SuggestedEraAdmin,
  Tagged,
  TaggedEditable,
  Untagged,
  WithThumbnail,
};
