import type { Meta, StoryObj } from "@storybook/react-vite";
import { Captions, Volume2 } from "lucide-react";
import { useState } from "react";
import { TrackSelectMenu } from "./track-select-menu";

const meta = {
  title: "UI/VideoPlayer/TrackSelectMenu",
  component: TrackSelectMenu,
  args: {
    icon: Volume2,
    label: "Audio",
    options: [{ value: "eng", label: "English · stereo" }],
    value: "eng",
    onChange: () => {},
  },
  decorators: [
    (Story) => (
      <div className="flex items-center gap-2 rounded-md bg-black p-4">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof TrackSelectMenu>;

export default meta;
type Story = StoryObj<typeof meta>;

const AUDIO = [
  { value: "auto", label: "Auto (default)" },
  { value: "eng", label: "English · stereo" },
  { value: "spa", label: "Spanish · 5.1" },
];

// Interactive audio menu (admin — options are selectable).
const AudioInteractive = () => {
  const [value, setValue] = useState("eng");
  return <TrackSelectMenu icon={Volume2} label="Audio" options={AUDIO} value={value} onChange={setValue} />;
};

export const Audio: Story = { render: () => <AudioInteractive /> };

export const Subtitles: Story = {
  args: {
    icon: Captions,
    label: "Subtitles",
    options: [
      { value: "off", label: "Off" },
      { value: "burn", label: "Burn in (English)" },
    ],
    value: "off",
    onChange: () => {},
  },
};

// A MEMBER sees the current track but the options are read-only (channel-wide, admin-set).
export const ReadOnlyMember: Story = {
  args: { icon: Volume2, label: "Audio", options: AUDIO, value: "eng", onChange: () => {}, readOnly: true },
};
