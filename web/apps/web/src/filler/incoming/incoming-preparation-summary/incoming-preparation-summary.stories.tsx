import type { Meta, StoryObj } from "@storybook/react-vite";
import { IncomingPreparationSummary } from "./incoming-preparation-summary";

const meta = {
  title: "Filler/Incoming/Preparation summary",
  component: IncomingPreparationSummary,
  decorators: [
    (Story) => (
      <div style={{ width: "100%", maxWidth: 460 }}>
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof IncomingPreparationSummary>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Measured: Story = { args: { preparation: { state: "working", percent: 47 } } };
export const Estimating: Story = { args: { preparation: { state: "estimating", percent: 33 } } };
export const EstimatedRange: Story = {
  args: {
    preparation: { state: "estimated", percent: 47, readyIn: { lowerSeconds: 120, upperSeconds: 240 } },
  },
};
export const Waiting: Story = { args: { preparation: { state: "waiting", percent: 55 } } };
export const Retrying: Story = { args: { preparation: { state: "retrying", percent: 55 } } };
export const Restarted: Story = { args: { preparation: { state: "restarted", percent: 0 } } };
export const Ready: Story = { args: { preparation: { state: "ready", percent: 100 } } };
export const Unavailable: Story = { args: { preparation: { state: "unavailable" } } };
