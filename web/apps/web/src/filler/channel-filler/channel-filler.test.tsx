import type { ChannelPolicy } from "@loomarr/api";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui";
import { RouterHarness } from "@/test/story-utils";
import { ChannelFiller } from "./channel-filler";

// The section needs three contexts in isolation: a QueryClient (live generated-API hooks),
// a TooltipProvider (the clip-list remove buttons), and a RouterProvider (the header/empty-
// state cross-links to /filler use TanStack `Link`, which throws without a router even in a
// unit). RouterHarness renders `content` AS its route, so the query+tooltip providers wrap
// the component INSIDE it — all three resolve.
const renderSection = (ui: ReactNode) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <RouterHarness
      content={
        <QueryClientProvider client={client}>
          <TooltipProvider>{ui}</TooltipProvider>
        </QueryClientProvider>
      }
    />,
  );
};

const jsonResponse = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const previewBody = {
  entries: [
    {
      path: "b1.mp4",
      tunarrProgramId: "b1",
      name: "Bumper",
      kind: "bumper",
      durationMs: 5000,
      isFallbackCard: false,
    },
    {
      path: "a1.mp4",
      tunarrProgramId: "a1",
      name: "Toy Ad",
      kind: "commercial",
      durationMs: 30000,
      isFallbackCard: false,
    },
  ],
  totalMs: 35000,
  matchLevel: "exact",
};

// Dispatches by method+path: POST …/pods/preview is the live draft preview; PATCH
// …/channels/{id} is apply; GET /v1/filler is both the catalog resolve and the add-search.
const stubFetch = (opts: { clips?: unknown[]; patchStatus?: number } = {}) => {
  const patches: unknown[] = [];
  const previews: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      const u = String(url);
      const method = init?.method ?? "GET";
      const body = init?.body ? JSON.parse(init.body as string) : undefined;
      if (method === "POST" && u.endsWith("/pods/preview")) {
        previews.push(body);
        return Promise.resolve(jsonResponse(200, previewBody));
      }
      if (method === "PATCH") {
        patches.push(body);
        return Promise.resolve(jsonResponse(opts.patchStatus ?? 200, { id: "ch-1" }));
      }
      // GET /v1/filler — catalog + add-search.
      return Promise.resolve(jsonResponse(200, { clips: opts.clips ?? [] }));
    }),
  );
  return { patches, previews };
};

const policy = (filler?: ChannelPolicy["filler"]): ChannelPolicy => ({
  ordering: "shuffle",
  scope: { era: { from: 1990, to: 1999 } },
  ...(filler ? { filler } : {}),
});

afterEach(() => vi.restoreAllMocks());

describe("ChannelFiller", () => {
  it("renders the criteria controls and the live break once a preview lands", async () => {
    stubFetch();
    renderSection(<ChannelFiller channelId="ch-1" policy={policy()} />);

    // findBy* awaits the router harness mounting its route (RouterProvider mounts via a
    // transition, so the content isn't in the DOM on the first synchronous pass).
    expect(await screen.findByLabelText("Audience")).toBeInTheDocument();
    expect(screen.getByText("Categories")).toBeInTheDocument();
    expect(screen.getByText("Clip kinds")).toBeInTheDocument();
    // The mount preview assembles the (saved) selection and renders the pod timeline.
    await waitFor(() => expect(screen.getByLabelText("Pod segments")).toBeInTheDocument());
  });

  it("editing a criterion re-previews and reveals Apply", async () => {
    const user = userEvent.setup();
    const { previews } = stubFetch();
    renderSection(<ChannelFiller channelId="ch-1" policy={policy()} />);

    // Toggle a category chip — the draft changes, so a preview fires and Apply appears.
    // (findBy awaits the router-harness mount.) No Apply until the draft diverges.
    const toys = await screen.findByRole("button", { name: "Toys" });
    expect(screen.queryByRole("button", { name: /apply filler/i })).not.toBeInTheDocument();
    await user.click(toys);

    await waitFor(() =>
      expect(
        previews.some((p) =>
          (p as { filler?: { categories?: string[] } }).filler?.categories?.includes("toys"),
        ),
      ).toBe(true),
    );
    expect(await screen.findByRole("button", { name: /apply filler/i })).toBeInTheDocument();
  });

  it("Apply PATCHes the draft merged onto the saved policy; Discard clears the dirty state", async () => {
    const user = userEvent.setup();
    const { patches } = stubFetch();
    renderSection(<ChannelFiller channelId="ch-1" policy={policy({ audience: "kids" })} />);

    await user.click(await screen.findByRole("button", { name: "Candy" }));
    const apply = await screen.findByRole("button", { name: /apply filler/i });
    await user.click(apply);

    await waitFor(() => expect(patches).toHaveLength(1));
    // The draft's filler audience is "kids"; the rest of the policy (ordering, scope) is
    // carried alongside it, not wiped — PATCH replaces `policy` whole.
    expect(patches[0]).toMatchObject({
      policy: {
        ordering: "shuffle",
        scope: { era: { from: 1990 } },
        filler: { categories: ["candy"], audience: "kids" },
      },
    });
  });

  it("resolves a pinned clip's id to its name via the catalog", async () => {
    stubFetch({
      clips: [
        {
          path: "p9.mp4",
          tunarrProgramId: "p9",
          name: "Frosted Flakes",
          kind: "commercial",
          durationMs: 30000,
          tagged: true,
          aiTagged: false,
        },
      ],
    });
    renderSection(<ChannelFiller channelId="ch-1" policy={policy({ pinned: ["p9.mp4"] })} />);
    // The pinned override shows the resolved clip name, not the bare id.
    expect(await screen.findByText("Frosted Flakes")).toBeInTheDocument();
  });

  it("surfaces a preview failure rather than a silently empty break", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((url: string, init?: RequestInit) => {
        const u = String(url);
        if ((init?.method ?? "GET") === "POST" && u.endsWith("/pods/preview")) {
          return Promise.resolve(jsonResponse(422, { title: "bad selection" }));
        }
        return Promise.resolve(jsonResponse(200, { clips: [] }));
      }),
    );
    renderSection(<ChannelFiller channelId="ch-1" policy={policy()} />);
    expect(await screen.findByText(/couldn't assemble a preview/i)).toBeInTheDocument();
  });
});
