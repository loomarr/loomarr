import { cleanReel, compilationReel, guessedEraAsk, untaggedAsk } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame, withRouter } from "@/test/story-utils";
import { IncomingPanel } from "./incoming-panel";

// The ingest conveyor: what has been downloaded but is not yet filed (V35).
//
// ⚠ The "no confidence bar" note that stood here is RETIRED (V38): the tagger now records a
// GROUNDING-CAPPED score, so the bar renders a real measurement. The reason line stays beside it —
// a number says how sure, only the sentence says why.
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

// V38: the confidence meter, on the three bands the colour switches between. ⚠ The score is
// GROUNDING-CAPPED — the 40 here is what an ungrounded era gets no matter how certain the model
// claimed to be, which is why it can never reach the auto-file threshold.
const WithConfidence: Story = {
  args: {
    asks: [
      { ...untaggedAsk, path: "low.mp4", name: "Unidentified toy spot", confidence: 40 },
      { ...untaggedAsk, path: "mid.mp4", name: "Cereal ad", confidence: 72 },
      { ...guessedEraAsk, path: "high.mp4", name: "Frosted Flakes 1993", confidence: 92 },
    ],
    reels: [],
    onConfirmEra: () => {},
    onEditTags: () => {},
    onFile: () => {},
    onFileAllAsSuggested: () => {},
  },
};

// ⚠ THE audit half (§10 V38). Auto-filing is ON by default, so this is what an operator who did
// not ask for it sees — rendered even with an EMPTY queue, because "nothing needs you" and "here
// is what I did without asking" are different statements and the second one matters most on
// exactly the install where the first is true.
const FiledWithoutAsking: Story = {
  args: {
    asks: [],
    reels: [],
    recentlyFiled: [
      {
        ...untaggedAsk,
        path: "auto-1.mp4",
        name: "Hot Wheels spot",
        confidence: 86,
        autoFiled: true,
        reason: "Loomarr was confident enough about these tags to file it without asking.",
      },
      {
        ...untaggedAsk,
        path: "auto-2.mp4",
        name: "Station ident",
        confidence: 95,
        autoFiled: true,
        reason: "Loomarr was confident enough about these tags to file it without asking.",
      },
    ],
    onSendBack: () => {},
  },
};

export default meta;
export { BothAskKinds, CompilationsToReview, FiledWithoutAsking, NothingWaiting, OneRowBusy, WithConfidence };
