import type { Meta, StoryObj } from "@storybook/react-vite";
import { LoginShell } from "../login-shell";
import { PasswordRecoveryRequest, PasswordRecoveryReset } from "./password-recovery";

const meta = {
  title: "Setup/Password recovery",
  component: PasswordRecoveryRequest,
  parameters: { layout: "fullscreen" },
  decorators: [
    (Story) => (
      <LoginShell className="py-10">
        <Story />
      </LoginShell>
    ),
  ],
  args: { onSubmit: () => undefined },
} satisfies Meta<typeof PasswordRecoveryRequest>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Request: Story = {};
export const Requested: Story = { args: { sent: true } };
export const Reset: Story = {
  render: () => (
    <PasswordRecoveryReset expiresAt={Date.UTC(2030, 2, 24, 13, 30)} onRedeem={() => undefined} />
  ),
};
export const ResetComplete: Story = {
  render: () => <PasswordRecoveryReset succeeded onRedeem={() => undefined} />,
};
