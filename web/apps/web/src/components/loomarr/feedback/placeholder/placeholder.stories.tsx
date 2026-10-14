import type { Meta, StoryObj } from "@storybook/react-vite";
import { Placeholder } from "./placeholder";

// The "dead air" empty pattern (§6) every unbuilt screen shows until its real surface
// lands (13.4): one idle, on-theme message with a single next action in the hint.
const meta = {
  title: "Feedback/Placeholder",
  component: Placeholder,
  parameters: { layout: "fullscreen" },
  args: { title: "Channels", hint: "No channels yet — create your first from an intent." },
} satisfies Meta<typeof Placeholder>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};

export default meta;
export { Default };
