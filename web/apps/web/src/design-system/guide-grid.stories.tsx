import type { GuideAiring } from "@loomarr/api/models/guideAiring";
import type { GuideChannelTimeline } from "@loomarr/api/models/guideChannelTimeline";
import type { GuideOutputBody } from "@loomarr/api/models/guideOutputBody";
import {
  createGuideController,
  type GuideController,
  type GuideControllerSnapshot,
  layoutGuide,
} from "@loomarr/core/guide";
import { AdaptiveSplit } from "@loomarr/design-system";
import { GuideGrid, GuideProgrammeDetail } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect, useState } from "react";
import { expect, waitFor } from "storybook/test";

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

const evening = {
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
} as GuideOutputBody;
const layout = layoutGuide(evening, now);

// The grid as the Guide page drives it: selection and keys go through the shared controller.
const Controlled = ({ detail = false, source }: { detail?: boolean; source: GuideOutputBody }) => {
  const [guide, setGuide] = useState<{ controller: GuideController; snapshot: GuideControllerSnapshot }>();
  useEffect(() => {
    const controller = createGuideController({ now: () => now, source: { load: async () => source } });
    const unsubscribe = controller.subscribe(() =>
      setGuide({ controller, snapshot: controller.getSnapshot() }),
    );
    void controller.refresh();
    return () => {
      unsubscribe();
      controller.dispose();
    };
  }, [source]);
  if (!guide?.snapshot.layout) return null;
  const grid = (
    <GuideGrid
      layout={guide.snapshot.layout}
      nowMs={now}
      onMove={guide.controller.move}
      onSelect={guide.controller.select}
      selection={guide.snapshot.selection}
    />
  );
  // The web Guide page's shape: the pointer guide's programme card beside the grid.
  return detail ? (
    <AdaptiveSplit
      accessibilityLabel="Programme guide"
      primary={grid}
      secondary={<GuideProgrammeDetail layout={guide.snapshot.layout} selection={guide.snapshot.selection} />}
      secondaryWidth={360}
    />
  ) : (
    grid
  );
};

// A hundred channels in a viewport-tall frame: the fixture the FlatList measurement on #1705
// ran against, and the virtualised-rows case the Guide page lands in.
const genres = ["Sci-Fi", "Cartoons", "Horror", "Westerns", "Sitcoms", "Noir", "Anime", "Drama"];
const hundredSource = {
  channels: Array.from({ length: 100 }, (_, i) => {
    const genre = genres[i % genres.length] ?? "Drama";
    // Staggered starts so the rows don't line up into columns.
    const offset = (i * 17) % 60;
    return channel(i + 1, `${genre} ${i + 1}`, i % 11 === 5 ? "paused" : "live", [
      program(at(19, 30 + offset), at(20, 45 + offset), `A ${genre.toLowerCase()} series`, "Part one"),
      pod(at(20, 45 + offset), at(20, 49 + offset)),
      program(at(20, 49 + offset), at(22, 15), `A ${genre.toLowerCase()} film`),
      program(at(22, 15), at(24), `A late ${genre.toLowerCase()} film`),
    ]);
  }),
  fromMs: at(20),
  timezone: "UTC",
  toMs: at(24),
} as GuideOutputBody;
const hundred = layoutGuide(hundredSource, now);

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

const viewportTall: Story["decorators"] = [
  (Story) => (
    <div style={{ display: "flex", flexDirection: "column", height: 720 }}>
      <Story />
    </div>
  ),
];

const HundredChannels: Story = {
  args: { layout: hundred },
  decorators: viewportTall,
};

const focusedName = () => document.activeElement?.getAttribute("aria-label") ?? "";
const pause = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

// Tab enters on the block airing now, arrows move through the controller, and typing a name or a
// number jumps to that channel. The grid is one Tab stop.
const Keyboard: Story = {
  play: async ({ canvas, canvasElement, userEvent }) => {
    await canvas.findByRole("button", { name: /The pilot/ });
    expect(canvasElement.querySelectorAll('[tabindex="0"]')).toHaveLength(1);

    await userEvent.tab();
    expect(focusedName()).toMatch(/^A sci-fi anthology · The pilot, 8:00/);
    await userEvent.keyboard("{ArrowDown}");
    expect(focusedName()).toMatch(/^An animated hero series · Two sides/);
    await userEvent.keyboard("{ArrowRight}");
    expect(focusedName()).toMatch(/^Finding more cartoons/);
    await userEvent.keyboard("cozy");
    expect(focusedName()).toMatch(/^A cult horror film/);
    // A pause ends the query; digits then name a channel number.
    await pause(900);
    await userEvent.keyboard("55");
    expect(focusedName()).toMatch(/^A frontier western/);
    expect(canvasElement.querySelectorAll('[tabindex="0"]')).toHaveLength(1);
    // The programme card follows focus: the block's label and the card's title.
    expect(canvas.getAllByText("A frontier western")).toHaveLength(2);
  },
  render: () => <Controlled detail source={evening} />,
};

// Type-to-jump reaches a row the virtualiser hasn't mounted: it scrolls there, then focuses it.
const JumpToAFarChannel: Story = {
  decorators: viewportTall,
  play: async ({ canvas, userEvent }) => {
    await canvas.findAllByRole("button", { name: /Part one/ });
    expect(canvas.queryByText("Noir 86")).toBeNull();
    await userEvent.tab();
    await userEvent.keyboard("86");
    await waitFor(() => expect(focusedName()).toMatch(/^A noir /));
    expect(canvas.getByText("Noir 86")).toBeVisible();
  },
  render: () => <Controlled source={hundredSource} />,
};

export default meta;
export { Evening, HundredChannels, JumpToAFarChannel, Keyboard };
