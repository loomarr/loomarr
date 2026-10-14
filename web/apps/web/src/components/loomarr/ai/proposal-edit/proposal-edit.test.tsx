import type { ProposalItem } from "@loomarr/api";
import { getResolveMovieCollectionsMockHandler, getSearchMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as rtlRender, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement, ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui";
import { server } from "@/test/msw/server";
import { ProposalEdit } from "./proposal-edit";

const makeWrapper = () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <TooltipProvider>{children}</TooltipProvider>
    </QueryClientProvider>
  );
};

const render = (ui: ReactElement) => rtlRender(ui, { wrapper: makeWrapper() });

const stubSearch = (candidates: ProposalItem[]) => {
  server.use(
    getSearchMockHandler({ candidates }),
    getResolveMovieCollectionsMockHandler({ collections: [], complete: true }),
  );
};

const cartographer: ProposalItem = {
  name: "The Velvet Cartographer",
  year: 1995,
  mediaType: "movie",
  tmdbId: 90000949,
  inLibrary: true,
};
const kettleKids: ProposalItem = {
  name: "Kettle Kids",
  year: 1989,
  mediaType: "series",
  tvdbId: 90071663,
  inLibrary: false,
};
const copperKey: ProposalItem = {
  name: "Wren Halloway and the Copper Key",
  year: 2001,
  mediaType: "movie",
  tmdbId: 90000671,
  inLibrary: true,
  libraryItemId: "library-671",
};
const glassOrchard: ProposalItem = {
  name: "Wren Halloway and the Glass Orchard",
  year: 2002,
  mediaType: "movie",
  tmdbId: 90000672,
  inLibrary: false,
};
const lanternTide: ProposalItem = {
  name: "Wren Halloway and the Lantern Tide",
  year: 2004,
  mediaType: "movie",
  tmdbId: 90000673,
  inLibrary: true,
  libraryItemId: "library-673",
};
const wrenHalloway = {
  tmdbId: 90001241,
  name: "Wren Halloway Collection",
  members: [copperKey, glassOrchard, lanternTide],
};

