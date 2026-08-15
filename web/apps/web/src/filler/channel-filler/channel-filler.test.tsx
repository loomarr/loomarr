import type { ChannelPolicy, ClipDTO, CoverageDTO, PreviewDraftPodsOutputBody } from "@loomarr/api";
import {
  getChannelFillerCoverageMockHandler,
  getListFillerMockHandler,
  getListTaxonomyMockHandler,
  getPreviewDraftChannelPodsMockHandler,
  getSettingsListMockHandler,
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
import { setting } from "@/test/fixtures/settings";
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

// ⚠ Every coverage payload carries the per-setting breakdown since V51f. Nothing at zero, so the
// diagnosis panel stays hidden and these tests keep asserting what they were written to assert.
const HEALTHY: CoverageDTO["criteria"] = [
  { criterion: "era", clips: 4 },
  { criterion: "audience", clips: 4 },
  { criterion: "category", clips: 4 },
  { criterion: "kind", clips: 4 },
  { criterion: "duration", clips: 4 },
  { criterion: "quality", clips: 4 },
];

// ⚠ The DRAFT preview returns the pod AND the coverage for the same selection (§10 V51f), so the
// meter and the timeline below it can no longer describe two different things.
const previewBody: PreviewDraftPodsOutputBody = {
  coverage: { level: "exact", total: 4, rungs: [{ level: "exact", clips: 4 }], criteria: HEALTHY },
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
    // ⚠ ALSO FOUND BY THE GUARD, a second time. The pin list reads `filler.pod_max` so it can say
    // when a channel has more pins than one break can play (#237) — and the unhandled-request
    // assertion turned that new fetch into five red tests the moment it landed, rather than a
    // silent 404 the component would have rendered around.
    getSettingsListMockHandler({
      features: {},
      settings: [
        setting({ key: "filler.pod_max", value: "4", kind: "int", group: "filler" }),
        setting({ key: "filler.breaks_per_hour", value: "4", kind: "int", group: "filler" }),
      ],
    }),
    // ⚠ FOUND BY THE GUARD. The coverage meter reads this on mount and the old catch-all served
    // it a CLIP LIST — `{ clips: [...] }` where the component wants `{ level, total, rungs }`. It
    // rendered an empty meter rather than throwing, which is the wrong-shape-looks-fine failure
    // this file's own sibling comments already warned about.
    getChannelFillerCoverageMockHandler({
      level: "exact",
      total: 4,
      rungs: [{ level: "exact", clips: 4 }],
      criteria: HEALTHY,
    }),
  );

  return { patches, previews, catalogQueries };
};

const policy = (filler?: ChannelPolicy["filler"], breaksPerHour?: number): ChannelPolicy => ({
  ordering: "shuffle",
  scope: { era: { from: 1990, to: 1999 } },
  ...(filler ? { filler } : {}),
  ...(breaksPerHour !== undefined ? { breaksPerHour } : {}),
});

describe("ChannelFiller", () => {
  it("renders the criteria controls and the live break once a preview lands", async () => {
    stubChannelFiller();
    renderSection(<ChannelFiller channelId="ch-1" policy={policy()} />);

    // findBy* awaits the router harness mounting its route (RouterProvider mounts via a
    // transition, so the content isn't in the DOM on the first synchronous pass).
    expect(await screen.findByLabelText("Audience")).toBeInTheDocument();
    expect(screen.getByText("Categories")).toBeInTheDocument();
    expect(screen.getByText("Clip kinds")).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByRole("combobox", { name: "Break frequency" })).toHaveTextContent(
        "Follow default (4 per hour)",
      ),
    );
    // The mount preview assembles the (saved) selection and renders the pod timeline.
    await waitFor(() => expect(screen.getByLabelText("Pod segments")).toBeInTheDocument());
  });

  it("offers inherited, disabled, and custom break frequency without re-previewing the clip mix", async () => {
    const user = userEvent.setup();
    const { patches, previews } = stubChannelFiller();
    renderSection(<ChannelFiller channelId="ch-1" policy={policy()} />);

    await waitFor(() => expect(previews).toHaveLength(1));
    await user.click(screen.getByRole("combobox", { name: "Break frequency" }));
    await user.click(await screen.findByRole("option", { name: "No commercial breaks" }));

    expect(await screen.findByRole("button", { name: /apply filler/i })).toBeInTheDocument();
    expect(previews).toHaveLength(1);
    await user.click(screen.getByRole("button", { name: /apply filler/i }));
    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toMatchObject({ policy: { breaksPerHour: 0 } });
  });

  it("editing a criterion re-previews and reveals Apply", async () => {
    const user = userEvent.setup();
    const { previews } = stubChannelFiller();
    renderSection(<ChannelFiller channelId="ch-1" policy={policy()} />);

    await user.click(await screen.findByRole("button", { name: /choose categories/i }));
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

    await user.click(await screen.findByRole("button", { name: /choose categories/i }));
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
      getChannelFillerCoverageMockHandler({ level: "exact", total: 0, rungs: [], criteria: HEALTHY }),
      // This test builds its own handler set rather than using `stubChannelFiller`, so it needs
      // the settings read too — the pin list asks for `filler.pod_max` (#237).
      getSettingsListMockHandler({ features: {}, settings: [] }),
    );
    renderSection(<ChannelFiller channelId="ch-1" policy={policy()} />);
    expect(await screen.findByText(/couldn't assemble a preview/i)).toBeInTheDocument();
  });
});
