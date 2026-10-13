import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { PolicyFieldSource } from "./policy-field-source";

// Where a Programming field's value comes from, and the Reset that writes its default.
const meta = {
  title: "Channels/PolicyFieldSource",
  component: PolicyFieldSource,
  args: { onReset: fn(), label: "Audience ceiling" },
} satisfies Meta<typeof PolicyFieldSource>;

type Story = StoryObj<typeof meta>;

// The channel sets its own value: Reset is live.
const Overridden: Story = { args: { overridden: true } };

// The field follows its default: Reset stays in place, disabled.
const Default: Story = { args: { overridden: false } };

// The auto-curate opt-in's wording, whose default offers no Reset at all.
const OptedIn: Story = {
  args: {
    overridden: true,
    label: "Add new titles without asking",
    overrideLabel: "Opted in",
    defaultLabel: "Default: off",
    hideResetWhenDefault: true,
  },
};

const OptedOut: Story = { args: { ...OptedIn.args, overridden: false } };

export default meta;
export { Default, OptedIn, OptedOut, Overridden };
