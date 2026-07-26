import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "../button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "./dialog";

// Dialog — confirmations and short focused flows. Forced `open` for the same reason the tooltip
// is: a closed dialog snapshots as nothing.
const meta = {
  title: "Primitives/Dialog",
  component: Dialog,
} satisfies Meta<typeof Dialog>;

type Story = StoryObj<typeof meta>;

const Default: Story = {
  render: () => (
    <Dialog open>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete Springfield Classics?</DialogTitle>
          <DialogDescription>
            The channel stops broadcasting immediately. Acquired titles stay in your library.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter className="gap-2">
          <Button variant="outline">Cancel</Button>
          <Button variant="destructive">Delete channel</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  ),
};

export default meta;
export { Default };
