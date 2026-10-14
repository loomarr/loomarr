import { proposal } from "@loomarr/fixtures";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { outlook } from "@/test/fixtures/outlook";
import { widthFrame } from "@/test/story-utils";
import { ProposalReview } from "./proposal-review";

const noop = () => {};
const withQueryClient: Decorator = (Story) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    <Story />
  </QueryClientProvider>
);

// The human-in-the-loop review before the approval gate (§3, §8): draft · submitted ·
// approved · denied · partially-edited.
const meta = {
  title: "AI/ProposalReview",
  component: ProposalReview,
  args: {
    proposal,
    assessment: outlook(),
    onApprove: noop,
    onDeny: noop,
    onEdit: noop,
    onRevise: noop,
    selfService: true,
  },
  decorators: [withQueryClient, widthFrame(720)],
} satisfies Meta<typeof ProposalReview>;

type Story = StoryObj<typeof meta>;

const Draft: Story = { args: { status: "draft" } };
const Submitted: Story = { args: { status: "submitted" } };
const NoSuggestionsYet: Story = {
  args: { status: "submitted", proposal: { ...proposal, alternates: [] } },
};
const Approved: Story = { args: { status: "approved" } };
const Denied: Story = { args: { status: "denied" } };
const PartiallyEdited: Story = { args: { status: "partially-edited" } };
const Updating: Story = { args: { status: "submitted", revising: true } };
const RevisionFailed: Story = {
  args: { status: "submitted", revisionError: "The provider timed out." },
};

export default meta;
export { Approved, Denied, Draft, NoSuggestionsYet, PartiallyEdited, RevisionFailed, Submitted, Updating };
