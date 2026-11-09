import { layoutGuide } from "@loomarr/core/guide";
import { guideChannels, guideFrom, guideNow, guideTo } from "@loomarr/fixtures";
import type { NeedsLineItem } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { withRouter } from "@/test/story-utils";
import { ChannelWall } from "./channel-wall";

// Home's channel wall (#2764) with the admin's "Needs you" line above the tiles (#2785, Direction
// mock layout B). The line lists only the counts above zero and is gone when there are none; a
// member never gets it. The fixture's thumbnail URLs are dropped so every tile draws its generated
// art: a story never reaches for an image.
const layout = layoutGuide(
  {
    channels: guideChannels.map((channel) => ({
      ...channel,
      airings: channel.airings.map(({ thumbUrl: _thumb, ...airing }) => airing),
    })),
    fromMs: guideFrom,
    timezone: "America/New_York",
    toMs: guideTo,
  },
  guideNow,
);

const needs: NeedsLineItem[] = [
  {
    id: "approvals",
    value: "3",
    label: "requests to approve",
    short: "to approve",
    link: <a href="/requests/needs-you">Requests</a>,
  },
  {
    id: "filler",
    value: "2",
    label: "filler choices to make",
    short: "filler choices",
    link: <a href="/filler/manage">Filler</a>,
  },
  {
    id: "tasks",
    value: "1",
    label: "task overdue",
    short: "overdue",
    link: <a href="/settings/system/tasks">Background tasks</a>,
  },
  {
    id: "quota",
    value: "91%",
    label: "of filler space used",
    short: "filler space",
    warn: true,
    link: <a href="/filler/settings">Filler settings</a>,
  },
];

const meta = {
  title: "Home/Channel Wall",
  component: ChannelWall,
  decorators: [
    withRouter("/dashboard"),
    (Story) => (
      <div style={{ boxSizing: "border-box", maxWidth: 1200, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
  args: { layout, nowMs: guideNow, needs },
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof ChannelWall>;

type Story = StoryObj<typeof meta>;

// An admin with something waiting: the counts sit in one quiet line under the header.
const AdminSomeCounts: Story = {};
// Every count is zero, so the line is gone and the wall starts right under its header.
const AdminAllZero: Story = { args: { needs: [] } };
// A member's Home never reads the counts, so the wall has no line at all.
const Member: Story = { args: { needs: undefined } };

export default meta;
export { AdminAllZero, AdminSomeCounts, Member };
