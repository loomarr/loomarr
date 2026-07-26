import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { StatCard } from "./stat-card";

// One number on the dashboard, over a label and a one-line explanation. The note is not
// decoration: a bare "12" does not say of what, nor whether more is better or worse.
const meta = {
  title: "Dashboard/StatCard",
  component: StatCard,
  decorators: [widthFrame(260)],
} satisfies Meta<typeof StatCard>;

type Story = StoryObj<typeof meta>;

const OnAir: Story = {
  args: { label: "On air", value: 3, note: "channels in the guide right now", tone: "onair" },
};

// The call-to-action card takes its colour only when the number is non-zero — a permanently
// coloured zero trains the eye to ignore it.
const NeedsYou: Story = {
  args: { label: "Needs you", value: 2, note: "requests waiting on approval", tone: "suggest" },
};

const Quiet: Story = {
  args: { label: "Needs you", value: 0, note: "nothing waiting", tone: "neutral" },
};

export default meta;
export { NeedsYou, OnAir, Quiet };
