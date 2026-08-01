import { splitProposal } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
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

const Review: Story = { args: { proposal: splitProposal, onConfirm: noop, onBack: noop } };

// The confirm mutation is in flight: the footer locks so a double-click can't commit twice.
const Confirming: Story = {
  args: { proposal: splitProposal, confirming: true, onConfirm: noop, onBack: noop },
};

export default meta;
export { Confirming, Review };
