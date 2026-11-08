import { StatusWord } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

// StatusWord — the one vocabulary for "how is this doing" across Settings. Five states, one word
// and one colour each; the word is the meaning, the colour only backs it up. Expired is the one
// that needs explaining, so it carries a tooltip.
const meta = {
  title: "Primitives/StatusWord",
  component: StatusWord,
  args: { kind: "ok" },
} satisfies Meta<typeof StatusWord>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

const AllStates: Story = {
  render: () => (
    <div className="flex flex-col items-start gap-3">
      {(["ok", "attention", "notSetUp", "overdue", "expired"] as const).map((kind) => (
        <StatusWord key={kind} kind={kind} />
      ))}
    </div>
  ),
};

export default meta;
export { AllStates, Default };
