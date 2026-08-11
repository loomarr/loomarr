import type { SearchOutputBody } from "@loomarr/api";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { TooltipProvider } from "@/components/ui";
import { widthFrame } from "@/test/story-utils";
import { ChannelSeriesScope } from "./channel-series-scope";

const noop = () => {};

// ⚠ Generic, and every MEANINGFUL call site passes its type argument explicitly (GH #281). The
// parameter itself stays open because this helper only serialises — it is the call sites that
// must be checked against a DTO. An unchecked body is invisible to `tsc`, so a response type
// gaining a required field leaves the stub stale, the component reads fields off `undefined`, and
// the first sign is a Playwright baseline failure that reads like a rendering regression.
const jsonResponse = <T,>(body: T) =>
  new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });

// Like ChannelLineupEditor's story, this component owns a live generated-API hook
// (searchApi.useSearch for the picker) rather than taking injectable state, so `fetch` is
// stubbed deterministically — no backend, no new dependency (§14). The gallery snapshots the
// initial render only (no play()), so the picker never opens here and a flat 200 suffices.
const withStubbedSearch = (): Decorator => (Story) => {
  window.fetch = ((url: string) => {
    if (typeof url === "string" && url.includes("/v1/search")) {
      return Promise.resolve(
        jsonResponse<SearchOutputBody>({
          candidates: [
            { mediaType: "series", tvdbId: 71663, name: "The Simpsons", year: 1989, inLibrary: true },
          ],
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

// `policy.scope.series` — the "only these shows" narrowing. The field holds RESOLVED
// provisioning keys, never names, which is why it needs a search picker rather than a text box.
const meta = {
  title: "Channels/ChannelSeriesScope",
  component: ChannelSeriesScope,
  args: { onChange: noop },
  decorators: [withStubbedSearch(), widthFrame(480)],
} satisfies Meta<typeof ChannelSeriesScope>;

type Story = StoryObj<typeof meta>;

// Empty = no restriction; the lineup decides on its own.
const Empty: Story = { args: { policy: {} } };

// Picked shows render as chips carrying the key's source + id — accurate, since the policy
// stores no display name and a guessed title would be fiction.
const Picked: Story = {
  args: { policy: { scope: { series: ["series:tvdb:71663", "series:tvdb:73739"] } } },
};

export default meta;
export { Empty, Picked };
