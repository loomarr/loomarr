import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { CoverageMeter } from "./coverage-meter";

// How well the catalog covers a channel's breaks (§10 fallback ladder, V29b). Every number
// comes from `filler.Coverage`, which calls the same pools `Assemble` calls — the meter cannot
// disagree with what airs, and a Go test pins that.
const meta = {
  title: "Filler/CoverageMeter",
  component: CoverageMeter,
  decorators: [widthFrame(420)],
} satisfies Meta<typeof CoverageMeter>;

type Story = StoryObj<typeof meta>;

// The healthy case: era- and audience-matched commercials are available.
const Exact: Story = {
  args: {
    coverage: {
      level: "exact",
      total: 9,
      rungs: [
        { level: "exact", clips: 4 },
        { level: "widened", clips: 5 },
        { level: "audience", clips: 9 },
      ],
    },
  },
};

// Nothing in the exact year, so breaks fall to the decade. The tightest NON-EMPTY rung is the
// one highlighted — the ladder never widens further than it must.
const Widened: Story = {
  args: {
    coverage: {
      level: "widened",
      total: 6,
      rungs: [
        { level: "exact", clips: 0 },
        { level: "widened", clips: 2 },
        { level: "audience", clips: 6 },
      ],
    },
  },
};

// ⚠ Under the strict-era setting there is no widened rung at all — it is ABSENT, not zero.
// A 0 row would read as a catalog gap rather than a setting the operator chose.
const EraStrict: Story = {
  args: {
    coverage: {
      level: "exact",
      total: 7,
      rungs: [
        { level: "exact", clips: 3 },
        { level: "audience", clips: 7 },
      ],
    },
  },
};

// The state an operator most needs named: breaks are running on the built-in card.
const NothingFits: Story = { args: { coverage: { level: "bumper_card", total: 0, rungs: [] } } };

export default meta;
export { EraStrict, Exact, NothingFits, Widened };
