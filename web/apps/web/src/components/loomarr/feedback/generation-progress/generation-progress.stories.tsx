import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { GenerationProgress } from "./generation-progress";

// One calm status follows the SSE phase without exposing the model's internal loop.
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
// A slow run adds patient copy, not a ticking timer or internal pass count.
const SlowRun: Story = { args: { phase: "reasoning", round: 3, elapsedSeconds: 24 } };

export default meta;
export { Done, Reasoning, Scoring, Searching, SlowRun };
