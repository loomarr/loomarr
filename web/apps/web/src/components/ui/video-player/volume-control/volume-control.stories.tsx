import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { VolumeControl } from "./volume-control";

const meta = {
  title: "UI/VideoPlayer/VolumeControl",
  component: VolumeControl,
  args: { volume: 0.7, muted: false, onVolumeChange: () => {}, onMutedChange: () => {} },
  decorators: [
    (Story) => (
      <div className="flex items-center rounded-md bg-black p-4">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof VolumeControl>;

export default meta;
type Story = StoryObj<typeof meta>;

// Interactive: the slider + mute toggle drive local state, so the story behaves like the real
// control (a static story cannot show the slider moving).
const Interactive = () => {
  const [volume, setVolume] = useState(0.7);
  const [muted, setMuted] = useState(false);
  return <VolumeControl volume={volume} muted={muted} onVolumeChange={setVolume} onMutedChange={setMuted} />;
};

export const Default: Story = { render: () => <Interactive /> };
export const Muted: Story = {
  args: { volume: 0.7, muted: true, onVolumeChange: () => {}, onMutedChange: () => {} },
};
