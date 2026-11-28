import {
  LibraryCollectionEditor,
  LibraryCollections,
  type LibraryNeed,
  LibraryNeedsCards,
  type LibraryOrderNeed,
  type LibraryProblem,
  type LibraryScan,
  LibraryShelf,
  type LibraryShelfAdmin,
  type LibraryShelfProps,
  type LibraryShelfTitle,
  LibraryTitle,
  LibraryTitleMissing,
  LibraryWizardStep,
} from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, waitFor } from "storybook/test";
import { AppToaster } from "@/components/loomarr/feedback/app-toaster";
import { toastRemoved, toastRestoreRefused } from "@/library/extra-copy-toasts";
import { IgnoredList, LibraryNeedsView } from "@/library/library-needs-page";

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
  { id: "90000808", kind: "movie", name: "Copper Hill", year: 1971, ready: true, poster: art("#5a4a1f") },
  { id: "90000809", kind: "series", name: "The Glass Pier", year: 2015, ready: true, poster: art("#284e5c") },
  { id: "90000810", kind: "movie", name: "Winter Lantern", year: 1958, ready: true },
  { id: "90000811", kind: "movie", name: "Field Notes", year: 2022, ready: true, poster: art("#4a2f5a") },
  { id: "90000812", kind: "series", name: "Upriver", year: 2008, ready: true, poster: art("#2e4a2b") },
];

// The tabs' counts: the whole library, whatever the search.
const COUNTS = { all: 3412, movie: 2180, series: 1232 };

const IDLE: LibraryScan = {
  enabled: true,
  step: "idle",
  stepNumber: 0,
  done: 0,
  total: 0,
  paused: false,
  quiet: false,
};
// The three steps of an import, each with its own bar, and a pause for watching.
const WALKING: LibraryScan = {
  ...IDLE,
  step: "walking",
  stepNumber: 1,
  done: 112,
  total: 533,
  current: "TV / Night Ferry / Season 2",
  etaSeconds: 540,
};
const READING: LibraryScan = {
  ...IDLE,
  step: "reading",
  stepNumber: 2,
  done: 412,
  total: 1200,
  current: "Night Ferry S02E03.mkv",
};
const LOOKING_UP: LibraryScan = {
  ...IDLE,
  step: "lookingUp",
  stepNumber: 3,
  done: 40,
  total: 533,
  current: "Saturday Club (1987)",
  etaSeconds: 1500,
};
const PAUSED: LibraryScan = { ...READING, paused: true };
type ShelfStatus = LibraryShelfAdmin["status"];

const CLEAR: ShelfStatus = { health: "ok", scan: IDLE, held: [], offline: [], needsYou: 0 };

// An admin's wall: the status line, Needs you's count and Library settings.
const admin = (status: Partial<ShelfStatus> = {}): LibraryShelfAdmin => ({
  status: { ...CLEAR, ...status },
  checking: false,
  onCheckAgain: () => {},
  onRescan: () => {},
  onOpenSettings: () => {},
  onOpenNeeds: () => {},
});

const status = (scan: LibraryScan) => admin({ scan });

