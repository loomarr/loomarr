import type { LineupEntryDTO } from "@loomarr/api";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui";
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

const jsonResponse = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const heat: LineupEntryDTO = { key: "movie:tmdb:949", name: "Heat", year: 1995 };
const pointBreak: LineupEntryDTO = { key: "movie:tmdb:9426", name: "Point Break", year: 1991 };

// Dispatches by method+path: GET /v1/search answers the add-a-title palette, PATCH
// /v1/channels/{id} is the whole-list-replace commit this editor exists to drive.
// `search.candidates` is read fresh on every call so a test can change it mid-flow
// (e.g. proving a re-search still excludes whatever just got added).
const stubFetch = (opts: { candidates?: unknown[]; patchStatus?: number } = {}) => {
  const patches: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      if (typeof url === "string" && url.includes("/v1/search")) {
        return Promise.resolve(jsonResponse(200, { candidates: opts.candidates ?? [] }));
      }
      if (init?.method === "PATCH") {
        patches.push(init.body ? JSON.parse(init.body as string) : undefined);
        return Promise.resolve(jsonResponse(opts.patchStatus ?? 200, { id: "ch-1" }));
      }
      return Promise.resolve(jsonResponse(200, {}));
    }),
  );
  return { patches };
};

afterEach(() => vi.restoreAllMocks());

describe("ChannelLineupEditor", () => {
  it("renders the current lineup with drag handles and remove buttons", () => {
    stubFetch();
    render(<ChannelLineupEditor channelId="ch-1" lineup={[heat, pointBreak]} />, {
      wrapper: makeWrapper(),
    });

    expect(screen.getByText("Heat")).toBeInTheDocument();
    expect(screen.getByText("Point Break")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reorder Heat" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Remove Heat" })).toBeInTheDocument();
  });

  it("shows an empty state when the lineup is empty", () => {
    stubFetch();
    render(<ChannelLineupEditor channelId="ch-1" lineup={[]} />, { wrapper: makeWrapper() });
    expect(screen.getByText(/nothing in the lineup yet/i)).toBeInTheDocument();
  });

  it("add: searching, picking a result appends it and commits the full ordered list", async () => {
    const { patches } = stubFetch({
      candidates: [{ mediaType: "movie", tmdbId: 106, name: "Predator", year: 1987, inLibrary: true }],
    });
    render(<ChannelLineupEditor channelId="ch-1" lineup={[heat, pointBreak]} />, {
      wrapper: makeWrapper(),
    });

    await userEvent.click(screen.getByRole("button", { name: /add a title/i }));
    await userEvent.type(screen.getByLabelText("Search"), "predator");

    const result = await screen.findByText("Predator");
    await userEvent.click(result);

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toEqual({
      lineup: [heat, pointBreak, { key: "movie:tmdb:106", name: "Predator", year: 1987 }],
    });
  });

  it("a not-in-library pick still commits and reads as Pending", async () => {
    const { patches } = stubFetch({
      candidates: [
        { mediaType: "series", tvdbId: 81189, name: "Breaking Bad", year: 2008, inLibrary: false },
      ],
    });
    render(<ChannelLineupEditor channelId="ch-1" lineup={[heat]} />, { wrapper: makeWrapper() });

    await userEvent.click(screen.getByRole("button", { name: /add a title/i }));
    await userEvent.type(screen.getByLabelText("Search"), "breaking");
    await userEvent.click(await screen.findByText("Breaking Bad"));

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toEqual({
      lineup: [heat, { key: "series:tvdb:81189", name: "Breaking Bad", year: 2008 }],
    });
    // The add closed the palette and returned to the populated list, where the new
    // pick — added from a candidate the search marked !inLibrary — now shows Pending.
    expect(screen.getByText("Pending")).toBeInTheDocument();
  });

  it("remove drops the key and commits the remaining entries, in order", async () => {
    const { patches } = stubFetch();
    render(<ChannelLineupEditor channelId="ch-1" lineup={[heat, pointBreak]} />, {
      wrapper: makeWrapper(),
    });

    await userEvent.click(screen.getByRole("button", { name: "Remove Heat" }));

    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toEqual({ lineup: [pointBreak] });
  });

  it("a candidate whose key is already in the lineup is not offered", async () => {
    stubFetch({
      candidates: [
        { mediaType: "movie", tmdbId: 949, name: "Heat", year: 1995, inLibrary: true },
        { mediaType: "movie", tmdbId: 106, name: "Predator", year: 1987, inLibrary: true },
      ],
    });
    render(<ChannelLineupEditor channelId="ch-1" lineup={[heat]} />, { wrapper: makeWrapper() });

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
    const { patches } = stubFetch({
      candidates: [
        { mediaType: "movie", name: "Some Obscure Film", year: 1980, inLibrary: false }, // no tmdbId
        { mediaType: "movie", tmdbId: 106, name: "Predator", year: 1987, inLibrary: true },
      ],
    });
    render(<ChannelLineupEditor channelId="ch-1" lineup={[]} />, { wrapper: makeWrapper() });

    await userEvent.click(screen.getByRole("button", { name: /add a title/i }));
    await userEvent.type(screen.getByLabelText("Search"), "film");

    // The id-bearing Predator is offered; the id-less film is not.
    await screen.findByText("Predator");
    expect(screen.queryByText("Some Obscure Film")).not.toBeInTheDocument();
    // And nothing is committed for the unaddable one.
    expect(patches).toHaveLength(0);
  });

  it("cancelling the add palette closes it without committing", async () => {
    const { patches } = stubFetch();
    render(<ChannelLineupEditor channelId="ch-1" lineup={[heat]} />, { wrapper: makeWrapper() });

    await userEvent.click(screen.getByRole("button", { name: /add a title/i }));
    expect(screen.getByLabelText("Search")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /cancel/i }));

    expect(screen.queryByLabelText("Search")).not.toBeInTheDocument();
    expect(patches).toHaveLength(0);
  });
});
