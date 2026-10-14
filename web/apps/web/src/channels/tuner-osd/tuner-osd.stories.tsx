import type { Meta, StoryObj } from "@storybook/react-vite";
import { TunerOSD } from "./tuner-osd";

// The OSD card shows while a tune is being acknowledged over the still-playing channel (black here).
// Once the stream loads, the tuner's wash names the channel and this card becomes screen-reader only
// (#1620), so the two are never drawn together.
const meta = {
  title: "Channels/TunerOSD",
  component: TunerOSD,
  parameters: { layout: "centered" },
  decorators: [
    (Story) => (
      <div className="relative aspect-video w-[calc(100vw-2rem)] max-w-[720px] overflow-hidden rounded-xl border border-border bg-black">
        <Story />
      </div>
    ),
  ],
  args: {
    number: 42,
    name: "Late Night Noir",
    currentTitle: "The Big Sleep",
    className: "absolute top-4 left-4 z-[2]",
  },
} satisfies Meta<typeof TunerOSD>;

export default meta;
type Story = StoryObj<typeof meta>;

const Tuning: Story = {};

const LongMetadata: Story = {
  args: {
    number: 108,
    name: "Saturday Morning Animation Marathon",
    currentTitle: "The Incredibly Long Adventures of the Galaxy Rangers",
  },
};

export { LongMetadata, Tuning };
