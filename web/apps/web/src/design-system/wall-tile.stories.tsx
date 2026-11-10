import { WallTile } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";

// WallTile — one channel on Home's wall (#2764, Direction mock "Channel wall"). A 16:9 picture with
// the channel number and name along the top, what is on and a thin progress line along the foot.
const meta = {
  title: "Loomarr Components/WallTile",
  component: WallTile,
  args: {
    art: null,
    channelName: "Coast 90s",
    number: "07",
    title: "Coastline Mysteries",
  },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 420, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof WallTile>;

type Story = StoryObj<typeof meta>;

// A code-drawn stand-in picture from the design's own hues, so the story needs no image.
const Still = ({ hue }: { hue: number }) => (
  <div
    aria-hidden
    style={{
      background: `linear-gradient(135deg, hsl(${hue} 30% 26%), hsl(${(hue + 30) % 360} 25% 10%))`,
      height: "100%",
      width: "100%",
    }}
  />
);

const OnAir: Story = {
  args: { art: <Still hue={205} />, detail: "S2 E4 · 13 min left", progressPercent: 56 },
};

// The channel the viewer tuned last: a ring and a pill.
const LastWatched: Story = {
  args: { art: <Still hue={28} />, detail: "S1 E12 · 9 min left", last: true, progressPercent: 60 },
};

// An admin sees who is watching, by name, in the top-right corner (#2785).
const Watched: Story = { args: { ...OnAir.args, watchers: ["Sam", "Jo"] } };
const WatchedCompact: Story = {
  args: {
    art: <Still hue={142} />,
    channelName: "Nature Hour",
    compact: true,
    detail: "41m left",
    number: "12",
    watchers: ["Sam", "Jo"],
  },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 180 }}>
        <Story />
      </div>
    ),
  ],
};
const LightWatched: Story = { ...Watched, globals: { theme: "light" } };

// A channel still downloading its titles is dimmer and says so.
const FillingUp: Story = {
  args: {
    art: <Still hue={262} />,
    channelName: "Late Night Movies",
    detail: "2 of 9 downloaded",
    dim: true,
    number: "05",
    title: "Commercials until titles arrive",
  },
};

// A phone's two columns.
const Compact: Story = {
  args: {
    art: <Still hue={142} />,
    channelName: "Nature Hour",
    compact: true,
    detail: "41m left",
    number: "12",
  },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 180 }}>
        <Story />
      </div>
    ),
  ],
};

const CompactLast: Story = {
  args: { ...Compact.args, last: true },
  decorators: Compact.decorators ?? [],
};

// The number block in the channel's decade colour (#2767), the only place the tile shows it. OnAir is
// the channel with no single-decade era: a neutral block.
// The words over the brightest and the busiest pictures (#3407): the head scrim holds the number and
// name at 4.5:1 over plain white, and over a checkered, striped picture.
const WhitePicture = () => (
  <div aria-hidden style={{ background: "#ffffff", height: "100%", width: "100%" }} />
);
const BusyPicture = () => (
  <div
    aria-hidden
    style={{
      background:
        "repeating-linear-gradient(45deg, #f4f1e8 0 14px, #1b6ea8 14px 28px, #e8b923 28px 42px, #ffffff 42px 56px)",
      height: "100%",
      width: "100%",
    }}
  />
);

const OverWhitePicture: Story = {
  args: { art: <WhitePicture />, detail: "S2 E4 · 13 min left", progressPercent: 56 },
};
const OverBusyPicture: Story = {
  args: { art: <BusyPicture />, detail: "S2 E4 · 13 min left", progressPercent: 56 },
};

// A long channel name ends in an ellipsis before the "You were here" pill, and the pill covers nothing.
const LastWatchedLongName: Story = {
  args: {
    art: <WhitePicture />,
    channelName: "Late Night Coastal Mysteries and Other Stories",
    detail: "S1 E12 · 9 min left",
    last: true,
    progressPercent: 60,
  },
};

// A phone's tile, 165px wide (the mock): the number alone on top, the title on one line, then one
// caption line, channel name first.
const phoneWidth: Story["decorators"] = [
  (Story) => (
    <div style={{ maxWidth: 165 }}>
      <Story />
    </div>
  ),
];
const CompactWithArt: Story = {
  args: {
    art: <BusyPicture />,
    channelName: "Nature Hour",
    compact: true,
    detail: "13 min left",
    number: "12",
    title: "Heron Country: A Long Season on the Marsh",
  },
  decorators: phoneWidth,
};
const CompactNoArt: Story = { args: { ...CompactWithArt.args, art: null }, decorators: phoneWidth };
const CompactLastLongName: Story = {
  args: { ...CompactWithArt.args, channelName: "Late Night Coastal Mysteries", last: true },
  decorators: phoneWidth,
};

const Decade1970: Story = { args: { ...OnAir.args, decade: 1970 } };
const Decade1980: Story = { args: { ...OnAir.args, decade: 1980 } };
const Decade1990: Story = { args: { ...OnAir.args, decade: 1990 } };

// The light theme (#3178): the words over the picture are light on the scrim, as in the dark theme.
const LightOnAir: Story = { ...OnAir, globals: { theme: "light" } };
const LightLastWatched: Story = { ...LastWatched, globals: { theme: "light" } };
const LightFillingUp: Story = { ...FillingUp, globals: { theme: "light" } };
const LightCompact: Story = { ...Compact, globals: { theme: "light" } };

// Every decade and none side by side. The decade colours are the same in both themes.
const Decades: Story = {
  render: (args) => (
    <div style={{ display: "grid", gap: 12 }}>
      {[1970, 1980, 1990, undefined].map((decade, i) => (
        <WallTile
          key={decade ?? "none"}
          {...args}
          art={<Still hue={[40, 0, 190, 205][i] ?? 205} />}
          number={String(3 + i * 2)}
          {...(decade === undefined ? {} : { decade })}
        />
      ))}
    </div>
  ),
};

export default meta;
export {
  Compact,
  CompactLast,
  CompactLastLongName,
  CompactNoArt,
  CompactWithArt,
  Decade1970,
  Decade1980,
  Decade1990,
  Decades,
  FillingUp,
  LastWatched,
  LastWatchedLongName,
  LightCompact,
  LightFillingUp,
  LightLastWatched,
  LightOnAir,
  LightWatched,
  OnAir,
  OverBusyPicture,
  OverWhitePicture,
  Watched,
  WatchedCompact,
};
