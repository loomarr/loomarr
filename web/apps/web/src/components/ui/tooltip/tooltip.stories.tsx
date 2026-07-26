import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "../button";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "./tooltip";

// Tooltip — icon-button affordances and short clarifications.
//
// `open` is forced rather than driven by hover, because the visual suite screenshots without a
// pointer. ⚠ Note what that does and does not buy: Radix PORTALS the tooltip content to
// document.body, and the suite snapshots `#storybook-root`, so the baseline captures the
// TRIGGER and not the bubble. The story still passes (the trigger is a visible child of the
// root, which is what the suite waits for) — but do not read the green tick as pixel coverage
// of the tooltip surface itself. Dialog hit the harder version of this and could not render at
// all; see its story.
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
