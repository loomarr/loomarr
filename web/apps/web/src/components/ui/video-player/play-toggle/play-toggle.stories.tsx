import type { Meta, StoryObj } from "@storybook/react-vite";
import { PlayToggle } from "./play-toggle";

const meta = {
  title: "UI/VideoPlayer/PlayToggle",
  component: PlayToggle,
  args: { playing: false, onToggle: () => {} },
  // On a dark scrim in the player; give the story a dark backing so the amber reads.
  decorators: [
    (Story) => (
      <div className="flex items-center rounded-md bg-black p-4">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof PlayToggle>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Paused: Story = {};
export const Playing: Story = { args: { playing: true } };
