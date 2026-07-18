import { proposal } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { ProposalReview } from "./proposal-review";

const noop = () => {};

// The human-in-the-loop review before the approval gate (§3, §8): draft · submitted ·
// approved · denied · partially-edited.
const meta = {
  title: "Loomarr/ProposalReview",
  component: ProposalReview,
  args: { proposal, onApprove: noop, onDeny: noop, onEditItem: noop },
  decorators: [widthFrame(640)],
} satisfies Meta<typeof ProposalReview>;

type Story = StoryObj<typeof meta>;

const Draft: Story = { args: { status: "draft" } };
const Submitted: Story = { args: { status: "submitted" } };
const Approved: Story = { args: { status: "approved" } };
const Denied: Story = { args: { status: "denied" } };
const PartiallyEdited: Story = { args: { status: "partially-edited" } };

export default meta;
export { Approved, Denied, Draft, PartiallyEdited, Submitted };
