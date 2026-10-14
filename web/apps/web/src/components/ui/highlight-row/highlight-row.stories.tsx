import type { Meta, StoryObj } from "@storybook/react-vite";
import { ListGroup } from "../list-row";
import { HighlightRow } from "./highlight-row";

// HighlightRow — Tonight's highlights (#1659 web mock).
const meta = {
  title: "Primitives/HighlightRow",
  component: HighlightRow,
  args: { time: "9:04 PM", channelNumber: "07", title: "A sci-fi drama" },
  decorators: [
    (Story) => (
      <div className="max-w-[1120px] p-6">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof HighlightRow>;

type Story = StoryObj<typeof meta>;

const Tonight: Story = {
  render: () => (
    <ListGroup aria-label="Tonight">
      <HighlightRow
        time="9:04 PM"
        channelNumber="07"
        title="A sci-fi drama"
        detail=" · Pilot"
        reason="Season 4 premiere"
      />
      <HighlightRow
        time="9:30 PM"
        channelNumber="42"
        title="A bar sitcom"
        detail=" · A late episode"
        reason="Back-to-back until 11"
      />
      <HighlightRow time="10:04 PM" channelNumber="07" title="A space horror film" reason="Movie · 1979" />
      <HighlightRow
        time="11:00 PM"
        channelNumber="22"
        title="A cult horror film"
        reason="New on the autumn horror channel"
      />
    </ListGroup>
  ),
};

// A long title truncates in its column; the reason never wraps under it.
const LongTitle: Story = {
  render: () => (
    <ListGroup aria-label="Tonight">
      <HighlightRow
        time="9:04 PM"
        channelNumber="07"
        title="A sci-fi drama with an unusually long title that has to give way"
        detail=" · An episode name that is also long"
        reason="Season 4 premiere"
      />
    </ListGroup>
  ),
};

export default meta;
export { LongTitle, Tonight };
