import type { Meta, StoryObj } from "@storybook/react-vite";
import { LoginShell } from "@/components/loomarr/setup/login-shell";
import { InvitationJoin } from "./invitation-join";

const meta = {
  title: "People/Invitation join",
  component: InvitationJoin,
  args: {
    preview: {
      kind: "local",
      username: "Ada",
      role: "member",
      credentialPath: "local_password",
      expiresAt: Date.UTC(2030, 2, 24, 13, 30),
    },
    onRedeem: () => undefined,
  },
  parameters: { layout: "fullscreen" },
  decorators: [
    (Story) => (
      <LoginShell className="py-10">
        <Story />
      </LoginShell>
    ),
  ],
} satisfies Meta<typeof InvitationJoin>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Local: Story = {};

export const LibraryAdmin: Story = {
  args: {
    preview: {
      kind: "library",
      displayName: "Grace Hopper",
      role: "admin",
      credentialPath: "library_password",
      expiresAt: Date.UTC(2030, 2, 24, 13, 30),
    },
  },
};
