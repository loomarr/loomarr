import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "../button";
import { ListGroup, ListRow } from "./list-row";

// ListGroup + ListRow — state rows (#1659 web mock: On the way, Your requests).
const meta = {
  title: "Primitives/ListRow",
  component: ListRow,
  args: { tone: "progress", title: "A sci-fi anthology" },
  decorators: [
    (Story) => (
      <div className="max-w-[1120px] p-6">
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
          <Button size="sm" variant="outline">
            Watch
          </Button>
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
          <Button size="sm" variant="outline">
            Edit and retry
          </Button>
        }
      />
    </ListGroup>
  ),
};

export default meta;
export { Downloading, Requests };
