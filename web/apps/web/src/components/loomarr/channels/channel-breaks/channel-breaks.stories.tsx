import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { widthFrame } from "@/test/story-utils";
import { ChannelBreaks } from "./channel-breaks";

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });

// ChannelBreaks owns the usePreviewChannelPods hook, so a story stubs `fetch` to answer
// GET /v1/channels/{id}/pods with a fixed body — the same approach ChannelUpcoming's and
// ChannelCyclePreview's stories use.
//
// ⚠ **NOT `parameters.msw`.** The first version of this file used it, and Storybook here has no
// MSW addon — so the handler did nothing, the fetch never resolved, the component correctly
// rendered `null`, and the visual harness hung waiting for `#storybook-root > *` to become
// visible. It failed even under `--update-snapshots`, which is the useful part: a story that
// renders nothing cannot be "fixed" by accepting a new baseline.
const withStubbedPods =
  (pod: unknown): Decorator =>
  (Story) => {
    window.fetch = (() => Promise.resolve(jsonResponse(pod))) as typeof fetch;
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    return (
      <QueryClientProvider client={client}>
        <Story />
      </QueryClientProvider>
    );
  };

// What plays BETWEEN the shows, on the channel Info panel (§10, §12).
//
// ⚠ The one surface for `GET /v1/channels/{id}/pods`, which had no caller at all until now. It is
// member-readable where the Filler tab's draft sandbox is admin-only, so this is how a viewer sees
// why a channel sounds the way it does.
//
// ⚠ **There is deliberately no Empty story.** The component renders `null` with no break pool —
// which is the right behaviour on a member's only screen — and a story of it would snapshot an
// empty root and hang the visual harness rather than proving anything. The empty and 501 cases are
// covered by the unit tests, where "renders nothing" is an assertion instead of a blank image.
const meta = {
  title: "Channels/ChannelBreaks",
  component: ChannelBreaks,
  decorators: [widthFrame(520)],
  args: { channelId: "ch-1" },
} satisfies Meta<typeof ChannelBreaks>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  decorators: [
    withStubbedPods({
      entries: [
        {
          path: "b1.mp4",
          tunarrProgramId: "b1",
          name: "We'll be right back",
          kind: "bumper",
          durationMs: 5000,
          isFallbackCard: false,
        },
        {
          path: "a1.mp4",
          tunarrProgramId: "a1",
          name: "Frosted Flakes",
          kind: "commercial",
          durationMs: 30000,
          isFallbackCard: false,
          era: 1992,
        },
        {
          path: "a2.mp4",
          tunarrProgramId: "a2",
          name: "TMNT figures",
          kind: "commercial",
          durationMs: 30000,
          isFallbackCard: false,
          era: 1992,
        },
      ],
      totalMs: 65000,
      matchLevel: "exact",
    }),
  ],
};
