import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "../button";
import { Dialog, DialogTrigger } from "./dialog";

// Dialog — confirmations and short focused flows.
//
// ⚠ THIS STORY SHOWS THE TRIGGER, NOT AN OPEN DIALOG, and that is a constraint of the visual
// suite rather than a judgement about what is worth seeing. Radix portals dialog content to
// `document.body`, while the suite snapshots `#storybook-root` and waits for a visible child of
// it — so a forced-open dialog hangs that wait until the test times out. Which is exactly what
// happened when this story first shipped with `<Dialog open>`.
//
// The dialog's own surface is covered where it really appears: ChannelDangerZone and
// PinClipDialog have their own stories. Reproducing the content here with unportalled markup
// would snapshot a lookalike rather than the component, which is worse than not snapshotting it.
const meta = {
  title: "Primitives/Dialog",
  component: Dialog,
} satisfies Meta<typeof Dialog>;

type Story = StoryObj<typeof meta>;

const Default: Story = {
  render: () => (
    <Dialog>
      <DialogTrigger render={<Button variant="destructive" />}>Delete channel</DialogTrigger>
    </Dialog>
  ),
};

export default meta;
export { Default };
