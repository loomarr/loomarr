import { splitProposal } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { pauseStoryVideo } from "@/test/story-video";
import { TINY_WEBM } from "@/test/video-fixture";
import { SplitReviewEditor } from "./split-review-editor";

const noop = () => {};

// The §10 V34 review gate: the operator reads every proposed cut before anything enters
// the catalog. The fixture proposal covers all four segment states — clean, an
// unconfirmed era suggestion, a duplicate flag, and an unsplittable span.
const meta = {
  title: "Filler/SplitReviewEditor",
  component: SplitReviewEditor,
  // This editor fills the split-review page's content column. Storybook's global centered
  // layout is shrink-to-content, so a long timeline can make the canvas itself thousands of
  // pixels wide instead of exercising the component's horizontal scroller. A full-width frame
  // mirrors the real route (including its page gutter) and keeps narrow-viewport stories honest.
  parameters: { layout: "fullscreen" },
  decorators: [
    (Story) => (
      <div className="w-full p-6">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof SplitReviewEditor>;

type Story = StoryObj<typeof meta>;

const reviewProposal = { ...splitProposal, clipHash: TINY_WEBM };

const Review: Story = { args: { proposal: reviewProposal, onConfirm: noop, onBack: noop } };

// The gallery snapshots and axe-checks the actual shared Sheet, not only the closed overview.
// The player still loads the real offline fixture; freeze it after opening so its frame is stable.
const SelectedClip: Story = {
  args: { proposal: reviewProposal, onConfirm: noop, onBack: noop },
  tags: ["portal"],
  play: async ({ canvas, canvasElement, userEvent }) => {
    await userEvent.click(await canvas.findByRole("button", { name: "Open clip 1: Sunny D — Dude!" }));
    await pauseStoryVideo(canvasElement.ownerDocument.body);
  },
};

// The confirm mutation is in flight: the footer locks so a double-click can't commit twice.
const Confirming: Story = {
  args: { proposal: reviewProposal, confirming: true, onConfirm: noop, onBack: noop },
};

const longReel = (count: number) => ({
  ...reviewProposal,
  id: `split-long-reel-${count}`,
  segments: Array.from({ length: count }, (_, index) => ({
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
    ...(index === count - 1 ? { unsplittable: true } : {}),
    ...(index % 7 === 6 ? {} : { artwork: splitProposal.segments[0]?.artwork }),
  })),
});

const TwentyFourClips: Story = {
  args: {
    proposal: longReel(24),
    onConfirm: noop,
    onBack: noop,
  },
};

const FiftyClips: Story = {
  args: {
    proposal: longReel(50),
    onConfirm: noop,
    onBack: noop,
  },
};

export default meta;
export { Confirming, FiftyClips, Review, SelectedClip, TwentyFourClips };
