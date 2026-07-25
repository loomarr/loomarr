import { guideNow, guidePod } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { GuideDetailCard } from "./guide-detail-card";

// Fixed times rather than indexing into the fixture channels: the card only needs A window,
// and reaching into `guideChannels[0].airings[1]` couples every story to that array's shape.
const START = guideNow;
const STOP = guideNow + 30 * 60_000;

// What a guide block actually IS (§12). Two shapes, because a break and a programme prompt
// different questions: "what is this and will it play?" vs "why are my commercials these ones?".
const meta = {
  title: "Loomarr/GuideDetailCard",
  component: GuideDetailCard,
  decorators: [widthFrame(360)],
} satisfies Meta<typeof GuideDetailCard>;

type Story = StoryObj<typeof meta>;

// A film: year, rating, genre, runtime, description.
const Movie: Story = {
  args: {
    airing: {
      kind: "program",
      title: "Heat",
      startMs: START,
      stopMs: STOP,
      year: 1995,
      rating: "R",
      genres: ["Action", "Crime"],
      runtimeMs: 170 * 60_000,
      description: "A crew of professional thieves works a string of scores in Los Angeles.",
      provenance: "in library",
    },
  },
};

// An episode carries BOTH names plus its numbering — either alone says less than the block did.
const Episode: Story = {
  args: {
    airing: {
      kind: "program",
      title: "Bart the Mother",
      series: "The Simpsons",
      season: 10,
      episode: 3,
      startMs: START,
      stopMs: STOP,
      year: 1998,
      rating: "TV-PG",
      genres: ["Animation", "Comedy"],
      runtimeMs: 22 * 60_000,
      provenance: "in library",
    },
  },
};

// THE BREAK CASE — impossible before V13b, when the API had only a channel-wide pool and
// nothing could say what plays in THIS break.
const Break: Story = {
  args: {
    airing: {
      kind: "filler",
      title: "Commercials",
      startMs: START,
      stopMs: STOP,
      pod: guidePod,
    },
  },
};

// The ladder had to widen — the answer to "why are my commercials wrong", stated in words.
const BreakEraWidened: Story = {
  args: {
    airing: {
      kind: "filler",
      title: "Commercials",
      startMs: START,
      stopMs: STOP,
      pod: { ...guidePod, matchLevel: "widened" },
    },
  },
};

// Nothing matched: the embedded card stands in rather than dead air (§10's floor).
const BreakBumperCardOnly: Story = {
  args: {
    airing: {
      kind: "filler",
      title: "Commercials",
      startMs: START,
      stopMs: STOP,
      pod: {
        matchLevel: "bumper_card",
        totalMs: 5_000,
        entries: [
          { name: "Loomarr — We'll be right back", kind: "bumper", durationMs: 5_000, isFallbackCard: true },
        ],
      },
    },
  },
};

// A slot still acquiring: the times are an estimate, and provenance carries the real status.
const PendingSlot: Story = {
  args: {
    airing: {
      kind: "pending",
      title: "Star Trek: First Contact",
      nominal: true,
      startMs: START,
      stopMs: STOP,
      provenance: "acquiring · 62% · 8m left",
    },
  },
};

export default meta;
export { Break, BreakBumperCardOnly, BreakEraWidened, Episode, Movie, PendingSlot };
