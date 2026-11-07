import {
  type LibraryNeed,
  LibraryNeedsCards,
  LibraryNeedsStrip,
  LibraryShelf,
  type LibraryShelfProps,
  type LibraryShelfTitle,
  LibraryTitle,
} from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";

// The Library (#2151, design/library variant D): the poster grid in each scan state and a title's
// page. Stories load no remote images: a title with artwork gets a flat stand-in in its own colour.

const art = (color: string) => <div style={{ background: color, height: "100%", width: "100%" }} />;

const TITLES: LibraryShelfTitle[] = [
  { id: "90000801", kind: "series", name: "Harbour Lights", year: 1999, ready: true, poster: art("#3b4a63") },
  { id: "90000802", kind: "movie", name: "Paper Kites", year: 1996, ready: true, poster: art("#6b3a2e") },
  { id: "90000803", kind: "movie", name: "The Long Way to the Coast", year: 2004, ready: false },
  { id: "90000804", kind: "series", name: "Night Ferry", year: 2011, ready: true },
  { id: "90000805", kind: "series", name: "Saturday Club", year: 1987, ready: false },
  { id: "90000806", kind: "movie", name: "Quiet Orchard", year: 2019, ready: true, poster: art("#2f5a46") },
  { id: "90000807", kind: "movie", name: "Lantern Season", ready: true },
];

const IDLE = { enabled: true, stage: "idle", headersDone: 0, headersTotal: 0, paused: false } as const;

const shelf: LibraryShelfProps = {
  status: { health: "ok", scan: IDLE, held: [], offline: [] },
  checking: false,
  onCheckAgain: () => {},
  onRescan: () => {},
  onOpenSettings: () => {},
  filter: "all",
  onFilterChange: () => {},
  search: "",
  onSearchChange: () => {},
  titles: TITLES,
  loading: false,
  onOpenTitle: () => {},
};

const meta = {
  title: "Loomarr Components/Library",
  component: LibraryShelf,
  args: shelf,
  parameters: { layout: "fullscreen" },
  decorators: [
    (Story) => (
      <div style={{ padding: 32 }}>
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof LibraryShelf>;

type Story = StoryObj<typeof meta>;

const CaughtUp: Story = {};

const Adding: Story = {
  args: {
    status: {
      health: "ok",
      scan: { enabled: true, stage: "reading", headersDone: 412, headersTotal: 1200, paused: false },
      held: [],
      offline: [],
    },
  },
};

const Paused: Story = {
  args: {
    status: {
      health: "ok",
      scan: { enabled: true, stage: "reading", headersDone: 412, headersTotal: 1200, paused: true },
      held: [],
      offline: [],
    },
  },
};

const HeldRemoval: Story = {
  args: {
    status: {
      health: "ok",
      scan: IDLE,
      held: [{ path: "/media/movies", movies: 2406, episodes: 0, other: 0 }],
      offline: [],
    },
  },
};

const FolderOffline: Story = {
  args: { status: { health: "folder_offline", scan: IDLE, held: [], offline: ["/media/movies"] } },
};

const Fresh: Story = {
  args: { status: { health: "needs_folders", scan: IDLE, held: [], offline: [] }, titles: [] },
};

const NoMatch: Story = { args: { search: "zebra", titles: [] } };

// Needs you: one guess the name agrees with, one with nothing to go on.
const NEEDS: LibraryNeed[] = [
  {
    id: "90000811",
    kind: "movie",
    files: ["/media/movies/Paper.Kites.1997.1080p/Paper.Kites.1997.1080p.mkv"],
    best: {
      tmdbId: "90000901",
      kind: "movie",
      name: "Paper Kites",
      year: 1996,
      overview: "A small-town salesman hatches a plan that goes wrong the moment it starts.",
      poster: art("#6b3a2e"),
      nameMatches: true,
      yearMatches: false,
      lengthMatches: true,
    },
    others: [
      {
        tmdbId: "90000902",
        kind: "movie",
        name: "Paper Kites",
        year: 2014,
        nameMatches: true,
        yearMatches: false,
        lengthMatches: false,
      },
    ],
  },
  {
    id: "90000812",
    kind: "series",
    files: ["/media/tv/disc2/title_t01.mkv", "/media/tv/disc2/title_t02.mkv"],
    others: [],
  },
];

const answered = () => Promise.resolve();
const search = () => Promise.resolve([]);

const NeedsStrip: Story = {
  args: { needs: <LibraryNeedsStrip needs={NEEDS} onLink={answered} onOpenNeeds={() => {}} /> },
};

const NeedsCards: Story = {
  args: {
    showTitles: false,
    needs: <LibraryNeedsCards needs={NEEDS} onIgnore={answered} onLink={answered} onSearch={search} />,
  },
};

const NothingNeedsYou: Story = {
  args: {
    showTitles: false,
    needs: <LibraryNeedsCards needs={[]} onIgnore={answered} onLink={answered} onSearch={search} />,
  },
};

const Movie: Story = {
  render: () => (
    <LibraryTitle
      onBack={() => {}}
      title={{
        kind: "movie",
        name: "Paper Kites",
        year: 1996,
        poster: art("#6b3a2e"),
        overview:
          "A small-town salesman, bursting at the seams with debt, hatches a plan that goes wrong the moment it starts.",
        genres: ["Crime", "Thriller"],
        rating: "R",
        runtimeMillis: 5_880_000,
        files: [{ path: "/media/movies/Paper Kites (1996)/Paper Kites (1996).mkv", size: 8_100_000_000 }],
        seasons: [],
      }}
    />
  ),
};

const Series: Story = {
  render: () => (
    <LibraryTitle
      onBack={() => {}}
      title={{
        kind: "series",
        name: "Harbour Lights",
        year: 1999,
        poster: art("#3b4a63"),
        overview: "The keepers of a lighthouse town and the families who wait for the boats to come home.",
        genres: ["Drama", "Political"],
        rating: "TV-14",
        files: [],
        seasons: [1, 2, 3, 4].map((number) => ({
          number,
          episodes: Array.from({ length: 22 }, (_, i) => ({
            id: `9000${number}${String(i + 1).padStart(3, "0")}`,
            name: `Episode ${i + 1}`,
            number: i + 1,
            ready: true,
          })),
        })),
      }}
    />
  ),
};

export default meta;
export {
  Adding,
  CaughtUp,
  FolderOffline,
  Fresh,
  HeldRemoval,
  Movie,
  NeedsCards,
  NeedsStrip,
  NoMatch,
  NothingNeedsYou,
  Paused,
  Series,
};
