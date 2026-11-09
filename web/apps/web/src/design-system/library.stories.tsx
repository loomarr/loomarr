import {
  LibraryCollectionEditor,
  LibraryCollections,
  type LibraryNeed,
  LibraryNeedsCards,
  LibraryNeedsStrip,
  type LibraryOrderNeed,
  type LibraryProblem,
  LibraryShelf,
  type LibraryShelfProps,
  type LibraryShelfTitle,
  LibraryTitle,
  LibraryWizardStep,
} from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";

// The Library (#2151, design/library variant D): the poster grid in each scan state and a title's
// page. Stories load no remote images: a title with artwork gets a flat stand-in in its own colour.

const art = (color: string) => <div style={{ background: color, height: "100%", width: "100%" }} />;

const TITLES: LibraryShelfTitle[] = [
  { id: "90000801", kind: "series", name: "Harbour Lights", year: 1999, ready: true, poster: art("#3b4a63") },
  { id: "90000802", kind: "movie", name: "Paper Kites", year: 1996, ready: true, poster: art("#6b3a2e") },
  { id: "90000803", kind: "movie", name: "The Long Way to the Coast", year: 2004, ready: false },
  { id: "90000804", kind: "series", name: "Night Ferry", year: 2011, ready: true, attention: true },
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
      externalId: { namespace: "tmdb", value: "90000901" },
      kind: "movie",
      name: "Paper Kites",
      year: 1996,
      overview: "A small-town salesman hatches a plan that goes wrong the moment it starts.",
      poster: art("#6b3a2e"),
      nameMatches: true,
      yearMatches: false,
      lengthMatches: true,
      episodesMatches: false,
    },
    others: [
      {
        externalId: { namespace: "tmdb", value: "90000902" },
        kind: "movie",
        name: "Paper Kites",
        year: 2014,
        nameMatches: true,
        yearMatches: false,
        lengthMatches: false,
        episodesMatches: false,
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

// Titles that need a look: episodes that can't be played, and a movie's extra copy.
const PROBLEMS: LibraryProblem[] = [
  { id: "90000804", kind: "series", name: "Night Ferry", unplayable: 2, extraBytes: 0 },
  {
    id: "90000806",
    kind: "movie",
    name: "Quiet Orchard",
    poster: art("#2f5a46"),
    unplayable: 0,
    extraBytes: 4_300_000_000,
  },
];

// Shows whose episode order to say (#2151 decision 53): one TMDB lists in several orders, the DVD
// one fitting the disk, and one whose second season on disk runs a file longer than TMDB's.
const tideFile = (episode: number) => ({
  path: `/media/tv/Tidewater/Season 02/Tidewater.S02E${String(episode).padStart(2, "0")}.mkv`,
  season: 2,
  episode,
});
const ORDERS: LibraryOrderNeed[] = [
  {
    id: "90000821",
    name: "Tidewater",
    poster: art("#3b4a63"),
    disk: [
      { season: 1, episodes: 12 },
      { season: 2, episodes: 14 },
    ],
    files: [tideFile(6), tideFile(7), tideFile(8), tideFile(9), tideFile(10)],
    choices: [
      {
        choice: "catalog",
        seasons: [
          { season: 1, episodes: 13 },
          { season: 2, episodes: 13 },
        ],
        episodes: 26,
        fits: false,
      },
      {
        choice: "order",
        orderId: "90000799a",
        kind: "dvd",
        name: "DVD Order",
        seasons: [
          { season: 1, episodes: 12 },
          { season: 2, episodes: 14 },
        ],
        episodes: 26,
        fits: true,
      },
      {
        choice: "order",
        orderId: "90000799b",
        kind: "absolute",
        name: "Absolute Order",
        seasons: [{ season: 1, episodes: 26 }],
        episodes: 26,
        fits: false,
      },
    ],
  },
  {
    id: "90000822",
    name: "Kestrel Bay",
    disk: [{ season: 1, episodes: 14 }],
    files: [{ path: "/media/tv/Kestrel Bay/Kestrel.Bay.S01E14.mkv", season: 1, episode: 14 }],
    choices: [{ choice: "catalog", seasons: [{ season: 1, episodes: 13 }], episodes: 13, fits: false }],
  },
];

const answered = () => Promise.resolve();
const search = () => Promise.resolve([]);

// With Needs you waiting, the status line says only "Up to date" (#2779).
const NEEDS_WAITING = { ...shelf.status, needsYou: true };

const NeedsStrip: Story = {
  args: {
    status: NEEDS_WAITING,
    needs: (
      <LibraryNeedsStrip
        needs={NEEDS}
        onLink={answered}
        onOpenNeeds={() => {}}
        onOpenTitle={() => {}}
        orders={ORDERS}
        problems={PROBLEMS}
      />
    ),
  },
};

const NeedsCards: Story = {
  args: {
    status: NEEDS_WAITING,
    showTitles: false,
    needs: (
      <LibraryNeedsCards
        needs={NEEDS}
        onChooseOrder={answered}
        onIgnore={answered}
        onLink={answered}
        onOpenTitle={() => {}}
        onSearch={search}
        orders={ORDERS}
        problems={PROBLEMS}
      />
    ),
  },
};

const NothingNeedsYou: Story = {
  args: {
    showTitles: false,
    needs: (
      <LibraryNeedsCards
        needs={[]}
        onChooseOrder={answered}
        onIgnore={answered}
        onLink={answered}
        onOpenTitle={() => {}}
        onSearch={search}
        orders={[]}
        problems={[]}
      />
    ),
  },
};

// A title's page acts through these; in a story every action goes through.
const titleActions = {
  onBack: () => {},
  onUpdateDetails: answered,
  onCheckFiles: answered,
  onSearch: search,
  onLink: answered,
  onForget: answered,
  onRemoveExtra: answered,
};

const Movie: Story = {
  render: () => (
    <LibraryTitle
      {...titleActions}
      onOpenApp={() => {}}
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
        files: [
          {
            path: "/media/movies/Paper Kites (1996)/Paper Kites (1996).mkv",
            size: 8_100_000_000,
            lengthMillis: 5_880_000,
            picture: { width: 3840, height: 2160, hdr: true },
            audioChannels: 6,
            subtitles: [{ language: "en" }, { language: "es" }],
            extra: false,
          },
          {
            path: "/media/movies/Paper Kites (1996)/Paper Kites (1996) DVD.mkv",
            size: 4_300_000_000,
            lengthMillis: 5_880_000,
            picture: { width: 720, height: 480, hdr: false },
            audioChannels: 2,
            subtitles: [],
            extra: true,
          },
        ],
        unplayable: 0,
        seasons: [],
        match: { by: "app", app: "Radarr" },
        detailsFrom: { description: "TMDB", rating: "TMDB" },
        channels: [{ id: "90000971", name: "Quiet Hours", number: 21 }],
      }}
    />
  ),
};

