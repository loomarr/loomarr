import type { Meta, StoryObj } from "@storybook/react-vite";
import { TunerLoader } from "@/components/loomarr/shell";
import { TunerOSD } from "./tuner-osd";

const meta = {
  title: "Channels/TunerOSD",
  component: TunerOSD,
  parameters: { layout: "centered" },
  decorators: [
    (Story) => (
      <div className="relative aspect-video w-[calc(100vw-2rem)] max-w-[720px] overflow-hidden rounded-xl border border-border bg-black">
        <TunerLoader />
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
