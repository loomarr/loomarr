import type { DiscoveredClip } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { DiscoverPanel } from "./discover-panel";

// Search archive.org for clips to add, downloading nothing (§10, V33). ⚠ No per-result licence
// badge: archive.org declares one on ~8% of items and yt-dlp on none, so a chip would read
// "unknown" on nearly every row and imply a check that never happened (build plan §6.3).
const meta = {
  title: "Filler/DiscoverPanel",
  component: DiscoverPanel,
  decorators: [widthFrame(560)],
  args: { query: "1980s cereal commercial", onQueryChange: () => {}, onSearch: () => {} },
} satisfies Meta<typeof DiscoverPanel>;

type Story = StoryObj<typeof meta>;

const items: DiscoveredClip[] = [
  {
    id: "cm-1993-4",
    title: "Commercials 1993 vol. 4",
    year: 1993,
    url: "https://archive.org/details/cm-1993-4",
  },
  {
    id: "polaroid-1965",
    title: "1965 Polaroid Instant Camera commercial",
    year: 1965,
    url: "https://archive.org/details/polaroid-1965",
  },
  // No year — the common case, since Solr omits an absent field.
  { id: "untitled-reel", title: "Vintage advert reel", url: "https://archive.org/details/untitled-reel" },
  // No title either: still addable, shown by id rather than as a blank row.
  { id: "NJY-006_151284", url: "https://archive.org/details/NJY-006_151284" },
];

const note = "Licence information isn't available for most results. Check before reusing.";

// The everyday state: results found, some pickable, the licence caveat stated once.
const Results: Story = {
  args: { items, total: 54, licenceNote: note, searched: true, onAdd: () => {} },
};

// Before anyone has searched. ⚠ No "nothing matched" — saying it here would tell an operator
// their catalog is empty when they have not asked anything yet.
const Untouched: Story = { args: { items: [], total: 0, query: "" } };

const Searching: Story = { args: { items: [], total: 0, searching: true } };

const NothingMatched: Story = { args: { items: [], total: 0, searched: true } };

// ⚠ Without the filler image the RESULTS still show — an operator can fetch them by hand — so
// only the Add action disappears. Hiding the search behind a download gate would remove a
// capability that has nothing to do with downloading.
const CannotDownload: Story = {
  args: { items, total: 54, licenceNote: note, searched: true },
};

export default meta;
export { CannotDownload, NothingMatched, Results, Searching, Untouched };