const shelf: LibraryShelfProps = {
  admin: admin(),
  filter: "all",
  onFilterChange: () => {},
  counts: COUNTS,
  sort: "name",
  onSortChange: () => {},
  onGenreChange: () => {},
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

// The mock's "titles needing a look": the count on the status line, Review beside it.
const NeedsYou: Story = { args: { admin: admin({ needsYou: 6 }) } };

// A file nothing in the catalogue matches: the needs-a-look mark, and no claim that it is a movie.
const Unidentified: Story = {
  args: {
    admin: admin({ needsYou: 1 }),
    titles: [
      { id: "90000813", kind: "movie", name: "Clip 12", year: 2019, ready: true, unidentified: true },
      ...TITLES,
    ],
  },
};

// Importing: the step's words over the preparation bar, with Needs you beside them.
const AddingStep1: Story = { args: { admin: status(WALKING) } };
const Adding: Story = { args: { admin: admin({ scan: READING, needsYou: 3 }) } };
const AddingStep3: Story = { args: { admin: status(LOOKING_UP) } };
const Paused: Story = { args: { admin: status(PAUSED) } };
const Checking: Story = { args: { admin: status({ ...WALKING, quiet: true }) } };

// The first page on its way: the tabs and the search, then placeholders where the posters go.
const Loading: Story = {
  args: { admin: admin({ needsYou: 6 }), counts: undefined, loading: true, titles: [] },
};

// Recently added first: the order the mock's home videos are shown in.
const RecentlyAdded: Story = { args: { sort: "recent", titles: [...TITLES].reverse() } };

const HeldRemoval: Story = {
  args: {
    admin: admin({
      held: [
        {
          path: "/media/movies",
          movies: 2406,
          episodes: 0,
          other: 0,
          titles: ["Amber Hollow", "Brightwater", "Calder Point"],
          titleCount: 2406,
        },
      ],
    }),
  },
};

const FolderOffline: Story = {
  args: { admin: admin({ health: "folder_offline", offline: ["/media/movies"] }) },
};

const Fresh: Story = {
  args: { admin: admin({ health: "needs_folders" }), titles: [] },
};

const NoMatch: Story = { args: { admin: admin({ needsYou: 6 }), search: "zebra 1977", titles: [] } };

const GENRES = [
  { name: "Comedy", titles: 640 },
  { name: "Documentary", titles: 212 },
  { name: "Drama", titles: 1204 },
];

// One genre of the wall, chosen beside the order.
const GenreChosen: Story = { args: { genres: GENRES, genre: "Drama" } };

// The letter rail beside the wall, on a desktop window: letters with no titles are disabled.
const LetterJump: Story = {
  args: {
    genres: GENRES,
    letters: [
      { letter: "#", offset: 0 },
      { letter: "A", offset: 12 },
      { letter: "B", offset: 240 },
      { letter: "H", offset: 1310 },
      { letter: "P", offset: 2204 },
      { letter: "T", offset: 2890 },
    ],
    onJump: () => {},
  },
};

// Under a search the list is in match order, so a letter is its first occurrence; the rail still
// reads A to Z.
const LetterJumpSearch: Story = {
  args: {
    genres: GENRES,
    search: "light",
    letters: [
      { letter: "P", offset: 0 },
      { letter: "H", offset: 1 },
      { letter: "N", offset: 3 },
    ],
    onJump: () => {},
  },
};

// A member browses the same wall read-only: no status line, no Needs you, no settings.
const Member: Story = { args: { admin: undefined } };
const MemberNoMatch: Story = { args: { admin: undefined, search: "zebra 1977", titles: [] } };
const MemberEmpty: Story = {
  args: { admin: undefined, counts: { all: 0, movie: 0, series: 0 }, titles: [] },
};

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

// Needs you's own page, with what the app puts in it.
const needsStory = (needs: LibraryNeed[], orders: LibraryOrderNeed[], problems: LibraryProblem[]) => (
  <LibraryNeedsView
    ignored={1}
    onBack={() => {}}
    onTabChange={() => {}}
    tab="waiting"
    waiting={needs.length + orders.length + problems.length}
  >
    <LibraryNeedsCards
      hideHeading
      needs={needs}
      onChooseOrder={answered}
      onIgnore={answered}
      onIgnoreFolder={answered}
      onLink={answered}
      onOpenTitle={() => {}}
      onSearch={search}
      orders={orders}
      problems={problems}
    />
  </LibraryNeedsView>
);

// Two titles of one folder, so the folder has its own answer.
const FOLDER = NEEDS.map((need) => ({ ...need, folder: "Discs" }));

const NeedsCards: Story = { render: () => needsStory(FOLDER, ORDERS, PROBLEMS) };

const NeedsPhone: Story = {
  globals: { viewport: { value: "mobile2" } },
  render: () => needsStory(FOLDER, ORDERS, PROBLEMS),
};

const NeedsIgnored: Story = {
  render: () => (
    <LibraryNeedsView ignored={1} onBack={() => {}} onTabChange={() => {}} tab="ignored" waiting={2}>
      <IgnoredList
        items={[
          {
            id: "90000813",
            kind: "movie",
            name: "title_t03.mkv",
            folder: "Discs",
            ignoredAt: "2026-01-02T03:04:05Z",
          },
        ]}
        onBringBack={() => {}}
      />
    </LibraryNeedsView>
  ),
};

const NothingNeedsYou: Story = { render: () => needsStory([], [], []) };

// A title's page acts through these; in a story every action goes through.
const titleFixes = {
  onUpdateDetails: answered,
  onCheckFiles: answered,
  onSearch: search,
  onLink: answered,
  onForget: answered,
  onRemoveExtra: answered,
};

// What the TV-app page draws beyond the story: the facts, the next airings and the people.
const movieExtras = {
  tagline: "Every plan has a loose thread.",
  score: 7.4,
  credits: [
    { name: "Imelda Varge", character: "Agnes Fell", role: "actor" as const },
    { name: "Tobin Ashcroft", character: "Rafe Fell", role: "actor" as const },
    { name: "Lorna Pettigrew", role: "director" as const },
  ],
  airings: [
    {
      channel: { id: "90000971", name: "Quiet Hours", number: 21 },
      when: "Next tonight at 9:00 PM",
      forecast: false,
    },
    {
      channel: { id: "90000972", name: "Late Show", number: 7 },
      when: "Next Saturday at 11:30 PM",
      forecast: true,
    },
  ],
  playing: {
    picture: { width: 1920, height: 1080, hdr: false },
    audioChannels: 2,
    subtitleLanguages: ["English"],
  },
  about: [{ label: "Country", value: "Ireland" }],
};

const Movie: Story = {
  render: () => (
    <LibraryTitle
      fix={{ ...titleFixes, onOpenApp: () => {} }}
      onBack={() => {}}
      title={{
        kind: "movie",
        name: "Paper Kites",
        year: 1996,
        backdrop: art("#6b3a2e"),
        ...movieExtras,
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

// A member's movie (#4476): the server leaves out its files and match, and the page offers no fix.
const MemberMovie: Story = {
  render: () => (
    <LibraryTitle
      onBack={() => {}}
      title={{
        kind: "movie",
        name: "Paper Kites",
        year: 1996,
        backdrop: art("#6b3a2e"),
        ...movieExtras,
        overview:
          "A small-town salesman, bursting at the seams with debt, hatches a plan that goes wrong the moment it starts.",
        genres: ["Crime", "Thriller"],
        rating: "R",
        runtimeMillis: 5_880_000,
        files: [],
        unplayable: 0,
        seasons: [],
        match: { by: "none" },
        detailsFrom: {},
        channels: [{ id: "90000971", name: "Quiet Hours", number: 21 }],
      }}
    />
  ),
};

const Series: Story = {
  render: () => (
    <LibraryTitle
      fix={titleFixes}
      onBack={() => {}}
      title={{
        kind: "series",
        name: "Harbour Lights",
        year: 1999,
        backdrop: art("#3b4a63"),
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

// The Fix menu open, on its solid surface over the backdrop.
const MovieFixOpen: Story = {
  ...Movie,
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(canvas.getByRole("button", { name: "Fix" }));
  },
};

const memberSeries = {
  kind: "series" as const,
  name: "Harbour Lights",
  year: 1999,
  genres: ["Drama"],
  files: [],
  unplayable: 0,
  seasons: [{ number: 1, episodes: [{ id: "90000011", name: "Dead Line", number: 1, ready: true }] }],
  match: { by: "none" as const },
  detailsFrom: {},
  channels: [{ id: "90000972", name: "Late Show", number: 7 }],
  airings: [
    {
      channel: { id: "90000972", name: "Late Show", number: 7 },
      when: "Next Thursday at 10:00 PM",
      forecast: true,
      episode: "Season 1, episode 1 · Dead Line",
    },
  ],
};

const MemberSeries: Story = {
  render: () => <LibraryTitle onBack={() => {}} title={{ ...memberSeries, backdrop: art("#3b4a63") }} />,
};

// A home video: no catalog details and on no channel.
const HomeVideo: Story = {
  render: () => (
    <LibraryTitle
      fix={titleFixes}
      onBack={() => {}}
      title={{
        kind: "movie",
        name: "Summer at the lake",
        year: 2019,
        genres: [],
        files: [],
        unplayable: 0,
        seasons: [],
        match: { by: "you" },
        detailsFrom: {},
        channels: [],
      }}
    />
  ),
};

const NotInLibrary: Story = {
  render: () => <LibraryTitleMissing onBack={() => {}} />,
};

// A show whose files don't say which of TMDB's orders they follow (#2151 decision 53): a hint
// that opens Needs you, and the waiting episodes named from their files. Season 2 starts open.
const SeriesOrderWaiting: Story = {
  render: () => (
    <LibraryTitle
      fix={{ ...titleFixes, onChooseOrder: () => {} }}
      onBack={() => {}}
      title={{
        kind: "series",
        name: "Tidewater",
        year: 2011,
        backdrop: art("#3b4a63"),
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

const COLLECTIONS = [
  {
    id: "c1",
    name: "Rainy Sundays",
    titles: 12,
    pile: pile("#3b4a63", "#6b3a2e", "#2f5a46"),
    importedFrom: "Emby",
    usedBy: [{ name: "Saturday Morning", number: 4 }],
  },
  { id: "c2", name: "Harbour nights", titles: 2, pile: pile("#3b4a63", undefined), usedBy: [] },
];

const SETS = [
  {
    id: "90000611",
    name: "The Lantern Keeper Collection",
    have: 2,
    total: 3,
    pile: pile("#6b3a2e", "#2f5a46"),
  },
  { id: "90000612", name: "Copper Hill films", have: 5, total: 9, pile: pile("#5a4a1f", "#284e5c") },
];

const manage = { onCopy: answered, onEdit: () => {}, onNew: () => {} };

const Collections: Story = {
  args: {
    filter: "collections",
    collections: <LibraryCollections collections={COLLECTIONS} manage={manage} sets={SETS} />,
  },
};

// None made yet: what a collection is, New collection, and the ready-made sets to start from.
const CollectionsNoneYet: Story = {
  args: {
    filter: "collections",
    collections: <LibraryCollections collections={[]} manage={manage} sets={SETS} />,
  },
};

// A member's Collections: the household's, read-only, and where they come from when there are none.
const MemberCollections: Story = {
  args: {
    admin: undefined,
    filter: "collections",
    collections: <LibraryCollections collections={COLLECTIONS} sets={SETS} />,
  },
};
const MemberCollectionsNoneYet: Story = {
  args: {
    admin: undefined,
    filter: "collections",
    collections: <LibraryCollections collections={[]} sets={SETS} />,
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
      readySoFar={TITLES.map((t) => ({ id: t.id, name: t.name, ready: t.ready, poster: t.poster }))}
      scan={READING}
    />
  ),
};

const wizardAdding = (scan: LibraryScan) => () => (
  <LibraryWizardStep
    {...wizardStep}
    folders={[{ ...wizardFolders[0], inUse: true } as (typeof wizardFolders)[number]]}
    scan={scan}
  />
);
const WizardStepStep1: Story = { render: wizardAdding(WALKING) };
const WizardStepStep3: Story = { render: wizardAdding(LOOKING_UP) };
const WizardStepPaused: Story = { render: wizardAdding(PAUSED) };
const WizardStepChecking: Story = { render: wizardAdding({ ...WALKING, quiet: true }) };

// Stop using asks first: the folder in use, with its confirm open.
const StopUsingFolder: Story = {
  render: wizardAdding(IDLE),
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole("button", { name: "Stop using" }));
  },
};

const episodeFile = {
  path: "/media/tv/Harbour Lights/Season 1/Harbour Lights S01E01.mkv",
  size: 2_100_000_000,
  lengthMillis: 2_580_000,
  picture: { width: 1920, height: 1080, hdr: false },
  audioChannels: 6,
  subtitles: [{ language: "en" }],
  extra: false,
  container: "matroska,webm",
  bitrate: 9_800_000,
  streams: [
    {
      index: 0,
      kind: "video" as const,
      codec: "h264",
      profile: "High",
      bitDepth: 8,
      frameRate: "24000/1001",
    },
    {
      index: 1,
      kind: "audio" as const,
      codec: "ac3",
      channels: 6,
      bitrate: 640_000,
      languageName: "English",
      default: true,
    },
    { index: 2, kind: "subtitle" as const, codec: "subrip", languageName: "English", default: true },
  ].map((track) => ({
    default: false,
    forced: false,
    commentary: false,
    descriptive: false,
    hearingImpaired: false,
    external: false,
    ...track,
  })),
  chapters: [
    { startMillis: 0, name: "Cold open" },
    { startMillis: 610_000, name: "The boats" },
  ],
};

// Season tabs with the specials last, and episodes with stills, summaries and lengths. One has no
// still, so its quiet frame shows.
const SeasonsWithStills: Story = {
  render: () => (
    <LibraryTitle
      fix={{ ...titleFixes, onEpisodeFiles: () => Promise.resolve([episodeFile]) }}
      onBack={() => {}}
      title={{
        ...memberSeries,
        backdrop: art("#3b4a63"),
        airings: [],
        seasons: [0, 1, 2].map((number) => ({
          number,
          episodes: [1, 2, 3].map((n) => ({
            id: `9000${number}${n}`,
            name: ["Dead Line", "Low Water", "The Long Night"][n - 1] ?? "",
            number: n,
            ready: true,
            overview: "A boat is late and the whole town finds a reason to walk down to the harbour wall.",
            lengthMillis: 2_580_000,
            still: n === 3 ? undefined : art(n === 1 ? "#4a5d7a" : "#6b4a3a"),
          })),
        })),
      }}
    />
  ),
};

// An episode row open, with More about its file showing the tracks and chapters.
const EpisodeMore: Story = {
  ...SeasonsWithStills,
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(canvas.getByRole("button", { name: /1\. Dead Line/ }));
    // A wide screen opens More itself.
    const more = await canvas.findByRole("button", { name: /(More|Less) about this file/ });
    if (more.textContent?.includes("More")) await userEvent.click(more);
    await expect(canvas.getByText("0:00 · Cold open")).toBeVisible();
  },
};

// A narrow page whose facts wrap: no line shows a dot at its start or its end.
const FactsWrapped: Story = {
  render: () => (
    <div style={{ maxWidth: 260 }}>
      <LibraryTitle
        onBack={() => {}}
        title={{
          ...memberSeries,
          score: 7.4,
          rating: "TV-14",
          genres: ["Drama", "Political", "Mystery"],
          backdrop: art("#3b4a63"),
        }}
      />
    </div>
  ),
  play: async ({ canvas }) => {
    const dots = canvas.getAllByText("·");
    // The clipped row: the nearest ancestor that hides its overflow.
    let clip = dots[0]?.parentElement;
    while (clip && getComputedStyle(clip).overflowX !== "hidden") clip = clip.parentElement;
    const row = clip?.getBoundingClientRect();
    const lines = new Map<number, DOMRect[]>();
    for (const dot of dots) {
      const fact = dot.parentElement?.parentElement?.getBoundingClientRect();
      if (fact) lines.set(Math.round(fact.top), [...(lines.get(Math.round(fact.top)) ?? []), fact]);
    }
    await expect(lines.size).toBeGreaterThan(1);
    for (const line of lines.values()) {
      const first = Math.min(...line.map((r) => r.left));
      // The line's first dot column ends where the clipped row begins.
      await expect(first + 20).toBeLessThanOrEqual((row?.left ?? Number.NaN) + 1);
    }
  },
};

// A movie with an extra copy, whose removal shows the toast the app shows. Its Undo is refused,
// as it is once the undo has run out.
const ExtraCopyRemoved: Story = {
  render: () => (
    <>
      <LibraryTitle
        fix={{
          ...titleFixes,
          onRemoveExtra: async () => {
            toastRemoved("Paper Kites DVD.mkv", () =>
              toastRestoreRefused(
                "The file isn't in the trash",
                "It may have been emptied or moved. Check that its drive is connected.",
              ),
            );
          },
        }}
        onBack={() => {}}
        title={{
          kind: "movie",
          name: "Paper Kites",
          year: 1996,
          backdrop: art("#6b3a2e"),
          genres: ["Crime"],
          files: [
            {
              path: "/media/movies/Paper Kites (1996)/Paper Kites DVD.mkv",
              size: 4_300_000_000,
              subtitles: [],
              extra: true,
            },
          ],
          unplayable: 0,
          seasons: [],
          match: { by: "app", app: "Radarr" },
          detailsFrom: {},
          channels: [],
        }}
      />
      <AppToaster />
    </>
  ),
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(canvas.getByRole("button", { name: "Remove Paper Kites DVD.mkv" }));
    const undo = await screen.findByRole("button", { name: "Undo" });
    // A toast fades in.
    await waitFor(() => expect(undo).toBeVisible());
  },
};

const ExtraCopyUndoRefused: Story = {
  ...ExtraCopyRemoved,
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(canvas.getByRole("button", { name: "Remove Paper Kites DVD.mkv" }));
    await userEvent.click(await screen.findByRole("button", { name: "Undo" }));
    const said = await screen.findByText("The file isn't in the trash");
    await waitFor(() => expect(said).toBeVisible());
  },
};

export default meta;
export {
  Adding,
  AddingStep1,
  AddingStep3,
  CaughtUp,
  Checking,
  CollectionEditor,
  CollectionEditorInUse,
  Collections,
  CollectionsNoneYet,
  EpisodeMore,
  ExtraCopyRemoved,
  ExtraCopyUndoRefused,
  FactsWrapped,
  FolderOffline,
  Fresh,
  GenreChosen,
  HeldRemoval,
  HomeVideo,
  LetterJump,
  LetterJumpSearch,
  Loading,
  Member,
  MemberCollections,
  MemberCollectionsNoneYet,
  MemberEmpty,
  MemberMovie,
  MemberNoMatch,
  MemberSeries,
  Movie,
  MovieFixOpen,
  NeedsCards,
  NeedsIgnored,
  NeedsPhone,
  NeedsYou,
  NewCollection,
  NoMatch,
  NothingNeedsYou,
  NotInLibrary,
  Paused,
  RecentlyAdded,
  SeasonsWithStills,
  Series,
  SeriesOrderWaiting,
  StopUsingFolder,
  Unidentified,
  WizardStep,
  WizardStepAdding,
  WizardStepChecking,
  WizardStepPaused,
  WizardStepStep1,
  WizardStepStep3,
};
