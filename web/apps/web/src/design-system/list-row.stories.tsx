import { Action } from "@loomarr/design-system";
import { HighlightRow, ListGroup, ListRow } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";

// ListGroup + ListRow — state rows (#1659 web mock: On the way, Your requests) — and HighlightRow,
// Tonight's highlights, which share the group.
const meta = {
  title: "Loomarr Components/ListRow",
  component: ListRow,
  args: { tone: "progress", title: "A sci-fi anthology" },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 1120, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof ListRow>;

type Story = StoryObj<typeof meta>;

// The admin's On the way: downloads with a bar and an eta.
const Downloading: Story = {
  render: () => (
    <ListGroup aria-label="On the way">
      <ListRow
        tone="progress"
        title="A sci-fi anthology"
        sub="For the late-night sci-fi channel · 8 of 36 episodes"
        progress={{ value: 22, label: "Downloading", eta: "about 40 min" }}
      />
      <ListRow
        tone="progress"
        title="A horror anthology"
        sub="For the autumn horror channel"
        progress={{ value: 64, label: "Downloading", eta: "about 10 min" }}
      />
      <ListRow
        tone="progress"
        title="A cartoon series"
        sub="For the Saturday cartoons channel · waiting for a download slot"
        progress={{ label: "Queued", eta: "queued" }}
      />
    </ListGroup>
  ),
};

// The member's Your requests: each row's state, and an action where there is one to take.
const Requests: Story = {
  render: () => (
    <ListGroup aria-label="Your requests">
      <ListRow tone="attention" title="A mystery channel" sub="Waiting for an admin to approve" />
      <ListRow
        tone="ok"
        title="An autumn horror channel"
        sub="On its channel · 22"
        action={
          <Action density="pointer" onPress={() => {}} variant="outline">
            Watch
          </Action>
        }
      />
      <ListRow
        tone="progress"
        title="Late-night talk shows"
        sub="Loomarr is finding titles · requested today"
      />
      <ListRow
        tone="error"
        title="Game shows with the original hosts"
        sub="Couldn't be built: only 2 titles found"
        action={
          <Action density="pointer" onPress={() => {}} variant="outline">
            Edit and retry
          </Action>
        }
      />
    </ListGroup>
  ),
};

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
const TonightLongTitle: Story = {
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
export { Downloading, Requests, Tonight, TonightLongTitle };
