import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame, withRouter } from "@/test/story-utils";
import { CoverageMeter } from "./coverage-meter";

// How well the catalog covers a channel's breaks (§10 fallback ladder, V29b). Every number
// comes from `filler.Coverage`, which calls the same pools `Assemble` calls — the meter cannot
// disagree with what airs, and a Go test pins that.
// withRouter because the "Find clips" CTA is a TanStack Link, which needs a RouterProvider
// even in isolation.
const meta = {
  title: "Filler/CoverageMeter",
  component: CoverageMeter,
  decorators: [widthFrame(420), withRouter("/guide")],
} satisfies Meta<typeof CoverageMeter>;

type Story = StoryObj<typeof meta>;

// A healthy per-setting breakdown — nothing at zero, so the diagnosis panel stays hidden. Shared
// by the stories that are about the RUNGS rather than about the breakdown (§10 V51f).
const HEALTHY = [
  { criterion: "era" as const, clips: 9 },
  { criterion: "audience" as const, clips: 9 },
  { criterion: "category" as const, clips: 9 },
  { criterion: "kind" as const, clips: 9 },
  { criterion: "duration" as const, clips: 9 },
  { criterion: "quality" as const, clips: 9 },
];

// The healthy case: era- and audience-matched commercials are available.
const Exact: Story = {
  args: {
    coverage: {
      level: "exact",
      total: 9,
      criteria: HEALTHY,
      rungs: [
        { level: "exact", clips: 4 },
        { level: "widened", clips: 5 },
        { level: "audience", clips: 9 },
      ],
    },
  },
};

// Nothing in the exact year, so breaks fall to the decade. The tightest NON-EMPTY rung is the
// one highlighted — the ladder never widens further than it must. This is also where the
// "Find clips" CTA appears (F4): a widened ladder is the condition an operator can fix.
const Widened: Story = {
  args: {
    coverage: {
      level: "widened",
      total: 6,
      criteria: HEALTHY,
      rungs: [
        { level: "exact", clips: 0 },
        { level: "widened", clips: 2 },
        { level: "audience", clips: 6 },
      ],
    },
  },
};

// ⚠ **`EraStrict` (retired-ok) was DELETED here (§10 V51f), story and setting together.** It documented a rung
// being absent under the strict-era policy — a `filler.Policy` field that was set in tests and
// nowhere else: no settings key, no env var, no policy field, no way for an operator to reach it.
// A narrow era range is how a channel gets strictness now, and unlike the flag it is a control
// that appears on screen. Its replacement below is the state operators actually hit.

// The per-setting breakdown doing its job: the catalog is full of clips and ONE setting rules
// out all of them. "Nothing in the catalog fits" reads as "acquire more clips"; naming the
// audience says the catalog is fine and a setting is not.
const OneSettingIsEmpty: Story = {
  args: {
    coverage: {
      level: "bumper_card",
      total: 0,
      rungs: [],
      criteria: [
        { criterion: "era", clips: 214 },
        { criterion: "audience", clips: 0 },
        { criterion: "category", clips: 112 },
        { criterion: "kind", clips: 198 },
        { criterion: "duration", clips: 214 },
        { criterion: "quality", clips: 214 },
      ],
    },
  },
};

// The state an operator most needs named: breaks are running on the built-in card.
const NothingFits: Story = {
  args: { coverage: { level: "bumper_card", total: 0, rungs: [], criteria: HEALTHY } },
};

export default meta;
export { Exact, NothingFits, OneSettingIsEmpty, Widened };