describe("ProposalEdit", () => {
  it("lists the lineup and the acquisitions as one review list", () => {
    stubSearch([]);
    render(<ProposalEdit lineup={[cartographer]} acquisitions={[kettleKids]} onChange={vi.fn()} />);

    expect(screen.getByText("The Velvet Cartographer")).toBeInTheDocument();
    expect(screen.getByText("Kettle Kids")).toBeInTheDocument();
    expect(screen.getByText("In your library")).toBeInTheDocument();
    expect(screen.getByText("Will be added")).toBeInTheDocument();
  });

  // ⚠ THE load-bearing case. An unmodified approval must be indistinguishable from the
  // pre-V25 behaviour all the way down — the handler maps a body with no drops/adds/note to a
  // NIL edit, so `{drop: [], add: [], note: ""}` would still record "approved with
  // modifications: none", a different and false claim. Emitting undefined is what lets the
  // caller send no body at all.
  it("emits undefined — never an empty edit — when a change is undone", async () => {
    stubSearch([]);
    const onChange = vi.fn();
    render(<ProposalEdit lineup={[cartographer]} acquisitions={[]} onChange={onChange} />);

    await userEvent.click(screen.getByRole("checkbox", { name: "Include The Velvet Cartographer" }));
    expect(onChange).toHaveBeenLastCalledWith({ drop: ["movie:tmdb:90000949"] });

    await userEvent.click(screen.getByRole("checkbox", { name: "Include The Velvet Cartographer" }));
    expect(onChange).toHaveBeenLastCalledWith(undefined);
  });

  // Drops carry the PROVISIONING KEY, not an index or a name: an index means "the third one in
  // the list I was looking at", which is wrong the moment anything reorders between render and
  // submit. The derivation must match Go's exactly or the key matches nothing and the title the
  // admin removed is acquired anyway.
  it("drops by provisioning key, preferring TVDB for a series", async () => {
    stubSearch([]);
    const onChange = vi.fn();
    render(<ProposalEdit lineup={[]} acquisitions={[kettleKids]} onChange={onChange} />);

    await userEvent.click(screen.getByRole("checkbox", { name: "Include Kettle Kids" }));
    expect(onChange).toHaveBeenLastCalledWith({ drop: ["series:tvdb:90071663"] });
  });

  it("marks a dropped pick without removing it from view, so the choice is reversible", async () => {
    stubSearch([]);
    render(<ProposalEdit lineup={[cartographer]} acquisitions={[]} onChange={vi.fn()} />);

    await userEvent.click(screen.getByRole("checkbox", { name: "Include The Velvet Cartographer" }));
    // Still rendered — struck through, with the control now offering to put it back.
    expect(screen.getByText(/The Velvet Cartographer/)).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Include The Velvet Cartographer" })).not.toBeChecked();
  });

  it("adds a searched title as an acquisition", async () => {
    stubSearch([
      { name: "Pelican Point Motel", year: 1997, mediaType: "movie", tmdbId: 90001701, inLibrary: false },
    ]);
    const onChange = vi.fn();
    render(<ProposalEdit lineup={[cartographer]} acquisitions={[]} onChange={onChange} />);

    await userEvent.click(screen.getByRole("button", { name: "Add title" }));
    await userEvent.type(screen.getByRole("combobox"), "pelican");
    await userEvent.click(await screen.findByText("Pelican Point Motel"));

    expect(onChange).toHaveBeenLastCalledWith({
      add: [
        { name: "Pelican Point Motel", mediaType: "movie", inLibrary: false, year: 1997, tmdbId: 90001701 },
      ],
    });
  });

  it("keeps authoritative library availability on a searched title", async () => {
    stubSearch([
      {
        name: "Pelican Point Motel",
        year: 1997,
        mediaType: "movie",
        tmdbId: 90001701,
        inLibrary: true,
        libraryItemId: "library-pelican-point",
      },
    ]);
    const onChange = vi.fn();
    render(<ProposalEdit lineup={[]} acquisitions={[]} onChange={onChange} />);

    await userEvent.click(screen.getByRole("button", { name: "Add title" }));
    await userEvent.type(screen.getByRole("combobox"), "pelican");
    await userEvent.click(await screen.findByText("Pelican Point Motel"));

    expect(onChange.mock.lastCall?.[0].add[0]).toMatchObject({
      inLibrary: true,
      libraryItemId: "library-pelican-point",
    });
    expect(screen.getByText("In your library")).toBeVisible();
  });

  it("does not offer a title already on the proposal", async () => {
    stubSearch([
      { name: "The Velvet Cartographer", year: 1995, mediaType: "movie", tmdbId: 90000949, inLibrary: true },
    ]);
    render(<ProposalEdit lineup={[cartographer]} acquisitions={[]} onChange={vi.fn()} />);

    await userEvent.click(screen.getByRole("button", { name: "Add title" }));
    await userEvent.type(screen.getByRole("combobox"), "velvet");

    // "The Velvet Cartographer" still appears as the existing pick, but not as a search result to add again.
    await waitFor(() => expect(screen.queryAllByText("The Velvet Cartographer")).toHaveLength(1));
  });

  it("carries the note, trimmed, and drops it when cleared", async () => {
    stubSearch([]);
    const onChange = vi.fn();
    render(<ProposalEdit lineup={[cartographer]} acquisitions={[]} onChange={onChange} showNote />);

    const note = screen.getByLabelText("Note to the requester");
    await userEvent.type(note, "  too violent for this channel  ");
    expect(onChange).toHaveBeenLastCalledWith({ note: "  too violent for this channel  " });

    await userEvent.clear(note);
    // Whitespace-only is not a note, so clearing returns to "unmodified".
    expect(onChange).toHaveBeenLastCalledWith(undefined);
  });

  it("combines drops, adds and a note into one edit", async () => {
    stubSearch([
      { name: "Pelican Point Motel", year: 1997, mediaType: "movie", tmdbId: 90001701, inLibrary: false },
    ]);
    const onChange = vi.fn();
    render(<ProposalEdit lineup={[cartographer]} acquisitions={[]} onChange={onChange} showNote />);

    await userEvent.click(screen.getByRole("checkbox", { name: "Include The Velvet Cartographer" }));
    await userEvent.click(screen.getByRole("button", { name: "Add title" }));
    await userEvent.type(screen.getByRole("combobox"), "pelican");
    await userEvent.click(await screen.findByText("Pelican Point Motel"));
    await userEvent.type(screen.getByLabelText("Note to the requester"), "swapped");

    expect(onChange).toHaveBeenLastCalledWith({
      drop: ["movie:tmdb:90000949"],
      add: [expect.objectContaining({ name: "Pelican Point Motel" })],
      note: "swapped",
    });
  });

  it("resets every change back to unmodified", async () => {
    stubSearch([]);
    const onChange = vi.fn();
    render(<ProposalEdit lineup={[cartographer]} acquisitions={[]} onChange={onChange} showNote />);

    await userEvent.click(screen.getByRole("checkbox", { name: "Include The Velvet Cartographer" }));
    await userEvent.type(screen.getByLabelText("Note to the requester"), "nope");
    await userEvent.click(screen.getByRole("button", { name: /Reset/ }));

    expect(onChange).toHaveBeenLastCalledWith(undefined);
    expect(screen.getByRole("checkbox", { name: "Include The Velvet Cartographer" })).toBeChecked();
  });

  // A pick with no tmdbId/tvdbId has no usable key, so it could never have been enqueued and
  // there is nothing to drop. The control is omitted rather than rendered inert.
  it("offers no drop control for a pick with no usable id", () => {
    stubSearch([]);
    const unkeyed: ProposalItem = { name: "Mystery Film", mediaType: "movie", inLibrary: false };
    render(<ProposalEdit lineup={[unkeyed]} acquisitions={[]} onChange={vi.fn()} />);

    expect(screen.getByText("Mystery Film")).toBeInTheDocument();
    expect(screen.queryByRole("checkbox", { name: /Include Mystery Film/ })).not.toBeInTheDocument();
  });

  it("keeps more suggestions unselected until the reviewer adds one", async () => {
    stubSearch([]);
    const onChange = vi.fn();
    const backup = { ...cartographer, name: "Lemon Tree Heist", tmdbId: 90000754, inLibrary: false };
    render(
      <ProposalEdit lineup={[cartographer]} acquisitions={[]} alternates={[backup]} onChange={onChange} />,
    );

    await userEvent.click(screen.getByText("More suggestions"));
    expect(screen.queryByRole("checkbox", { name: "Include Lemon Tree Heist" })).not.toBeInTheDocument();
    const suggestion = screen.getByRole("button", { name: "Add Lemon Tree Heist" }).closest("li");
    expect(suggestion).not.toBeNull();
    expect(within(suggestion!).getByText("Not in your library")).toBeVisible();
    expect(within(suggestion!).queryByText("Will be added")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Add Lemon Tree Heist" }));
    expect(onChange).toHaveBeenLastCalledWith({
      drop: ["movie:tmdb:90000754"],
      add: [backup],
    });
  });

  it("offers one compact collection choice when visible films share it", async () => {
    stubSearch([]);
    render(
      <ProposalEdit
        lineup={[copperKey, lanternTide]}
        acquisitions={[]}
        movieCollections={[wrenHalloway]}
        onChange={vi.fn()}
      />,
    );

    expect(screen.getAllByText("Wren Halloway Collection")).toHaveLength(1);
    expect(screen.getByText("2 of 3 selected")).toBeVisible();
    expect(screen.getByText("2 in your library")).toBeVisible();
    expect(screen.getAllByText("Wren Halloway and the Glass Orchard")[0]).not.toBeVisible();

    await userEvent.click(screen.getByRole("button", { name: "Choose films from Wren Halloway Collection" }));
    expect(screen.getAllByText("Wren Halloway and the Glass Orchard")[0]).toBeVisible();
  });

  it("checks the visible TMDB movies and renders the grounded collection response", async () => {
    let requestedKeys: string[] = [];
    server.use(
      getSearchMockHandler({ candidates: [] }),
      getResolveMovieCollectionsMockHandler(({ request }) => {
        requestedKeys = new URL(request.url).searchParams.getAll("key");
        return { collections: [wrenHalloway], complete: true };
      }),
    );

    render(
      <ProposalEdit
        lineup={[copperKey]}
        acquisitions={[]}
        optionalSuggestions={[lanternTide]}
        value={{ add: [glassOrchard] }}
        onChange={vi.fn()}
      />,
    );

    expect(await screen.findByText("Wren Halloway Collection")).toBeVisible();
    expect(requestedKeys).toEqual(["movie:tmdb:90000671", "movie:tmdb:90000672", "movie:tmdb:90000673"]);
  });

  it("shows calm progress while collection choices are being checked", () => {
    stubSearch([]);
    render(
      <ProposalEdit lineup={[copperKey]} acquisitions={[]} movieCollectionsLoading onChange={vi.fn()} />,
    );

    expect(screen.getByRole("status")).toHaveTextContent("Checking for movie collections…");
  });

  it("adds only missing collection members through the existing approval edit", async () => {
    stubSearch([]);
    const onChange = vi.fn();
    render(
      <ProposalEdit
        lineup={[copperKey]}
        acquisitions={[]}
        movieCollections={[wrenHalloway]}
        onChange={onChange}
      />,
    );

    await userEvent.click(
      screen.getByRole("button", { name: "Add all films from Wren Halloway Collection" }),
    );

    expect(onChange).toHaveBeenLastCalledWith({
      add: [
        {
          name: "Wren Halloway and the Glass Orchard",
          mediaType: "movie",
          inLibrary: false,
          year: 2002,
          tmdbId: 90000672,
        },
        {
          name: "Wren Halloway and the Lantern Tide",
          mediaType: "movie",
          inLibrary: true,
          libraryItemId: "library-673",
          year: 2004,
          tmdbId: 90000673,
        },
      ],
    });
    expect(screen.queryByRole("region", { name: "Movie collections" })).not.toBeInTheDocument();
    expect(
      screen.getByRole("checkbox", { name: "Include Wren Halloway and the Lantern Tide" }),
    ).toBeChecked();
  });

  it("lets the reviewer choose one collection film without adding the rest", async () => {
    stubSearch([]);
    const onChange = vi.fn();
    render(
      <ProposalEdit
        lineup={[copperKey]}
        acquisitions={[]}
        movieCollections={[wrenHalloway]}
        onChange={onChange}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Choose films from Wren Halloway Collection" }));
    await userEvent.click(screen.getByRole("button", { name: "Add Wren Halloway and the Glass Orchard" }));

    expect(onChange).toHaveBeenLastCalledWith({
      add: [expect.objectContaining({ name: "Wren Halloway and the Glass Orchard", tmdbId: 90000672 })],
    });
    expect(onChange.mock.lastCall?.[0].add).toHaveLength(1);
  });

  it("re-includes an excluded installment instead of adding a duplicate", async () => {
    stubSearch([]);
    const onChange = vi.fn();
    render(
      <ProposalEdit
        lineup={[copperKey]}
        acquisitions={[]}
        movieCollections={[wrenHalloway]}
        onChange={onChange}
      />,
    );

    await userEvent.click(screen.getByRole("checkbox", { name: "Include Wren Halloway and the Copper Key" }));
    expect(onChange).toHaveBeenLastCalledWith({ drop: ["movie:tmdb:90000671"] });

    await userEvent.click(
      screen.getByRole("button", { name: "Add all films from Wren Halloway Collection" }),
    );
    expect(onChange).toHaveBeenLastCalledWith({
      add: [expect.objectContaining({ tmdbId: 90000672 }), expect.objectContaining({ tmdbId: 90000673 })],
    });
  });

  it("promotes a collection member out of optional suggestions exactly once", async () => {
    stubSearch([]);
    const onChange = vi.fn();
    render(
      <ProposalEdit
        lineup={[copperKey]}
        acquisitions={[]}
        optionalSuggestions={[glassOrchard]}
        movieCollections={[wrenHalloway]}
        onChange={onChange}
      />,
    );

    await userEvent.click(
      screen.getByRole("button", { name: "Add all films from Wren Halloway Collection" }),
    );
    expect(onChange).toHaveBeenLastCalledWith({
      drop: ["movie:tmdb:90000672"],
      add: [expect.objectContaining({ tmdbId: 90000672 }), expect.objectContaining({ tmdbId: 90000673 })],
    });
    expect(onChange.mock.lastCall?.[0].add).toHaveLength(2);
  });

  it("offers an honest search action before any extra suggestions exist", async () => {
    stubSearch([]);
    const onFindMore = vi.fn();
    const view = render(
      <ProposalEdit
        lineup={[cartographer]}
        acquisitions={[]}
        alternates={[]}
        onChange={vi.fn()}
        onFindMore={onFindMore}
      />,
    );

    expect(screen.queryByText("0 options")).not.toBeInTheDocument();
    expect(screen.queryByText("There aren't any extra options yet.")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Find more suggestions" }));
    expect(onFindMore).toHaveBeenCalledOnce();

    view.rerender(
      <ProposalEdit
        lineup={[cartographer]}
        acquisitions={[]}
        alternates={[]}
        onChange={vi.fn()}
        onFindMore={onFindMore}
        findingMore
      />,
    );
    expect(screen.getByRole("button", { name: "Finding more suggestions…" })).toBeDisabled();
  });

  it("renders a read-only title summary when no edit handler is provided", () => {
    stubSearch([]);
    render(<ProposalEdit lineup={[cartographer]} acquisitions={[kettleKids]} />);

    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add title" })).not.toBeInTheDocument();
  });
});
