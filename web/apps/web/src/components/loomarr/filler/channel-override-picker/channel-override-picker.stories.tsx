import { channelFits } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { ChannelOverridePicker } from "./channel-override-picker";

const noop = () => {};

const meta = {
  title: "Filler/ChannelOverridePicker",
  component: ChannelOverridePicker,
  args: {
    clipName: "Frosted Flakes — They're Grrreat!",
    channels: channelFits,
    onSet: noop,
    onReset: noop,
  },
  decorators: [widthFrame(680)],
} satisfies Meta<typeof ChannelOverridePicker>;

type Story = StoryObj<typeof meta>;

// ⚠ The baseline that matters: all five renderings side by side. Four of them look alike from
// a distance and mean different things — an automatic match, an automatic NON-match with its
// reason, a pin (no rung, no reason), and a block. If a change makes any two of these read the
// same, the image is where it shows.
const Default: Story = {};

// A fresh install where nothing has been decided: every row automatic. The row of unticked
// boxes is exactly why the mode note above them exists.
const AllAutomatic: Story = {
  args: {
    channels: channelFits.map((c) => ({
      ...c,
      pinned: false,
      excluded: false,
      ...(c.reason === "excluded" ? { reason: undefined } : {}),
    })),
  },
};

const Loading: Story = {
  args: { channels: [], loading: true },
};

// A clip with nowhere to play. Rendered as a sentence rather than an empty list, which reads
// as a broken panel.
const NoChannels: Story = {
  args: { channels: [] },
};

const OneChannelWriting: Story = {
  args: { busyChannelId: "ch-1" },
};

const Failed: Story = {
  args: { error: "Couldn't save that change. Try again." },
};

export default meta;
export { AllAutomatic, Default, Failed, Loading, NoChannels, OneChannelWriting };
