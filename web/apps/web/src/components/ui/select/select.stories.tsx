import type { Meta, StoryObj } from "@storybook/react-vite";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "./select";

// Select — Radix underneath. The CLOSED trigger is what a page mostly shows, so that is the
// default story; the open list is portalled and worth its own frame.
const meta = {
  title: "Primitives/Select",
  component: Select,
  decorators: [(S) => <div className="w-56">{S()}</div>],
} satisfies Meta<typeof Select>;

type Story = StoryObj<typeof meta>;

const Default: Story = {
  render: () => (
    <Select defaultValue="240">
      <SelectTrigger aria-label="Window span">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="120">2 hours</SelectItem>
        <SelectItem value="240">4 hours</SelectItem>
        <SelectItem value="360">6 hours</SelectItem>
      </SelectContent>
    </Select>
  ),
};

export default meta;
export { Default };
