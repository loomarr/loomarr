import { getPreviewChannelPodsMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { ReactElement } from "react";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { ChannelBreaks } from "./channel-breaks";

const renderBreaks = (ui: ReactElement) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>);
};

const pod = {
  entries: [
    {
      path: "b1.mp4",
      tunarrProgramId: "b1",
      name: "Bumper",
      kind: "bumper" as const,
      durationMs: 5000,
      isFallbackCard: false,
    },
    {
      path: "a1.mp4",
      tunarrProgramId: "a1",
      name: "Toy Ad",
      kind: "commercial" as const,
      durationMs: 30000,
      isFallbackCard: false,
    },
  ],
  totalMs: 35000,
  matchLevel: "exact" as const,
};

// ⚠ **`GET /v1/channels/{id}/pods` had NO caller before this** — the route shipped, three cache
// invalidations pointed at its query key, and nothing ever filled that cache. It survived because
// deleting it would have removed a capability: it is MEMBER-readable where the draft sandbox is
// admin-only, so it is the only way a viewer can see why a channel sounds the way it does.
describe("ChannelBreaks", () => {
  it("renders the channel's saved break pool", async () => {
    server.use(getPreviewChannelPodsMockHandler(pod));
    renderBreaks(<ChannelBreaks channelId="ch-1" />);

    expect(await screen.findByText("In the breaks")).toBeInTheDocument();
    expect(screen.getByLabelText("Pod segments")).toBeInTheDocument();
    expect(
      screen.getByText("2 clips between shows, assembled exactly as the channel builds them."),
    ).toBeInTheDocument();
  });

  // ⚠ A channel with no break pool is an ORDINARY state, not a fault — most obviously on an
  // install with no filler catalog at all. Rendering an empty panel on the member's only screen
  // would turn "this channel has no commercials" into something that looks broken.
  it("renders nothing when the channel has no break pool", async () => {
    server.use(getPreviewChannelPodsMockHandler({ entries: [], totalMs: 0, matchLevel: "bumper_card" }));
    const { container } = renderBreaks(<ChannelBreaks channelId="ch-1" />);

    await waitFor(() => expect(container).toBeEmptyDOMElement());
  });

  // ⚠ An install without filler configured answers 501. Same treatment: absence, not an error —
  // and `retry: false`, or three retries of a permanent 501 fill the console on every page view.
  it("renders nothing when filler is not set up", async () => {
    server.use(
      http.get("*/v1/channels/:id/pods", () =>
        HttpResponse.json({ title: "not implemented" }, { status: 501 }),
      ),
    );
    const { container } = renderBreaks(<ChannelBreaks channelId="ch-1" />);

    await waitFor(() => expect(container).toBeEmptyDOMElement());
  });
});
