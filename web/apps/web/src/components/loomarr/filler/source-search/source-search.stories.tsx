import type { DiscoveredClip } from "@loomarr/api";
import { discoveredClips } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { SourceSearch } from "./source-search";

const noop = () => {};

const meta = {
  title: "Filler/SourceSearch",
  component: SourceSearch,
  args: {
    results: discoveredClips,
    query: "1980s cereal commercial",
    onQueryChange: noop,
    onSearch: noop,
    onQueue: noop,
  },
  decorators: [widthFrame(760)],
} satisfies Meta<typeof SourceSearch>;

type Story = StoryObj<typeof meta>;

// ⚠ The fixture is deliberately non-uniform, and this story is where that shows: row 1 has
// everything, row 2 has no date, row 3 knows neither duration nor quality. Absence is the
// COMMON case — archive.org has not probed every item — and the baseline is what proves an
// unknown renders as nothing rather than as "0:00".
const Default: Story = {};

const Searching: Story = {
  args: { searching: true },
};

// Nothing found yet: the footnote still has to be there, because "will this download something?"
// is a question an operator has before they see any results.
const NoResults: Story = {
  args: { results: [], query: "" },
};

// ⚠ The page is capped at 25; `total` is the FULL match count. Reporting the page length would
// tell an operator a 3000-match search found 25.
const CappedPage: Story = {
  args: {
    results: Array.from(
      { length: 25 },
      (_, i): DiscoveredClip => ({
        ...(discoveredClips[0] as DiscoveredClip),
        id: `clip-${i}`,
        title: `1980s cereal commercial — reel ${i + 1}`,
      }),
    ),
    total: 3000,
  },
};

const Queued: Story = {
  args: { queued: ["kelloggs-bran-flakes"] },
};

const SearchFailed: Story = {
  args: { results: [], error: "Couldn't reach archive.org. Try again in a moment." },
};

export default meta;
export { CappedPage, Default, NoResults, Queued, SearchFailed, Searching };
