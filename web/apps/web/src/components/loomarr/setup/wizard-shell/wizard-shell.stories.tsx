import type { Meta, StoryObj } from "@storybook/react-vite";
import { WizardShell } from "./wizard-shell";

const noop = () => {};

const steps = [
  { id: "bootstrap", title: "Admin" },
  { id: "checklist", title: "Connections" },
  { id: "library", title: "Library" },
  { id: "users", title: "Users", optional: true },
  { id: "channel", title: "First channel" },
];

const body = <p className="text-muted-foreground text-sm">The step's form renders here.</p>;

// The operator first-run frame (§13): the rail is resume-safe — it renders whatever the
// caller derived from GET /v1/setup/status, so a refresh loses nothing.
const meta = {
  title: "Setup/WizardShell",
  component: WizardShell,
  parameters: { layout: "fullscreen" },
  args: {
    steps,
    currentId: "checklist",
    statusById: { bootstrap: "done", checklist: "current" },
    title: "Connect your services",
    description: "Loomarr live-tests each dependency. A red check tells you exactly what to fix.",
    children: body,
    onBack: noop,
    onNext: noop,
  },
} satisfies Meta<typeof WizardShell>;

type Story = StoryObj<typeof meta>;

const FirstStep: Story = {
  args: {
    currentId: "bootstrap",
    statusById: { bootstrap: "current" },
    title: "Create your admin account",
    description: "This account owns Loomarr. You can set it up before connecting a media server.",
    onBack: undefined,
    nextLabel: "Create admin",
  },
};

const MidFlow: Story = {};

const WithSkippable: Story = {
  args: {
    currentId: "users",
    statusById: {
      bootstrap: "done",
      checklist: "done",
      library: "done",
      users: "current",
    },
    title: "Import media-server users",
    description: "Only imported accounts can sign in. Skippable for a solo install.",
    onSkip: noop,
  },
};

const Busy: Story = { args: { busy: true, nextLabel: "Testing…" } };

export default meta;
export { Busy, FirstStep, MidFlow, WithSkippable };
