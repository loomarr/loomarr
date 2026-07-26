import type { ProposalItem } from "@loomarr/api";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { TooltipProvider } from "@/components/ui";
import { widthFrame } from "@/test/story-utils";
import { ProposalEdit } from "./proposal-edit";

const noop = () => {};

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });

// Owns a live generated-API hook (searchApi.useSearch for the add picker), so `fetch` is stubbed
// deterministically — no backend, no new dependency (§14), the same shape as the lineup editor's
// story. The gallery snapshots the initial render only, so the picker never opens here.
const withStubbedSearch = (): Decorator => (Story) => {
  window.fetch = ((url: string) => {
    if (typeof url === "string" && url.includes("/v1/search")) {
      return Promise.resolve(
        jsonResponse({
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

// Edit-before-approve (V25b): drop a title, add one via search, leave the requester a note. The
// edit is a DELTA passed to the one approval gate, never a client-applied "final" list.
const meta = {
  title: "AI/ProposalEdit",
  component: ProposalEdit,
  args: { onChange: noop, lineup, acquisitions },
  decorators: [withStubbedSearch(), widthFrame(560)],
} satisfies Meta<typeof ProposalEdit>;

type Story = StoryObj<typeof meta>;

// Nothing modified — the state an approver lands on. No Reset offered; the note is optional.
const Default: Story = {};

// Mid-approval: the controls lock so a click cannot race the request in flight.
const Busy: Story = { args: { disabled: true } };

export default meta;
export { Busy, Default };
