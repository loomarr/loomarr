import type { Meta, StoryObj } from "@storybook/react-vite";
import { StatusDot } from "./status-dot";

// StatusDot — the small round state indicator (§5.1c). Extracted from three independent copies
// that each had their own size, colour mapping, and answer to whether "live" pulses.
const meta = {
  title: "Primitives/StatusDot",
  component: StatusDot,
  args: { tone: "live", label: "On air" },
} satisfies Meta<typeof StatusDot>;

type Story = StoryObj<typeof meta>;

// The pulse is reserved for `live` alone — it means "happening right now", so spending it on
// any other tone would cost the dot its one piece of motion vocabulary.
const Live: Story = {};

// Every tone at once, labelled. Colour is never the ONLY signal: each dot carries an
// accessible name, which is what makes the state legible without seeing it.
const AllTones: Story = {
  render: () => (
    <div className="flex flex-col gap-2">
      {(
        [
          ["live", "On air"],
          ["pending", "Reconciling"],
          ["ok", "Healthy"],
          ["warn", "Drifted"],
          ["off", "Paused"],
        ] as const
      ).map(([tone, label]) => (
        <div key={tone} className="flex items-center gap-2">
          <StatusDot tone={tone} label={label} />
          <span className="text-sm">{label}</span>
          <span className="ml-auto font-mono text-2xs text-static-400">{tone}</span>
        </div>
      ))}
    </div>
  ),
};

export default meta;
export { AllTones, Live };
