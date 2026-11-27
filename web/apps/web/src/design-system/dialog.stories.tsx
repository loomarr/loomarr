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
} from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

// The design-system Dialog, open. It portals into the story root rather than `document.body`, so
// the visual suite, which snapshots `#storybook-root`, sees the real popup and not a lookalike.
type DialogWorkshopProps = {
  /** A dialog that must be answered: no corner close, and Escape and a press outside do nothing. */
  dismissible?: boolean;
  layout?: "padded" | "sectioned";
  size?: "default" | "form" | "reading";
  spread?: boolean;
  /** The footer's actions stack at the panel's full width, as a phone's do. */
  stacked?: boolean;
};

const DialogWorkshop = ({
  dismissible = true,
  layout = "padded",
  size = "default",
  spread = false,
  stacked = false,
}: DialogWorkshopProps) => (
  <div style={{ height: "100vh", width: "100vw" }}>
    <Dialog defaultOpen dismissible={dismissible}>
      <DialogTrigger>
        <Action variant="outline">Edit task</Action>
      </DialogTrigger>
      <DialogContent layout={layout} portalContainer={document.getElementById("storybook-root")} size={size}>
        <DialogHeader>
          <DialogTitle>Modify Library scan</DialogTitle>
          <DialogDescription>How often this task runs.</DialogDescription>
        </DialogHeader>
        <p style={{ margin: 0, padding: layout === "sectioned" ? "0 24px" : 0 }}>
          Every 6 hours, starting at midnight.
        </p>
        <DialogFooter spread={spread} stacked={stacked}>
          {spread ? (
            <Action tone="danger" variant="outline">
              Remove task
            </Action>
          ) : null}
          {stacked ? (
            <>
              <Action>Save</Action>
              <DialogClose>
                <Action variant="outline">Cancel</Action>
              </DialogClose>
            </>
          ) : (
            <div style={{ display: "flex", gap: 8 }}>
              <DialogClose>
                <Action variant="outline">Cancel</Action>
              </DialogClose>
              <Action>Save</Action>
            </div>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
);

const meta = {
  title: "Loomarr Components/Dialog",
  component: DialogWorkshop,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof DialogWorkshop>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};
const FormWidth: Story = { args: { size: "form", spread: true } };
const Sectioned: Story = { args: { layout: "sectioned", size: "reading" } };
const MustBeAnswered: Story = { args: { dismissible: false } };
const Stacked: Story = { args: { stacked: true } };

export default meta;
export { Default, FormWidth, MustBeAnswered, Sectioned, Stacked };
