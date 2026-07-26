import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "../button";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "./tooltip";

// Tooltip — icon-button affordances and short clarifications.
//
// `open` is forced in these stories rather than driven by hover: the visual suite screenshots
// without a pointer, so an unforced tooltip would snapshot as an empty frame — a baseline that
// silently asserts nothing.
const meta = {
  title: "Primitives/Tooltip",
  component: Tooltip,
  decorators: [(S) => <TooltipProvider>{S()}</TooltipProvider>],
} satisfies Meta<typeof Tooltip>;

type Story = StoryObj<typeof meta>;

const Default: Story = {
  render: () => (
    <div className="flex justify-center py-12">
      <Tooltip open>
        <TooltipTrigger asChild>
          <Button variant="ghost" size="icon" aria-label="Reconcile now">
            ↻
          </Button>
        </TooltipTrigger>
        <TooltipContent>Reconcile now</TooltipContent>
      </Tooltip>
    </div>
  ),
};

export default meta;
export { Default };
