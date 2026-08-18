import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { GenerationProgress } from "./generation-progress";

// The SSE suggester stepper (§3). Phases: reasoning/searching (one "Find the titles" step,
// alternating in a loop) → scoring → done, or failed.
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
// A slow run several passes in: the pass count and elapsed seconds are what distinguish
// "the model is still working" from "this is stuck", which is the whole reason they exist.
const SlowRun: Story = { args: { phase: "reasoning", round: 3, elapsedSeconds: 24 } };

export default meta;
export { Done, Failed, Reasoning, Scoring, Searching, SlowRun };
