import { Notice } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

const meta = {
  title: "Primitives/Notice",
  component: Notice,
  decorators: [
    (Story) => (
      <div style={{ boxSizing: "border-box", maxWidth: 640, padding: 48 }}>
        <Story />
      </div>
    ),
  ],
  args: {
    body: "Showing what Loomarr had a moment ago.",
    title: "Couldn't refresh the Guide",
  },
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof Notice>;

type Story = StoryObj<typeof meta>;

const Neutral: Story = { args: { tone: "neutral" } };
const Warn: Story = { args: { tone: "warn" } };
const WarnWithAction: Story = {
  args: { action: { label: "Try again", onPress: () => undefined }, tone: "warn" },
};
const NeutralWithAction: Story = {
  args: {
    action: { label: "Open Settings", onPress: () => undefined },
    body: "Your changes are saved on this device.",
    title: "You're working offline",
  },
};
const Success: Story = {
  args: {
    body: "Secrets, URL credentials, and local paths are redacted again.",
    title: undefined,
    tone: "success",
  },
};
const Danger: Story = {
  args: {
    body: "The report could not be built. Try again in a moment.",
    role: "alert",
    title: undefined,
    tone: "danger",
  },
};
const DangerWithTitle: Story = {
  args: { body: "Nothing was changed.", role: "alert", title: "Couldn't save", tone: "danger" },
};
const Light: Story = { ...WarnWithAction, globals: { theme: "light" } };
const LightDanger: Story = { ...Danger, globals: { theme: "light" } };

export default meta;
export {
  Danger,
  DangerWithTitle,
  Light,
  LightDanger,
  Neutral,
  NeutralWithAction,
  Success,
  Warn,
  WarnWithAction,
};
