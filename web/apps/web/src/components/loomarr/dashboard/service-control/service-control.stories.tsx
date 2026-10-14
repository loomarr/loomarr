import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { ServiceControl } from "./service-control";

const noop = () => {};

const meta = {
  title: "Dashboard/ServiceControl",
  component: ServiceControl,
  args: {
    cost: { available: true, streamingChannels: 0, restartRequired: false },
    onRestart: noop,
  },
  decorators: [widthFrame(760)],
} satisfies Meta<typeof ServiceControl>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

// The confirm dialog on an install where Loomarr owns the encoder — the DROPS fact only
// appears when something is actually streaming.
const ConfirmingWithLiveStreams: Story = {
  args: { cost: { available: true, streamingChannels: 3, restartRequired: false } },
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole("button", { name: /Restart\.\.\./ }));
  },
};

// A Tunarr-backed install streams nothing itself, so the dialog has no DROPS line to show.
const ConfirmingWithNothingStreaming: Story = {
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole("button", { name: /Restart\.\.\./ }));
  },
};

const Restarting: Story = {
  args: { restarting: true },
};

// ⚠ Not a hidden button: a build with no restart loop says so, and says what to do
// instead. Hiding it would leave an operator hunting for a feature the docs mention.
const CannotSelfRestart: Story = {
  args: { cost: { available: false, streamingChannels: 0, restartRequired: false } },
};

const Failed: Story = {
  args: { error: "Couldn't restart. Loomarr is still running the old settings." },
};

export default meta;
export {
  CannotSelfRestart,
  ConfirmingWithLiveStreams,
  ConfirmingWithNothingStreaming,
  Default,
  Failed,
  Restarting,
};
