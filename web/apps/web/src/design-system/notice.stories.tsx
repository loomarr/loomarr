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
const Light: Story = { ...WarnWithAction, globals: { theme: "light" } };

export default meta;
export { Light, Neutral, NeutralWithAction, Warn, WarnWithAction };
