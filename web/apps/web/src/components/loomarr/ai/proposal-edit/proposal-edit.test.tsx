import type { ProposalItem } from "@loomarr/api";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as rtlRender, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement, ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui";
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

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });

const stubSearch = (candidates: unknown[]) => {
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => {
      if (typeof url === "string" && url.includes("/v1/search")) {
        return Promise.resolve(jsonResponse({ candidates }));
      }
      return Promise.resolve(jsonResponse({}));
    }),
  );
};

afterEach(() => vi.unstubAllGlobals());

const heat: ProposalItem = { name: "Heat", year: 1995, mediaType: "movie", tmdbId: 949, inLibrary: true };
const simpsons: ProposalItem = {
  name: "The Simpsons",
  year: 1989,
  mediaType: "series",
  tvdbId: 71663,
  inLibrary: false,
};

describe("ProposalEdit", () => {
  it("lists the lineup and the acquisitions as one review list", () => {
    stubSearch([]);
    render(<ProposalEdit lineup={[heat]} acquisitions={[simpsons]} onChange={vi.fn()} />);

    expect(screen.getByText("Heat")).toBeInTheDocument();
    expect(screen.getByText("The Simpsons")).toBeInTheDocument();
    expect(screen.getByText("In library")).toBeInTheDocument();
    expect(screen.getByText("Will acquire")).toBeInTheDocument();
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

    await userEvent.click(screen.getByRole("button", { name: "Remove Heat" }));
    expect(onChange).toHaveBeenLastCalledWith({ drop: ["movie:tmdb:949"] });

    await userEvent.click(screen.getByRole("button", { name: "Keep Heat" }));
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

    await userEvent.click(screen.getByRole("button", { name: "Remove The Simpsons" }));
    expect(onChange).toHaveBeenLastCalledWith({ drop: ["series:tvdb:71663"] });
  });

  it("marks a dropped pick without removing it from view, so the choice is reversible", async () => {
    stubSearch([]);
    render(<ProposalEdit lineup={[heat]} acquisitions={[]} onChange={vi.fn()} />);

    await userEvent.click(screen.getByRole("button", { name: "Remove Heat" }));
    // Still rendered — struck through, with the control now offering to put it back.
    expect(screen.getByText(/Heat/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Keep Heat" })).toBeInTheDocument();
  });

  it("adds a searched title as an acquisition", async () => {
    stubSearch([{ name: "Con Air", year: 1997, mediaType: "movie", tmdbId: 1701, inLibrary: false }]);
    const onChange = vi.fn();
    render(<ProposalEdit lineup={[heat]} acquisitions={[]} onChange={onChange} />);

    await userEvent.click(screen.getByRole("button", { name: /Add a title/ }));
    await userEvent.type(screen.getByRole("combobox"), "con air");
    await userEvent.click(await screen.findByText("Con Air"));

    expect(onChange).toHaveBeenLastCalledWith({
      add: [{ name: "Con Air", mediaType: "movie", inLibrary: false, year: 1997, tmdbId: 1701 }],
    });
  });

  // An added title is never "in library" from here: it goes through the same idempotent enqueue
  // as anything the model proposed, and the provisioner decides what is already present.
  it("marks an added title as not-in-library even when the search says otherwise", async () => {
    stubSearch([{ name: "Con Air", year: 1997, mediaType: "movie", tmdbId: 1701, inLibrary: true }]);
    const onChange = vi.fn();
    render(<ProposalEdit lineup={[]} acquisitions={[]} onChange={onChange} />);

    await userEvent.click(screen.getByRole("button", { name: /Add a title/ }));
    await userEvent.type(screen.getByRole("combobox"), "con air");
    await userEvent.click(await screen.findByText("Con Air"));

    expect(onChange.mock.lastCall?.[0].add[0].inLibrary).toBe(false);
  });

  it("does not offer a title already on the proposal", async () => {
    stubSearch([{ name: "Heat", year: 1995, mediaType: "movie", tmdbId: 949, inLibrary: true }]);
    render(<ProposalEdit lineup={[heat]} acquisitions={[]} onChange={vi.fn()} />);

    await userEvent.click(screen.getByRole("button", { name: /Add a title/ }));
    await userEvent.type(screen.getByRole("combobox"), "heat");

    // "Heat" still appears as the existing pick, but not as a search result to add again.
    await waitFor(() => expect(screen.queryAllByText("Heat")).toHaveLength(1));
  });

  it("carries the note, trimmed, and drops it when cleared", async () => {
    stubSearch([]);
    const onChange = vi.fn();
    render(<ProposalEdit lineup={[heat]} acquisitions={[]} onChange={onChange} />);

    const note = screen.getByLabelText("Note to the requester");
    await userEvent.type(note, "  too violent for this channel  ");
    expect(onChange).toHaveBeenLastCalledWith({ note: "too violent for this channel" });

    await userEvent.clear(note);
    // Whitespace-only is not a note, so clearing returns to "unmodified".
    expect(onChange).toHaveBeenLastCalledWith(undefined);
  });

  it("combines drops, adds and a note into one edit", async () => {
    stubSearch([{ name: "Con Air", year: 1997, mediaType: "movie", tmdbId: 1701, inLibrary: false }]);
    const onChange = vi.fn();
    render(<ProposalEdit lineup={[heat]} acquisitions={[]} onChange={onChange} />);

    await userEvent.click(screen.getByRole("button", { name: "Remove Heat" }));
    await userEvent.click(screen.getByRole("button", { name: /Add a title/ }));
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
    render(<ProposalEdit lineup={[heat]} acquisitions={[]} onChange={onChange} />);

    await userEvent.click(screen.getByRole("button", { name: "Remove Heat" }));
    await userEvent.type(screen.getByLabelText("Note to the requester"), "nope");
    await userEvent.click(screen.getByRole("button", { name: /Reset/ }));

    expect(onChange).toHaveBeenLastCalledWith(undefined);
    expect(screen.getByRole("button", { name: "Remove Heat" })).toBeInTheDocument();
  });

  // A pick with no tmdbId/tvdbId has no usable key, so it could never have been enqueued and
  // there is nothing to drop. The control is omitted rather than rendered inert.
  it("offers no drop control for a pick with no usable id", () => {
    stubSearch([]);
    const unkeyed: ProposalItem = { name: "Mystery Film", mediaType: "movie", inLibrary: false };
    render(<ProposalEdit lineup={[unkeyed]} acquisitions={[]} onChange={vi.fn()} />);

    expect(screen.getByText("Mystery Film")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Remove Mystery Film/ })).not.toBeInTheDocument();
  });
});
