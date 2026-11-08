import type { GuideAiring } from "@loomarr/api/models/guideAiring";
import type { GuideChannelTimeline } from "@loomarr/api/models/guideChannelTimeline";
import type { GuideOutputBody } from "@loomarr/api/models/guideOutputBody";
import { formatGuideTime, layoutGuide } from "@loomarr/core/guide";
import { SegmentedControl } from "@loomarr/design-system";
import { GuideNowNext, GuidePhoneHeader } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

// The phone guide's Now/Next list (#2766, the Direction mock's variant C), on the mock's evening:
// 9:07 PM, with a show on, a break on, a channel still downloading, and one with nothing listed next.
const day = Date.UTC(2026, 6, 26, 0, 0);
const at = (minutes: number) => day + minutes * 60_000;
const now = at(21 * 60 + 7);

let block = 0;
const show = (start: number, stop: number, title: string, series?: string): GuideAiring => ({
  kind: "program",
  scheduleBlockId: `b${block++}`,
  series,
  startMs: at(start),
  stopMs: at(stop),
  title,
});
const commercials = (start: number, stop: number): GuideAiring => ({
  kind: "filler",
  scheduleBlockId: `b${block++}`,
  startMs: at(start),
  stopMs: at(stop),
  title: "",
});
const channel = (
  number: number,
  name: string,
  status: GuideChannelTimeline["status"],
  airings: GuideAiring[],
): GuideChannelTimeline => ({ airings, channelId: `ch${number}`, name, number, pendingCount: 0, status });

const evening = {
  channels: [
    channel(2, "Cartoon Classics", "live", [
      show(21 * 60 - 9, 21 * 60 + 13, "The Upside-Down Picnic", "Kettle Kids"),
      show(21 * 60 + 13, 21 * 60 + 35, "Snow Day for Gulls", "Kettle Kids"),
    ]),
    channel(5, "Sitcom Corner", "live", [
      commercials(21 * 60 + 3, 21 * 60 + 12),
      show(21 * 60 + 12, 21 * 60 + 34, "The Spare Key", "Two Doors Down"),
    ]),
    channel(7, "Matinee Westerns", "live", [
      show(20 * 60 + 40, 23 * 60 + 10, "A Quiet Day in Dry Gulch and the Long Ride Home Afterwards"),
    ]),
    channel(9, "Late Movies", "building", [commercials(20 * 60 + 30, 24 * 60)]),
  ],
  fromMs: at(20 * 60 + 37),
  timezone: "America/New_York",
  toMs: at(25 * 60 + 7),
} as GuideOutputBody;
const layout = layoutGuide(evening, now);

const meta = {
  title: "Loomarr Components/Guide Now And Next",
  component: GuideNowNext,
  args: { layout, nowMs: now, onWatch: () => undefined },
  decorators: [
    (Story) => (
      <div style={{ display: "flex", flexDirection: "column", height: 720, width: 390 }}>
        <Story />
      </div>
    ),
  ],
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof GuideNowNext>;

type Story = StoryObj<typeof meta>;

const Evening: Story = {};

const VIEWS = [
  { label: "Now & next", value: "list" },
  { label: "Full grid", value: "grid" },
] as const;

// The page's heading with its view switch above the list, as the Guide page draws it on a phone.
const WithHeader: Story = {
  render: (args) => {
    const [view, setView] = useState<"grid" | "list">("list");
    return (
      <>
        <GuidePhoneHeader
          subtitle={`${formatGuideTime(now, "America/New_York")} · ${args.layout.channels.length} channels`}
          title="What's on"
        >
          <SegmentedControl
            accessibilityLabel="Guide view"
            onValueChange={setView}
            options={VIEWS}
            value={view}
          />
        </GuidePhoneHeader>
        <GuideNowNext {...args} />
      </>
    );
  },
};

export default meta;
export { Evening, WithHeader };
