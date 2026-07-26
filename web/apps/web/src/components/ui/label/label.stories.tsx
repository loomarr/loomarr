import type { Meta, StoryObj } from "@storybook/react-vite";
import { Input } from "../input";
import { Label } from "./label";

// Label — always bound to a control. An unbound label is an accessibility bug the axe gate
// catches, so the story shows the bound form rather than a bare string.
const meta = {
  title: "Primitives/Label",
  component: Label,
} satisfies Meta<typeof Label>;

type Story = StoryObj<typeof meta>;

const Default: Story = {
  render: () => (
    <div className="flex w-80 flex-col gap-1.5">
      <Label htmlFor="channel-name">Channel name</Label>
      <Input id="channel-name" defaultValue="Springfield Classics" />
    </div>
  ),
};

export default meta;
export { Default };
