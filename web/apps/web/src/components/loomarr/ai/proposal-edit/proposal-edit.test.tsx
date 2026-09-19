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

const heat: ProposalItem = { name: "Heat", year: 1995, mediaType: "movie", tmdbId: 949, inLibrary: true };
const simpsons: ProposalItem = {
  name: "The Simpsons",
  year: 1989,
  mediaType: "series",
  tvdbId: 71663,
  inLibrary: false,
};
const philosopherStone: ProposalItem = {
  name: "Harry Potter and the Philosopher's Stone",
  year: 2001,
  mediaType: "movie",
  tmdbId: 671,
  inLibrary: true,
  libraryItemId: "library-671",
};
const chamberSecrets: ProposalItem = {
  name: "Harry Potter and the Chamber of Secrets",
  year: 2002,
  mediaType: "movie",
  tmdbId: 672,
  inLibrary: false,
};
const prisonerAzkaban: ProposalItem = {
  name: "Harry Potter and the Prisoner of Azkaban",
  year: 2004,
  mediaType: "movie",
  tmdbId: 673,
  inLibrary: true,
  libraryItemId: "library-673",
};
const harryPotter = {
  tmdbId: 1241,
  name: "Harry Potter Collection",
  members: [philosopherStone, chamberSecrets, prisonerAzkaban],
};

