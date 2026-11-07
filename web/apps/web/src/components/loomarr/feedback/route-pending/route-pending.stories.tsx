import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { RoutePending } from "./route-pending";

// The router's default pending state: what a route shows once its guard or loader is slow.
const meta = {
  title: "Feedback/RoutePending",
  component: RoutePending,
  decorators: [widthFrame(640)],
} satisfies Meta<typeof RoutePending>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

export default meta;
export { Default };
