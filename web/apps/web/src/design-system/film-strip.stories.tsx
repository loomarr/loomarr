import { formatGuideTime } from "@loomarr/core/guide";
import { Surface } from "@loomarr/design-system";
import {
  FilmStrip,
  type FilmStripLook,
  type FilmStripProps,
  ProgrammeFacts,
  type StripAiring,
} from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";

// The film strip of The Cross (frontend-design §9.2, target). Genre words only: the repo is public,
// so no real titles. The clock is fixed, and stills are `data:` stand-ins, never remote images.

const day = Date.UTC(2026, 9, 29);
const at = (h: number, m = 0) => day + (h * 60 + m) * 60_000;
const NOW = at(20, 26);
const formatTime = (ms: number) => formatGuideTime(ms, "UTC");

const still = (hue: number) =>
  `data:image/svg+xml,${encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg" width="320" height="180"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="hsl(${hue} 55% 46%)"/><stop offset="1" stop-color="hsl(${hue + 40} 45% 16%)"/></linearGradient></defs><rect width="320" height="180" fill="url(#g)"/></svg>`,
  )}`;

const airing = (start: number, stop: number, hue: number, fields: Partial<StripAiring>): StripAiring => ({
  kind: "program",
  scheduleBlockId: `s${start}`,
  startMs: start,
  stopMs: stop,
  thumbUrl: still(hue),
  title: "",
  ...fields,
});
const spots = (start: number, stop: number): StripAiring => ({
  kind: "filler",
  scheduleBlockId: `s${start}`,
  startMs: start,
  stopMs: stop,
  title: "",
});

const evening: StripAiring[] = [
  airing(at(19, 30), at(20), 20, { title: "A cooking show" }),
  airing(at(20), at(20, 56), 210, {
    description: "The night shift takes a call nobody else will answer.",
    episode: 4,
    genres: ["Drama", "Mystery"],
    rating: "TV-14",
    season: 2,
    series: "A police drama",
    title: "The long watch",
  }),
  spots(at(20, 56), at(21)),
  airing(at(21), at(21, 25), 130, { episode: 1, season: 1, series: "A sitcom", title: "Moving day" }),
  spots(at(21, 25), at(21, 29)),
  airing(at(21, 29), at(21, 55), 140, { episode: 2, season: 1, series: "A sitcom", title: "The sofa" }),
  airing(at(21, 55), at(22, 45), 300, {
    description: "A small town wakes to find every clock an hour fast, and one person remembers why.",
    genres: ["Science fiction"],
    rating: "PG",
    title: "A space mystery film",
  }),
];
const bare = evening.map(({ thumbUrl: _, ...rest }) => rest);

const Strip = (props: FilmStripProps) => {
  const [look, setLook] = useState<FilmStripLook | undefined>(undefined);
  return (
    <Surface backgroundColor="surfaceCanvas" borderWidth={0} gap={16} padding={24} width={1100}>
      <FilmStrip {...props} onLook={setLook} />
      <div style={{ minHeight: 120 }}>
        {look ? (
          <ProgrammeFacts
            airing={look.airing}
            formatTime={formatTime}
            nowMs={props.nowMs}
            startMs={look.startMs}
            stopMs={look.stopMs}
          />
        ) : null}
      </div>
    </Surface>
  );
};

const meta = {
  title: "Loomarr Components/Film Strip",
  component: FilmStrip,
  args: {
    airings: evening,
    formatTime,
    nowMs: NOW,
    window: { startMs: at(19, 45), stopMs: at(22, 30) },
  },
  render: (args) => <Strip {...args} />,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof FilmStrip>;

type Story = StoryObj<typeof meta>;

const light = { theme: "light" } as const;

// Ended, on now, a break, a run of two episodes and a later programme.
const Day: Story = {};
const DayLight: Story = { globals: light };
const DayNoArtwork: Story = { args: { airings: bare } };
const DayNoArtworkLight: Story = { args: { airings: bare }, globals: light };

const GuideRow: Story = { args: { layout: "row" } };
const GuideRowLight: Story = { args: { layout: "row" }, globals: light };
const GuideRowNoArtwork: Story = { args: { airings: bare, layout: "row" } };
const GuideRowNoArtworkLight: Story = { args: { airings: bare, layout: "row" }, globals: light };

const BehindLive: Story = { args: { playheadMs: at(20, 12) } };
const BehindLiveLight: Story = { args: { playheadMs: at(20, 12) }, globals: light };
const BehindLiveNoArtwork: Story = { args: { airings: bare, playheadMs: at(20, 12) } };
const BehindLiveNoArtworkLight: Story = {
  args: { airings: bare, playheadMs: at(20, 12) },
  globals: light,
};

// A pointer draws the plain ring and opens the facts.
const Pointed: Story = {
  play: async ({ canvasElement }) => {
    const film = within(canvasElement).getByRole("button", { name: /space mystery/ });
    await userEvent.hover(film);
    await expect(within(canvasElement).getByRole("group", { name: "Programme facts" })).toHaveTextContent(
      "Starts",
    );
  },
};
const PointedLight: Story = { ...Pointed, globals: light };
const PointedNoArtwork: Story = { ...Pointed, args: { airings: bare } };
const PointedNoArtworkLight: Story = { ...Pointed, args: { airings: bare }, globals: light };

// The keyboard draws the amber ring and opens the same facts.
const KeyboardFocus: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.tab();
    await userEvent.tab();
    await userEvent.tab();
    await expect(within(canvasElement).getByRole("group", { name: "Programme facts" })).toHaveTextContent(
      "Starts",
    );
    await expect(within(canvasElement).getByTestId("film-strip-ring-actionFocus")).toBeVisible();
  },
};
const KeyboardFocusLight: Story = { ...KeyboardFocus, globals: light };
const KeyboardFocusNoArtwork: Story = { ...KeyboardFocus, args: { airings: bare } };
const KeyboardFocusNoArtworkLight: Story = {
  ...KeyboardFocus,
  args: { airings: bare },
  globals: light,
};

export {
  BehindLive,
  BehindLiveLight,
  BehindLiveNoArtwork,
  BehindLiveNoArtworkLight,
  Day,
  DayLight,
  DayNoArtwork,
  DayNoArtworkLight,
  GuideRow,
  GuideRowLight,
  GuideRowNoArtwork,
  GuideRowNoArtworkLight,
  KeyboardFocus,
  KeyboardFocusLight,
  KeyboardFocusNoArtwork,
  KeyboardFocusNoArtworkLight,
  Pointed,
  PointedLight,
  PointedNoArtwork,
  PointedNoArtworkLight,
};
export default meta;
