import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { RefineReview } from "./refine-review";

const noop = () => {};

const heat = { name: "Heat", year: 1995, mediaType: "movie", tmdbId: 949, inLibrary: true };
const pointBreak = { name: "Point Break", year: 1991, mediaType: "movie", tmdbId: 9426, inLibrary: true };
const predator = { name: "Predator", year: 1987, mediaType: "movie", tmdbId: 106, inLibrary: true };
const conAir = { name: "Con Air", year: 1997, mediaType: "movie", inLibrary: false };

const currentHeat = { name: "Heat", year: 1995, key: "movie:tmdb:949" };
const currentPointBreak = { name: "Point Break", year: 1991, key: "movie:tmdb:9426" };

// RefineReview — the diff a "Refine with AI" run lands on (§7 refine): kept/added/
// removed by provisioning key, acquisitions called out as needing a download, and the
// no-changes state that hides Apply entirely.
const meta = {
  title: "AI/RefineReview",
  component: RefineReview,
  args: { onApply: noop, onDiscard: noop },
  decorators: [widthFrame(560)],
} satisfies Meta<typeof RefineReview>;

type Story = StoryObj<typeof meta>;

// The common case: some kept, a new pick, one dropped, and a missing title to acquire.
const WithChanges: Story = {
  args: {
    proposed: [heat, predator],
    acquisitions: [conAir],
    current: [currentHeat, currentPointBreak],
  },
};

// The refine landed on exactly what's already playing — Apply has nothing to do, so it
// disappears rather than approve a no-op.
const NoChanges: Story = {
  args: {
    proposed: [heat, pointBreak],
    current: [currentHeat, currentPointBreak],
  },
};

// A brand-new channel with nothing on it yet — an empty current lineup is a real state
// (not "diff unavailable"), so everything proposed reads as Adding.
const EmptyChannel: Story = {
  args: {
    proposed: [heat, pointBreak],
    current: [],
  },
};

export default meta;
export { EmptyChannel, NoChanges, WithChanges };
