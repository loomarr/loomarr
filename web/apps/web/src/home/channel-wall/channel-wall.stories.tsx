import { layoutGuide } from "@loomarr/core/guide";
import { guideChannels, guideFrom, guideNow, guideTo } from "@loomarr/fixtures";
import type { NeedsLineItem } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { withRouter } from "@/test/story-utils";
import { CHANNEL_GROUP_KEY } from "../use-channel-group-preference";
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

// The fixture's channels with decades, the last of them without one.
const decaded = {
  ...layout,
  channels: layout.channels.map((channel, index) => ({
    ...channel,
    source: {
      ...channel.source,
      ...(index === layout.channels.length - 1 ? {} : { decade: index % 2 === 0 ? 1990 : 1980 }),
    },
  })),
};
const ids = layout.channels.map((channel) => channel.source.channelId);
const [first = "", second = "", third = ""] = ids;

// The remembered group is set before the wall reads it, and forgotten afterwards.
const inGroup = (group: string) => () => {
  localStorage.setItem(CHANNEL_GROUP_KEY, group);
  return () => localStorage.removeItem(CHANNEL_GROUP_KEY);
};

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
  args: { layout, nowMs: guideNow, needs, favouriteIds: [third, second], recentIds: [third, first] },
  beforeEach: inGroup("all"),
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof ChannelWall>;

type Story = StoryObj<typeof meta>;

// An admin with something waiting: the counts sit in one quiet line under the header.
const AdminSomeCounts: Story = {};
// Every count is zero, so the line is gone and the wall starts right under its header.
const AdminAllZero: Story = { args: { needs: [] } };
// A member's Home never reads the counts, so the wall has no line at all.
const Member: Story = { args: { needs: undefined } };

const All: Story = { args: { needs: [] } };
const Favourites: Story = { args: { needs: [] }, beforeEach: inGroup("favourites") };
const Recent: Story = { args: { needs: [] }, beforeEach: inGroup("recent") };
const LastWatchedFirst: Story = { args: { needs: [], lastChannelId: third } };
const ByDecade: Story = {
  args: { needs: [], layout: decaded, lastChannelId: second },
  beforeEach: inGroup("decade"),
};
const ByDecadeLight: Story = { ...ByDecade, globals: { theme: "light" } };
const EmptyFavourites: Story = { args: { needs: [], favouriteIds: [] }, beforeEach: inGroup("favourites") };
const NoArtwork: Story = {
  args: {
    needs: [],
    layout: {
      ...layout,
      channels: layout.channels.map((channel) => ({
        ...channel,
        airings: channel.airings.map((airing) => ({
          ...airing,
          source: { ...airing.source, backdropImage: undefined, thumbImage: undefined, thumbUrl: undefined },
        })),
      })),
    },
  },
};
const PhoneWidth: Story = {
  args: { needs: [] },
  // The tabs stay on one line, in a row that scrolls where they do not fit.
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const tabs = await canvas.findAllByRole("tab");
    await expect(new Set(tabs.map((tab) => tab.getBoundingClientRect().top)).size).toBe(1);
    const scroller = canvas.getByRole("tablist").closest<HTMLElement>("[data-testid='wall-tabs']");
    await expect(getComputedStyle(scroller as HTMLElement).overflowX).toBe("auto");
  },
  decorators: [
    (Story) => (
      <div style={{ width: 342 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
export {
  AdminAllZero,
  AdminSomeCounts,
  All,
  ByDecade,
  ByDecadeLight,
  EmptyFavourites,
  Favourites,
  LastWatchedFirst,
  Member,
  NoArtwork,
  PhoneWidth,
  Recent,
};
