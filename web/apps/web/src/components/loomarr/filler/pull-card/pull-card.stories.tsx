import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { PullCard } from "./pull-card";

// A proposed filler acquisition awaiting a human (V35) — the approval gate's face.
//
// §10 has said "the machine proposes, a human commits" since the starter pack shipped; until
// V35 there was no object to commit. Nothing downloads while this card sits in the queue.
const meta = {
  title: "Filler/PullCard",
  component: PullCard,
  decorators: [widthFrame(720)],
} satisfies Meta<typeof PullCard>;

type Story = StoryObj<typeof meta>;

// Declared separately so a single-source story can reference it without indexing into
// `base.plan` — under noUncheckedIndexedAccess that yields `Row | undefined`, which the build's
// tsc rejects even though the typecheck pass let it through.
const classicRow = {
  sourceId: "classic",
  tag: "archive",
  name: "Classic TV commercials",
  why: "A source you added and left switched on.",
  estimateClips: 40,
  dropped: false,
};

const base = {
  id: "pull_1",
  title: "Top up the 1990s",
  reason: "Saturday Mornings falls back to bumpers, because nothing in the catalog matches its era.",
  proposedBy: "ada",
  status: "pending" as const,
  estimateClips: 52,
  createdAt: "2026-08-01T12:00:00Z",
  plan: [
    classicRow,
    {
      sourceId: "psa",
      tag: "archive",
      name: "Public service announcements",
      why: "A source you added and left switched on.",
      estimateClips: 12,
      dropped: false,
    },
  ],
};

const Pending: Story = {
  args: { pull: base, onApprove: () => {}, onDismiss: () => {} },
};

// ⚠ Estimates carry no number here, and the card simply omits the figure rather than showing a
// zero. The composer reports 0 when it has not measured a source, and "~0 clips" would be a
// forecast that reads as a prediction of nothing.
const NoEstimates: Story = {
  args: {
    pull: { ...base, estimateClips: 0, plan: base.plan.map((row) => ({ ...row, estimateClips: 0 })) },
    onApprove: () => {},
    onDismiss: () => {},
  },
};

// A single source, which is the ordinary shape on a small install.
const OneSource: Story = {
  args: { pull: { ...base, plan: [classicRow] }, onApprove: () => {}, onDismiss: () => {} },
};

// Mid-decision: the approve button says what it is doing rather than looking inert.
const Deciding: Story = {
  args: { pull: base, deciding: true, onApprove: () => {}, onDismiss: () => {} },
};

export default meta;
export { Deciding, NoEstimates, OneSource, Pending };
