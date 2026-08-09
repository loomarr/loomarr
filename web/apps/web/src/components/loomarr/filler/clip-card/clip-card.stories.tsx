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

// ⚠ The image-service path (§22, V52 phase 6). `thumbnail` is still set, exactly as it is on a
// real clip during the migration window — so this story also proves the component PREFERS
// `thumbImage` when both are present. Without that, a card would keep rendering the legacy route
// forever after adoption and nothing would notice.
//
// Assets are same-origin static files (comma-free, so they work in srcset) rather than data URIs;
// each ladder rung is a different colour, so the baseline shows which one a card-sized box chose.
const ServiceHostedArtwork: Story = {
  args: {
    clip: {
      ...thumbnailedClip,
      thumbImage: {
        hash: "3c1f9ab2e4d7650a3c1f9ab2e4d7650a3c1f9ab2e4d7650a3c1f9ab2e4d7650a",
        role: "thumb",
        width: 500,
        height: 750,
        placeholder: "1QcSHQRnh493V4dIh4eXh1h4kJUI",
        dominantHex: "#2b4a5e",
        animated: false,
        srcSetWebp: "/poster-154.webp 154w, /poster-342.webp 342w, /poster-500.webp 500w",
        srcSetAvif: "",
        src: "/poster-fallback.jpg",
      },
    },
  },
};

export default meta;
// Retag from the card (the v2 mock's cycleEra/cycleAud/cycleCat). ⚠ These two are the ONLY
// stories carrying `onCycle`, and they exist because the chips are invisible without it: the
// gallery showed static badges for every admin story while the feature shipped, which is the
// built-but-unseen shape one layer down from built-but-unimported.
const Cycleable: Story = { args: { clip: taggedClip, onCycle: noop, onTag: noop } };
// The UNSET rendering: dashed and dim, so three empty chips read as controls rather than as
// three alarms beside the "Untagged" badge that already said it.
const CycleableUnset: Story = { args: { clip: untaggedClip, onCycle: noop, onTag: noop } };

export {
  AdminActions,
  AiSuggestedTags,
  Cycleable,
  CycleableUnset,
  ServiceHostedArtwork,
  SplitAction,
  SplitPending,
  SuggestedEra,
  SuggestedEraAdmin,
  Tagged,
  TaggedEditable,
  Untagged,
  WithThumbnail,
};
