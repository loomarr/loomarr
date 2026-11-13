import type { GuideAiring } from "@loomarr/api/models/guideAiring";
import { formatGuideTime } from "@loomarr/core/guide";
import { LoomarrProvider, ThemeScope } from "@loomarr/design-system";
import {
  PlayerAudio,
  PlayerBottomSheet,
  PlayerChrome,
  type PlayerChromeProps,
  PlayerCover,
  PlayerDetails,
  PlayerDetailsPanel,
  PlayerInfo,
  PlayerMenu,
  PlayerMore,
  PlayerOptions,
  PlayerSheet,
  PlayerVolume,
  pictureIn,
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
  onChannelDown: fn(),
  onChannelUp: fn(),
  onMore: fn(),
  onTogglePlay: fn(),
  playing: true,
  pointer: true,
  volume: { muted: false, onPress: fn(), open: false },
};

const utilities = {
  audioOpen: false,
  density: "pointer" as const,
  fullscreen: false,
  onAudio: fn(),
  onFullscreen: fn(),
  pointer: true,
};

// The band, keys and film strip for a playhead. A phone's strip is in More, so its stories drop it.
const chromeArgs = (state: { lagSeconds: number; mode: "behind" | "live" | "paused" }, playheadAt = NOW) => {
  const playheadMs = playheadAt - state.lagSeconds * 1000;
  const data = playbarFromTimeline({ airings, channel, live: state, nowMs: NOW, playheadMs });
  return {
    band: data.band,
    keys: { ...keys, playing: state.mode !== "paused" },
    live: state,
    schedule: { onSeek: fn(), playheadMs, segments: data.segments, window: data.window },
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

// The picture the chrome sits on: a dark stand-in filling the frame. The overlay layout is never
// drawn narrower than 48rem (the app switches to the portrait layout below it, usePhoneWidth), so
// an overlay story keeps that width on the gallery's phone-sized viewport rather than piling its
// keys onto the band.
const Picture = ({ children, portrait = false }: { children: ReactNode; portrait?: boolean }) => (
  <div
    style={{
      background: "radial-gradient(ellipse at 40% 35%, #3a4150, #101216 70%)",
      height: "100vh",
      minWidth: portrait ? undefined : 768,
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
    (Story, { args }) => (
      <Picture portrait={args.layout === "portrait"}>
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
    onLeave: fn(),
    utilities,
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

// Nothing scheduled: the band names the channel and says so, with no Now, Next or strip.
const NoSchedule: Story = {
  args: { band: { channelName: channel.name, channelNumber: channel.number }, schedule: undefined },
};

// Without the schedule's window the band draws the progress line and Next, as a phone's does.
const NoStrip: Story = { args: { schedule: undefined } };

// Last is hidden until there is a channel to return to; this one has it, as does the player after a flip.
const WithLast: Story = {
  args: { keys: { ...keys, last: { number: "3", onPress: fn() } } },
};

// Touch, the phone held sideways: 48 px keys, no legends, no Volume, the strip left to More.
const touchArgs = {
  density: "touch" as const,
  keys: { ...keys, density: "touch" as const, pointer: false, volume: undefined },
  schedule: undefined,
  utilities: { ...utilities, density: "touch" as const, pointer: false },
};
const Touch: Story = { args: touchArgs };

// A television (§9.7): the 960 × 540 frame with 5% clear at every edge, four keys (Last, Subtitles,
// Audio, More), no utilities row and no Leave; the strip's blocks take the remote's focus.
const TV = { height: 540, width: 960 } as const;
const Television: Story = {
  args: {
    density: "tv",
    keys: { ...keys, density: "tv", last: { number: "3", onPress: fn() }, pointer: false, volume: undefined },
    layout: "tv",
    onLeave: undefined,
    utilities: { ...utilities, density: "tv", onSubtitles: fn(), pointer: false, subtitlesOn: false },
  },
  render: (args) => (
    <div style={{ ...TV, margin: "0 auto", overflow: "hidden", position: "relative" }}>
      <PlayerChrome {...(args as PlayerChromeProps)} />
    </div>
  ),
};

// A break on a television, paused a little behind, leading back to a long title: the TV's large title
// shares its row with `Back in n min`, and the title is what truncates.
const longReturn = airings.map((airing) =>
  airing.scheduleBlockId === "p2"
    ? {
        ...airing,
        episode: undefined,
        season: undefined,
        series: undefined,
        title: "A Lighthouse Keeper's Long Winter",
      }
    : airing,
);
const tvBreakState = { lagSeconds: 75, mode: "paused" as const };
const tvBreakPlayhead = NOW - tvBreakState.lagSeconds * 1000;
const tvBreak = playbarFromTimeline({
  airings: longReturn,
  channel,
  live: tvBreakState,
  nowMs: NOW,
  playheadMs: tvBreakPlayhead,
});
const TelevisionCommercials: Story = {
  ...Television,
  args: {
    ...Television.args,
    band: tvBreak.band,
    keys: { ...Television.args?.keys, playing: false },
    live: tvBreakState,
    schedule: {
      onSeek: fn(),
      playheadMs: tvBreakPlayhead,
      segments: tvBreak.segments,
      window: tvBreak.window,
    },
  } as Story["args"],
};

// A television's subtitle is short (about 32 characters a line, as broadcast captions are).
const TelevisionSubtitles: Story = {
  ...Television,
  args: {
    ...Television.args,
    captions: ["I thought you said tonight", "would be quiet."],
    utilities: { ...Television.args?.utilities, subtitlesOn: true },
  } as Story["args"],
};

// What a key opens (§9.5, variant J): on a pointer and a television, a menu in the control cluster,
// right over the keys, with the picture whole; on a phone held upright, a sheet from the bottom.
const audioOptions = [
  { label: "Auto", selected: false, value: "auto" },
  { label: "English", selected: true, value: "eng" },
  { label: "Spanish", selected: false, value: "spa" },
];
const AudioMenu: Story = {
  args: {
    menu: (
      <PlayerMenu density="pointer" title="Audio">
        <PlayerAudio density="pointer" options={audioOptions} switchSound={{ on: true, onChange: fn() }} />
      </PlayerMenu>
    ),
    utilities: { ...utilities, audioOpen: true },
  },
};

// Subtitles (§9.5, variant J): Off, then each language, a hearing-impaired track as "English, with
// sounds"; the line on the picture, centred and lifted clear of the band while the controls are up.
const subtitleOptions = [
  { label: "Off", selected: false, value: "off" },
  { label: "English", selected: true, value: "en" },
  {
    detail: "For the deaf and hard of hearing",
    label: "English, with sounds",
    selected: false,
    value: "en-sdh",
  },
  { label: "Spanish", selected: false, value: "es" },
];
const subtitleLine = ["I thought you said tonight would be quiet."];
const subtitlesOn = { ...utilities, onSubtitles: fn(), subtitlesOn: true };
const SubtitlesMenu: Story = {
  args: {
    captions: subtitleLine,
    menu: (
      <PlayerMenu density="pointer" title="Subtitles">
        <PlayerOptions density="pointer" label="Subtitles" onSelect={fn()} options={subtitleOptions} />
      </PlayerMenu>
    ),
    utilities: { ...subtitlesOn, subtitlesOpen: true },
  },
};
// The same on a television (#3905): the menu rises over the keys, so the line centres on the picture
// left of it rather than running under its heading.
const TelevisionSubtitlesMenu: Story = {
  ...TelevisionSubtitles,
  args: {
    ...TelevisionSubtitles.args,
    menu: (
      <PlayerMenu density="tv" title="Subtitles">
        <PlayerOptions density="tv" label="Subtitles" onSelect={fn()} options={subtitleOptions} />
      </PlayerMenu>
    ),
    utilities: { ...TelevisionSubtitles.args?.utilities, subtitlesOpen: true },
  } as Story["args"],
};
// With the controls down, the line drops to the picture's foot.
const SubtitlesIdle: Story = { args: { captions: subtitleLine, utilities: subtitlesOn, visible: false } };

// The volume level (§9.5, desktop): the Volume key opens it in the menu's place; M and the slider show
// it for a moment.
const VolumeLevel: Story = {
  args: {
    keys: { ...keys, volume: { muted: false, onPress: fn(), open: true } },
    menu: <PlayerVolume muted={false} onVolume={fn()} volume={0.6} />,
  },
};

// Digits being typed with the controls up: the readout on a solid ink backing, white, top right.
const TypingChannel: Story = { args: { readout: { digits: "1–" } } };

const NoSuchChannel: Story = { args: { readout: { digits: "99", missed: true } } };

// Flipping channels (§9.2, mock "surfing"): Channel up tuned 12, the cards ride along in the band's
// place with 12 lit, and the readout names it. No art here, so every card's picture is its stand-in.
const surf = {
  channels: [
    { id: "3", name: "Saturday Mornings", nowPlaying: "Cartoon block", number: "3" },
    { id: "5", name: "Late Show Reruns", nowPlaying: "A talk show", number: "5" },
    { decade: 1990, id: "7", name: "Coast 90s", nowPlaying: "The Diner Years", number: "7" },
    { id: "9", name: "Movie Matinee", nowPlaying: "A heist film", number: "9" },
    { id: "12", name: "Game Night", nowPlaying: "A quiz show", number: "12" },
    { decade: 1970, id: "14", name: "Sunday Westerns", nowPlaying: "A cattle drive", number: "14" },
    { id: "21", name: "Sports Replay", nowPlaying: "A baseball classic", number: "21" },
  ],
  tunedId: "12",
};
const Surfing: Story = { args: { readout: { digits: "12", name: "Game Night", quiet: true }, surf } };
const TelevisionSurfing: Story = {
  ...Television,
  args: {
    ...Television.args,
    readout: { digits: "12", name: "Game Night", quiet: true },
    surf,
  } as Story["args"],
};

// The channel's watermark is burned into the picture and never moves. The chrome steps around it
// (§9.6, #2772): a 16:9 frame so the picture fills it, and a stand-in bug drawn where playout puts it.
const FRAME = { height: 720, width: 1280 } as const;
const BugFrame = ({
  corner,
  ...args
}: Story["args"] & { corner: "bottom-left" | "bottom-right" | "top-left" | "top-right" }) => {
  const watermark = { corner, margin: 0.05, size: 0.06 };
  const picture = pictureIn(FRAME);
  const h = watermark.size * picture.height;
  const inset = watermark.margin * picture.height;
  return (
    <div style={{ ...FRAME, margin: "0 auto", position: "relative" }}>
      <div
        style={{
          background: "rgba(255, 255, 255, 0.4)",
          [corner.startsWith("top") ? "top" : "bottom"]: picture.y + inset,
          [corner.endsWith("right") ? "right" : "left"]: picture.x + inset,
          height: h,
          position: "absolute",
          width: h * 2.5,
        }}
      />
      <PlayerChrome {...(args as PlayerChromeProps)} watermark={watermark} />
    </div>
  );
};
const bugStory = (corner: "bottom-left" | "bottom-right" | "top-left" | "top-right"): Story => ({
  render: (args) => <BugFrame {...args} corner={corner} />,
});
const WatermarkTopRight = bugStory("top-right");
const WatermarkTopLeft = bugStory("top-left");
const WatermarkBottomRight = bugStory("bottom-right");
const WatermarkBottomLeft = bugStory("bottom-left");
// A top-right bug with digits being typed: the readout starts below the bug too.
const WatermarkTopRightTyping: Story = {
  args: { readout: { digits: "1–" } },
  render: (args) => <BugFrame {...args} corner="top-right" />,
};

// Picture is More's second page: its heading is the way back.
const PictureMenu: Story = {
  args: {
    keys: { ...keys, moreOpen: true },
    menu: (
      <PlayerMenu density="pointer" onBack={fn()} title="Picture">
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
      </PlayerMenu>
    ),
  },
};

// Playback details (§9.5): mono values for what the player really knows, and Copy details.
const detailRows = [
  { label: "Resolution", value: "1920×1080 · 29.97 fps" },
  { label: "Video", value: "avc1.640029,mp4a.40.2" },
  { label: "HDR", value: "None (SDR)" },
  { label: "Bitrate", value: "4.6 Mbps" },
  { label: "Buffer ahead", value: "6.2 s" },
  { label: "Dropped frames", value: "3 of 12,418" },
  { label: "Behind live", value: "2.3 s" },
  { label: "Server", value: "loomarr.lan" },
  { label: "Session", value: "4f2a9c1d" },
];

// On a pointer it is a panel at the upper left, over the picture.
const PlaybackDetails: Story = {
  args: {
    panel: <PlayerDetailsPanel density="pointer" onClose={fn()} onCopy={fn()} rows={detailRows} />,
  },
};

// A native player has no level to read: only the rows that are true are left.
const PlaybackDetailsBare: Story = {
  args: {
    panel: (
      <PlayerDetailsPanel
        density="pointer"
        onClose={fn()}
        rows={[
          { label: "Behind live", value: "0.0 s" },
          { label: "Server", value: "loomarr.lan" },
        ]}
      />
    ),
  },
};

// On a phone it is a sheet.
const PlaybackDetailsSheet: Story = {
  args: {
    ...touchArgs,
    sheet: (
      <PlayerSheet channel={sheetChannel} density="touch" onClose={fn()} title="Playback details">
        <PlayerDetails density="touch" onCopy={fn()} rows={detailRows} />
      </PlayerSheet>
    ),
  },
};

// More holds only what has no key of its own; the band's strip is the schedule, so More has none.
const MoreMenu: Story = {
  args: {
    keys: { ...keys, moreOpen: true },
    menu: (
      <PlayerMenu density="pointer" heading={false} title="More">
        <PlayerMore
          channels={{ onPress: fn() }}
          density="pointer"
          formatTime={formatTime}
          info={{ onPress: fn() }}
          picture={{ detail: "Auto · 1080p", onPress: fn() }}
          pictureInPicture={{ onPress: fn() }}
          playbackDetails={{ onPress: fn() }}
        />
      </PlayerMenu>
    ),
  },
};

// Behind live, More adds Start over and Go live.
const MoreBehind: Story = {
  args: {
    ...chromeArgs({ lagSeconds: 134, mode: "paused" }),
    keys: { ...keys, moreOpen: true, playing: false },
    menu: (
      <PlayerMenu density="pointer" heading={false} title="More">
        <PlayerMore
          channels={{ onPress: fn() }}
          density="pointer"
          formatTime={formatTime}
          goLive={{ detail: formatTime(NOW), onPress: fn() }}
          info={{ onPress: fn() }}
          picture={{ detail: "Auto · 1080p", onPress: fn() }}
          playbackDetails={{ onPress: fn() }}
          startOver={moreData({ lagSeconds: 134, mode: "paused" }).startOver}
        />
      </PlayerMenu>
    ),
  },
};

// A television's More: the remote's ring on its rows; Back closes it.
const TelevisionMore: Story = {
  args: {
    density: "tv",
    keys: {
      ...keys,
      density: "tv",
      last: { number: "3", onPress: fn() },
      moreOpen: true,
      pointer: false,
      volume: undefined,
    },
    layout: "tv",
    menu: (
      <PlayerMenu density="tv" heading={false} title="More">
        <PlayerMore
          channels={{ onPress: fn() }}
          density="tv"
          formatTime={formatTime}
          info={{ onPress: fn() }}
        />
      </PlayerMenu>
    ),
    onLeave: undefined,
    utilities: { ...utilities, density: "tv", onSubtitles: fn(), pointer: false, subtitlesOn: false },
  },
  render: (args) => (
    <div style={{ ...TV, margin: "0 auto", overflow: "hidden", position: "relative" }}>
      <PlayerChrome {...(args as PlayerChromeProps)} />
    </div>
  ),
};

// The first play of the session starts muted: a pill under the controls takes one tap to undo it.
const UnmutePill: Story = { args: { unmute: fn() } };

// An upright iPhone (§9.7, variant J): the picture a strip mid-screen, a toolbar under the Dynamic
// Island (Leave and the rating, then the utilities), the band and keys in the black below, grounded
// above the home indicator, nothing over the picture. A stand-in strip marks where the player draws
// the video; the insets are an iPhone's safe areas.
const PHONE = { height: 844, width: 390 } as const;
const IPHONE_INSETS = { bottom: 34, left: 0, right: 0, top: 59 } as const;
const PhoneFrame = (args: PlayerChromeProps) => (
  <div style={{ ...PHONE, background: "#000", margin: "0 auto", position: "relative" }}>
    <div
      style={{
        aspectRatio: "16 / 9",
        background: "radial-gradient(ellipse at 40% 35%, #3a4150, #101216 70%)",
        position: "absolute",
        top: "50%",
        transform: "translateY(-50%)",
        width: "100%",
      }}
    />
    {/* The safe areas reach the chrome as the native app hands them down: through the provider. */}
    <LoomarrProvider insets={IPHONE_INSETS} theme="dark">
      <PlayerChrome {...args} />
    </LoomarrProvider>
  </div>
);
const portraitArgs = {
  ...touchArgs,
  keys: { ...touchArgs.keys, last: { number: "3", onPress: fn() } },
  layout: "portrait" as const,
};
const Portrait: Story = { args: portraitArgs, render: (args) => <PhoneFrame {...args} /> };
// Flipping on an upright phone: three cards where the band was, the readout where the toolbar was.
const PortraitSurfing: Story = {
  args: { ...portraitArgs, readout: { digits: "12", name: "Game Night", quiet: true }, surf },
  render: (args) => <PhoneFrame {...args} />,
};
// Nothing covers an upright phone's picture, so its line sits at the picture's own foot, smaller.
const PortraitSubtitles: Story = {
  args: {
    ...portraitArgs,
    captions: ["I thought you said tonight would be quiet."],
    utilities: { ...portraitArgs.utilities, onSubtitles: fn(), subtitlesOn: true },
  },
  render: (args) => <PhoneFrame {...args} />,
};
const PortraitBehind: Story = {
  args: {
    ...portraitArgs,
    ...chromeArgs({ lagSeconds: 134, mode: "paused" }),
    keys: { ...portraitArgs.keys, playing: false },
    schedule: undefined,
  },
  render: (args) => <PhoneFrame {...args} />,
};
const PortraitCommercials: Story = {
  args: {
    ...portraitArgs,
    ...chromeArgs({ lagSeconds: 75, mode: "behind" }),
    keys: portraitArgs.keys,
    schedule: undefined,
  },
  render: (args) => <PhoneFrame {...args} />,
};
// The band's text column at its tightest (#3408): a 30-character title that takes two lines under a
// long episode name, and a 25-character channel name on its own line.
const PortraitLongTitle: Story = {
  args: {
    ...portraitArgs,
    band: {
      ...liveData.band,
      line: "S01E02 “The Upside-Down Garden Party”",
      title: "The Lantern Keeper's Menagerie",
    },
  },
  render: (args) => <PhoneFrame {...args} />,
};
const PortraitLongChannel: Story = {
  // A three-digit number too: the common case in a real library, and the narrow block's tightest fit.
  args: {
    ...portraitArgs,
    band: { ...liveData.band, channelName: "Saturday Morning Cartoons", channelNumber: "101" },
  },
  render: (args) => <PhoneFrame {...args} />,
};
const PortraitUnmute: Story = {
  args: { ...portraitArgs, unmute: fn() },
  render: (args) => <PhoneFrame {...args} />,
};
// Held upright, what a key opens rises from the bottom, as tall as what it holds and never over the
// toolbar, with the picture above it undimmed. A phone has no strip in its band, so More keeps the
// schedule.
const PortraitMore: Story = {
  args: {
    ...portraitArgs,
    sheet: (
      <PlayerBottomSheet onClose={fn()} title="More">
        <PlayerMore
          channels={{ onPress: fn() }}
          density="touch"
          formatTime={formatTime}
          info={{ onPress: fn() }}
          picture={{ detail: "Auto · 1080p", onPress: fn() }}
          playbackDetails={{ onPress: fn() }}
          {...moreData(live)}
        />
      </PlayerBottomSheet>
    ),
  },
  render: (args) => <PhoneFrame {...args} />,
};
// Forty minutes into a film, the last fifteen hold no break, so the past is a sliver of a phone's
// strip: "Live" sits near the left edge, and "−15m" gives way to it (#3566).
const midFilm: GuideAiring[] = [
  programme("m1", -40, 50, { genres: ["Drama"], title: "A detective film", year: 1994 }, 280),
  breakOf("m2", 50, [spot("commercial", "Toy robot", 30, "A toy maker")]),
  programme("m3", 51, 140, { genres: ["Comedy"], title: "An ensemble comedy" }, 20),
];
const midFilmData = playbarFromTimeline({ airings: midFilm, channel, live, nowMs: NOW, playheadMs: NOW });
const PortraitMoreMidFilm: Story = {
  args: {
    ...portraitArgs,
    sheet: (
      <PlayerBottomSheet onClose={fn()} title="More">
        <PlayerMore
          channels={{ onPress: fn() }}
          density="touch"
          formatTime={formatTime}
          info={{ onPress: fn() }}
          schedule={{
            onSeek: fn(),
            playheadMs: NOW,
            segments: midFilmData.segments,
            window: midFilmData.window,
          }}
        />
      </PlayerBottomSheet>
    ),
  },
  render: (args) => <PhoneFrame {...args} />,
};
const PortraitAudio: Story = {
  args: {
    ...portraitArgs,
    sheet: (
      <PlayerBottomSheet onClose={fn()} title="Audio">
        <PlayerAudio density="touch" options={audioOptions} switchSound={{ on: true, onChange: fn() }} />
      </PlayerBottomSheet>
    ),
    utilities: { ...portraitArgs.utilities, audioOpen: true },
  },
  render: (args) => <PhoneFrame {...args} />,
};
// A phone has no strip to pin a card on, so its Info is a sheet.
const PortraitInfo: Story = {
  args: {
    ...portraitArgs,
    sheet: liveData.info ? (
      <PlayerBottomSheet onClose={fn()} title="Info">
        <PlayerInfo density="touch" formatTime={formatTime} info={liveData.info} />
      </PlayerBottomSheet>
    ) : undefined,
  },
  render: (args) => <PhoneFrame {...args} />,
};

// Title art, drawn as shapes (no real titles in a public repo): a wide wordmark and a 16:9 fanart.
const svgArt = (width: number, height: number, body: string) => ({
  height,
  src: `data:image/svg+xml,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}">${body}</svg>`)}`,
  srcSetWebp: "",
  width,
});
const wordmark = svgArt(
  600,
  100,
  '<rect width="600" height="100" rx="12" fill="#F4F6FA"/><rect x="24" y="30" width="552" height="40" rx="8" fill="#0B0C0E"/>',
);
const fanart = svgArt(
  1280,
  720,
  '<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="hsl(20 60% 50%)"/><stop offset="1" stop-color="hsl(280 45% 18%)"/></linearGradient></defs><rect width="1280" height="720" fill="url(#g)"/>',
);

// A title with a logo: the band still types it. A logo is set on the strip's preview card (§9.2).
const LogoWide: Story = { args: { band: { ...liveData.band, logo: wordmark } } };

// The station-ID card: a settled tune, the controls hidden — the band alone, with the station's era
// and place.
const StationId: Story = { args: { ident: true, visible: false } };

// On the programme's fanart, heavily dimmed and contained at 16:9 (letterboxed), never stretched.
const StationIdBackdrop: Story = {
  args: { band: { ...liveData.band, backdrop: fanart, logo: wordmark }, ident: true, visible: false },
};

// The channel block in the channel's decade colour (#2767); without one it is amber (Playing).
const Decade1970: Story = { args: { band: { ...liveData.band, decade: 1970 } } };
const Decade1980: Story = { args: { band: { ...liveData.band, decade: 1980 } } };
const Decade1990: Story = { args: { band: { ...liveData.band, decade: 1990 } } };
const StationIdDecade: Story = {
  args: { band: { ...liveData.band, decade: 1990 }, ident: true, visible: false },
};

// Info on a pointer and a television: the block on air's own card on the strip, pinned open in full.
const Info: Story = {
  args: { schedule: { ...chromeArgs(live).schedule, info: true } },
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
  AudioMenu,
  Behind,
  Commercials,
  Decade1970,
  Decade1980,
  Decade1990,
  Idle,
  Info,
  LightPlaying,
  LogoWide,
  MoreBehind,
  MoreMenu,
  NoSchedule,
  NoStrip,
  NoSuchChannel,
  OffAir,
  PictureMenu,
  PlaybackDetails,
  PlaybackDetailsBare,
  PlaybackDetailsSheet,
  Playing,
  Portrait,
  PortraitAudio,
  PortraitBehind,
  PortraitCommercials,
  PortraitInfo,
  PortraitLongChannel,
  PortraitLongTitle,
  PortraitMore,
  PortraitMoreMidFilm,
  PortraitSubtitles,
  PortraitSurfing,
  PortraitUnmute,
  StationId,
  StationIdBackdrop,
  StationIdDecade,
  StreamStopped,
  SubtitlesIdle,
  SubtitlesMenu,
  Surfing,
  Television,
  TelevisionCommercials,
  TelevisionMore,
  TelevisionSubtitles,
  TelevisionSubtitlesMenu,
  TelevisionSurfing,
  Touch,
  TypingChannel,
  UnmutePill,
  VolumeLevel,
  WatermarkBottomLeft,
  WatermarkBottomRight,
  WatermarkTopLeft,
  WatermarkTopRight,
  WatermarkTopRightTyping,
  WithLast,
};
