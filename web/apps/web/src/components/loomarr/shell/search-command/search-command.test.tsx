import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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

  // --- keyboard + listbox semantics (§12 drift claim `:757`) ---
  //
  // The palette was mouse-only: a keyboard user could type a query and had no way to choose a
  // result. §12 described it as cmdk/shadcn `Command`, which was never true. Worth noting CI's
  // axe sweep passed the entire time — missing arrow-key navigation is a keyboard-INTERACTION
  // defect, invisible to a static-markup scan. These tests are the coverage axe cannot give.

  it("exposes combobox/listbox/option roles", () => {
    render(<SearchCommand query="a" onQueryChange={() => {}} results={results} />);

    const input = screen.getByRole("combobox", { name: "Search" });
    const listbox = screen.getByRole("listbox", { name: "Search results" });
    expect(input).toHaveAttribute("aria-controls", listbox.id);
    expect(input).toHaveAttribute("aria-expanded", "true");
    expect(screen.getAllByRole("option")).toHaveLength(3);
  });

  // Each scope is a `group`, not bare text: a listbox may contain only `option`/`group`
  // children, so the previous <p> headings rendered straight into the list were invalid.
  it("sections each scope as a labelled group", () => {
    render(<SearchCommand query="a" onQueryChange={() => {}} results={results} />);
    expect(screen.getByRole("group", { name: "Channels" })).toBeInTheDocument();
    expect(screen.getByRole("group", { name: "In your library" })).toBeInTheDocument();
  });

  it("marks the first result active on render", () => {
    render(<SearchCommand query="a" onQueryChange={() => {}} results={results} />);
    const options = screen.getAllByRole("option");
    expect(options[0]).toHaveAttribute("aria-selected", "true");
    expect(options[1]).toHaveAttribute("aria-selected", "false");
    // aria-activedescendant points at the active option; DOM focus stays in the input.
    expect(screen.getByRole("combobox")).toHaveAttribute("aria-activedescendant", options[0]?.id);
  });

  // The keyboard walks ONE flat sequence in render order, so ↓ crosses a scope heading rather
  // than stopping at it — three results in three different groups are still 1→2→3.
  it("moves the active option with ArrowDown/ArrowUp across group boundaries", async () => {
    render(<SearchCommand query="a" onQueryChange={() => {}} results={results} />);
    const input = screen.getByRole("combobox");
    input.focus();

    await userEvent.keyboard("{ArrowDown}");
    expect(screen.getAllByRole("option")[1]).toHaveAttribute("aria-selected", "true");

    await userEvent.keyboard("{ArrowDown}");
    expect(screen.getAllByRole("option")[2]).toHaveAttribute("aria-selected", "true");

    await userEvent.keyboard("{ArrowUp}");
    expect(screen.getAllByRole("option")[1]).toHaveAttribute("aria-selected", "true");
  });

  it("wraps at both ends", async () => {
    render(<SearchCommand query="a" onQueryChange={() => {}} results={results} />);
    screen.getByRole("combobox").focus();

    // ↑ from the first wraps to the last…
    await userEvent.keyboard("{ArrowUp}");
    expect(screen.getAllByRole("option")[2]).toHaveAttribute("aria-selected", "true");
    // …and ↓ from the last returns to the first.
    await userEvent.keyboard("{ArrowDown}");
    expect(screen.getAllByRole("option")[0]).toHaveAttribute("aria-selected", "true");
  });

  it("jumps to first/last with Home/End", async () => {
    render(<SearchCommand query="a" onQueryChange={() => {}} results={results} />);
    screen.getByRole("combobox").focus();

    await userEvent.keyboard("{End}");
    expect(screen.getAllByRole("option")[2]).toHaveAttribute("aria-selected", "true");
    await userEvent.keyboard("{Home}");
    expect(screen.getAllByRole("option")[0]).toHaveAttribute("aria-selected", "true");
  });

  // The whole point of the fix: a result reachable by keyboard alone.
  it("selects the active result with Enter", async () => {
    const onSelect = vi.fn();
    render(<SearchCommand query="a" onQueryChange={() => {}} results={results} onSelect={onSelect} />);
    screen.getByRole("combobox").focus();

    await userEvent.keyboard("{ArrowDown}{Enter}");
    expect(onSelect).toHaveBeenCalledWith(results[1]);
  });

  // Typing must still reach onQueryChange — the key handler intercepts navigation keys only.
  it("does not swallow ordinary typing", async () => {
    const onQueryChange = vi.fn();
    render(<SearchCommand query="" onQueryChange={onQueryChange} results={results} />);
    screen.getByRole("combobox").focus();

    await userEvent.keyboard("h");
    expect(onQueryChange).toHaveBeenCalledWith("h");
  });

  // After typing another letter "the 2nd result" is a different title, so the highlight must
  // return to the top rather than sit on something the user never looked at.
  it("resets the active option when the result set changes", async () => {
    const { rerender } = render(<SearchCommand query="a" onQueryChange={() => {}} results={results} />);
    screen.getByRole("combobox").focus();
    await userEvent.keyboard("{End}");
    expect(screen.getAllByRole("option")[2]).toHaveAttribute("aria-selected", "true");

    rerender(
      <SearchCommand
        query="ab"
        onQueryChange={() => {}}
        results={[results[2] as SearchResult, results[0] as SearchResult]}
      />,
    );
    expect(screen.getAllByRole("option")[0]).toHaveAttribute("aria-selected", "true");
  });

  // Enter on an empty list must not throw or call back — the handler returns early.
  it("ignores navigation keys with no results", async () => {
    const onSelect = vi.fn();
    render(<SearchCommand query="zzz" onQueryChange={() => {}} results={[]} onSelect={onSelect} />);
    const input = screen.getByRole("combobox");
    input.focus();

    await userEvent.keyboard("{ArrowDown}{Enter}");
    expect(onSelect).not.toHaveBeenCalled();
    expect(input).not.toHaveAttribute("aria-activedescendant");
    expect(input).toHaveAttribute("aria-expanded", "false");
  });

  // An EMPTY listbox violates axe's `aria-required-children` (critical): the role promises
  // `option` children. A first cut kept the listbox mounted always — reasoning that
  // `aria-controls` should never dangle — and the gallery's axe sweep failed on the Idle and
  // Empty stories while every unit test passed. Adding ARIA can introduce a violation as
  // easily as it fixes one, so both directions are pinned here.
  it("renders no listbox at all when there is nothing to list", () => {
    render(<SearchCommand query="zzz" onQueryChange={() => {}} results={[]} />);
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    // …and the input must not point at one that isn't there.
    expect(screen.getByRole("combobox")).not.toHaveAttribute("aria-controls");
  });

  it("wires aria-controls to the listbox once results exist", () => {
    render(<SearchCommand query="a" onQueryChange={() => {}} results={results} />);
    expect(screen.getByRole("combobox")).toHaveAttribute("aria-controls", screen.getByRole("listbox").id);
  });

  // Pointer and keyboard must agree about which row Enter would pick, so hovering moves the
  // active option too.
  it("syncs the active option to the hovered row", async () => {
    const onSelect = vi.fn();
    render(<SearchCommand query="a" onQueryChange={() => {}} results={results} onSelect={onSelect} />);

    const options = screen.getAllByRole("option");
    fireEvent.mouseMove(options[2] as HTMLElement);
    expect(options[2]).toHaveAttribute("aria-selected", "true");

    screen.getByRole("combobox").focus();
    await userEvent.keyboard("{Enter}");
    expect(onSelect).toHaveBeenCalledWith(results[2]);
  });

  it("calls onEscape when Escape is pressed", async () => {
    const onEscape = vi.fn();
    render(<SearchCommand query="a" onQueryChange={() => {}} results={results} onEscape={onEscape} />);

    screen.getByRole("combobox").focus();
    await userEvent.keyboard("{Escape}");
    expect(onEscape).toHaveBeenCalledOnce();
  });

  // ⚠ Escape must reach onEscape even with NO results. It is handled before the empty-list
  // guard in onKeyDown, because a picker showing nothing is exactly when a user wants out —
  // binding it after that guard makes the key work only when it is least needed.
  it("calls onEscape even when the result list is empty", async () => {
    const onEscape = vi.fn();
    render(<SearchCommand query="zzz" onQueryChange={() => {}} results={[]} onEscape={onEscape} />);

    screen.getByRole("combobox").focus();
    await userEvent.keyboard("{Escape}");
    expect(onEscape).toHaveBeenCalledOnce();
  });

  // ⚠ The ⌘K palette passes NO onEscape and binds Escape at the window level itself. If this
  // component ever bound the key unconditionally, the palette would close twice — which is the
  // whole reason the behaviour is opt-in rather than a default. This is the control case: it
  // fails if someone "fixes" the missing default.
  it("does not swallow Escape when no onEscape is supplied", async () => {
    const onWindowEscape = vi.fn();
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onWindowEscape();
    };
    window.addEventListener("keydown", handler);
    render(<SearchCommand query="a" onQueryChange={() => {}} results={results} />);

    screen.getByRole("combobox").focus();
    await userEvent.keyboard("{Escape}");
    window.removeEventListener("keydown", handler);
    expect(onWindowEscape).toHaveBeenCalledOnce();
  });
});
