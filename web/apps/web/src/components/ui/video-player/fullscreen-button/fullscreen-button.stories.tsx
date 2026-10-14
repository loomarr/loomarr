import type { Meta, StoryObj } from "@storybook/react-vite";
import { FullscreenButton } from "./fullscreen-button";

const meta = {
  title: "UI/VideoPlayer/FullscreenButton",
  component: FullscreenButton,
  args: { active: false, onToggle: () => {} },
  decorators: [
    (Story) => (
      <div className="rounded-md bg-black p-4">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof FullscreenButton>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Enter: Story = {};
export const Exit: Story = { args: { active: true } };
