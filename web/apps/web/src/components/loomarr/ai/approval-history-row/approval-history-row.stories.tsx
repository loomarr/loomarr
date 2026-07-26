import type { ProposalDTO } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { ApprovalHistoryRow } from "./approval-history-row";

// A FIXED timestamp, not `Date.now()` — the row renders a relative time ("2h ago"), and a
// moving clock would make the visual baseline drift every run.
const APPROVED_AT = "2026-07-26T10:00:00Z";

const base = (over: Partial<ProposalDTO> = {}): ProposalDTO =>
  ({
    id: "p1",
    jobId: "j1",
    status: "approved",
    createdBy: "kid",
    approvedBy: "boss",
    approvedAt: APPROVED_AT,
    proposal: { intent: { description: "Saturday morning cartoons for the kids" } },
    ...over,
  }) as ProposalDTO;

// One decided proposal in Queue's History tab — the audit trail for the approval gate. Every
// field here was persisted long before anything could show it.
const meta = {
  title: "AI/ApprovalHistoryRow",
  component: ApprovalHistoryRow,
  decorators: [widthFrame(600)],
} satisfies Meta<typeof ApprovalHistoryRow>;

type Story = StoryObj<typeof meta>;

const Approved: Story = { args: { proposal: base() } };

// Approved-with-changes is a distinct outcome: the lineup that shipped is not the one requested.
const ApprovedWithChanges: Story = {
  args: { proposal: base({ modSummary: "dropped 2, added 1" }) },
};

// A denial carries no approval time — the slot stays empty rather than showing a wrong one.
const Denied: Story = {
  args: {
    proposal: base({
      status: "denied",
      approvedBy: undefined,
      approvedAt: undefined,
      denyReason: "over the acquisition cap this week — ask again Monday",
    }),
  },
};

export default meta;
export { Approved, ApprovedWithChanges, Denied };
