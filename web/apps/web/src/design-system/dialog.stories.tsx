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
  layout?: "padded" | "sectioned";
  size?: "default" | "form" | "reading";
  spread?: boolean;
};

const DialogWorkshop = ({ layout = "padded", size = "default", spread = false }: DialogWorkshopProps) => (
  <div style={{ height: "100vh", width: "100vw" }}>
    <Dialog defaultOpen>
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
        <DialogFooter spread={spread}>
          {spread ? (
            <Action tone="danger" variant="outline">
              Remove task
            </Action>
          ) : null}
          <div style={{ display: "flex", gap: 8 }}>
            <DialogClose>
              <Action variant="outline">Cancel</Action>
            </DialogClose>
            <Action>Save</Action>
          </div>
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

export default meta;
export { Default, FormWidth, Sectioned };
