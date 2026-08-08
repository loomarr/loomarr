import type { Meta, StoryObj } from "@storybook/react-vite";
import { LiveIndicator } from "./live-indicator";

const meta = {
  title: "UI/VideoPlayer/LiveIndicator",
  component: LiveIndicator,
  decorators: [
    (Story) => (
      <div className="rounded-md bg-black p-4">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof LiveIndicator>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
