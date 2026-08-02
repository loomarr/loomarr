import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame, withRouter } from "@/test/story-utils";
import { IncomingPanel } from "./incoming-panel";

// The ingest conveyor: what has been downloaded but is not yet filed (V35).
//
// ⚠ No confidence bar. The mock draws one per row; the tagger records neither a score nor a
// rationale, so a bar here would be a number no code produced. Each row shows the REASON it is
// waiting, which the server derives from real state.
// withRouter because a compilation row links to the split-review route.
const meta = {
  title: "Filler/IncomingPanel",
  component: IncomingPanel,
  decorators: [widthFrame(760), withRouter("/filler")],
} satisfies Meta<typeof IncomingPanel>;

type Story = StoryObj<typeof meta>;

const guessed = {
  path: "1988/toys.mp4",
  name: "Transformers holiday spot",
  from: "archive",
  durationMs: 30_000,
  kind: "commercial",
  audience: "kids",
  category: "toys",
  suggestedEra: 1988,
  reason: "The year isn't written anywhere in this clip's name or description, so Loomarr guessed it.",
};

const untagged = {
  path: "mystery.mp4",
  name: "mystery.mp4",
  durationMs: 25_000,
  kind: "commercial",
  reason: "Loomarr couldn't work out what this is, so it will only match broadly.",
};

// The two ask kinds side by side. They are different QUESTIONS, which is why the buttons differ:
// a guessed era has a proposed answer to confirm, an untagged clip has nothing to confirm.
const BothAskKinds: Story = {
  args: {
    asks: [guessed, untagged],
    reels: [],
    onConfirmEra: () => {},
    onEditTags: () => {},
    onDismiss: () => {},
  },
};

// A compilation mid-split. The count of segments needing a look is shown BEFORE the review is
// opened, because twelve clean segments and twelve with three problems are different jobs.
const CompilationsToReview: Story = {
  args: {
    asks: [],
    reels: [
      {
        proposalId: "sp_1",
        clipPath: "comps/1987-saturday.mp4",
        segments: 12,
        needsAttention: 3,
        createdAt: "2026-08-01T12:00:00Z",
      },
      {
        proposalId: "sp_2",
        clipPath: "comps/1993-toys.mp4",
        segments: 8,
        needsAttention: 0,
        createdAt: "2026-08-01T13:00:00Z",
      },
    ],
  },
};

// One row writing. Only that row disables — a page that greys out entirely while a single
// confirm lands reads as having frozen.
const OneRowBusy: Story = {
  args: {
    asks: [guessed, untagged],
    reels: [],
    busyPath: guessed.path,
    onConfirmEra: () => {},
    onEditTags: () => {},
  },
};

// The all-clear. Worth a story of its own: it is the state an operator should see most of the
// time, and it has to read as success rather than as an empty page.
const NothingWaiting: Story = {
  args: { asks: [], reels: [] },
};

export default meta;
export { BothAskKinds, CompilationsToReview, NothingWaiting, OneRowBusy };
