import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { Label, TooltipProvider } from "@/components/ui";
import { FieldHelp } from "./field-help";

// FieldHelp needs a TooltipProvider ancestor (mounted at the app root in the real app, but
// Storybook renders each story in isolation). The tooltip content only appears on hover, so the
// snapshot captures the (i) trigger in a label row — the resting state the form shows.
const withTooltip: Decorator = (Story) => (
  <TooltipProvider>
    <Story />
  </TooltipProvider>
);

const meta = {
  title: "Feedback/FieldHelp",
  component: FieldHelp,
  decorators: [withTooltip],
} satisfies Meta<typeof FieldHelp>;

type Story = StoryObj<typeof meta>;

// In context: a field label with its help icon beside it — the pattern across the settings forms.
const InALabelRow: Story = {
  render: (args) => (
    <div className="flex items-center gap-2">
      <Label>Ordering</Label>
      <FieldHelp {...args} />
    </div>
  ),
  args: { label: "Ordering", children: "How programs are sequenced on this channel." },
};

export default meta;
export { InALabelRow };
