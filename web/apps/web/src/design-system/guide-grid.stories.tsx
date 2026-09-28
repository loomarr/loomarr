import type { GuideAiring } from "@loomarr/api/models/guideAiring";
import type { GuideChannelTimeline } from "@loomarr/api/models/guideChannelTimeline";
import { layoutGuide } from "@loomarr/core/guide";
import { GuideGrid } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";

// The shared time grid (#1659, N7), on the web mock's evening: 8–midnight, now at 8:26 PM.
const day = Date.UTC(2026, 8, 28);
const at = (h: number, m = 0) => day + (h * 60 + m) * 60_000;
const now = at(20, 26);

let block = 0;
const program = (start: number, stop: number, series: string, title = ""): GuideAiring => ({
  kind: "program",
  scheduleBlockId: `b${block++}`,
  series: title ? series : undefined,
  startMs: start,
  stopMs: stop,
  title: title || series,
});
const pending = (start: number, stop: number, title: string): GuideAiring => ({
  kind: "pending",
  scheduleBlockId: `b${block++}`,
  startMs: start,
  stopMs: stop,
  title,
});
const pod = (start: number, stop: number): GuideAiring => ({
  kind: "filler",
  pod: {
    entries: [
      { durationMs: 20_000, isFallbackCard: false, kind: "bumper", name: "bumper" },
      { durationMs: 40_000, isFallbackCard: false, kind: "commercial", name: "Soda spot" },
      { durationMs: 40_000, isFallbackCard: false, kind: "commercial", name: "Toy spot" },
      { durationMs: 20_000, isFallbackCard: false, kind: "station_id", name: "ident" },
    ],
    matchLevel: "exact",
    totalMs: 120_000,
  },
  scheduleBlockId: `b${block++}`,
  startMs: start,
  stopMs: stop,
  title: "",
});

const channel = (
  number: number,
  name: string,
  status: GuideChannelTimeline["status"],
  airings: GuideAiring[],
  pendingCount = 0,
): GuideChannelTimeline => ({ airings, channelId: `ch${number}`, name, number, pendingCount, status });

const layout = layoutGuide(
  {
    channels: [
      channel(7, "Late Night Sci-Fi", "live", [
        program(at(20), at(21), "A sci-fi anthology", "The pilot"),
        pod(at(21), at(21, 4)),
        program(at(21, 4), at(22, 4), "A paranormal drama", "Home"),
        program(at(22, 4), at(23, 40), "A space horror film"),
      ]),
      channel(
        12,
        "Saturday Morning Cartoons",
        "live",
        [
          program(at(19, 30), at(20, 30), "An animated hero series", "Two sides"),
          pending(at(20, 30), at(22), "Finding more cartoons"),
        ],
        3,
      ),
      channel(22, "Cozy Autumn Horror", "drifted", [
        program(at(20), at(21, 30), "A cult horror film"),
        program(at(21, 30), at(23), "A harvest horror film"),
      ]),
      channel(55, "Paused Westerns", "paused", [program(at(20), at(22), "A frontier western")]),
    ],
    fromMs: at(20),
    timezone: "UTC",
    toMs: at(24),
  } as Parameters<typeof layoutGuide>[0],
  now,
);

const meta = {
  title: "Loomarr Components/Guide Grid",
  component: GuideGrid,
  args: { layout, nowMs: now },
  decorators: [
    (Story) => (
      <div style={{ width: 1180 }}>
        <Story />
      </div>
    ),
  ],
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof GuideGrid>;

type Story = StoryObj<typeof meta>;

const Evening: Story = {};

export default meta;
export { Evening };
