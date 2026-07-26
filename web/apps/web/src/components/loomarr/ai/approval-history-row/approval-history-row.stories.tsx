import type { ProposalDTO } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { ApprovalHistoryRow } from "./approval-history-row";

// ⚠ RELATIVE TO NOW, not a fixed date. The row renders `formatRelative(approvedAt)`, so what
// matters for a stable baseline is the DISTANCE from the current clock, not the timestamp
// itself: a pinned "2026-07-26T10:00:00Z" renders "9h ago" today and "3d ago" next week, and the
// snapshot rots on a calendar rather than on a code change.
//
// Two hours back always renders exactly "2h ago" (formatRelative buckets at hour granularity
// between 1h and 24h), so the pixels are identical on every run.
const APPROVED_AT = new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString();

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
  // The row IS an `<li>` — its real parent is ApprovalHistory's `<ul>`. Mounting it bare makes
  // axe fail `listitem` (serious): a list item outside a list is invalid markup, not a styling
  // detail. The decorator supplies the context the component actually ships in, rather than
  // changing the component to suit the gallery.
  decorators: [
    (Story) => (
      <ul className="flex flex-col gap-2">
        <Story />
      </ul>
    ),
    widthFrame(600),
  ],
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
