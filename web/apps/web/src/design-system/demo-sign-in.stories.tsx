import { DemoSignIn } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";

// The demo site's way in (#2268): the sign-in screen offers the account the server names, so a
// visitor looks around without hunting for a password.

const meta = {
  title: "Loomarr Components/Demo Sign In",
  component: DemoSignIn,
  decorators: [
    (Story) => (
      <div style={{ boxSizing: "border-box", maxWidth: 384, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
  args: { username: "guest", onSignIn: () => undefined },
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof DemoSignIn>;

type Story = StoryObj<typeof meta>;

const Ready: Story = {};
const SigningIn: Story = { args: { busy: true } };
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Light, Ready, SigningIn };
