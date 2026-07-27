import type { Meta, StoryObj } from "@storybook/react-vite";
import { withRouter } from "@/test/story-utils";
import { AppShell } from "./app-shell";

const noop = () => {};

const body = (
  <div className="p-6">
    <h1 className="font-semibold text-xl">Channels</h1>
    <p className="mt-1 text-muted-foreground text-sm">The broadcast console frame.</p>
  </div>
);

// The console frame (§3): admin (Users + Settings visible) · member (they're hidden).
// The "mobile-web collapsed" state is covered by the mobile viewport in the visual suite.
const meta = {
  title: "Shell/AppShell",
  component: AppShell,
  parameters: { layout: "fullscreen" },
  decorators: [withRouter("/guide")],
  args: { onOpenCommand: noop, onLogout: noop, children: body },
} satisfies Meta<typeof AppShell>;

type Story = StoryObj<typeof meta>;

const Admin: Story = { args: { isAdmin: true, userName: "Ada Lovelace" } };
const Member: Story = { args: { isAdmin: false, userName: "Guest Viewer" } };

export default meta;
export { Admin, Member };
