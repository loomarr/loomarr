import type { ClipDTO } from "@loomarr/api";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PinClipDialog } from "./pin-clip-dialog";

const makeWrapper = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
};

const jsonResponse = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const clip: ClipDTO = {
  path: "clip-9.mp4",
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
const channelDetail = {
  id: "ch-1",
  name: "90s Action Hour",
  number: 42,
  status: "live",
  policy: {
    ordering: "shuffle",
    scope: { era: { from: 1990, to: 1999 } },
    filler: { audience: "general", pinned: ["already-here"] },
  },
};

// Dispatches by method+path: GET /v1/channels (list), GET /v1/channels/{id} (the live-policy
// read the merge depends on), PATCH /v1/channels/{id} (the pin write). PATCH bodies captured.
const stubFetch = (opts: { detail?: unknown; pinned?: boolean } = {}) => {
  const patches: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      const u = String(url);
      const method = init?.method ?? "GET";
      if (method === "PATCH") {
        patches.push(init?.body ? JSON.parse(init.body as string) : undefined);
        return Promise.resolve(jsonResponse(200, { id: "ch-1" }));
      }
      if (method === "GET" && /\/v1\/filler\/fit/.test(u)) {
        return Promise.resolve(
          jsonResponse(200, {
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
        );
      }
      if (method === "GET" && /\/v1\/channels\/ch-1$/.test(u)) {
        return Promise.resolve(jsonResponse(200, opts.detail ?? channelDetail));
      }
      // GET /v1/channels — the list.
      return Promise.resolve(
        jsonResponse(200, {
          channels: [
            { id: "ch-1", name: "90s Action Hour", number: 42, status: "live", policy: channelDetail.policy },
          ],
        }),
      );
    }),
  );
  return { patches };
};

afterEach(() => vi.restoreAllMocks());

describe("PinClipDialog", () => {
  // ⚠ Still the load-bearing property after V35 rewrote this surface: PATCH replaces `policy`
  // WHOLE, so a bare {filler} wipes scope/ordering/audience. The dialog reads the channel live
  // and merges onto it.
  it("appends the clip to filler.pinned and MERGES onto the whole saved policy", async () => {
    const user = userEvent.setup();
    const { patches } = stubFetch();
    render(<PinClipDialog clip={clip} onClose={() => {}} />, { wrapper: makeWrapper() });

    await user.click(await screen.findByRole("checkbox", { name: /Always play/ }));

    await waitFor(() => expect(patches).toHaveLength(1));
    // The new pin is appended to the existing one (not replacing it)…
    expect(patches[0]).toMatchObject({
      policy: { filler: { pinned: ["already-here", "clip-9.mp4"], audience: "general" } },
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
    const { patches } = stubFetch({ pinned: true });
    render(<PinClipDialog clip={clip} onClose={() => {}} />, { wrapper: makeWrapper() });

    await user.click(await screen.findByRole("checkbox", { name: /Always play/ }));

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toMatchObject({
      policy: { filler: { pinned: ["already-here"], excluded: ["clip-9.mp4"] } },
    });
  });

  // ⚠ "Back to automatic" clears BOTH lists — the only route to the third state, which a
  // checkbox cannot express.
  it("back to automatic clears both lists", async () => {
    const user = userEvent.setup();
    const { patches } = stubFetch({ pinned: true });
    render(<PinClipDialog clip={clip} onClose={() => {}} />, { wrapper: makeWrapper() });

    await user.click(await screen.findByRole("button", { name: /Back to automatic/ }));

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toMatchObject({
      policy: { filler: { pinned: ["already-here"], excluded: [] } },
    });
  });

  it("renders nothing when no clip is given", () => {
    stubFetch();
    const { container } = render(<PinClipDialog onClose={() => {}} />, { wrapper: makeWrapper() });
    expect(container).toBeEmptyDOMElement();
  });
});
