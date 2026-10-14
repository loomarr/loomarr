import { channelsApi, type LineupEntryDTO, type SearchCandidate } from "@loomarr/api";
import {
  getClearDiscoveryFeedbackMockHandler,
  getGetChannelMockHandler,
  getListDiscoveryFeedbackMockHandler,
  getRecordDiscoveryFeedbackMockHandler,
  getSearchMockHandler,
  getUpdateChannelMockHandler,
} from "@loomarr/api/msw";
import { unwrap } from "@loomarr/api/unwrap";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { TooltipProvider } from "@/components/ui";
import { channel } from "@/test/fixtures/channels";
import { server } from "@/test/msw/server";
import { ChannelLineupEditor } from "./channel-lineup-editor";

// The remove button carries a tooltip, and Radix tooltips need a provider ancestor
// (mounted once at the app root, __root.tsx / Storybook's preview.tsx) — wrap renders
// so this isolated component test has one too (same fix as ProposalReview's test).
const makeWrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <TooltipProvider>{children}</TooltipProvider>
    </QueryClientProvider>
  );
};

const cartographer: LineupEntryDTO = {
  key: "movie:tmdb:90000949",
  name: "The Velvet Cartographer",
  year: 1995,
};
const satelliteChoir: LineupEntryDTO = {
  key: "movie:tmdb:90009426",
  name: "The Seventh Satellite Choir",
  year: 1991,
};

// GET /v1/search answers the add-a-title palette; PATCH /v1/channels/{id} is the
// whole-list-replace commit this editor exists to drive. `candidates` is read fresh on every call
// so a test can change it mid-flow (e.g. proving a re-search still excludes whatever just got
// added).
//
// ⚠ The PATCH branch dispatched on `init?.method === "PATCH"` with NO url check — a PATCH to any
// endpoint would have been recorded as a lineup commit — and answered with `{ id: "ch-1" }`, ten
// fields short of a ChannelDTO.
const stubEditor = (opts: { candidates?: SearchCandidate[] } = {}) => {
  const patches: unknown[] = [];
  server.use(
    getListDiscoveryFeedbackMockHandler([]),
    getSearchMockHandler(() => ({ candidates: opts.candidates ?? [] })),
    // The channel writer reads the revision to save at.
    getGetChannelMockHandler(channel()),
    getUpdateChannelMockHandler(async ({ request }) => {
      patches.push(await request.json());
      return channel();
    }),
  );
  return { patches };
};

