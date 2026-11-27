import { DemoSignIn } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { LoginForm } from "../login-form";
import { LoginShell } from "./login-shell";

const noop = () => {};

// The assembled sign-in PAGE (§11): brand hero + the form in its card. This closes the gallery gap
// the component stories left: LoginForm alone snapshots the form, never the page framing, so a
// drift in the login identity (bars, wordmark, layout) would go unseen. Rendering a real LoginForm
// inside keeps the baseline honest. The gallery's reduced motion leaves the idle snow out.
const meta = {
  title: "Setup/LoginShell",
  component: LoginShell,
  tags: ["route"],
  parameters: { layout: "fullscreen" },
  args: {
    children: <LoginForm onSubmit={noop} />,
  },
} satisfies Meta<typeof LoginShell>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

// A demo site (#2268): the server names a visitor account, so the card above the form signs in
// with it in one press. The form stays for the maintainer's own account.
const DemoSite: Story = {
  args: {
    children: (
      <>
        <DemoSignIn onSignIn={noop} username="guest" />
        <LoginForm onSubmit={noop} />
      </>
    ),
  },
};

export default meta;
export { Default, DemoSite };
