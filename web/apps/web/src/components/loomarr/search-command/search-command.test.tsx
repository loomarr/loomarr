import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { SearchCommand } from "./search-command";
import type { SearchResult } from "./search-command.type";

const results: SearchResult[] = [
  { id: "c1", scope: "channels", name: "90s Action", meta: "#42" },
  { id: "l1", scope: "library", name: "Heat", meta: "1995", inLibrary: true },
  { id: "t1", scope: "tmdb", name: "Con Air", meta: "1997" },
];

describe("SearchCommand", () => {
  it("groups results by scope and flags in-library items", () => {
    render(<SearchCommand query="a" onQueryChange={() => {}} results={results} onSelect={() => {}} />);
    expect(screen.getByText("Channels")).toBeInTheDocument();
    expect(screen.getByText("In your library")).toBeInTheDocument();
    expect(screen.getByText("In library")).toBeInTheDocument();
  });

  it("selects a result", () => {
    const onSelect = vi.fn();
    render(<SearchCommand query="heat" onQueryChange={() => {}} results={results} onSelect={onSelect} />);
    fireEvent.click(screen.getByText("Heat"));
    expect(onSelect).toHaveBeenCalledWith(results[1]);
  });

  it("shows an empty state carrying the query", () => {
    render(<SearchCommand query="zzz" onQueryChange={() => {}} results={[]} />);
    expect(screen.getByText(/No matches for/)).toHaveTextContent("zzz");
  });
});
