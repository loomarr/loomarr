import type { ChannelPolicy, ClipDTO, PodPoolDTO } from "@loomarr/api";
import {
  getChannelFillerCoverageMockHandler,
  getListFillerMockHandler,
  getListTaxonomyMockHandler,
  getPreviewDraftChannelPodsMockHandler,
  getUpdateChannelMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { TooltipProvider } from "@/components/ui";
import { channel } from "@/test/fixtures/channels";
import { server } from "@/test/msw/server";
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

const previewBody: PodPoolDTO = {
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
// ⚠ `catalogQueries` records the URL of every `GET /v1/filler` the component made, and ONLY those.
// The old `calls` array recorded every request at any path and a test then filtered it with
// `u.includes("/v1/filler?")` — which is a substring of `/v1/filler/pool`, `/v1/filler/watch`,
// `/v1/filler/incoming` and every other filler read. The assertion it powers ("every resolve must
// name the hashes it wants") is precisely the kind that a too-broad filter turns into a coin flip.
const stubChannelFiller = (opts: { clips?: ClipDTO[] } = {}) => {
  const patches: unknown[] = [];
  const previews: unknown[] = [];
  const catalogQueries: string[] = [];

  server.use(
    getPreviewDraftChannelPodsMockHandler(async ({ request }) => {
      previews.push(await request.json());
      return previewBody;
    }),
    getUpdateChannelMockHandler(async ({ request }) => {
      patches.push(await request.json());
      return channel();
    }),
    // GET /v1/taxonomy — the category vocabulary the criteria's product chips are now FETCHED from
    // (§10 V45a; the hardcoded list is gone). A minimal product forest with the taxa these tests
    // toggle (Toys, Candy) so the chips render for the interaction.
    getListTaxonomyMockHandler({
      taxa: [
        { slug: "toys", label: "Toys", axis: "product" },
        { slug: "candy", label: "Candy", axis: "product" },
        { slug: "cars", label: "Cars", axis: "product" },
      ],
    }),
    // GET /v1/filler — catalog + add-search.
    getListFillerMockHandler(({ request }) => {
      catalogQueries.push(request.url);
      const clips = opts.clips ?? [];
      return { clips, total: clips.length };
    }),
    // ⚠ FOUND BY THE GUARD. The coverage meter reads this on mount and the old catch-all served
    // it a CLIP LIST — `{ clips: [...] }` where the component wants `{ level, total, rungs }`. It
    // rendered an empty meter rather than throwing, which is the wrong-shape-looks-fine failure
    // this file's own sibling comments already warned about.
    getChannelFillerCoverageMockHandler({ level: "exact", total: 4, rungs: [{ level: "exact", clips: 4 }] }),
  );

  return { patches, previews, catalogQueries };
};

const policy = (filler?: ChannelPolicy["filler"]): ChannelPolicy => ({
  ordering: "shuffle",
  scope: { era: { from: 1990, to: 1999 } },
  ...(filler ? { filler } : {}),
});

// ⚠ The section starts CLOSED, and since V50c a closed CollapsibleSection panel carries
// `hidden="until-found"` — so its contents are out of the accessibility tree until opened.
// `*ByRole` queries honour that tree, which means any test that reaches a control in the body
// has to open the section first, exactly as a user does.
//
// This is not a workaround for the port; it is the port removing a defect these tests were
// resting on. The old `.reveal` closed with `grid-template-rows: 0fr` + `overflow:hidden` —
// zero height but NOT `display:none` — so collapsed controls stayed focusable and announced.
// A keyboard user could Tab into a section they could not see. `findByRole` reaching into a
// closed body was that bug, visible in a test rather than in a bug report.
//
// ⚠ The failure mode is deliberately unhelpful, so recognise it: `asyncUtilTimeout` and
// `testTimeout` are both 5000ms, so findBy's own "Unable to find role" never surfaces — the
// test times out first and reports only "Test timed out in 5000ms".
const openFiller = async (user: ReturnType<typeof userEvent.setup>) => {
  await user.click(await screen.findByRole("button", { name: /^filler/i }));
};

describe("ChannelFiller", () => {
  it("renders the criteria controls and the live break once a preview lands", async () => {
    stubChannelFiller();
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
    const { previews } = stubChannelFiller();
    renderSection(<ChannelFiller channelId="ch-1" policy={policy()} />);

    await openFiller(user);
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
    const { patches } = stubChannelFiller();
    renderSection(<ChannelFiller channelId="ch-1" policy={policy({ audience: "kids" })} />);

    await openFiller(user);
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

  // ⚠ It resolves by asking for THOSE HASHES, not by loading the catalog and mapping it
  // client-side (§10 V51d). The old shape worked only while the listing was unbounded: against
  // a paged catalog it would resolve whichever pins happened to land on page one and render the
  // rest as bare hashes — an override that looks like it has gone missing.
  it("resolves a pinned clip's id by asking for that hash, not by loading the catalog", async () => {
    const { catalogQueries } = stubChannelFiller({
      clips: [
        {
          hash: "p9-hash",
          tunarrProgramId: "p9",
          name: "Frosted Flakes",
          kind: "commercial",
          durationMs: 30000,
          tagged: true,
          aiTagged: false,
          playCount: 0,
          playsCounted: true,
        },
      ],
    });
    renderSection(<ChannelFiller channelId="ch-1" policy={policy({ pinned: ["p9-hash"] })} />);
    // The pinned override shows the resolved clip name, not the bare id.
    expect(await screen.findByText("Frosted Flakes")).toBeInTheDocument();

    expect(catalogQueries.length, "the resolver must issue a listing request").toBeGreaterThan(0);
    expect(
      catalogQueries.every((u) => new URL(u).searchParams.get("hashes") === "p9-hash"),
      "every resolve must name the hashes it wants — an unfiltered listing is a catalog read",
    ).toBe(true);
  });

  it("surfaces a preview failure rather than a silently empty break", async () => {
    // ⚠ Hand-written, and it has to be: the spec declares errors via `default:` (RFC 7807) with
    // no explicit 422, so orval generates no failing handler. The catalog read still comes from
    // the generated one — only the endpoint under test is replaced.
    server.use(
      http.post("*/v1/channels/:id/pods/preview", () =>
        HttpResponse.json({ title: "bad selection" }, { status: 422 }),
      ),
      getListFillerMockHandler({ clips: [], total: 0 }),
      getListTaxonomyMockHandler({ taxa: [] }),
      getChannelFillerCoverageMockHandler({ level: "exact", total: 0, rungs: [] }),
    );
    renderSection(<ChannelFiller channelId="ch-1" policy={policy()} />);
    expect(await screen.findByText(/couldn't assemble a preview/i)).toBeInTheDocument();
  });
});
