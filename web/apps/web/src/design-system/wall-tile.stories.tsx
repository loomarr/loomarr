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

export default meta;
export { Compact, CompactLast, FillingUp, LastWatched, OnAir };
