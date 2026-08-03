import { taggedClip, thumbnailedClip, untaggedClip } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { ClipRow } from "./clip-row";

const noop = () => {};

// The catalog's dense LIST row (V35b, the mock's `catListView`) — the alternative to the card
// grid. Scanning and bulk-selecting, not acting on one clip.
const meta = {
  title: "Filler/ClipRow",
  component: ClipRow,
  // Wide enough for every column to render at the `lg:` step. The visual suite's mobile
  // viewport is what exercises the collapse — see the note at the bottom of this file.
  decorators: [widthFrame(900)],
} satisfies Meta<typeof ClipRow>;

type Story = StoryObj<typeof meta>;

const Tagged: Story = { args: { clip: taggedClip } };

// ⚠ The thumbnail column holds its 54×30 box either way — in a LIST an absent frame must not
// ragged the name column against its neighbours, unlike the card where the image is simply
// omitted. These two stories side by side are what make that visible.
const WithThumbnail: Story = { args: { clip: thumbnailedClip } };

// An untagged clip reads as em-dashes in the tag columns rather than blanks, so a missing era
// is distinguishable from a column that failed to render.
const Untagged: Story = { args: { clip: untaggedClip } };

// Selection is what the list view is FOR. `onToggleSelect` is what makes the row selectable at
// all — a member gets the row with no checkbox and no dead control.
const Selectable: Story = { args: { clip: thumbnailedClip, onToggleSelect: noop } };
const Selected: Story = { args: { clip: thumbnailedClip, onToggleSelect: noop, selected: true } };

// A long name must ELLIPSISE rather than push the tag columns off the row — the name is the
// column that must never be the one squeezed out. ⚠ This story exists because the first draft
// got it wrong (a grid item's default `min-width:auto` refuses to shrink below its content) and
// every assertion passed while the name was crushed to nothing. Only the baseline image showed it.
const LongName: Story = {
  args: { clip: { ...thumbnailedClip, name: `${thumbnailedClip.name} — extended broadcast cut` } },
};

export default meta;
export { LongName, Selectable, Selected, Tagged, Untagged, WithThumbnail };

// ⚠ There is deliberately NO "narrow" story. The row's responsive steps are `md:`/`lg:`, which
// Tailwind resolves against the VIEWPORT — a `widthFrame(360)` decorator narrows the container
// and changes nothing about which columns render, so such a story would draw a squashed
// desktop row and look like proof of a mobile layout it never exercised. The visual suite
// already runs every story at a mobile viewport, which is what actually tests the collapse.
