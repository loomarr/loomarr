import { NumberField } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";

// NumberField — a number in a design-system Field that keeps a half-typed decimal while it is typed.
const meta = {
  title: "Loomarr Components/NumberField",
  component: NumberField,
  args: { amount: 30, label: "Clips per check", onAmount: () => undefined },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 240, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof NumberField>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};
const Decimal: Story = { args: { amount: 1.5, label: "Hours between checks" } };
const Invalid: Story = { args: { amount: 0, invalid: true } };
const Disabled: Story = { args: { disabled: true } };

export default meta;
export { Decimal, Default, Disabled, Invalid };
