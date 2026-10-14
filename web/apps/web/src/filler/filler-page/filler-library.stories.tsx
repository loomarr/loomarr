import type {
  ClipDTO,
  FillerWatchOutputBody,
  ListFillerOutputBody,
  ListTaxonomyOutputBody,
  MeBody,
  PoolDTO,
  SettingsListOutputBody,
} from "@loomarr/api";
import { thumbnailedClip } from "@loomarr/fixtures";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { withRouter } from "@/test/story-utils";
import { FillerPage } from "./filler-page";

// The Library as a household sees it (#1817, decided on #1908). Ready clips carry no status chip;
// only a clip that isn't ready names its state. The visual suite shoots this at 390 too, where
// the grid goes 2-up compact and the stats and filters fold to one line each.
//
// Every name here is invented. The artwork is the shared same-origin image record from
// @loomarr/fixtures (served from .storybook/story-assets), never a remote URL.
const NAMES = [
  "Fizzwell Soda — summer jingle",
  "Northbay Motors — weekend sale",
  "Late-night station ident",
  "Crunchwell Oat Clusters — cartoon mascot spot with a long descriptive title",
  "Harborlight Savings — rainy day",
  "Coming up next bumper",
];

const clips: ClipDTO[] = NAMES.map((name, index) => ({
  ...thumbnailedClip,
  name,
  hash: `story-library-${index}`,
  brand: index % 2 === 0 ? name.split(" — ")[0] : undefined,
  kind: index === 2 ? "station_id" : index === 5 ? "bumper" : "commercial",
  era: index % 3 === 0 ? 1990 : 1987,
  durationMs: index === 5 ? 8_000 : 30_000,
  quality: "480p",
  playCount: index * 3,
  playsCounted: true,
  // One clip still preparing: the only chip a grid of ready clips should show.
  held: index === 1,
}));

const watch: FillerWatchOutputBody = {
  clips: 1284,
  health: "healthy",
  held: 0,
  sourcesOn: 3,
  sourcesReady: 3,
  sourcesTotal: 4,
};

const pool: PoolDTO = {
  clips: 1284,
  breakBody: 1150,
  eligible: 1052,
  untagged: 0,
  channels: [
    {
      channelId: "ch-12",
      name: "Weather Desk",
      number: 12,
      level: "widened",
      total: 41,
      durationMs: 3_480_000,
      categories: 3,
      brands: 6,
    },
    {
      channelId: "ch-4",
      name: "Night Owl Theatre",
      number: 4,
      level: "exact",
      total: 212,
      durationMs: 15_600_000,
      categories: 9,
      brands: 31,
    },
  ],
};

const taxonomy: ListTaxonomyOutputBody = {
  taxa: [],
  totalClips: 1284,
  taggedClips: 1284,
  unclassifiedClips: 0,
  axisCoverage: [],
};

const settings: SettingsListOutputBody = { settings: [], features: { filler: true } };

const me = (role: MeBody["role"]): MeBody => ({
  id: `${role}-1`,
  name: role === "admin" ? "Admin" : "Member",
  role,
  disabled: false,
  local: true,
  offlineLogin: false,
  quota: 0,
  autoApprove: false,
});

const list: ListFillerOutputBody = { clips, total: 1284 };

const withLibrary =
  (role: MeBody["role"]): Decorator =>
  (Story) => {
    window.fetch = ((input: RequestInfo | URL) => {
      const url = String(input);
      const body = url.includes("/auth/me")
        ? me(role)
        : url.includes("/v1/settings")
          ? settings
          : url.includes("/v1/filler/watch")
            ? watch
            : url.includes("/v1/filler/pool")
              ? pool
              : url.includes("/v1/taxonomy")
                ? taxonomy
                : list;
      return Promise.resolve(
        new Response(JSON.stringify(body), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      );
    }) as typeof fetch;
    return (
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <div style={{ height: 1100, display: "flex", flexDirection: "column" }}>
          <Story />
        </div>
      </QueryClientProvider>
    );
  };

const meta = {
  title: "Filler/Library",
  component: FillerPage,
  tags: ["route"],
  args: { tab: "library" },
  decorators: [withRouter("/filler/library")],
} satisfies Meta<typeof FillerPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Admin: Story = { decorators: [withLibrary("admin")] };

// A member browses the same library with no select checkboxes.
export const Member: Story = { decorators: [withLibrary("member")] };
