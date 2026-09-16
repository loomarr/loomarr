import { aiTaggedClip, suggestedEraClip, taggedClip, thumbnailedClip, untaggedClip } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { ClipCard } from "./clip-card";

const noop = () => {};

// Ready clips with known, missing, or model-assisted optional metadata.
const meta = {
  title: "Filler/ClipCard",
  component: ClipCard,
  decorators: [widthFrame(280)],
} satisfies Meta<typeof ClipCard>;

type Story = StoryObj<typeof meta>;

const Tagged: Story = { args: { clip: taggedClip } };

// Known metadata does not prevent inspection or optional corrections in the details panel.
const TaggedEditable: Story = { args: { clip: taggedClip, onOpen: noop } };
const Untagged: Story = { args: { clip: untaggedClip, onOpen: noop } };
const AiSuggestedTags: Story = { args: { clip: aiTaggedClip, onOpen: noop } };

// Inspection is the same calm entry point for members and admins.
const AdminActions: Story = { args: { clip: taggedClip, onOpen: noop } };

// The extracted frame (V17b), served by V30. ⚠ Only clips that HAVE one render it — the
// stories above are deliberately frameless, because on a Tunarr-backed install (or one where
// ffmpeg never ran) that is the whole catalog, and it must not look broken.
const WithThumbnail: Story = { args: { clip: thumbnailedClip, onOpen: noop } };

// An ungrounded suggestion must not become a known year on the card.
const SuggestedEra: Story = { args: { clip: suggestedEraClip } };
const SuggestedEraAdmin: Story = { args: { clip: suggestedEraClip, onOpen: noop } };

// The compilation-split entry point (§10 V34), and its in-flight state while detection runs.
const SplitAction: Story = { args: { clip: taggedClip, onOpen: noop, onSplit: noop } };
const SplitPending: Story = { args: { clip: taggedClip, onSplit: noop, splitPending: true } };

// ⚠ **`ServiceHostedArtwork` was folded into the shared fixture in V52 phase 8.** It existed to
// prove the card PREFERRED `thumbImage` when a clip carried both it and a legacy `thumbnail`.
// There is no longer a both-present state to prefer between — the legacy field and its route are
// retired — so a story asserting the preference would be asserting a choice the code cannot make.
// `thumbnailedClip` now carries the image record itself, which puts real artwork in every story
// built on it rather than in one opt-in variant.

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
