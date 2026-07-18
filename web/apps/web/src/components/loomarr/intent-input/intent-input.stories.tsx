import { intentTemplates, sampleIntent } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { IntentInput } from "./intent-input";

const noop = () => {};

// The hero (§3): empty · template-filled · submitting. (Focused — the magenta ring — is
// an interaction state; the visual suite captures it via a focus step, not a static arg.)
const meta = {
  title: "Loomarr/IntentInput",
  component: IntentInput,
  args: { onValueChange: noop, onSubmit: noop, templates: intentTemplates },
  decorators: [widthFrame(560)],
} satisfies Meta<typeof IntentInput>;

type Story = StoryObj<typeof meta>;

const Empty: Story = { args: { value: "" } };
const TemplateFilled: Story = { args: { value: sampleIntent } };
const Submitting: Story = { args: { value: "90s action movies, high energy", submitting: true } };

export default meta;
export { Empty, Submitting, TemplateFilled };