const Series: Story = {
  render: () => (
    <LibraryTitle
      {...titleActions}
      title={{
        kind: "series",
        name: "Harbour Lights",
        year: 1999,
        poster: art("#3b4a63"),
        overview: "The keepers of a lighthouse town and the families who wait for the boats to come home.",
        genres: ["Drama", "Political"],
        rating: "TV-14",
        files: [],
        unplayable: 2,
        seasons: [1, 2, 3, 4].map((number) => ({
          number,
          episodes: Array.from({ length: 22 }, (_, i) => ({
            id: `9000${number}${String(i + 1).padStart(3, "0")}`,
            name: `Episode ${i + 1}`,
            number: i + 1,
            ready: true,
          })),
        })),
        match: { by: "you" },
        detailsFrom: { description: "TMDB", rating: "TMDB", episodeNames: "Sonarr" },
        channels: [
          { id: "90000972", name: "Late Show", number: 7 },
          { id: "90000973", name: "Sunday Stories", number: 12 },
        ],
      }}
    />
  ),
};

// A show whose files don't say which of TMDB's orders they follow (#2151 decision 53): a hint
// that opens Needs you, and the waiting episodes named from their files. Season 2 starts open.
const SeriesOrderWaiting: Story = {
  render: () => (
    <LibraryTitle
      {...titleActions}
      onChooseOrder={() => {}}
      title={{
        kind: "series",
        name: "Tidewater",
        year: 2011,
        poster: art("#3b4a63"),
        genres: ["Drama"],
        files: [],
        unplayable: 0,
        seasons: [1, 2].map((number) => ({
          number,
          episodes: Array.from({ length: number === 1 ? 12 : 8 }, (_, i) => ({
            id: `9000${number}${String(i + 1).padStart(3, "0")}`,
            name: number === 2 && i >= 5 ? `Tidewater S02E0${i + 1}` : `Episode ${i + 1}`,
            number: i + 1,
            ready: true,
            fromFile: number === 2 && i >= 5,
          })),
        })),
        match: { by: "file" },
        detailsFrom: { description: "TMDB", episodeNames: "TMDB" },
        channels: [],
      }}
    />
  ),
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole("button", { name: /Season 2/ }));
  },
};

