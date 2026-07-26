import type { Meta, StoryObj } from "@storybook/react-vite";
import { Input } from "./input";

// Input — the text field. Focus uses the signal ring (§2.1); the disabled state uses the
// static-500 floor, which is the ONLY sanctioned use of that stop.
const meta = {
  title: "Primitives/Input",
  component: Input,
  args: { placeholder: "90s action movies, high energy" },
  decorators: [(S) => <div className="w-80">{S()}</div>],
} satisfies Meta<typeof Input>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};
const WithValue: Story = { args: { defaultValue: "Saturday morning cartoons" } };
const Disabled: Story = { args: { disabled: true, defaultValue: "Locked while reconciling" } };
const Invalid: Story = { args: { "aria-invalid": true, defaultValue: "not-a-url" } };

export default meta;
export { Default, Disabled, Invalid, WithValue };
