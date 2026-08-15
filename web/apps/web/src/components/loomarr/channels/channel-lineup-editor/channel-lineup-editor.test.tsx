import type { LineupEntryDTO, SearchCandidate } from "@loomarr/api";
import { getSearchMockHandler, getUpdateChannelMockHandler } from "@loomarr/api/msw";
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

const heat: LineupEntryDTO = { key: "movie:tmdb:949", name: "Heat", year: 1995 };
const pointBreak: LineupEntryDTO = { key: "movie:tmdb:9426", name: "Point Break", year: 1991 };

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
    getSearchMockHandler(() => ({ candidates: opts.candidates ?? [] })),
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
    render(<ChannelLineupEditor channelId="ch-1" revision={1} lineup={[heat, pointBreak]} />, {
      wrapper: makeWrapper(),
    });

    expect(screen.getByText("Heat")).toBeInTheDocument();
    expect(screen.getByText("Point Break")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reorder Heat" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Remove Heat" })).toBeInTheDocument();
  });

  it("shows an empty state when the lineup is empty", () => {
    stubEditor();
    render(<ChannelLineupEditor channelId="ch-1" revision={1} lineup={[]} />, { wrapper: makeWrapper() });
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
        revision={1}
        lineup={[
          { key: "movie:tmdb:949", name: "Heat", year: 1995, state: "available" },
          { key: "movie:tmdb:106", name: "Predator", year: 1987, state: "pending" },
          { key: "movie:tmdb:603", name: "The Matrix", year: 1999, state: "acquiring" },
          { key: "movie:tmdb:11", name: "Star Wars", year: 1977, state: "unavailable" },
        ]}
      />,
      { wrapper: makeWrapper() },
    );

    // available → no badge; the other three each show their state.
    expect(screen.getByText("Pending")).toBeInTheDocument();
    expect(screen.getByText("Acquiring…")).toBeInTheDocument();
    expect(screen.getByText("Unavailable")).toBeInTheDocument();
    // Heat (available) wears none of them.
    expect(screen.queryByText("Available")).not.toBeInTheDocument();
  });

  it("add: searching, picking a result appends it and commits the full ordered list", async () => {
    const { patches } = stubEditor({
      candidates: [{ mediaType: "movie", tmdbId: 106, name: "Predator", year: 1987, inLibrary: true }],
    });
    render(<ChannelLineupEditor channelId="ch-1" revision={1} lineup={[heat, pointBreak]} />, {
      wrapper: makeWrapper(),
    });

    await userEvent.click(screen.getByRole("button", { name: /add a title/i }));
    await userEvent.type(screen.getByLabelText("Search"), "predator");

    const result = await screen.findByText("Predator");
    await userEvent.click(result);

    await waitFor(() => expect(patches).toHaveLength(1));
    // The added entry carries an optimistic `state` seeded from the candidate's inLibrary
    // (here true ⇒ "available"); the backend re-derives the authoritative value on refetch.
    expect(patches[0]).toEqual({
      revision: 1,
      lineup: [heat, pointBreak, { key: "movie:tmdb:106", name: "Predator", year: 1987, state: "available" }],
    });
  });

  it("a not-in-library pick still commits and reads as Pending", async () => {
    const { patches } = stubEditor({
      candidates: [
        { mediaType: "series", tvdbId: 81189, name: "Breaking Bad", year: 2008, inLibrary: false },
      ],
    });
    render(<ChannelLineupEditor channelId="ch-1" revision={1} lineup={[heat]} />, { wrapper: makeWrapper() });

    await userEvent.click(screen.getByRole("button", { name: /add a title/i }));
    await userEvent.type(screen.getByLabelText("Search"), "breaking");
    await userEvent.click(await screen.findByText("Breaking Bad"));

    await waitFor(() => expect(patches).toHaveLength(1));
    // A candidate the search marked !inLibrary is added with an optimistic state "pending".
    expect(patches[0]).toEqual({
      revision: 1,
      lineup: [heat, { key: "series:tvdb:81189", name: "Breaking Bad", year: 2008, state: "pending" }],
    });
    // The add closed the palette and returned to the populated list, where the new
    // pick reads Pending (from its state, which now rides on the entry — durable, not
    // client-only memory).
    expect(screen.getByText("Pending")).toBeInTheDocument();
  });

  it("remove drops the key and commits the remaining entries, in order", async () => {
    const { patches } = stubEditor();
    render(<ChannelLineupEditor channelId="ch-1" revision={1} lineup={[heat, pointBreak]} />, {
      wrapper: makeWrapper(),
    });

    await userEvent.click(screen.getByRole("button", { name: "Remove Heat" }));

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toEqual({ revision: 1, lineup: [pointBreak] });
  });

  it("a candidate whose key is already in the lineup is not offered", async () => {
    stubEditor({
      candidates: [
        { mediaType: "movie", tmdbId: 949, name: "Heat", year: 1995, inLibrary: true },
        { mediaType: "movie", tmdbId: 106, name: "Predator", year: 1987, inLibrary: true },
      ],
    });
    render(<ChannelLineupEditor channelId="ch-1" revision={1} lineup={[heat]} />, { wrapper: makeWrapper() });

    await userEvent.click(screen.getByRole("button", { name: /add a title/i }));
    await userEvent.type(screen.getByLabelText("Search"), "he");

    // Predator (not yet on the lineup) surfaces; Heat (already on it, same tmdbId ⇒
    // same key) is filtered out of the results entirely rather than merely disabled.
    await screen.findByText("Predator");
    expect(screen.queryByText("Heat", { selector: "button *" })).not.toBeInTheDocument();
    // The lineup's own existing "Heat" row is unaffected (still exactly one).
    expect(screen.getAllByText("Heat")).toHaveLength(1);
  });

  // An id-less candidate (no tmdbId/tvdbId) has no provisioning key the backend can
  // accept — its key would fall back to `name:…`, which provision.ParseKey rejects with a
  // 422. Rather than let a user pick it and watch the optimistic add snap back, it is
  // filtered out of the results entirely (a title with no id can't be provisioned anyway).
  it("a candidate with no usable id is not offered (its key is unaddable)", async () => {
    const { patches } = stubEditor({
      candidates: [
        { mediaType: "movie", name: "Some Obscure Film", year: 1980, inLibrary: false }, // no tmdbId
        { mediaType: "movie", tmdbId: 106, name: "Predator", year: 1987, inLibrary: true },
      ],
    });
    render(<ChannelLineupEditor channelId="ch-1" revision={1} lineup={[]} />, { wrapper: makeWrapper() });

    await userEvent.click(screen.getByRole("button", { name: /add a title/i }));
    await userEvent.type(screen.getByLabelText("Search"), "film");

    // The id-bearing Predator is offered; the id-less film is not.
    await screen.findByText("Predator");
    expect(screen.queryByText("Some Obscure Film")).not.toBeInTheDocument();
    // And nothing is committed for the unaddable one.
    expect(patches).toHaveLength(0);
  });

  it("cancelling the add palette closes it without committing", async () => {
    const { patches } = stubEditor();
    render(<ChannelLineupEditor channelId="ch-1" revision={1} lineup={[heat]} />, { wrapper: makeWrapper() });

    await userEvent.click(screen.getByRole("button", { name: /add a title/i }));
    expect(screen.getByLabelText("Search")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /cancel/i }));

    expect(screen.queryByLabelText("Search")).not.toBeInTheDocument();
    expect(patches).toHaveLength(0);
  });
});