describe("ProposalEdit", () => {
  it("lists the lineup and the acquisitions as one review list", () => {
    stubSearch([]);
    render(<ProposalEdit lineup={[heat]} acquisitions={[simpsons]} onChange={vi.fn()} />);

    expect(screen.getByText("Heat")).toBeInTheDocument();
    expect(screen.getByText("The Simpsons")).toBeInTheDocument();
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
    render(<ProposalEdit lineup={[heat]} acquisitions={[]} onChange={onChange} />);

    await userEvent.click(screen.getByRole("checkbox", { name: "Include Heat" }));
    expect(onChange).toHaveBeenLastCalledWith({ drop: ["movie:tmdb:949"] });

    await userEvent.click(screen.getByRole("checkbox", { name: "Include Heat" }));
    expect(onChange).toHaveBeenLastCalledWith(undefined);
  });

  // Drops carry the PROVISIONING KEY, not an index or a name: an index means "the third one in
  // the list I was looking at", which is wrong the moment anything reorders between render and
  // submit. The derivation must match Go's exactly or the key matches nothing and the title the
  // admin removed is acquired anyway.
  it("drops by provisioning key, preferring TVDB for a series", async () => {
    stubSearch([]);
    const onChange = vi.fn();
    render(<ProposalEdit lineup={[]} acquisitions={[simpsons]} onChange={onChange} />);

    await userEvent.click(screen.getByRole("checkbox", { name: "Include The Simpsons" }));
    expect(onChange).toHaveBeenLastCalledWith({ drop: ["series:tvdb:71663"] });
  });

  it("marks a dropped pick without removing it from view, so the choice is reversible", async () => {
    stubSearch([]);
    render(<ProposalEdit lineup={[heat]} acquisitions={[]} onChange={vi.fn()} />);

    await userEvent.click(screen.getByRole("checkbox", { name: "Include Heat" }));
    // Still rendered — struck through, with the control now offering to put it back.
    expect(screen.getByText(/Heat/)).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Include Heat" })).not.toBeChecked();
  });

  it("adds a searched title as an acquisition", async () => {
    stubSearch([{ name: "Con Air", year: 1997, mediaType: "movie", tmdbId: 1701, inLibrary: false }]);
    const onChange = vi.fn();
    render(<ProposalEdit lineup={[heat]} acquisitions={[]} onChange={onChange} />);

    await userEvent.click(screen.getByRole("button", { name: "Add title" }));
    await userEvent.type(screen.getByRole("combobox"), "con air");
    await userEvent.click(await screen.findByText("Con Air"));

    expect(onChange).toHaveBeenLastCalledWith({
      add: [{ name: "Con Air", mediaType: "movie", inLibrary: false, year: 1997, tmdbId: 1701 }],
    });
  });

  it("keeps authoritative library availability on a searched title", async () => {
    stubSearch([
      {
        name: "Con Air",
        year: 1997,
        mediaType: "movie",
        tmdbId: 1701,
        inLibrary: true,
        libraryItemId: "library-con-air",
      },
    ]);
    const onChange = vi.fn();
    render(<ProposalEdit lineup={[]} acquisitions={[]} onChange={onChange} />);

    await userEvent.click(screen.getByRole("button", { name: "Add title" }));
    await userEvent.type(screen.getByRole("combobox"), "con air");
    await userEvent.click(await screen.findByText("Con Air"));

    expect(onChange.mock.lastCall?.[0].add[0]).toMatchObject({
      inLibrary: true,
      libraryItemId: "library-con-air",
    });
    expect(screen.getByText("In your library")).toBeVisible();
  });

  it("does not offer a title already on the proposal", async () => {
    stubSearch([{ name: "Heat", year: 1995, mediaType: "movie", tmdbId: 949, inLibrary: true }]);
    render(<ProposalEdit lineup={[heat]} acquisitions={[]} onChange={vi.fn()} />);

    await userEvent.click(screen.getByRole("button", { name: "Add title" }));
    await userEvent.type(screen.getByRole("combobox"), "heat");

    // "Heat" still appears as the existing pick, but not as a search result to add again.
    await waitFor(() => expect(screen.queryAllByText("Heat")).toHaveLength(1));
  });

  it("carries the note, trimmed, and drops it when cleared", async () => {
    stubSearch([]);
    const onChange = vi.fn();
    render(<ProposalEdit lineup={[heat]} acquisitions={[]} onChange={onChange} showNote />);

    const note = screen.getByLabelText("Note to the requester");
    await userEvent.type(note, "  too violent for this channel  ");
    expect(onChange).toHaveBeenLastCalledWith({ note: "  too violent for this channel  " });

    await userEvent.clear(note);
    // Whitespace-only is not a note, so clearing returns to "unmodified".
    expect(onChange).toHaveBeenLastCalledWith(undefined);
  });

  it("combines drops, adds and a note into one edit", async () => {
    stubSearch([{ name: "Con Air", year: 1997, mediaType: "movie", tmdbId: 1701, inLibrary: false }]);
    const onChange = vi.fn();
    render(<ProposalEdit lineup={[heat]} acquisitions={[]} onChange={onChange} showNote />);

    await userEvent.click(screen.getByRole("checkbox", { name: "Include Heat" }));
    await userEvent.click(screen.getByRole("button", { name: "Add title" }));
    await userEvent.type(screen.getByRole("combobox"), "con air");
    await userEvent.click(await screen.findByText("Con Air"));
    await userEvent.type(screen.getByLabelText("Note to the requester"), "swapped");

    expect(onChange).toHaveBeenLastCalledWith({
      drop: ["movie:tmdb:949"],
      add: [expect.objectContaining({ name: "Con Air" })],
      note: "swapped",
    });
  });

  it("resets every change back to unmodified", async () => {
    stubSearch([]);
    const onChange = vi.fn();
    render(<ProposalEdit lineup={[heat]} acquisitions={[]} onChange={onChange} showNote />);

    await userEvent.click(screen.getByRole("checkbox", { name: "Include Heat" }));
    await userEvent.type(screen.getByLabelText("Note to the requester"), "nope");
    await userEvent.click(screen.getByRole("button", { name: /Reset/ }));

    expect(onChange).toHaveBeenLastCalledWith(undefined);
    expect(screen.getByRole("checkbox", { name: "Include Heat" })).toBeChecked();
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
    const backup = { ...heat, name: "Face/Off", tmdbId: 754, inLibrary: false };
    render(<ProposalEdit lineup={[heat]} acquisitions={[]} alternates={[backup]} onChange={onChange} />);

    await userEvent.click(screen.getByText("More suggestions"));
    expect(screen.queryByRole("checkbox", { name: "Include Face/Off" })).not.toBeInTheDocument();
    const suggestion = screen.getByRole("button", { name: "Add Face/Off" }).closest("li");
    expect(suggestion).not.toBeNull();
    expect(within(suggestion!).getByText("Not in your library")).toBeVisible();
    expect(within(suggestion!).queryByText("Will be added")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Add Face/Off" }));
    expect(onChange).toHaveBeenLastCalledWith({
      drop: ["movie:tmdb:754"],
      add: [backup],
    });
  });

  it("offers one compact collection choice when visible films share it", async () => {
    stubSearch([]);
    render(
      <ProposalEdit
        lineup={[philosopherStone, prisonerAzkaban]}
        acquisitions={[]}
        movieCollections={[harryPotter]}
        onChange={vi.fn()}
      />,
    );

    expect(screen.getAllByText("Harry Potter Collection")).toHaveLength(1);
    expect(screen.getByText("2 of 3 selected")).toBeVisible();
    expect(screen.getByText("2 in your library")).toBeVisible();
    expect(screen.getAllByText("Harry Potter and the Chamber of Secrets")[0]).not.toBeVisible();

    await userEvent.click(screen.getByRole("button", { name: "Choose films from Harry Potter Collection" }));
    expect(screen.getAllByText("Harry Potter and the Chamber of Secrets")[0]).toBeVisible();
  });

  it("checks the visible TMDB movies and renders the grounded collection response", async () => {
    let requestedKeys: string[] = [];
    server.use(
      getSearchMockHandler({ candidates: [] }),
      getResolveMovieCollectionsMockHandler(({ request }) => {
        requestedKeys = new URL(request.url).searchParams.getAll("key");
        return { collections: [harryPotter], complete: true };
      }),
    );

    render(
      <ProposalEdit
        lineup={[philosopherStone]}
        acquisitions={[]}
        optionalSuggestions={[prisonerAzkaban]}
        value={{ add: [chamberSecrets] }}
        onChange={vi.fn()}
      />,
    );

    expect(await screen.findByText("Harry Potter Collection")).toBeVisible();
    expect(requestedKeys).toEqual(["movie:tmdb:671", "movie:tmdb:672", "movie:tmdb:673"]);
  });

  it("shows calm progress while collection choices are being checked", () => {
    stubSearch([]);
    render(
      <ProposalEdit
        lineup={[philosopherStone]}
        acquisitions={[]}
        movieCollectionsLoading
        onChange={vi.fn()}
      />,
    );

    expect(screen.getByRole("status")).toHaveTextContent("Checking for movie collections…");
  });

  it("adds only missing collection members through the existing approval edit", async () => {
    stubSearch([]);
    const onChange = vi.fn();
    render(
      <ProposalEdit
        lineup={[philosopherStone]}
        acquisitions={[]}
        movieCollections={[harryPotter]}
        onChange={onChange}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Add all films from Harry Potter Collection" }));

    expect(onChange).toHaveBeenLastCalledWith({
      add: [
        {
          name: "Harry Potter and the Chamber of Secrets",
          mediaType: "movie",
          inLibrary: false,
          year: 2002,
          tmdbId: 672,
        },
        {
          name: "Harry Potter and the Prisoner of Azkaban",
          mediaType: "movie",
          inLibrary: true,
          libraryItemId: "library-673",
          year: 2004,
          tmdbId: 673,
        },
      ],
    });
    expect(screen.queryByRole("region", { name: "Movie collections" })).not.toBeInTheDocument();
    expect(
      screen.getByRole("checkbox", { name: "Include Harry Potter and the Prisoner of Azkaban" }),
    ).toBeChecked();
  });

  it("lets the reviewer choose one collection film without adding the rest", async () => {
    stubSearch([]);
    const onChange = vi.fn();
    render(
      <ProposalEdit
        lineup={[philosopherStone]}
        acquisitions={[]}
        movieCollections={[harryPotter]}
        onChange={onChange}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Choose films from Harry Potter Collection" }));
    await userEvent.click(
      screen.getByRole("button", { name: "Add Harry Potter and the Chamber of Secrets" }),
    );

    expect(onChange).toHaveBeenLastCalledWith({
      add: [expect.objectContaining({ name: "Harry Potter and the Chamber of Secrets", tmdbId: 672 })],
    });
    expect(onChange.mock.lastCall?.[0].add).toHaveLength(1);
  });

  it("re-includes an excluded installment instead of adding a duplicate", async () => {
    stubSearch([]);
    const onChange = vi.fn();
    render(
      <ProposalEdit
        lineup={[philosopherStone]}
        acquisitions={[]}
        movieCollections={[harryPotter]}
        onChange={onChange}
      />,
    );

    await userEvent.click(
      screen.getByRole("checkbox", { name: "Include Harry Potter and the Philosopher's Stone" }),
    );
    expect(onChange).toHaveBeenLastCalledWith({ drop: ["movie:tmdb:671"] });

    await userEvent.click(screen.getByRole("button", { name: "Add all films from Harry Potter Collection" }));
    expect(onChange).toHaveBeenLastCalledWith({
      add: [expect.objectContaining({ tmdbId: 672 }), expect.objectContaining({ tmdbId: 673 })],
    });
  });

  it("promotes a collection member out of optional suggestions exactly once", async () => {
    stubSearch([]);
    const onChange = vi.fn();
    render(
      <ProposalEdit
        lineup={[philosopherStone]}
        acquisitions={[]}
        optionalSuggestions={[chamberSecrets]}
        movieCollections={[harryPotter]}
        onChange={onChange}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Add all films from Harry Potter Collection" }));
    expect(onChange).toHaveBeenLastCalledWith({
      drop: ["movie:tmdb:672"],
      add: [expect.objectContaining({ tmdbId: 672 }), expect.objectContaining({ tmdbId: 673 })],
    });
    expect(onChange.mock.lastCall?.[0].add).toHaveLength(2);
  });

  it("offers an honest search action before any extra suggestions exist", async () => {
    stubSearch([]);
    const onFindMore = vi.fn();
    const view = render(
      <ProposalEdit
        lineup={[heat]}
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
        lineup={[heat]}
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
    render(<ProposalEdit lineup={[heat]} acquisitions={[simpsons]} />);

    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add title" })).not.toBeInTheDocument();
  });
});
