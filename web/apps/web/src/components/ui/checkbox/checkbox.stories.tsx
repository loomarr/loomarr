import type { Meta, StoryObj } from "@storybook/react-vite";
import { Label } from "../label";
import { Checkbox } from "./checkbox";

// Checkbox — Radix underneath, so keyboard and ARIA come for free (§4.1). Every state gets a
// baseline because the checked mark is an accent fill and moves with the palette.
const meta = {
  title: "Primitives/Checkbox",
  component: Checkbox,
} satisfies Meta<typeof Checkbox>;

type Story = StoryObj<typeof meta>;

const Default: Story = {
  render: () => (
    <div className="flex items-center gap-2">
      <Checkbox id="purge" />
      <Label htmlFor="purge">Also remove it from Tunarr</Label>
    </div>
  ),
};

const Checked: Story = {
  render: () => (
    <div className="flex items-center gap-2">
      <Checkbox id="auto" defaultChecked />
      <Label htmlFor="auto">Keep this channel up to date automatically</Label>
    </div>
  ),
};

const Disabled: Story = {
  render: () => (
    <div className="flex items-center gap-2">
      <Checkbox id="off" disabled defaultChecked />
      <Label htmlFor="off">Set by an environment variable</Label>
    </div>
  ),
};

export default meta;
export { Checked, Default, Disabled };
