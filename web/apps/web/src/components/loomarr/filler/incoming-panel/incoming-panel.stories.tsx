import { cleanReel, compilationReel, guessedEraAsk, untaggedAsk } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame, withRouter } from "@/test/story-utils";
import { IncomingPanel } from "./incoming-panel";

// The ingest conveyor: what has been downloaded but is not yet filed (V35).
//
// ⚠ No confidence bar. The mock draws one per row; the tagger records neither a score nor a
// rationale, so a bar here would be a number no code produced. Each row shows the REASON it is
// waiting, which the server derives from real state.
//
// withRouter because a compilation row links to the split-review route. Args come from
// `@loomarr/fixtures` per frontend-design §5.1b.
const meta = {
  title: "Filler/IncomingPanel",
  component: IncomingPanel,
  decorators: [widthFrame(760), withRouter("/filler")],
} satisfies Meta<typeof IncomingPanel>;

type Story = StoryObj<typeof meta>;

// The two ask kinds side by side. They are different QUESTIONS, which is why the buttons differ:
// a guessed era has a proposed answer to confirm, an untagged clip has nothing to confirm.
const BothAskKinds: Story = {
  args: {
    asks: [guessedEraAsk, untaggedAsk],
    reels: [],
    onConfirmEra: () => {},
    onEditTags: () => {},
    onDismiss: () => {},
  },
};

// Compilations mid-split. The count of segments needing a look is shown BEFORE the review is
// opened, because twelve clean segments and twelve with three problems are different jobs.
const CompilationsToReview: Story = {
  args: { asks: [], reels: [compilationReel, cleanReel] },
};

// One row writing. Only that row disables — a page that greys out entirely while a single
// confirm lands reads as having frozen.
const OneRowBusy: Story = {
  args: {
    asks: [guessedEraAsk, untaggedAsk],
    reels: [],
    busyPath: guessedEraAsk.path,
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
