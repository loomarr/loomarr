import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { ApprovalQueueItem } from "./approval-queue-item";

const noop = () => {};

// The admin approval row (§3, §7): pending · approving · denied.
const meta = {
  title: "AI/ApprovalQueueItem",
  component: ApprovalQueueItem,
  args: { onApprove: noop, onDeny: noop },
  decorators: [widthFrame(640)],
} satisfies Meta<typeof ApprovalQueueItem>;

type Story = StoryObj<typeof meta>;

const Pending: Story = {
  args: {
    title: "90s Action",
    summary: "High-energy 90s action movies, PG-13.",
    requestedBy: "ada",
    acquisitions: 3,
    status: "pending",
  },
};

const Approving: Story = {
  args: {
    title: "90s Action",
    summary: "High-energy 90s action movies, PG-13.",
    requestedBy: "ada",
    acquisitions: 3,
    status: "approving",
  },
};

const Denied: Story = {
  args: {
    title: "Late Night Horror",
    summary: "Unrated late-night horror marathon.",
    requestedBy: "guest",
    status: "denied",
    denyReason: "Over the acquisition cap for this week.",
  },
};

export default meta;
export { Approving, Denied, Pending };
