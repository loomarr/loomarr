import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { EmptyState } from "./empty-state";

const noop = () => {};

// Mandatory for every list (§6) — exactly one next action. Per-surface copy variants.
const meta = {
  title: "Feedback/EmptyState",
  component: EmptyState,
  decorators: [widthFrame(420)],
} satisfies Meta<typeof EmptyState>;

type Story = StoryObj<typeof meta>;

const Channels: Story = {
  args: {
    title: "Dead air",
    description: "Create your first channel from an intent — Loomarr grounds a lineup against your library.",
    action: { label: "Create a channel", onClick: noop },
  },
};

const ApprovalQueue: Story = {
  args: { title: "Queue's clear", description: "Nothing awaiting approval right now." },
};

export default meta;
export { ApprovalQueue, Channels };
