import type { GuideAiring } from "@loomarr/api/models/guideAiring";
import { formatGuideTime } from "@loomarr/core/guide";
import { ThemeScope } from "@loomarr/design-system";
import {
  PlayerAudio,
  PlayerChrome,
  PlayerCover,
  PlayerInfo,
  PlayerMore,
  PlayerOptions,
  PlayerReadout,
  PlayerSheet,
  playbarFromTimeline,
} from "@loomarr/ui";
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
  role: "commercial" | "station_id" | "bumper",
  name: string,
  seconds: number,
  brand?: string,
) => ({
  brand,
  durationMs: seconds * 1000,
  era: role === "commercial" ? 1996 : undefined,
  hash: `${role}-${name}`,
  isFallbackCard: false,
  name,
  role,
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

const channel = { era: { from: 1990, to: 1999 }, name: "Coast 90s", number: "7", place: "a coastal town" };
const live = { lagSeconds: 0, mode: "live" as const };
const liveData = playbarFromTimeline({ airings, channel, live, nowMs: NOW, playheadMs: NOW });

const keys = {
  density: "pointer" as const,
  moreOpen: false,
  muted: false,
  onChannelDown: fn(),
  onChannelUp: fn(),
  onMore: fn(),
  onToggleMute: fn(),
  onTogglePlay: fn(),
  playing: true,
  pointer: true,
  volume: 0.8,
};

const top = {
  audioOpen: false,
  density: "pointer" as const,
  fullscreen: false,
  onAudio: fn(),
  onFullscreen: fn(),
  onLeave: fn(),
  pointer: true,
};

const chromeArgs = (state: { lagSeconds: number; mode: "behind" | "live" | "paused" }, playheadAt = NOW) => {
  const playheadMs = playheadAt - state.lagSeconds * 1000;
  const data = playbarFromTimeline({ airings, channel, live: state, nowMs: NOW, playheadMs });
  return {
    band: data.band,
    keys: { ...keys, playing: state.mode !== "paused" },
    live: state,
  };
};

// What the More sheet reads from the same timeline.
const moreData = (state: { lagSeconds: number; mode: "behind" | "live" | "paused" }) => {
  const playheadMs = NOW - state.lagSeconds * 1000;
  const data = playbarFromTimeline({ airings, channel, live: state, nowMs: NOW, playheadMs });
  return {
    schedule: { onSeek: fn(), playheadMs, segments: data.segments, window: data.window },
    startOver: data.startOverMs === undefined ? undefined : { detail: data.band.title ?? "", onPress: fn() },
  };
};

const sheetChannel = { name: "Coast 90s", number: "7" };

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
    onGoLive: fn(),
    top,
    visible: true,
  },
} satisfies Meta<typeof PlayerChrome>;

type Story = StoryObj<typeof meta>;

const Playing: Story = {};

// The chrome is light words on the picture in both themes (the flat player, #3178), so the light
// theme's page must not change it. The picture stays dark.
const LightPlaying: Story = { globals: { theme: "light" } };

// Controls hidden after three seconds: only the picture is left.
const Idle: Story = { args: { visible: false } };

// Paused in the seekable past: Go live in place of Live, a red tick on the progress line.
const Behind: Story = { args: chromeArgs({ lagSeconds: 134, mode: "paused" }) };

// A break airing at the playhead: the programme's title, `Back in n min` in the break colour.
const Commercials: Story = { args: chromeArgs({ lagSeconds: 75, mode: "behind" }) };

// The schedule not read yet: the band names the channel and nothing else.
const NoSchedule: Story = {
  args: { band: { channelName: channel.name, channelNumber: channel.number } },
};

// Last is hidden until there is a channel to return to; this one has it, as does the player after a flip.
const WithLast: Story = {
  args: { keys: { ...keys, last: { number: "3", onPress: fn() } } },
};

// Touch: 56 px keys, no legends, no Volume, no tooltips.
const Touch: Story = {
  args: {
    density: "touch",
    keys: { ...keys, density: "touch", pointer: false },
    top: { ...top, density: "touch", pointer: false },
  },
};

