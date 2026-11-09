import { Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { FieldHelp } from "./field-help";

// The tooltip content only appears on hover, so the snapshot captures the (i) trigger in a label
// row — the resting state the form shows.

const meta = {
  title: "Feedback/FieldHelp",
  component: FieldHelp,
} satisfies Meta<typeof FieldHelp>;

type Story = StoryObj<typeof meta>;

// In context: a field label with its help icon beside it — the pattern across the settings forms.
const InALabelRow: Story = {
  render: (args) => (
    <div className="flex items-center gap-2">
      <Text textRole="label">Ordering</Text>
      <FieldHelp {...args} />
    </div>
  ),
  args: { label: "Ordering", children: "How programs are sequenced on this channel." },
};

export default meta;
export { InALabelRow };
