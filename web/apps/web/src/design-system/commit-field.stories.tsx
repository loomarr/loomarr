import { CommitField } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";

// CommitField — a box whose draft is handed over only when it loses focus.
const meta = {
  title: "Loomarr Components/CommitField",
  component: CommitField,
  args: { initial: "4h", label: "Repeat spacing", onCommit: () => undefined, placeholder: "Use built-in" },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 360, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof CommitField>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};
const Narrow: Story = {
  args: { initial: "90", keyboardType: "number-pad", label: "Longest programme", width: 112 },
};
const Empty: Story = { args: { initial: "" } };

export default meta;
export { Default, Empty, Narrow };
