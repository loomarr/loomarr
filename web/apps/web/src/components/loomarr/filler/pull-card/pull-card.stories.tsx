import { pendingPull } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { PullCard } from "./pull-card";

// A proposed filler acquisition awaiting a human (V35) — the approval gate's face.
//
// §10 has said "the machine proposes, a human commits" since the starter pack shipped; until
// V35 there was no object to commit. Nothing downloads while this card sits in the queue.
//
// Args come from `@loomarr/fixtures` per frontend-design §5.1b.
const meta = {
  title: "Filler/PullCard",
  component: PullCard,
  decorators: [widthFrame(720)],
} satisfies Meta<typeof PullCard>;

type Story = StoryObj<typeof meta>;

const plan = pendingPull.plan ?? [];

const Pending: Story = {
  args: { pull: pendingPull, onApprove: () => {}, onDismiss: () => {} },
};

// ⚠ The composer reports 0 where it has measured nothing, and the card OMITS the figure rather
// than rendering "~0 clips" — a forecast of nothing reads as a prediction, not an absence.
const NoEstimates: Story = {
  args: {
    pull: {
      ...pendingPull,
      estimateClips: 0,
      plan: plan.map((row) => ({ ...row, estimateClips: 0 })),
    },
    onApprove: () => {},
    onDismiss: () => {},
  },
};

// A single source, which is the ordinary shape on a small install.
const OneSource: Story = {
  args: { pull: { ...pendingPull, plan: plan.slice(0, 1) }, onApprove: () => {}, onDismiss: () => {} },
};

// Mid-decision: the approve button says what it is doing rather than looking inert.
const Deciding: Story = {
  args: { pull: pendingPull, deciding: true, onApprove: () => {}, onDismiss: () => {} },
};

export default meta;
export { Deciding, NoEstimates, OneSource, Pending };
