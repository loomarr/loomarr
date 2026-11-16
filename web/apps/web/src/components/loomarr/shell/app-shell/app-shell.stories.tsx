import { Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { withRouter } from "@/test/story-utils";
import { AppShell } from "./app-shell";

const noop = () => {};

const body = (
  <div className="p-6">
    <Text as="h1" textRole="title">
      Channels
    </Text>
    <Text as="p" className="mt-1" textRole="compact" tone="secondary">
      The broadcast console frame.
    </Text>
  </div>
);

// The console frame (§3): admin (Users + Settings visible) · member (they're hidden).
// The "mobile-web collapsed" state is covered by the mobile viewport in the visual suite.
const meta = {
  title: "Shell/AppShell",
  component: AppShell,
  tags: ["route"],
  parameters: { layout: "fullscreen" },
  decorators: [withRouter("/guide")],
  args: { onOpenCommand: noop, onLogout: noop, children: body },
} satisfies Meta<typeof AppShell>;

type Story = StoryObj<typeof meta>;

const Admin: Story = { args: { isAdmin: true, userName: "Adela Hale" } };
const Member: Story = { args: { isAdmin: false, userName: "Guest Viewer" } };

// The Requests entry carrying its "needs you" count (#1405).
const RequestsBadge: Story = {
  args: { isAdmin: true, userName: "Adela Hale", badges: { "/requests": 3 } },
};

export default meta;
export { Admin, Member, RequestsBadge };
