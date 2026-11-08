import { YearField } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";

// YearField — a four-digit year, narrow enough that a From/To pair shares a row.
const meta = {
  title: "Loomarr Components/YearField",
  component: YearField,
  args: { label: "From year", onChangeText: () => undefined, value: "1990" },
  decorators: [
    (Story) => (
      <div style={{ padding: 24 }}>
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof YearField>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};
const Empty: Story = { args: { placeholder: "Any", value: "" } };
const Disabled: Story = { args: { disabled: true } };

export default meta;
export { Default, Disabled, Empty };
