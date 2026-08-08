import type { GuideAiring } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { TimelineScrubber } from "./timeline-scrubber";

// A fixed "now" so the story is deterministic (the component defaults to Date.now()). The blocks are
// laid out around it: a programme in progress, a break, then the next programme.
const NOW = 1_700_000_000_000;
const min = (n: number) => n * 60_000;

const airings: GuideAiring[] = [
  {
    kind: "program",
    series: "The Simpsons",
    title: "Some Enchanted Evening",
    season: 1,
    episode: 13,
    startMs: NOW - min(8),
    stopMs: NOW + min(14),
  },
  { kind: "filler", title: "", startMs: NOW + min(14), stopMs: NOW + min(16) },
  {
    kind: "program",
    series: "The Simpsons",
    title: "Bart the Genius",
    season: 1,
    episode: 2,
    startMs: NOW + min(16),
    stopMs: NOW + min(38),
  },
];

const meta = {
  title: "UI/VideoPlayer/TimelineScrubber",
  component: TimelineScrubber,
  args: { airings, nowMs: NOW },
  // It sits on a dark scrim in the player; give the story room above for the hover card.
  decorators: [
    (Story) => (
      <div className="w-[600px] rounded-md bg-black p-6 pt-24">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof TimelineScrubber>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Empty: Story = { args: { airings: [] } };