const AudioSheet: Story = {
  args: {
    sheet: (
      <PlayerSheet channel={sheetChannel} density="pointer" onClose={fn()} title="Audio">
        <PlayerAudio
          density="pointer"
          onSwitchSound={fn()}
          options={[
            { label: "Auto", selected: false, value: "auto" },
            { label: "English", selected: true, value: "eng" },
            { label: "Spanish", selected: false, value: "spa" },
          ]}
          switchSound
        />
      </PlayerSheet>
    ),
    top: { ...top, audioOpen: true },
  },
};

// Digits being typed with the controls up: the readout on a solid ink backing, white, under the top row.
const TypingChannel: Story = {
  render: (args) => (
    <>
      <PlayerChrome {...args} />
      <ThemeScope theme="dark">
        <PlayerReadout density="pointer" digits="1–" />
      </ThemeScope>
    </>
  ),
};

const NoSuchChannel: Story = {
  render: (args) => (
    <>
      <PlayerChrome {...args} />
      <ThemeScope theme="dark">
        <PlayerReadout density="pointer" digits="99" missed />
      </ThemeScope>
    </>
  ),
};

const PictureSheet: Story = {
  args: {
    sheet: (
      <PlayerSheet channel={sheetChannel} density="pointer" onClose={fn()} title="Picture">
        <PlayerOptions
          density="pointer"
          label="Picture quality"
          onSelect={fn()}
          options={[
            { detail: "Best for your connection", label: "Auto · 1080p", selected: true, value: "auto" },
            { label: "1080p", selected: false, value: "1080" },
            { label: "720p", selected: false, value: "720" },
          ]}
        />
      </PlayerSheet>
    ),
  },
};

const MoreSheet: Story = {
  args: {
    keys: { ...keys, moreOpen: true },
    sheet: (
      <PlayerSheet channel={sheetChannel} density="pointer" onClose={fn()} title="More">
        <PlayerMore
          channels={{ onPress: fn() }}
          density="pointer"
          formatTime={formatTime}
          info={{ onPress: fn() }}
          picture={{ detail: "Auto · 1080p", onPress: fn() }}
          {...moreData(live)}
        />
      </PlayerSheet>
    ),
  },
};

// Behind live, More adds Start over and Go live.
const MoreBehind: Story = {
  args: {
    ...chromeArgs({ lagSeconds: 134, mode: "paused" }),
    keys: { ...keys, moreOpen: true, playing: false },
    sheet: (
      <PlayerSheet channel={sheetChannel} density="pointer" onClose={fn()} title="More">
        <PlayerMore
          channels={{ onPress: fn() }}
          density="pointer"
          formatTime={formatTime}
          goLive={{ detail: formatTime(NOW), onPress: fn() }}
          info={{ onPress: fn() }}
          last={{ number: "3", onPress: fn() }}
          picture={{ detail: "Auto · 1080p", onPress: fn() }}
          {...moreData({ lagSeconds: 134, mode: "paused" })}
        />
      </PlayerSheet>
    ),
  },
};

// The station-ID card: a settled tune, the controls hidden — the band alone, with the station's era
// and place.
const StationId: Story = { args: { ident: true, visible: false } };

// The channel block in the channel's decade colour (#2767); without one it is amber (Playing).
const Decade1970: Story = { args: { band: { ...liveData.band, decade: 1970 } } };
const Decade1980: Story = { args: { band: { ...liveData.band, decade: 1980 } } };
const Decade1990: Story = { args: { band: { ...liveData.band, decade: 1990 } } };
const StationIdDecade: Story = {
  args: { band: { ...liveData.band, decade: 1990 }, ident: true, visible: false },
};

const Info: Story = {
  args: {
    sheet: liveData.info ? (
      <PlayerSheet channel={sheetChannel} density="pointer" onClose={fn()} title="Info">
        <PlayerInfo density="pointer" formatTime={formatTime} info={liveData.info} />
      </PlayerSheet>
    ) : undefined,
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
export {
  AudioSheet,
  Behind,
  Commercials,
  Decade1970,
  Decade1980,
  Decade1990,
  Idle,
  Info,
  LightPlaying,
  MoreBehind,
  MoreSheet,
  NoSchedule,
  NoSuchChannel,
  OffAir,
  PictureSheet,
  Playing,
  StationId,
  StationIdDecade,
  StreamStopped,
  Touch,
  TypingChannel,
  WithLast,
};
