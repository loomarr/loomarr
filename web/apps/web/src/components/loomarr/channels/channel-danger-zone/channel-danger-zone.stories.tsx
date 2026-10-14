import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { ChannelDangerZone } from "./channel-danger-zone";

const noop = () => {};

// The destructive-actions section (frontend-design §6): pause/resume plus explicit
// stop-managing and permanent-delete choices, isolated with onair styling.
const meta = {
  title: "Channels/ChannelDangerZone",
  component: ChannelDangerZone,
  args: { channelName: "90s Action", onPause: noop, onResume: noop, onDelete: noop },
  decorators: [widthFrame(480)],
} satisfies Meta<typeof ChannelDangerZone>;

type Story = StoryObj<typeof meta>;

const Normal: Story = { args: { status: "live" } };

const Paused: Story = { args: { status: "paused" } };

const Busy: Story = { args: { status: "live", busy: true } };

export default meta;
export { Busy, Normal, Paused };
