import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { GenerationProgress } from "./generation-progress";

// The SSE suggester stepper (§3): searching · reasoning · scoring · done · failed.
const meta = {
  title: "Feedback/GenerationProgress",
  component: GenerationProgress,
  decorators: [widthFrame(320)],
} satisfies Meta<typeof GenerationProgress>;

type Story = StoryObj<typeof meta>;

const Searching: Story = { args: { phase: "searching" } };
const Reasoning: Story = { args: { phase: "reasoning" } };
const Scoring: Story = { args: { phase: "scoring" } };
const Done: Story = { args: { phase: "done" } };
const Failed: Story = {
  args: { phase: "failed", error: "The model returned no grounded candidates. Try a broader intent." },
};

export default meta;
export { Done, Failed, Reasoning, Scoring, Searching };
