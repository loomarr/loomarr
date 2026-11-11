import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { ChannelDefaultsSummary } from "./channel-defaults-summary";

const meta = {
  title: "Channels/ChannelDefaultsSummary",
  component: ChannelDefaultsSummary,
  decorators: [widthFrame(760)],
} satisfies Meta<typeof ChannelDefaultsSummary>;

type Story = StoryObj<typeof meta>;

// A fresh channel: every field is a default. A channel with changes draws nothing here, so it has
// no story; the component's test holds that.
const Fresh: Story = { args: { policy: {} } };

export default meta;
export { Fresh };
