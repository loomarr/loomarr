import type { ChannelPolicy, ClipDTO, CoverageDTO, PreviewDraftPodsOutputBody } from "@loomarr/api";
import {
  getChannelFillerCoverageMockHandler,
  getGetChannelMockHandler,
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
import { eraDates } from "@/lib/era-dates";
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
const NO_LIMIT: CoverageDTO["geography"] = { country: "", market: "" };

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
  coverage: {
    level: "exact",
    total: 4,
    rungs: [{ level: "exact", clips: 4 }],
    criteria: HEALTHY,
    geography: NO_LIMIT,
  },
  entries: [
    {
      hash: "b1-hash",
      path: "b1.mp4",
      tunarrProgramId: "b1",
      name: "Bumper",
      kind: "bumper",
      durationMs: 5000,
      isFallbackCard: false,
    },
    {
      hash: "a1-hash",
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
const stubChannelFiller = (opts: { clips?: ClipDTO[]; revision?: number } = {}) => {
  const patches: unknown[] = [];
  const previews: unknown[] = [];
  const catalogQueries: string[] = [];

  server.use(
    getPreviewDraftChannelPodsMockHandler(async ({ request }) => {
      previews.push(await request.json());
      return previewBody;
    }),
    // The channel writer reads the revision to save at.
    getGetChannelMockHandler(channel({ revision: opts.revision ?? 1 })),
    getUpdateChannelMockHandler(async ({ request }) => {
      patches.push(await request.json());
      return channel();
    }),
    // GET /v1/taxonomy — the category vocabulary the criteria's product chips are now FETCHED from
    // (§10 V45a; the hardcoded list is gone). A minimal product forest with the taxa these tests
    // toggle (Toys, Candy) so the chips render for the interaction.
    getListTaxonomyMockHandler({
      taxa: [
        { slug: "toys", label: "Toys", axis: "product", assertedClips: 0, matchedClips: 0, storedClips: 0 },
        { slug: "candy", label: "Candy", axis: "product", assertedClips: 0, matchedClips: 0, storedClips: 0 },
        { slug: "cars", label: "Cars", axis: "product", assertedClips: 0, matchedClips: 0, storedClips: 0 },
      ],
      totalClips: 0,
      taggedClips: 0,
      unclassifiedClips: 0,
      axisCoverage: [],
    }),
    // GET /v1/filler — catalog + add-search.
    getListFillerMockHandler(({ request }) => {
      catalogQueries.push(request.url);
      const clips = opts.clips ?? [];
      return { clips, total: clips.length };
    }),
    // The channel editor reads inherited break defaults from settings so its labels and draft
    // preview describe the same values reconcile will use.
    getSettingsListMockHandler({
      features: {},
      settings: [
        setting({ key: "filler.pod_max", value: "4", kind: "int", group: "filler" }),
        setting({ key: "filler.breaks_per_hour", value: "4", kind: "int", group: "filler" }),
        setting({ key: "filler.break_duration", value: "5m", kind: "duration", group: "filler" }),
        setting({ key: "filler.cooldown_seconds", value: "14400", kind: "int", group: "filler" }),
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
      geography: { country: "US", market: "" },
    }),
  );

  return { patches, previews, catalogQueries };
};

const policy = (
  filler?: ChannelPolicy["filler"],
  breaksPerHour?: number,
  breakDuration?: string,
): ChannelPolicy => ({
  ordering: "shuffle",
  scope: { dates: eraDates({ from: 1990, to: 1999 }) },
  ...(filler ? { filler } : {}),
  ...(breaksPerHour !== undefined ? { breaksPerHour } : {}),
  ...(breakDuration !== undefined ? { breakDuration } : {}),
});

describe("ChannelFiller", () => {
  it("does not turn an empty automatic break into a tagging chore", async () => {
    stubChannelFiller();
    server.use(getPreviewDraftChannelPodsMockHandler({ ...previewBody, entries: [], totalMs: 0 }));
    renderSection(<ChannelFiller channelId="ch-1" policy={policy()} />);
    const library = await screen.findByRole("link", { name: "browse your filler library" });
    expect(library).toHaveAttribute("href", "/filler/library");
    expect(screen.getByText(/Breaks use the bumper card for now/)).toBeInTheDocument();
    expect(screen.queryByText(/add and tag clips/)).not.toBeInTheDocument();
  });

  // Reached from a Filler Overview coverage card, the page offers the way back (#1908); reached
  // any other way it is the channel's own page and adds nothing.
  it("offers a way back to the Filler overview when a coverage card opened it", async () => {
    stubChannelFiller();
    renderSection(<ChannelFiller channelId="ch-1" policy={policy()} from="filler" />);
    expect(await screen.findByRole("link", { name: "Filler overview" })).toHaveAttribute("href", "/filler");
  });

  it("adds no Filler return link when opened from the channel", async () => {
    stubChannelFiller();
    renderSection(<ChannelFiller channelId="ch-1" policy={policy()} />);
    expect(await screen.findByLabelText("Audience")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Filler overview" })).not.toBeInTheDocument();
  });

  it("renders the criteria controls and the live break once a preview lands", async () => {
    stubChannelFiller();
    renderSection(<ChannelFiller channelId="ch-1" policy={policy()} />);

    // findBy* awaits the router harness mounting its route (RouterProvider mounts via a
    // transition, so the content isn't in the DOM on the first synchronous pass).
    expect(await screen.findByLabelText("Audience")).toBeInTheDocument();
    expect(screen.getByText("Products & topics")).toBeInTheDocument();
    expect(screen.getByText("Clip kinds")).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByRole("combobox", { name: "Break frequency" })).toHaveTextContent(
        "Follow default (4 per hour)",
      ),
    );
    expect(screen.getByRole("combobox", { name: "Break length" })).toHaveTextContent(
      "Follow default (5 min)",
    );
    // The mount preview assembles the (saved) selection and renders the pod timeline.
    await waitFor(() => expect(screen.getByLabelText("Pod segments")).toBeInTheDocument());
  });

  it("plays an exact clip from the assembled break in the shared preview sheet", async () => {
    stubChannelFiller();
    renderSection(<ChannelFiller channelId="ch-1" policy={policy()} />);

    await userEvent.click(await screen.findByRole("button", { name: "Preview Toy Ad" }));
    expect(await screen.findByRole("dialog", { name: "Toy Ad" })).toBeInTheDocument();
    expect(document.querySelector('video[src="/v1/filler/media/a1-hash"]')).toBeInTheDocument();
  });

  it("shows saved coverage before controls and keeps clip overrides collapsed", async () => {
    const user = userEvent.setup();
    stubChannelFiller();
    renderSection(
      <ChannelFiller channelId="ch-1" policy={policy({ pinned: ["preferred"], excluded: ["blocked"] })} />,
    );

    const coverage = await screen.findByRole("heading", { name: "What this channel can play" });
    const controls = screen.getByRole("heading", { name: "Choose what fits" });
    expect(coverage.compareDocumentPosition(controls) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);

    const overrides = screen.getByRole("button", { name: /specific clips \(advanced\)/i });
    expect(overrides).toHaveAttribute("aria-expanded", "false");
    await user.click(overrides);
    expect(screen.getByText("Prefer on this channel")).toBeVisible();
    expect(screen.getByText("Exclude from this channel")).toBeVisible();
  });

  it("previews and applies a custom break length", async () => {
    const user = userEvent.setup();
    const { patches, previews } = stubChannelFiller();
    renderSection(<ChannelFiller channelId="ch-1" policy={policy()} />);

    await waitFor(() => expect(previews).toHaveLength(1));
    await user.click(screen.getByRole("combobox", { name: "Break length" }));
    await user.click(await screen.findByRole("option", { name: "Custom length" }));
    const input = await screen.findByLabelText("Custom break length in minutes");
    await user.clear(input);
    await user.type(input, "1.5");

    await waitFor(() => expect(previews.at(-1)).toMatchObject({ breakDuration: "1.5m" }));
    await user.click(screen.getByRole("button", { name: /apply filler/i }));
    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toMatchObject({ policy: { breakDuration: "1.5m" } });
  });

  it("follows the install's commercial spacing until the channel sets its own", async () => {
    const user = userEvent.setup();
    const { patches } = stubChannelFiller();
    renderSection(<ChannelFiller channelId="ch-1" policy={policy()} />);

    const spacing = await screen.findByRole("combobox", { name: "Repeat spacing · same commercial" });
    await waitFor(() => expect(spacing).toHaveTextContent("Follow default (4 hours)"));
    await user.click(spacing);
    await user.click(await screen.findByRole("option", { name: "Custom spacing" }));
    const input = await screen.findByLabelText("Custom commercial repeat spacing in hours");
    expect(input).toHaveValue(4);
    await user.clear(input);
    await user.type(input, "6");

    await user.click(screen.getByRole("button", { name: /apply filler/i }));
    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toMatchObject({ policy: { commercialSpacing: "6h" } });
  });

  it("reads back a saved spacing in the server's duration format", async () => {
    stubChannelFiller();
    renderSection(<ChannelFiller channelId="ch-1" policy={{ ...policy(), commercialSpacing: "1h30m0s" }} />);
    expect(await screen.findByLabelText("Custom commercial repeat spacing in hours")).toHaveValue(1.5);
  });

  it("names what the saved pool is limited to from the server's coverage", async () => {
    stubChannelFiller();
    renderSection(<ChannelFiller channelId="ch-1" policy={policy()} />);
    expect(await screen.findByText("US commercials only · 4 fit this channel")).toBeInTheDocument();
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

    await user.click(await screen.findByRole("button", { name: /choose products & topics/i }));
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

    await user.click(await screen.findByRole("button", { name: /choose products & topics/i }));
    await user.click(await screen.findByRole("button", { name: "Candy" }));
    const apply = await screen.findByRole("button", { name: /apply filler/i });
    await user.click(apply);

    await waitFor(() => expect(patches).toHaveLength(1));
    // The draft's filler audience is "kids"; the rest of the policy (ordering, scope) is
    // carried alongside it, not wiped — PATCH replaces `policy` whole.
    expect(patches[0]).toMatchObject({
      policy: {
        ordering: "shuffle",
        scope: { dates: { movieRelease: [{ from: 1990 }] } },
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
          name: "Crunchlings",
          kind: "commercial",
          durationMs: 30000,
          tagged: true,
          needsDetails: false,
          aiTagged: false,
          playCount: 0,
          playsCounted: true,
        },
      ],
    });
    renderSection(<ChannelFiller channelId="ch-1" policy={policy({ pinned: ["p9-hash"] })} />);
    // The pinned override shows the resolved clip name, not the bare id.
    expect(await screen.findByText("Crunchlings")).toBeInTheDocument();

    expect(catalogQueries.length, "the resolver must issue a listing request").toBeGreaterThan(0);
    expect(
      catalogQueries.every((u) => new URL(u).searchParams.get("hashes") === "p9-hash"),
      "every resolve must name the hashes it wants — an unfiltered listing is a catalog read",
    ).toBe(true);
  });

  it("preserves saved clip exceptions when applying ordinary criteria changes", async () => {
    const user = userEvent.setup();
    const { patches } = stubChannelFiller({ revision: 7 });
    renderSection(
      <ChannelFiller
        channelId="ch-1"
        policy={policy({ audience: "kids", pinned: ["preferred"], excluded: ["blocked"] })}
      />,
    );
    await user.click(await screen.findByRole("button", { name: /choose products & topics/i }));
    await user.click(await screen.findByRole("button", { name: "Candy" }));
    await user.click(await screen.findByRole("button", { name: /apply filler/i }));
    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toMatchObject({
      revision: 7,
      policy: {
        ordering: "shuffle",
        scope: { dates: eraDates({ from: 1990, to: 1999 }) },
        filler: { pinned: ["preferred"], excluded: ["blocked"], audience: "kids", categories: ["candy"] },
      },
    });
  });

  it.each(["pinned", "excluded"] as const)(
    "removing a saved %s exception returns only that list to automatic",
    async (list) => {
      const user = userEvent.setup();
      const savedClip: ClipDTO = {
        hash: "saved-clip",
        name: "Saved clip",
        kind: "commercial",
        durationMs: 30000,
        aiTagged: false,
        tagged: true,
        needsDetails: false,
        playCount: 0,
        playsCounted: true,
      };
      const { patches } = stubChannelFiller({ clips: [savedClip] });
      const other = list === "pinned" ? "excluded" : "pinned";
      renderSection(
        <ChannelFiller
          channelId="ch-1"
          policy={policy({ [list]: ["saved-clip"], [other]: ["other-clip"], audience: "kids" })}
        />,
      );
      await user.click(await screen.findByRole("button", { name: /Specific clips \(advanced\)/ }));
      await user.click(await screen.findByRole("button", { name: "Remove Saved clip" }));
      await user.click(await screen.findByRole("button", { name: /apply filler/i }));
      await waitFor(() => expect(patches).toHaveLength(1));
      const payload = patches[0] as { policy: ChannelPolicy };
      expect(payload.policy.filler?.[list] ?? []).toEqual([]);
      expect(payload.policy.filler?.[other]).toEqual(["other-clip"]);
      expect(payload.policy.scope).toEqual({ dates: eraDates({ from: 1990, to: 1999 }) });
      expect(payload.policy.filler?.audience).toBe("kids");
    },
  );

  it("surfaces a preview failure rather than a silently empty break", async () => {
    // ⚠ Hand-written, and it has to be: the spec declares errors via `default:` (RFC 7807) with
    // no explicit 422, so orval generates no failing handler. The catalog read still comes from
    // the generated one — only the endpoint under test is replaced.
    server.use(
      http.post("*/v1/channels/:id/pods/preview", () =>
        HttpResponse.json({ title: "bad selection" }, { status: 422 }),
      ),
      getListFillerMockHandler({ clips: [], total: 0 }),
      getListTaxonomyMockHandler({
        taxa: [],
        totalClips: 0,
        taggedClips: 0,
        unclassifiedClips: 0,
        axisCoverage: [],
      }),
      getChannelFillerCoverageMockHandler({
        level: "exact",
        total: 0,
        rungs: [],
        criteria: HEALTHY,
        geography: NO_LIMIT,
      }),
      // This test builds its own handler set rather than using `stubChannelFiller`, so it needs
      // the inherited-default settings read too.
      getSettingsListMockHandler({ features: {}, settings: [] }),
    );
    renderSection(<ChannelFiller channelId="ch-1" policy={policy()} />);
    expect(await screen.findByText(/couldn't assemble a preview/i)).toBeInTheDocument();
  });
});
