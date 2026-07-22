import type { ChannelPolicy } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { ChannelPolicyFields } from "./channel-policy-fields";

const noop = () => {};

// A channel's ChannelPolicy as plain-language editable chips (programming-design §8).
const meta = {
  title: "Loomarr/ChannelPolicyFields",
  component: ChannelPolicyFields,
  args: { onChange: noop },
  decorators: [widthFrame(480)],
} satisfies Meta<typeof ChannelPolicyFields>;

type Story = StoryObj<typeof meta>;

const Empty: Story = { args: { policy: {} } };

const populated: ChannelPolicy = {
  ordering: "shuffle",
  audience: { ceiling: "TV-14" },
  scope: { era: { from: 1990, to: 1999 } },
  separation: { movieNoRepeat: "168h", episodeNoRepeat: "24h" },
};

const Populated: Story = { args: { policy: populated } };

// The safety ceiling set to the strictest value, to check the caption reads correctly
// alongside a real restriction rather than only against "No limit".
const StrictCeiling: Story = {
  args: { policy: { audience: { ceiling: "TV-Y" } } },
};

export default meta;
export { Empty, Populated, StrictCeiling };
