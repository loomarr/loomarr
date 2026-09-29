import { type SurfChannelData, WatchingPanel } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";

// The panel under a portrait phone's picture (#1785, native mock 5e). Genre words only: the repo
// is public, so no real titles.
const favourite = (id: string, number: string, name: string, series: string, episode: string, left: string) =>
  ({
    channelLogoState: "missing",
    channelName: name,
    channelNumber: number,
    id,
    now: {
      artworkState: "missing",
      remainingLabel: left,
      seriesTitle: series,
      timeLabel: "8:00 PM–9:00 PM",
      title: `${series} · ${episode}`,
    },
  }) satisfies SurfChannelData;

const meta = {
  title: "Loomarr Components/Watching Panel",
  component: WatchingPanel,
  decorators: [
    (Story) => (
      <div style={{ boxSizing: "border-box", maxWidth: 390, minHeight: "100vh" }}>
        <Story />
      </div>
    ),
  ],
  parameters: { layout: "fullscreen" },
  args: {
    canPrevious: true,
    canSurf: true,
    channel: { name: "Late Night Science Fiction", number: "7" },
    clockLabel: "8:26 PM",
    density: "touch",
    favourites: [
      favourite("21", "21", "Nature Documentaries", "A nature series", "Caves", "24m left"),
      favourite("42", "42", "Sitcom Block", "A workplace sitcom", "The Long Weekend Shift", "11m left"),
      favourite("23", "23", "Martial Arts Theater", "A martial arts film", "Part One", "54m left"),
    ],
    live: { lagSeconds: 0, mode: "live" },
    onChannelDown: fn(),
    onChannelUp: fn(),
    onGoLive: fn(),
    onPause: fn(),
    onPlay: fn(),
    onPrevious: fn(),
    onTune: fn(),
    schedule: {
      next: { timeLabel: "9:04 PM", title: "A mystery series · Home" },
      now: {
        episodeLabel: "S2E4",
        facts: ["1996"],
        progressPercent: 43,
        remainingLabel: "34m left",
        seriesTitle: "An anthology series",
        timeLabel: "8:00 PM–9:00 PM",
        title: "An anthology series · Expanding Horizons",
      },
    },
  },
} satisfies Meta<typeof WatchingPanel>;

type Story = StoryObj<typeof meta>;

const Live: Story = {};
const PausedBehind: Story = { args: { live: { lagSeconds: 83, mode: "paused" } } };
const NoFavourites: Story = { args: { canPrevious: false, favourites: [] } };

export default meta;
export { Live, NoFavourites, PausedBehind };
