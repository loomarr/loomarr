import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { NowNextStrip } from "./now-next-strip";

// The "Now · Next" line (§3): playing · flex/pod gap (reads "Filler") · empty.
// Short invented titles: the 320px frame must fit both halves, or Next truncates away.
const meta = {
  title: "Channels/NowNextStrip",
  component: NowNextStrip,
  decorators: [widthFrame(320)],
} satisfies Meta<typeof NowNextStrip>;

type Story = StoryObj<typeof meta>;

const Playing: Story = { args: { now: { title: "Kiln", until: "9:42 PM" }, next: { title: "Ash Lane" } } };
const FlexPodGap: Story = { args: { gap: true, next: { title: "Ash Lane" } } };
const Empty: Story = { args: {} };

export default meta;
export { Empty, FlexPodGap, Playing };
