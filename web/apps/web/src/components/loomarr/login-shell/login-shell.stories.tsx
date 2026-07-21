import type { Meta, StoryObj } from "@storybook/react-vite";
import { LoginForm } from "../login-form";
import { LoginShell } from "./login-shell";

const noop = () => {};

// The assembled sign-in PAGE (§11): brand hero + CRT static + the form in its card. This
// closes the gallery gap the component stories left — LoginForm alone snapshots the form,
// never the page framing, so a drift in the login identity (bars, wordmark, layout) would
// go unseen. Rendering a real LoginForm inside keeps the baseline honest.
const meta = {
  title: "Loomarr/LoginShell",
  component: LoginShell,
  parameters: { layout: "fullscreen" },
  args: {
    children: <LoginForm onSubmit={noop} />,
  },
} satisfies Meta<typeof LoginShell>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

export default meta;
export { Default };
