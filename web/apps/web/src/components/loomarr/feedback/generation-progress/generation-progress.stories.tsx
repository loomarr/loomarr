import type { ProposalJourneyProgressDTO } from "@loomarr/api/models/proposalJourneyProgressDTO";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { GenerationProgress } from "./generation-progress";

// One calm status follows the real backend stage without exposing the model's internal loop, and
// the titles stream in beneath it as they are chosen. elapsedSeconds is pinned so snapshots are
// deterministic; live surfaces count from the server's start time instead.
const meta = {
  title: "Feedback/GenerationProgress",
  component: GenerationProgress,
  decorators: [widthFrame(360)],
} satisfies Meta<typeof GenerationProgress>;

type Story = StoryObj<typeof meta>;

const run = (over: Partial<ProposalJourneyProgressDTO>): ProposalJourneyProgressDTO => ({
  stage: "choosing",
  terms: ["90s action"],
  picks: [],
  target: 8,
  startedAt: "2026-09-25T12:00:00Z",
  ...over,
});

const Searching: Story = { args: { phase: "searching" } };
const Reasoning: Story = { args: { phase: "reasoning" } };
const Scoring: Story = { args: { phase: "scoring" } };
const Done: Story = { args: { phase: "done" } };
// A slow run adds patient copy, not a ticking timer or internal pass count.
const SlowRun: Story = { args: { phase: "reasoning", round: 3, elapsedSeconds: 24 } };

// Generating, stage only: the request has been read and a search is running. No titles yet.
const StageOnly: Story = {
  args: {
    phase: "searching",
    elapsedSeconds: 4,
    progress: run({ stage: "searching", terms: ["speed", "action"] }),
  },
};

// Streaming: some picks have resolved against the catalog and appear as the model chooses them.
const Streaming: Story = {
  args: {
    phase: "reasoning",
    elapsedSeconds: 12,
    progress: run({
      picks: [
        { key: "movie:tmdb:100", mediaType: "movie", name: "Speed", year: 1994, inLibrary: false },
        { key: "movie:tmdb:603", mediaType: "movie", name: "The Matrix", year: 1999, inLibrary: true },
        { key: "series:tmdb:1396", mediaType: "series", name: "Breaking Bad", year: 2008, inLibrary: true },
        { key: "movie:tmdb:101", mediaType: "movie", name: "The Rock", year: 1996, inLibrary: false },
      ],
    }),
  },
};

// Building: every pick is in and the lineup is being assembled.
const Building: Story = {
  args: {
    phase: "scoring",
    elapsedSeconds: 31,
    progress: run({
      stage: "building",
      picks: [{ key: "movie:tmdb:100", mediaType: "movie", name: "Speed", year: 1994, inLibrary: false }],
    }),
  },
};

export default meta;
export { Building, Done, Reasoning, Scoring, Searching, SlowRun, StageOnly, Streaming };
