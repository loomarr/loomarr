import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { PhoneHeader } from "./phone-header";

// The phone's top row (#2751): Search, top right, at a 390px phone width.
const meta = {
  title: "Shell/PhoneHeader",
  component: PhoneHeader,
  args: { onOpenSearch: () => {} },
  decorators: [widthFrame(390)],
} satisfies Meta<typeof PhoneHeader>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

export default meta;
export { Default };
