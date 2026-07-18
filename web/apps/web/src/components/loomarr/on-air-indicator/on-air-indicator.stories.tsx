import type { Meta, StoryObj } from "@storybook/react-vite";
import { OnAirIndicator } from "./on-air-indicator";

// The red dot (§3): off · live (pulses; still under reduced-motion) · reconciling.
const meta = {
  title: "Loomarr/OnAirIndicator",
  component: OnAirIndicator,
  args: { showLabel: true },
} satisfies Meta<typeof OnAirIndicator>;

type Story = StoryObj<typeof meta>;

const Off: Story = { args: { state: "off" } };
const Live: Story = { args: { state: "live" } };
const Reconciling: Story = { args: { state: "reconciling" } };

export default meta;
export { Live, Off, Reconciling };