describe("ChannelLineupEditor", () => {
  it("renders the current lineup with drag handles and remove buttons", () => {
    stubEditor();
    render(<ChannelLineupEditor channelId="ch-1" lineup={[cartographer, satelliteChoir]} />, {
      wrapper: makeWrapper(),
    });

    expect(screen.getByText("The Velvet Cartographer")).toBeInTheDocument();
    expect(screen.getByText("The Seventh Satellite Choir")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reorder The Velvet Cartographer" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Remove The Velvet Cartographer" })).toBeInTheDocument();
  });

  it("keeps a series row's Tab order: grip, seasons, feedback, then remove (#1896)", async () => {
    // A narrow column stacks the row, and remove comes last there too (#1899), so keyboard order
    // is the reading order at every width. tests/visual/lineup-layout.spec.ts checks the layout.
    stubEditor();
    const series: LineupEntryDTO = { key: "series:tvdb:90000021", name: "Bowling Alley Blues", year: 1974 };
    const user = userEvent.setup();
    render(<ChannelLineupEditor channelId="ch-1" lineup={[series]} />, {
      wrapper: makeWrapper(),
    });
    await waitFor(() => expect(screen.getByRole("button", { name: /^Keep — / })).toBeEnabled());

    screen.getByRole("button", { name: "Reorder Bowling Alley Blues" }).focus();
    const order: (string | null)[] = [];
    for (let i = 0; i < 8; i++) {
      order.push(document.activeElement?.getAttribute("aria-label") ?? null);
      await user.tab();
    }
    expect(order).toEqual([
      "Reorder Bowling Alley Blues",
      "Bowling Alley Blues first season",
      "Bowling Alley Blues last season",
      "Keep — Bowling Alley Blues",
      "Less like this — Bowling Alley Blues",
      "Never — Bowling Alley Blues",
      "Surprise me — Bowling Alley Blues",
      "Remove Bowling Alley Blues",
    ]);
  });

  it("shows inherited feedback, records a Channel override, and Undo reveals the fallback", async () => {
    stubEditor();
    const writes: unknown[] = [];
    const household = {
      id: "feedback-household",
      targetKey: cartographer.key,
      action: "never",
      scope: "household",
      actorId: "admin",
      createdAt: "2026-09-02T12:00:00Z",
    };
    let effective = household;

    server.use(
      getListDiscoveryFeedbackMockHandler(({ request }) => {
        const url = new URL(request.url);
        expect(url.searchParams.get("scope")).toBe("channel");
        expect(url.searchParams.get("scopeId")).toBe("ch-1");
        return [effective];
      }),
      getRecordDiscoveryFeedbackMockHandler(async ({ request }) => {
        const body = (await request.json()) as {
          action: string;
          scope: string;
          scopeId: string;
          targetKey: string;
        };
        writes.push(body);
        effective = { ...household, id: "feedback-channel", action: body.action, scope: "channel" };
        return effective;
      }),
      getClearDiscoveryFeedbackMockHandler(async ({ request }) => {
        writes.push(await request.json());
        effective = household;
        return { ...household, id: "feedback-clear", action: "clear", scope: "channel" };
      }),
    );

    render(<ChannelLineupEditor channelId="ch-1" lineup={[cartographer]} />, {
      wrapper: makeWrapper(),
    });

    expect(await screen.findByText(/Inherited household preference: Never/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Undo" })).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Keep — The Velvet Cartographer" }));
    expect(await screen.findByText(/This Channel: Keep/i)).toBeInTheDocument();
    expect(writes[0]).toEqual({
      scope: "channel",
      scopeId: "ch-1",
      targetKey: cartographer.key,
      action: "keep",
    });

    await userEvent.click(screen.getByRole("button", { name: "Undo" }));
    expect(await screen.findByText(/Inherited household preference: Never/i)).toBeInTheDocument();
    expect(writes[1]).toEqual({ scope: "channel", scopeId: "ch-1", targetKey: cartographer.key });
  });

  it("shows an empty state when the lineup is empty", () => {
    stubEditor();
    render(<ChannelLineupEditor channelId="ch-1" lineup={[]} />, { wrapper: makeWrapper() });
    expect(screen.getByText(/nothing in the lineup yet/i)).toBeInTheDocument();
  });

  // The state badge is DURABLE: it reads each entry's server-derived `state` straight off
  // the lineup prop (not client-only memory), so a channel loaded fresh with an acquiring/
  // unavailable title shows the right badge without ever having added it this session.
  it("renders each entry's server-provided state badge (durable across reloads)", () => {
    stubEditor();
    render(
      <ChannelLineupEditor
        channelId="ch-1"
        lineup={[
          { key: "movie:tmdb:90000949", name: "The Velvet Cartographer", year: 1995, state: "available" },
          { key: "movie:tmdb:90000106", name: "Marigold Protocol", year: 1987, state: "pending" },
          { key: "movie:tmdb:90000603", name: "Parallax Garden", year: 1999, state: "acquiring" },
          { key: "movie:tmdb:90000011", name: "Comet Harbor", year: 1977, state: "unavailable" },
        ]}
      />,
      { wrapper: makeWrapper() },
    );

    // available → no badge; the other three each show their state.
    expect(screen.getByText("Pending")).toBeInTheDocument();
    expect(screen.getByText("Acquiring…")).toBeInTheDocument();
    expect(screen.getByText("Unavailable")).toBeInTheDocument();
    // The Velvet Cartographer (available) wears none of them.
    expect(screen.queryByText("Available")).not.toBeInTheDocument();
  });

  it("add: searching, picking a result appends it and commits the full ordered list", async () => {
    const { patches } = stubEditor({
      candidates: [
        { mediaType: "movie", tmdbId: 90000106, name: "Marigold Protocol", year: 1987, inLibrary: true },
      ],
    });
    render(<ChannelLineupEditor channelId="ch-1" lineup={[cartographer, satelliteChoir]} />, {
      wrapper: makeWrapper(),
    });

    await userEvent.click(screen.getByRole("button", { name: /add a title/i }));
    await userEvent.type(screen.getByLabelText("Search"), "marigold");

    const result = await screen.findByText("Marigold Protocol");
    await userEvent.click(result);

    await waitFor(() => expect(patches).toHaveLength(1));
    // The added entry carries an optimistic `state` seeded from the candidate's inLibrary
    // (here true ⇒ "available"); the backend re-derives the authoritative value on refetch.
    expect(patches[0]).toEqual({
      revision: 1,
      lineup: [
        cartographer,
        satelliteChoir,
        { key: "movie:tmdb:90000106", name: "Marigold Protocol", year: 1987, state: "available" },
      ],
    });
  });

  it("a not-in-library pick still commits and reads as Pending", async () => {
    const { patches } = stubEditor({
      candidates: [
        { mediaType: "series", tvdbId: 90081189, name: "Copperline Foundry", year: 2008, inLibrary: false },
      ],
    });
    // Like the channel page: the editor's lineup is the channel query's, which the save updates.
    let saved = channel({ lineup: [cartographer] });
    server.use(
      getGetChannelMockHandler(() => saved),
      getUpdateChannelMockHandler(async ({ request }) => {
        const body = (await request.json()) as { lineup: LineupEntryDTO[] };
        patches.push(body);
        saved = { ...saved, revision: saved.revision + 1, lineup: body.lineup };
        return saved;
      }),
    );
    const Page = () => {
      const ch = unwrap(channelsApi.useGetChannel("ch-1").data);
      return ch ? <ChannelLineupEditor channelId="ch-1" lineup={ch.lineup} /> : null;
    };
    render(<Page />, { wrapper: makeWrapper() });

    await userEvent.click(await screen.findByRole("button", { name: /add a title/i }));
    await userEvent.type(screen.getByLabelText("Search"), "copperline");
    await userEvent.click(await screen.findByText("Copperline Foundry"));

    await waitFor(() => expect(patches).toHaveLength(1));
    // A candidate the search marked !inLibrary is added with an optimistic state "pending".
    expect(patches[0]).toEqual({
      revision: 1,
      lineup: [
        cartographer,
        { key: "series:tvdb:90081189", name: "Copperline Foundry", year: 2008, state: "pending" },
      ],
    });
    // The add closed the palette and returned to the populated list, where the new
    // pick reads Pending (from its state, which now rides on the entry — durable, not
    // client-only memory), saved as well as shown.
    expect(await screen.findByText("Pending")).toBeInTheDocument();
  });

  it("remove drops the key and commits the remaining entries, in order", async () => {
    const { patches } = stubEditor();
    render(<ChannelLineupEditor channelId="ch-1" lineup={[cartographer, satelliteChoir]} />, {
      wrapper: makeWrapper(),
    });

    await userEvent.click(screen.getByRole("button", { name: "Remove The Velvet Cartographer" }));

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toEqual({ revision: 1, lineup: [satelliteChoir] });
  });

  it("a candidate whose key is already in the lineup is not offered", async () => {
    stubEditor({
      candidates: [
        {
          mediaType: "movie",
          tmdbId: 90000949,
          name: "The Velvet Cartographer",
          year: 1995,
          inLibrary: true,
        },
        { mediaType: "movie", tmdbId: 90000106, name: "Marigold Protocol", year: 1987, inLibrary: true },
      ],
    });
    render(<ChannelLineupEditor channelId="ch-1" lineup={[cartographer]} />, { wrapper: makeWrapper() });

    await userEvent.click(screen.getByRole("button", { name: /add a title/i }));
    await userEvent.type(screen.getByLabelText("Search"), "ve");

    // Marigold Protocol (not yet on the lineup) surfaces; The Velvet Cartographer (already on
    // it, same tmdbId ⇒ same key) is filtered out of the results entirely rather than merely
    // disabled.
    await screen.findByText("Marigold Protocol");
    expect(screen.queryByText("The Velvet Cartographer", { selector: "button *" })).not.toBeInTheDocument();
    // The lineup's own existing "The Velvet Cartographer" row is unaffected (still exactly one).
    expect(screen.getAllByText("The Velvet Cartographer")).toHaveLength(1);
  });

  // An id-less candidate (no tmdbId/tvdbId) has no provisioning key the backend can
  // accept — its key would fall back to `name:…`, which provision.ParseKey rejects with a
  // 422. Rather than let a user pick it and watch the optimistic add snap back, it is
  // filtered out of the results entirely (a title with no id can't be provisioned anyway).
  it("a candidate with no usable id is not offered (its key is unaddable)", async () => {
    const { patches } = stubEditor({
      candidates: [
        { mediaType: "movie", name: "Some Obscure Film", year: 1980, inLibrary: false }, // no tmdbId
        { mediaType: "movie", tmdbId: 90000106, name: "Marigold Protocol", year: 1987, inLibrary: true },
      ],
    });
    render(<ChannelLineupEditor channelId="ch-1" lineup={[]} />, { wrapper: makeWrapper() });

    await userEvent.click(screen.getByRole("button", { name: /add a title/i }));
    await userEvent.type(screen.getByLabelText("Search"), "film");

    // The id-bearing Marigold Protocol is offered; the id-less film is not.
    await screen.findByText("Marigold Protocol");
    expect(screen.queryByText("Some Obscure Film")).not.toBeInTheDocument();
    // And nothing is committed for the unaddable one.
    expect(patches).toHaveLength(0);
  });

  it("cancelling the add palette closes it without committing", async () => {
    const { patches } = stubEditor();
    render(<ChannelLineupEditor channelId="ch-1" lineup={[cartographer]} />, { wrapper: makeWrapper() });

    await userEvent.click(screen.getByRole("button", { name: /add a title/i }));
    expect(screen.getByLabelText("Search")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /cancel/i }));

    expect(screen.queryByLabelText("Search")).not.toBeInTheDocument();
    expect(patches).toHaveLength(0);
  });
});
