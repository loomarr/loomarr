import { SearchField } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";

// SearchField — a list filter's text box with the magnifier inside its leading edge.
const meta = {
  title: "Loomarr Components/SearchField",
  component: SearchField,
  args: { label: "Search people", onChangeText: () => undefined, placeholder: "Name", value: "" },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 360, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof SearchField>;

type Story = StoryObj<typeof meta>;

const Empty: Story = {};
const Typed: Story = { args: { value: "Morgan" } };
const Disabled: Story = { args: { disabled: true, value: "Morgan" } };
const Clearable: Story = { args: { clearLabel: "Clear search", value: "Morgan" } };

export default meta;
export { Clearable, Disabled, Empty, Typed };
