import type { Meta, StoryObj } from "@storybook/react-vite";
import { Caption } from "./caption";

// Caption — mono metadata beside content (§2.2, §5.1c). Extracted from 21 hand-rolled copies
// that had drifted across three off-scale sizes; `text-2xs` (11px) is now the sanctioned step
// and this is the only component that should use it.
const meta = {
  title: "Primitives/Caption",
  component: Caption,
  args: { children: "22m · 1080p" },
} satisfies Meta<typeof Caption>;

type Story = StoryObj<typeof meta>;

// The everyday voice: quiet metadata that does not compete with the content it labels.
const Default: Story = {};

// Strong tone for a value the eye should land on — a duration in a list of durations.
const Strong: Story = { args: { tone: "strong" } };

// SHOUT is the section-label voice: uppercase with tracking, for a heading over a group.
const Shout: Story = { args: { shout: true, children: "Pod · 1:10" } };

const StrongShout: Story = { args: { shout: true, tone: "strong", children: "Filling in" } };

// The realistic use: captions carrying machine-emitted values beside sans content.
const InContext: Story = {
  render: () => (
    <div className="flex w-72 flex-col gap-1 rounded-md border border-border p-3">
      <Caption shout>Now playing</Caption>
      <span className="text-sm">The Simpsons · Bart the Mother</span>
      <Caption>4:00 PM–4:22 PM · S10E03</Caption>
    </div>
  ),
};

export default meta;
export { Default, InContext, Shout, Strong, StrongShout };
