import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "../button";
import { StatusStrip, StatusStripAction } from "./status-strip";

// StatusStrip — Home's headline (#1659 web mock), in each state the mock models.
const meta = {
  title: "Primitives/StatusStrip",
  component: StatusStrip,
  args: { tone: "ok", title: "All 6 channels are playing" },
  decorators: [
    (Story) => (
      <div className="max-w-[1120px] p-6">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof StatusStrip>;

type Story = StoryObj<typeof meta>;

const requests = {
  id: "requests",
  tone: "attention" as const,
  title: "3 requests need you",
  action: <StatusStripAction primary>Review</StatusStripAction>,
};
const restart = {
  id: "restart",
  tone: "notice" as const,
  title: "Restart Loomarr to finish saving 2 settings",
  action: <StatusStripAction>Restart…</StatusStripAction>,
};

const AdminPlaying: Story = { args: { items: [requests, restart] } };

const AdminFailing: Story = {
  args: {
    tone: "error",
    title: "Something needs fixing",
    items: [
      {
        id: "services",
        tone: "error",
        title: "Two connected services aren't answering",
        action: <StatusStripAction primary>Fix</StatusStripAction>,
      },
      requests,
    ],
  },
};

const Member: Story = {
  args: {
    title: "Everything's playing",
    items: [
      {
        id: "failed",
        tone: "error",
        title: "1 of your requests couldn't be built",
        action: <StatusStripAction>Edit and retry</StatusStripAction>,
      },
    ],
  },
};

const Loading: Story = { args: { tone: "idle", loading: true, title: "Checking your channels…" } };

const Empty: Story = {
  args: {
    tone: "idle",
    title: "Nothing on air yet",
    action: <Button size="sm">Add your first channel</Button>,
  },
};

export default meta;
export { AdminFailing, AdminPlaying, Empty, Loading, Member };
