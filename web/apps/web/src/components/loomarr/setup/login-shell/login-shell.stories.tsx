import type { Meta, StoryObj } from "@storybook/react-vite";
import { LoginForm } from "../login-form";
import { LoginShell } from "./login-shell";

const noop = () => {};

// The assembled sign-in PAGE (§11): brand hero + CRT static + the form in its card. This
// closes the gallery gap the component stories left — LoginForm alone snapshots the form,
// never the page framing, so a drift in the login identity (bars, wordmark, layout) would
// go unseen. Rendering a real LoginForm inside keeps the baseline honest. Tagged `motion` so the
// gallery shoots it with motion allowed: reduced motion hides the idle snow, and this is the page
// the snow belongs to (its seeded tile, frozen at frame 0, is the same every run).
const meta = {
  title: "Setup/LoginShell",
  component: LoginShell,
  tags: ["route", "motion"],
  parameters: { layout: "fullscreen" },
  args: {
    children: <LoginForm onSubmit={noop} />,
  },
} satisfies Meta<typeof LoginShell>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

export default meta;
export { Default };
