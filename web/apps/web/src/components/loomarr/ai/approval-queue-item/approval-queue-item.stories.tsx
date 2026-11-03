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

// Titles the proposal's own ceiling cannot air, shown before any click. A warning, so `caution`
// (#1909 Q5): amber is focus and the primary action. Only an LLM-grounded proposal produces these.
const WithRefused: Story = {
  args: {
    title: "Saturday Morning Toons",
    summary: "Bright cartoons for weekend mornings, TV-Y7.",
    requestedBy: "ada",
    acquisitions: 1,
    status: "pending",
    refused: [
      {
        item: {
          mediaType: "movie",
          name: "Midnight Toons",
          officialRating: "TV-MA",
          inLibrary: true,
        },
        reason: "over_ceiling",
      },
      {
        item: {
          mediaType: "series",
          name: "After Hours Doodles",
          officialRating: "TV-14",
          inLibrary: false,
        },
        reason: "over_ceiling",
      },
    ],
  },
};

// A holiday the household's country doesn't keep (#2030 decision 38): the channel won't follow its
// season, and the card says so before approval.
const WithSeasonNotKept: Story = {
  args: {
    title: "Thanksgiving Movies",
    summary: "Family films for the long weekend.",
    requestedBy: "ada",
    acquisitions: 0,
    status: "pending",
    seasonNotKept: { country: "GB", holidays: ["Thanksgiving"] },
  },
};

export default meta;
export { Approving, Denied, Pending, WithRefused, WithSeasonNotKept };
