import {
  Action,
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  Screen,
  Text,
} from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-native";

// The design-system Dialog on a device: a React Native Modal, closed by its corner button, Cancel,
// the scrim, or Back.
const NativeDialogWorkshop = ({ density = "touch" }: { density?: "touch" | "tv" }) => (
  <Screen density={density}>
    <Dialog defaultOpen>
      <DialogTrigger>
        <Action density={density} tone="secondary">
          Edit task
        </Action>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Modify Library scan</DialogTitle>
          <DialogDescription>How often this task runs.</DialogDescription>
        </DialogHeader>
        <Text density={density} textRole="body">
          Every 6 hours, starting at midnight.
        </Text>
        <DialogFooter>
          <DialogClose>
            <Action density={density} tone="secondary">
              Cancel
            </Action>
          </DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </Screen>
);

const meta = {
  title: "Loomarr Components/Dialog",
  component: NativeDialogWorkshop,
  args: { density: "touch" },
} satisfies Meta<typeof NativeDialogWorkshop>;

type Story = StoryObj<typeof meta>;
const Touch: Story = {};
const Tv: Story = { args: { density: "tv" } };
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Light, Touch, Tv };