// Collections: the household's own, one brought over and on a channel, and the ready-made sets.
const pile = (...colors: (string | undefined)[]) =>
  colors.map((color, i) => ({ name: TITLES[i]?.name ?? "", poster: color ? art(color) : undefined }));

const Collections: Story = {
  args: {
    filter: "collections",
    collections: (
      <LibraryCollections
        collections={[
          {
            id: "c1",
            name: "Rainy Sundays",
            titles: 12,
            pile: pile("#3b4a63", "#6b3a2e", "#2f5a46"),
            importedFrom: "Emby",
            usedBy: [{ name: "Saturday Morning", number: 4 }],
          },
          { id: "c2", name: "Harbour nights", titles: 2, pile: pile("#3b4a63", undefined), usedBy: [] },
        ]}
        onCopy={answered}
        onEdit={() => {}}
        onNew={() => {}}
        sets={[
          {
            id: "90000611",
            name: "The Lantern Keeper Collection",
            have: 2,
            total: 3,
            pile: pile("#6b3a2e", "#2f5a46"),
          },
        ]}
      />
    ),
  },
};

const editorActions = {
  onNameChange: () => {},
  onDescriptionChange: () => {},
  onMove: () => {},
  onRemove: () => {},
  onSearch: () => Promise.resolve([]),
  onAdd: () => {},
  onBack: () => {},
  onCancel: () => {},
  onSave: () => {},
  onDelete: () => {},
  busy: false,
};

const MEMBERS = TITLES.slice(0, 4).map((t) => ({ id: t.id, name: t.name, year: t.year, poster: t.poster }));

const CollectionEditor: Story = {
  render: () => (
    <LibraryCollectionEditor
      {...editorActions}
      description="The slow ones, for a wet afternoon."
      members={MEMBERS}
      name="Rainy Sundays"
      saved
      usedBy={[]}
    />
  ),
};

const CollectionEditorInUse: Story = {
  render: () => (
    <LibraryCollectionEditor
      {...editorActions}
      description=""
      members={MEMBERS}
      name="Rainy Sundays"
      saved
      usedBy={[{ name: "Saturday Morning", number: 4 }]}
    />
  ),
};

const NewCollection: Story = {
  render: () => (
    <LibraryCollectionEditor
      {...editorActions}
      description=""
      members={[]}
      name=""
      saved={false}
      usedBy={[]}
    />
  ),
};

// The setup wizard's Library step: one folder found through Emby and Radarr, one Loomarr can't open.
const wizardFolders = [
  {
    path: "/media/movies",
    inUse: false,
    readable: true,
    checkedAt: "2026-10-01T19:55:00Z",
    addedByHand: false,
    foundIn: [
      { authority: "emby" as const, path: "/data/movies" },
      { authority: "radarr" as const, path: "/movies" },
    ],
  },
  {
    path: "/mnt/tv",
    inUse: false,
    readable: false,
    addedByHand: false,
    foundIn: [{ authority: "sonarr" as const, path: "/tv" }],
  },
];

const wizardStep = {
  pathMap: [
    { authority: "emby" as const, authorityPath: "/data/movies", localPath: "/media/movies", byHand: false },
    { authority: "radarr" as const, authorityPath: "/movies", localPath: "/media/movies", byHand: false },
  ],
  checking: false,
  onCheckAgain: () => {},
  onUseFolder: () => {},
  onAddFolder: () => Promise.resolve(undefined),
  now: Date.parse("2026-10-01T20:00:00Z"),
};

const WizardStep: Story = {
  render: () => <LibraryWizardStep {...wizardStep} folders={wizardFolders} scan={IDLE} />,
};

const WizardStepAdding: Story = {
  render: () => (
    <LibraryWizardStep
      {...wizardStep}
      folders={[{ ...wizardFolders[0], inUse: true } as (typeof wizardFolders)[number]]}
      readySoFar={TITLES.filter((t) => t.ready).map((t) => ({ id: t.id, name: t.name, poster: t.poster }))}
      scan={{ enabled: true, stage: "reading", headersDone: 412, headersTotal: 1200, paused: false }}
    />
  ),
};

export default meta;
export {
  Adding,
  CaughtUp,
  CollectionEditor,
  CollectionEditorInUse,
  Collections,
  FolderOffline,
  Fresh,
  HeldRemoval,
  Movie,
  NeedsCards,
  NeedsStrip,
  NewCollection,
  NoMatch,
  NothingNeedsYou,
  Paused,
  Series,
  SeriesOrderWaiting,
  WizardStep,
  WizardStepAdding,
};
