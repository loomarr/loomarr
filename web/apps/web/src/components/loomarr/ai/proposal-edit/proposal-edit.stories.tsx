import type { ProposalItem, SearchOutputBody } from "@loomarr/api";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { TooltipProvider } from "@/components/ui";
import type { MovieCollectionChoice } from "@/suggest/movie-collection-choices";
import { widthFrame } from "@/test/story-utils";
import { ProposalEdit } from "./proposal-edit";

const noop = () => {};

// ⚠ Generic, with the meaningful call site passing its type argument explicitly (GH #281). The
// parameter stays open because this helper only serialises — it is the call sites that must be
// checked against a DTO. An unchecked body is invisible to `tsc`, so a response type gaining a
// required field leaves the stub stale and the failure surfaces as a Playwright baseline diff
// that reads like a rendering regression.
const jsonResponse = <T,>(body: T) =>
  new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });

// Owns a live generated-API hook (searchApi.useSearch for the add picker), so `fetch` is stubbed
// deterministically — no backend, no new dependency (§14), the same shape as the lineup editor's
// story. The gallery snapshots the initial render only, so the picker never opens here.
const withStubbedSearch = (): Decorator => (Story) => {
  window.fetch = ((url: string) => {
    if (typeof url === "string" && url.includes("/v1/search")) {
      return Promise.resolve(
        jsonResponse<SearchOutputBody>({
          candidates: [{ mediaType: "movie", tmdbId: 1701, name: "Con Air", year: 1997, inLibrary: false }],
        }),
      );
    }
    return Promise.resolve(jsonResponse({}));
  }) as typeof fetch;

  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <Story />
      </TooltipProvider>
    </QueryClientProvider>
  );
};

const lineup: ProposalItem[] = [
  { name: "Heat", year: 1995, mediaType: "movie", tmdbId: 949, inLibrary: true },
  { name: "Point Break", year: 1991, mediaType: "movie", tmdbId: 9426, inLibrary: true },
];

const acquisitions: ProposalItem[] = [
  { name: "The Simpsons", year: 1989, mediaType: "series", tvdbId: 71663, inLibrary: false },
];

const philosopherStone: ProposalItem = {
  name: "Harry Potter and the Philosopher's Stone",
  year: 2001,
  mediaType: "movie",
  tmdbId: 671,
  inLibrary: true,
  libraryItemId: "library-671",
};

const harryPotterCollection: MovieCollectionChoice = {
  tmdbId: 1241,
  name: "Harry Potter Collection",
  members: [
    philosopherStone,
    {
      name: "Harry Potter and the Chamber of Secrets",
      year: 2002,
      mediaType: "movie",
      tmdbId: 672,
      inLibrary: false,
    },
    {
      name: "Harry Potter and the Prisoner of Azkaban",
      year: 2004,
      mediaType: "movie",
      tmdbId: 673,
      inLibrary: true,
      libraryItemId: "library-673",
    },
    {
      name: "Harry Potter and the Goblet of Fire",
      year: 2005,
      mediaType: "movie",
      tmdbId: 674,
      inLibrary: false,
    },
  ],
};

// Edit-before-approve (V25b): drop a title, add one via search, leave the requester a note. The
// edit is a DELTA passed to the one approval gate, never a client-applied "final" list.
const meta = {
  title: "AI/ProposalEdit",
  component: ProposalEdit,
  args: { onChange: noop, lineup, acquisitions, showNote: true },
  decorators: [withStubbedSearch(), widthFrame(560)],
} satisfies Meta<typeof ProposalEdit>;

type Story = StoryObj<typeof meta>;

// Nothing modified — the state an approver lands on. No Reset offered; the note is optional.
const Default: Story = {};

// Mid-approval: the controls lock so a click cannot race the request in flight.
const Busy: Story = { args: { disabled: true } };

// A proposal containing one franchise film exposes one restrained collection choice. The full
// roster stays collapsed until the reviewer asks to choose individual films.
const MovieCollection: Story = {
  args: {
    lineup: [philosopherStone],
    acquisitions: [],
    movieCollections: [harryPotterCollection],
    showNote: false,
  },
};

export default meta;
export { Busy, Default, MovieCollection };
