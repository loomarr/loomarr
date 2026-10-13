import type { Meta, StoryObj } from "@storybook/react-vite";
import { eraDates } from "@/lib/era-dates";
import { widthFrame } from "@/test/story-utils";
import { ChannelDefaultsSummary } from "./channel-defaults-summary";

const meta = {
  title: "Channels/ChannelDefaultsSummary",
  component: ChannelDefaultsSummary,
  decorators: [widthFrame(760)],
} satisfies Meta<typeof ChannelDefaultsSummary>;

type Story = StoryObj<typeof meta>;

// A fresh channel: every field is a default.
const Fresh: Story = { args: { policy: {} } };

// A lightly customized channel: a ceiling, an order and an era.
const Customized: Story = {
  args: {
    policy: {
      audience: { ceiling: "TV-14" },
      ordering: "shuffle",
      scope: { dates: eraDates({ from: 1990, to: 2015 }) },
    },
  },
};

export default meta;
export { Customized, Fresh };
