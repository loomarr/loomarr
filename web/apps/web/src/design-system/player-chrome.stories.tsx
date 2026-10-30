import type { GuideAiring } from "@loomarr/api/models/guideAiring";
import { formatGuideTime } from "@loomarr/core/guide";
import { ThemeScope } from "@loomarr/design-system";
import { PlayerChrome, PlayerCover, PlayerSettings, PlayerSheet, playbarFromTimeline } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { fn } from "storybook/test";

// The §9 player chrome (#2088; the mock of record is design/watch-player), over a stand-in picture.
// Genre words only: the repo is public, so no real titles. The clock is fixed so the playbar's
// window, ticks and labels are the same on every run, and stills are `data:` stand-ins, never
// remote images.

const MINUTE = 60_000;
const NOW = Date.UTC(2026, 9, 29, 20, 44);
const formatTime = (ms: number) => formatGuideTime(ms, "UTC");

// A hue-lit stand-in still, as the mock's are: a gradient, no picture.
const still = (hue: number) =>
  `data:image/svg+xml,${encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg" width="320" height="180"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="hsl(${hue} 55% 46%)"/><stop offset="1" stop-color="hsl(${hue + 40} 45% 16%)"/></linearGradient></defs><rect width="320" height="180" fill="url(#g)"/></svg>`,
  )}`;

const image = (hue: number) => ({
  animated: false,
  dominantHex: "#333333",
  hash: `still-${hue}`,
  height: 180,
  placeholder: "",
  role: "thumb",
  src: still(hue),
  srcSetAvif: "",
  srcSetWebp: "",
  width: 320,
});

const at = (minutes: number) => NOW + minutes * MINUTE;

const programme = (
  id: string,
  from: number,
  to: number,
  fields: Partial<GuideAiring> & { title: string },
  hue: number,
): GuideAiring => ({
  kind: "program",
  scheduleBlockId: id,
  startMs: at(from),
  stopMs: at(to),
  thumbImage: image(hue),
  ...fields,
});

const spot = (
  kind: "commercial" | "station_id" | "bumper",
  name: string,
  seconds: number,
  brand?: string,
) => ({
  brand,
  durationMs: seconds * 1000,
  era: kind === "commercial" ? 1996 : undefined,
  hash: `${kind}-${name}`,
  isFallbackCard: false,
  kind,
  name,
});

const breakOf = (id: string, from: number, entries: ReturnType<typeof spot>[]): GuideAiring => {
  const totalMs = entries.reduce((sum, entry) => sum + entry.durationMs, 0);
  return {
    kind: "filler",
    pod: { entries, matchLevel: "exact", totalMs },
    scheduleBlockId: id,
    startMs: at(from),
    stopMs: at(from) + totalMs,
    title: "Commercials",
  };
};

// An evening on a stand-in channel: a sitcom airing now, a break, a film, another break, a series.
const airings: GuideAiring[] = [
  programme(
    "p1",
    -30,
    -2,
    { episode: 3, genres: ["Sitcom"], season: 2, series: "A workplace sitcom", title: "The Long Shift" },
    210,
  ),
  breakOf("b1", -2, [
    spot("commercial", "Breakfast cereal", 30, "A cereal brand"),
    spot("commercial", "Family car", 30, "A car maker"),
    spot("station_id", "", 10),
    spot("commercial", "Soft drink", 30, "A soda brand"),
  ]),
  programme(
    "p2",
    0,
    28,
    {
      description: "Two friends open a diner and learn the hard way who their regulars are.",
      episode: 4,
      genres: ["Comedy"],
      rating: "TV-PG",
      season: 2,
      series: "An ensemble comedy",
      title: "Opening Night",
    },
    20,
  ),
  breakOf("b2", 28, [
    spot("commercial", "Toy robot", 30, "A toy maker"),
    spot("bumper", "We'll be right back", 5),
    spot("commercial", "Pizza night", 30, "A pizza chain"),
  ]),
  programme(
    "p3",
    30,
    142,
    {
      description: "A detective takes one last case.",
      genres: ["Drama"],
      title: "A detective film",
      year: 1994,
    },
    280,
  ),
];

const channel = { name: "Coast 90s", number: "7" };
const live = { lagSeconds: 0, mode: "live" as const };

const chromeArgs = (state: { lagSeconds: number; mode: "behind" | "live" | "paused" }) => {
  const playheadMs = NOW - state.lagSeconds * 1000;
  const data = playbarFromTimeline({ airings, channel, live: state, nowMs: NOW, playheadMs });
  return {
    band: data.band,
    playbar: { playheadMs, segments: data.segments, window: data.window },
    transport: { ...transport, live: state },
  };
};

const transport = {
  channelsOpen: false,
  density: "pointer" as const,
  fullscreen: false,
  live,
  muted: false,
  onChannels: fn(),
  onFullscreen: fn(),
  onGoLive: fn(),
  onLeave: fn(),
  onSettings: fn(),
  onToggleMute: fn(),
  onTogglePlay: fn(),
  onVolume: fn(),
  playing: true,
  pointer: true,
  settingsOpen: false,
  volume: 0.8,
};

// The picture the chrome sits on: a dark stand-in filling the frame.
const Picture = ({ children }: { children: ReactNode }) => (
  <div
    style={{
      background: "radial-gradient(ellipse at 40% 35%, #3a4150, #101216 70%)",
      height: "100vh",
      overflow: "hidden",
      position: "relative",
      width: "100%",
    }}
  >
    {children}
  </div>
);

const meta = {
  title: "Loomarr Components/Player Chrome",
  component: PlayerChrome,
  decorators: [
    (Story) => (
      <Picture>
        <Story />
      </Picture>
    ),
  ],
  parameters: { layout: "fullscreen" },
  args: {
    ...chromeArgs(live),
    density: "pointer",
    formatTime,
    onPreviewChange: fn(),
    visible: true,
  },
} satisfies Meta<typeof PlayerChrome>;

type Story = StoryObj<typeof meta>;

const Playing: Story = {};

// Paused in the seekable past: the band reads how far behind, and Go live offers the way back.
const Behind: Story = { args: chromeArgs({ lagSeconds: 134, mode: "paused" }) };

// A break airing at the playhead: the band names the spot, and its line takes the break's tint.
const Commercials: Story = { args: chromeArgs({ lagSeconds: 75, mode: "behind" }) };

// The schedule not read yet: the band and the transport, no playbar.
const NoSchedule: Story = {
  args: { band: { channelName: channel.name, channelNumber: channel.number }, playbar: undefined },
};

const Touch: Story = {
  args: { density: "touch", transport: { ...transport, density: "touch", pointer: false } },
};

const Settings: Story = {
  args: {
    sheet: (
      <PlayerSheet
        channelLabel="CH 7 · Coast 90s"
        density="pointer"
        icon="settings"
        onClose={fn()}
        title="Settings"
      >
        <PlayerSettings
          audio={[
            { label: "Auto", selected: false, value: "auto" },
            { label: "English", selected: true, value: "eng" },
            { label: "Spanish", selected: false, value: "spa" },
          ]}
          density="pointer"
          onSwitchSound={fn()}
          switchSound
        />
      </PlayerSheet>
    ),
    transport: { ...transport, settingsOpen: true },
  },
};

// The full-picture states take the chrome's place.
const StreamStopped: Story = {
  render: () => (
    <ThemeScope theme="dark">
      <PlayerCover
        channelLabel="CH 7 · Coast 90s"
        density="pointer"
        kind="error"
        onLeave={fn()}
        onRetry={fn()}
        title="The stream stopped."
      />
    </ThemeScope>
  ),
};

const OffAir: Story = {
  render: () => (
    <ThemeScope theme="dark">
      <PlayerCover
        channelLabel="CH 7 · Coast 90s"
        density="pointer"
        kind="off-air"
        message="This channel is paused, so there's nothing to watch."
        onLeave={fn()}
        title="Coast 90s is off air"
      />
    </ThemeScope>
  ),
};

export default meta;
export { Behind, Commercials, NoSchedule, OffAir, Playing, Settings, StreamStopped, Touch };
