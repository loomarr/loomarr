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
  tunarrProgramId: "clip-9",
  name: "Frosted Flakes",
  kind: "commercial",
  durationMs: 30000,
  tagged: true,
  aiTagged: false,
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
const stubFetch = (opts: { detail?: unknown } = {}) => {
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
  it("appends the clip to filler.pinned and MERGES onto the whole saved policy", async () => {
    const user = userEvent.setup();
    const { patches } = stubFetch();
    render(<PinClipDialog clip={clip} onClose={() => {}} />, { wrapper: makeWrapper() });

    await user.click(await screen.findByRole("button", { name: "Pin" }));

    await waitFor(() => expect(patches).toHaveLength(1));
    // The new pin is appended to the existing one (not replacing it)…
    expect(patches[0]).toMatchObject({
      policy: { filler: { pinned: ["already-here", "clip-9"], audience: "general" } },
    });
    // …and the rest of the policy survives — PATCH replaces policy whole, so a bare
    // {filler} would have wiped scope/ordering.
    expect(patches[0]).toMatchObject({
      policy: { ordering: "shuffle", scope: { era: { from: 1990 } } },
    });
  });

  it("shows an already-pinned channel as Pinned, with no Pin button", async () => {
    // The list row carries this channel's policy with the clip ALREADY pinned, so the row
    // renders as Pinned straight away — no write path to reach.
    vi.stubGlobal(
      "fetch",
      vi.fn((_url: string, init?: RequestInit) => {
        if ((init?.method ?? "GET") === "PATCH") throw new Error("must not PATCH an already-pinned channel");
        return Promise.resolve(
          jsonResponse(200, {
            channels: [
              {
                id: "ch-1",
                name: "90s Action Hour",
                number: 42,
                status: "live",
                policy: { filler: { pinned: ["clip-9"] } },
              },
            ],
          }),
        );
      }),
    );
    render(<PinClipDialog clip={clip} onClose={() => {}} />, { wrapper: makeWrapper() });

    expect(await screen.findByText("Pinned")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Pin" })).not.toBeInTheDocument();
  });

  it("renders nothing when no clip is given", () => {
    stubFetch();
    const { container } = render(<PinClipDialog onClose={() => {}} />, { wrapper: makeWrapper() });
    expect(container).toBeEmptyDOMElement();
  });
});
