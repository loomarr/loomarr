import type { ChannelDTO, ClipDTO } from "@loomarr/api";
import {
  getClipChannelFitMockHandler,
  getGetChannelMockHandler,
  getListChannelsMockHandler,
  getUpdateChannelMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { channel } from "@/test/fixtures/channels";
import { server } from "@/test/msw/server";
import { PinClipDialog } from "./pin-clip-dialog";

const makeWrapper = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

const clip: ClipDTO = {
  hash: "clip-9-hash",
  tunarrProgramId: "clip-9",
  name: "Frosted Flakes",
  kind: "commercial",
  durationMs: 30000,
  tagged: true,
  aiTagged: false,
  playCount: 0,
  playsCounted: true,
};

// A channel with a full policy AND an existing pin, so the test can prove the merge keeps
// both the rest of the policy and the prior pin.
//
// ⚠ Was an inline five-field object served as a ChannelDTO. `channel()` carries the eleven the
// wire requires — and this dialog READS the channel back to merge onto it, so a short channel is
// exactly the input that would make a merge bug invisible.
const channelDetail: ChannelDTO = channel({
  id: "ch-1",
  name: "90s Action Hour",
  number: 42,
  policy: {
    ordering: "shuffle",
    scope: { era: { from: 1990, to: 1999 } },
    filler: { audience: "general", pinned: ["already-here"] },
  },
});

// GET /v1/filler/fit (which channels this clip suits), GET /v1/channels/{id} (the live-policy
// read the merge depends on), GET /v1/channels (the list), PATCH /v1/channels/{id} (the pin
// write). PATCH bodies captured from the resolver.
//
// ⚠ The PATCH branch was `method === "PATCH"` with no path check at all, and the FINAL branch was
// an unconditional channel list — so any request that was not a fit-read or the by-id read got
// answered with a list of channels, whatever it had asked for.
const stubPin = (opts: { detail?: ChannelDTO; pinned?: boolean } = {}) => {
  const patches: unknown[] = [];
  server.use(
    getClipChannelFitMockHandler({
      channels: [
        {
          channelId: "ch-1",
          name: "90s Action Hour",
          number: 42,
          level: "exact",
          pinned: opts.pinned ?? false,
          excluded: false,
        },
      ],
    }),
    getGetChannelMockHandler(opts.detail ?? channelDetail),
    getListChannelsMockHandler({ channels: [channelDetail] }),
    getUpdateChannelMockHandler(async ({ request }) => {
      patches.push(await request.json());
      return channelDetail;
    }),
  );
  return { patches };
};

describe("PinClipDialog", () => {
  // ⚠ Still the load-bearing property after V35 rewrote this surface: PATCH replaces `policy`
  // WHOLE, so a bare {filler} wipes scope/ordering/audience. The dialog reads the channel live
  // and merges onto it.
  it("appends the clip to filler.pinned and MERGES onto the whole saved policy", async () => {
    const user = userEvent.setup();
    const { patches } = stubPin();
    render(<PinClipDialog clip={clip} onClose={() => {}} />, { wrapper: makeWrapper() });

    await user.click(await screen.findByRole("checkbox", { name: /Always play/ }));

    await waitFor(() => expect(patches).toHaveLength(1));
    // The new pin is appended to the existing one (not replacing it)…
    expect(patches[0]).toMatchObject({
      policy: { filler: { pinned: ["already-here", "clip-9-hash"], audience: "general" } },
    });
    // …and the rest of the policy survives.
    expect(patches[0]).toMatchObject({
      policy: { ordering: "shuffle", scope: { era: { from: 1990 } } },
    });
  });

  // ⚠ The state the old pin-only dialog could not express, and the reason V35 replaced it:
  // unticking must BLOCK the clip on that channel, which is a different write from clearing
  // the pin. A single flag would make this a no-op and the operator's intent would vanish.
  it("unticking an overridden channel writes an exclusion, not just a missing pin", async () => {
    const user = userEvent.setup();
    const { patches } = stubPin({ pinned: true });
    render(<PinClipDialog clip={clip} onClose={() => {}} />, { wrapper: makeWrapper() });

    await user.click(await screen.findByRole("checkbox", { name: /Always play/ }));

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toMatchObject({
      policy: { filler: { pinned: ["already-here"], excluded: ["clip-9-hash"] } },
    });
  });

  // ⚠ "Back to automatic" clears BOTH lists — the only route to the third state, which a
  // checkbox cannot express.
  it("back to automatic clears both lists", async () => {
    const user = userEvent.setup();
    const { patches } = stubPin({ pinned: true });
    render(<PinClipDialog clip={clip} onClose={() => {}} />, { wrapper: makeWrapper() });

    await user.click(await screen.findByRole("button", { name: /Back to automatic/ }));

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toMatchObject({
      policy: { filler: { pinned: ["already-here"], excluded: [] } },
    });
  });

  it("renders nothing when no clip is given", () => {
    stubPin();
    const { container } = render(<PinClipDialog onClose={() => {}} />, { wrapper: makeWrapper() });
    expect(container).toBeEmptyDOMElement();
  });
});
