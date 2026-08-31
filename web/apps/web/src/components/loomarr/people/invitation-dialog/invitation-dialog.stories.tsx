import type { InvitationBody } from "@loomarr/api/models/invitationBody";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { InvitationDialog } from "./invitation-dialog";

const expiresAt = Date.UTC(2030, 0, 8, 12);
const invitation: InvitationBody = {
  id: "invitation-dorothy",
  kind: "local",
  username: "Dorothy",
  role: "member",
  status: "pending",
  createdAt: Date.UTC(2030, 0, 1, 12),
  expiresAt,
  contactAddress: { email: "dorothy@example.com", provenance: "admin", status: "pending" },
};
const deterministicGrant = {
  url: "https://loomarr.example/join#grant=storybook-only-deterministic-grant",
  expiresAt,
};

const meta = {
  title: "People/InvitationDialog",
  component: InvitationDialog,
  args: {
    candidates: [
      { id: "library-ada", name: "Ada Lovelace", imported: false, disabled: false, isAdmin: true },
      { id: "library-grace", name: "Grace Hopper", imported: true, disabled: false, isAdmin: false },
    ],
    defaultOpen: true,
    libraryAvailable: true,
    onCreate: async () => invitation,
    onIssueGrant: async () => deterministicGrant,
    onRevoke: async () => {},
    onSendEmail: async () => {},
  },
  decorators: [widthFrame(760)],
  parameters: { layout: "fullscreen" },
  render: (args) => (
    <div className="h-screen w-screen p-6">
      <InvitationDialog {...args} portalContainer={document.getElementById("storybook-root")} />
    </div>
  ),
} satisfies Meta<typeof InvitationDialog>;

type Story = StoryObj<typeof meta>;

const Editing: Story = {};
const EditingMobile: Story = { decorators: [widthFrame(390)] };
const Pending: Story = {
  args: { existing: invitation, onIssueGrant: async () => await new Promise(() => {}) },
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole("button", { name: "Generate QR and link" }));
  },
};
const Ready: Story = {
  args: { existing: invitation },
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole("button", { name: "Generate QR and link" }));
    await canvas.findByRole("img", { name: /Scan to accept/ });
  },
};
const ReadyMobile: Story = { ...Ready, decorators: [widthFrame(390)] };
const Copied: Story = {
  args: { existing: invitation },
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole("button", { name: "Generate QR and link" }));
    await userEvent.click(await canvas.findByRole("button", { name: "Copy invitation link" }));
    await canvas.findByRole("button", { name: "Copied" });
  },
};
const Expired: Story = { args: { existing: { ...invitation, status: "expired" } } };
const Revoked: Story = { args: { existing: { ...invitation, status: "revoked" } } };
const Problem: Story = {
  args: { existing: invitation, onIssueGrant: async () => await Promise.reject(new Error("Try again.")) },
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole("button", { name: "Generate QR and link" }));
    await canvas.findByRole("alert");
  },
};

export default meta;
export { Copied, Editing, EditingMobile, Expired, Pending, Problem, Ready, ReadyMobile, Revoked };
