import { splitProposal } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { TINY_WEBM } from "@/test/video-fixture";
import { SplitReviewEditor } from "./split-review-editor";

const noop = () => {};

// The §10 V34 review gate: the operator reads every proposed cut before anything enters
// the catalog. The fixture proposal covers all four segment states — clean, an
// unconfirmed era suggestion, a duplicate flag, and an unsplittable span.
const meta = {
  title: "Filler/SplitReviewEditor",
  component: SplitReviewEditor,
} satisfies Meta<typeof SplitReviewEditor>;

type Story = StoryObj<typeof meta>;

const reviewProposal = { ...splitProposal, clipHash: TINY_WEBM };

const Review: Story = { args: { proposal: reviewProposal, onConfirm: noop, onBack: noop } };

// The confirm mutation is in flight: the footer locks so a double-click can't commit twice.
const Confirming: Story = {
  args: { proposal: reviewProposal, confirming: true, onConfirm: noop, onBack: noop },
};

const TwentyFourClips: Story = {
  args: {
    proposal: {
      ...reviewProposal,
      id: "split-long-reel",
      segments: Array.from({ length: 24 }, (_, index) => ({
        index,
        startMs: index * 30_000,
        endMs: (index + 1) * 30_000,
        name:
          index === 12
            ? "A very long local commercial title with a station, sponsor, product, campaign, and uncertain recording date"
            : `Commercial ${index + 1}`,
        audience: index % 3 === 0 ? "family" : "general",
        category: index % 2 === 0 ? "food & drink" : "retail",
        language: "en",
        languageChecked: true,
        ...(index === 23 ? { unsplittable: true } : {}),
        ...(index % 7 === 6 ? {} : { artwork: splitProposal.segments[0]?.artwork }),
      })),
    },
    onConfirm: noop,
    onBack: noop,
  },
};

export default meta;
export { Confirming, Review, TwentyFourClips };
