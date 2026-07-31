import type { DiscoveredClip } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { DiscoverPanel } from "./discover-panel";

const items: DiscoveredClip[] = [
  { id: "cm-1993-4", title: "Commercials 1993", year: 1993, url: "https://archive.org/details/cm-1993-4" },
  { id: "no-year", title: "Untitled reel", url: "https://archive.org/details/no-year" },
  { id: "no-title", url: "https://archive.org/details/no-title" },
];

const base = {
  items,
  total: 54,
  query: "cereal",
  onQueryChange: () => {},
  onSearch: () => {},
  searched: true,
};

describe("DiscoverPanel", () => {
  it("lists what the search found", () => {
    render(<DiscoverPanel {...base} />);
    expect(screen.getByText("Commercials 1993")).toBeInTheDocument();
    expect(screen.getByText("Untitled reel")).toBeInTheDocument();
  });

  // ⚠ The SOURCE's count, not the page's. "54 hits, showing 3" is a different situation from
  // "3 hits", and the total is what an operator judges the search by.
  it("reports the source's total, not the page length", () => {
    render(<DiscoverPanel {...base} />);
    expect(screen.getByText("3 of 54 matches")).toBeInTheDocument();
  });

  it("says just the count when the page is the whole result", () => {
    render(<DiscoverPanel {...base} total={3} />);
    expect(screen.getByText("3 matches")).toBeInTheDocument();
  });

  // An item with no title is still addable; a blank row would look broken.
  it("falls back to the id when an item has no title", () => {
    render(<DiscoverPanel {...base} />);
    expect(screen.getByText("no-title")).toBeInTheDocument();
  });

  // Absent year is the common case — Solr omits the field — and must render as nothing rather
  // than as the year 0.
  it("renders no year rather than a zero", () => {
    render(<DiscoverPanel {...base} />);
    expect(screen.getByText("1993")).toBeInTheDocument();
    expect(screen.queryByText("0")).not.toBeInTheDocument();
  });

  // ⚠ Build plan §6.3: the caveat is stated ONCE, about the search. A per-row chip would read
  // "unknown" on nearly every result and imply a check that never happened.
  it("shows the licence caveat once, not per row", () => {
    render(<DiscoverPanel {...base} licenceNote="Licence information isn't available for most results." />);
    expect(screen.getAllByText(/licence information isn't available/i)).toHaveLength(1);
  });

  it("adds the picked ids", async () => {
    const onAdd = vi.fn();
    render(<DiscoverPanel {...base} onAdd={onAdd} />);

    await userEvent.click(screen.getByLabelText("Commercials 1993"));
    await userEvent.click(screen.getByRole("button", { name: "Add 1 clip" }));
    expect(onAdd).toHaveBeenCalledWith(["cm-1993-4"]);
  });

  it("cannot add nothing", () => {
    render(<DiscoverPanel {...base} onAdd={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Add selected" })).toBeDisabled();
  });

  // ⚠ Without ingest tooling the RESULTS still show — an operator can fetch them by hand — so
  // only the action disappears. Hiding the whole search behind a download gate would remove a
  // capability that has nothing to do with downloading.
  it("still lists results when this image cannot download", () => {
    render(<DiscoverPanel {...base} onAdd={undefined} />);
    expect(screen.getByText("Commercials 1993")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /add/i })).not.toBeInTheDocument();
  });

  // ⚠ Only AFTER a search. Saying "nothing matched" before anyone has searched tells an
  // operator their catalog is empty when they have not asked anything yet.
  it("stays quiet about empty results until a search has run", () => {
    const { rerender } = render(<DiscoverPanel {...base} items={[]} searched={false} />);
    expect(screen.queryByText(/nothing matched/i)).not.toBeInTheDocument();

    rerender(<DiscoverPanel {...base} items={[]} searched />);
    expect(screen.getByText(/nothing matched/i)).toBeInTheDocument();
  });

  it("searches on Enter", async () => {
    const onSearch = vi.fn();
    render(<DiscoverPanel {...base} onSearch={onSearch} />);

    await userEvent.type(screen.getByLabelText("Search archive.org"), "{Enter}");
    expect(onSearch).toHaveBeenCalled();
  });

  it("will not search on an empty query", async () => {
    const onSearch = vi.fn();
    render(<DiscoverPanel {...base} query="  " onSearch={onSearch} />);

    expect(screen.getByRole("button", { name: "Search" })).toBeDisabled();
    await userEvent.type(screen.getByLabelText("Search archive.org"), "{Enter}");
    expect(onSearch).not.toHaveBeenCalled();
  });
});
