import { Dialog, DialogContent } from "@loomarr/design-system";
import { fireEvent, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { render } from "@/test/render";
import { SearchCommand } from "./search-command";
import type { SearchResult } from "./search-command.type";

const results: SearchResult[] = [
  { id: "c1", scope: "channels", name: "90s Action", meta: "#42" },
  { id: "l1", scope: "library", name: "The Velvet Cartographer", meta: "1995", inLibrary: true },
  { id: "t1", scope: "tmdb", name: "Pelican Point Motel", meta: "1997" },
];

// The active option is the one the input's aria-activedescendant points at: DOM focus stays in the
// input while the highlight moves, so the option itself never reports it.
const expectActive = (index: number) => {
  const option = screen.getAllByRole("option")[index];
  expect(option).toBeDefined();
  expect(screen.getByRole("combobox")).toHaveAttribute("aria-activedescendant", option?.id);
};

describe("SearchCommand", () => {
  it("groups results by scope and flags in-library items", () => {
    render(<SearchCommand query="a" onQueryChange={() => {}} results={results} onSelect={() => {}} />);
    expect(screen.getByText("Channels")).toBeInTheDocument();
    expect(screen.getByText("In your library")).toBeInTheDocument();
    expect(screen.getByText("In library")).toBeInTheDocument();
  });

  it("selects a result", () => {
    const onSelect = vi.fn();
    render(<SearchCommand query="velvet" onQueryChange={() => {}} results={results} onSelect={onSelect} />);
    fireEvent.click(screen.getByText("The Velvet Cartographer"));
    expect(onSelect).toHaveBeenCalledWith(results[1]);
  });

  it("tells two results with the same id in different scopes apart", () => {
    const onSelect = vi.fn();
    const twins: SearchResult[] = [
      { id: "7", scope: "channels", name: "Channel Seven" },
      { id: "7", scope: "library", name: "Title Seven" },
    ];
    render(<SearchCommand query="seven" onQueryChange={() => {}} results={twins} onSelect={onSelect} />);
    fireEvent.click(screen.getByText("Title Seven"));
    expect(onSelect).toHaveBeenCalledWith(twins[1]);
  });

  it("shows an empty state carrying the query", () => {
    render(<SearchCommand query="zzz" onQueryChange={() => {}} results={[]} />);
    expect(screen.getByText(/No matches for/)).toHaveTextContent("zzz");
  });

  it("prompts when idle and stays quiet while a search is running", () => {
    const idle = render(<SearchCommand query="" onQueryChange={() => {}} results={[]} />);
    expect(screen.getByText(/Type to search/)).toBeInTheDocument();
    idle.unmount();

    render(<SearchCommand query="zzz" onQueryChange={() => {}} results={[]} loading />);
    expect(screen.queryByText(/No matches for/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Searching/)).not.toBeInTheDocument();
  });

  // --- keyboard + listbox semantics (§12 drift claim `:757`) ---
  //
  // The palette was mouse-only: a keyboard user could type a query and had no way to choose a
  // result. Worth noting CI's axe sweep passed the entire time — missing arrow-key navigation is a
  // keyboard-INTERACTION defect, invisible to a static-markup scan. These tests are the coverage axe
  // cannot give; the design-system Combobox pins the same keys against its own fixtures.

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
    expectActive(0);
  });

  // The keyboard walks ONE flat sequence in render order, so ↓ crosses a scope heading rather
  // than stopping at it — three results in three different groups are still 1→2→3.
  it("moves the active option with ArrowDown/ArrowUp across group boundaries", async () => {
    render(<SearchCommand query="a" onQueryChange={() => {}} results={results} />);
    screen.getByRole("combobox").focus();

    await userEvent.keyboard("{ArrowDown}");
    expectActive(1);
    await userEvent.keyboard("{ArrowDown}");
    expectActive(2);
    await userEvent.keyboard("{ArrowUp}");
    expectActive(1);
  });

  it("keeps DOM focus in the input while the active option moves", async () => {
    render(<SearchCommand query="a" onQueryChange={() => {}} results={results} />);
    const input = screen.getByRole("combobox");
    input.focus();

    await userEvent.keyboard("{ArrowDown}{ArrowDown}");
    expect(document.activeElement).toBe(input);
  });

  it("orders groups by scope, not by the order results arrive in", async () => {
    const shuffled = [results[2], results[0], results[1]] as SearchResult[];
    render(<SearchCommand query="a" onQueryChange={() => {}} results={shuffled} />);
    screen.getByRole("combobox").focus();

    expect(screen.getAllByRole("option").map((option) => option.textContent)).toEqual([
      "90s Action#42",
      "The Velvet Cartographer1995In library",
      "Pelican Point Motel1997",
    ]);
    await userEvent.keyboard("{ArrowDown}{ArrowDown}");
    expectActive(2);
  });

  it("wraps at both ends", async () => {
    render(<SearchCommand query="a" onQueryChange={() => {}} results={results} />);
    screen.getByRole("combobox").focus();

    // ↑ from the first wraps to the last…
    await userEvent.keyboard("{ArrowUp}");
    expectActive(2);
    // …and ↓ from the last returns to the first.
    await userEvent.keyboard("{ArrowDown}");
    expectActive(0);
  });

  it("jumps to first/last with Home/End", async () => {
    render(<SearchCommand query="a" onQueryChange={() => {}} results={results} />);
    screen.getByRole("combobox").focus();

    await userEvent.keyboard("{End}");
    expectActive(2);
    await userEvent.keyboard("{Home}");
    expectActive(0);
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
    expectActive(2);

    rerender(
      <SearchCommand
        query="ab"
        onQueryChange={() => {}}
        results={[results[2] as SearchResult, results[0] as SearchResult]}
      />,
    );
    expectActive(0);
  });

  // Enter on an empty list must not throw or call back.
  it("ignores navigation keys with no results", async () => {
    const onSelect = vi.fn();
    render(<SearchCommand query="zzz" onQueryChange={() => {}} results={[]} onSelect={onSelect} />);
    const input = screen.getByRole("combobox");
    input.focus();

    await userEvent.keyboard("{ArrowDown}{Enter}{Home}{End}");
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

    await userEvent.hover(screen.getAllByRole("option")[2] as HTMLElement);
    expectActive(2);

    screen.getByRole("combobox").focus();
    await userEvent.keyboard("{Enter}");
    expect(onSelect).toHaveBeenCalledWith(results[2]);
  });

  it("swaps the ESC hint for a Close button when onClose is given", async () => {
    const onClose = vi.fn();
    const hint = render(<SearchCommand query="a" onQueryChange={() => {}} results={results} />);
    expect(screen.getByText("ESC")).toBeInTheDocument();
    hint.unmount();

    render(<SearchCommand query="a" onQueryChange={() => {}} results={results} onClose={onClose} />);
    expect(screen.queryByText("ESC")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Close search" }));
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("calls onEscape when Escape is pressed", async () => {
    const onEscape = vi.fn();
    render(<SearchCommand query="a" onQueryChange={() => {}} results={results} onEscape={onEscape} />);

    screen.getByRole("combobox").focus();
    await userEvent.keyboard("{Escape}");
    expect(onEscape).toHaveBeenCalledOnce();
  });

  // ⚠ Escape must reach onEscape even with NO results: a picker showing nothing is exactly when a
  // user wants out, so the key must not work only when it is least needed.
  it("calls onEscape even when the result list is empty", async () => {
    const onEscape = vi.fn();
    render(<SearchCommand query="zzz" onQueryChange={() => {}} results={[]} onEscape={onEscape} />);

    screen.getByRole("combobox").focus();
    await userEvent.keyboard("{Escape}");
    expect(onEscape).toHaveBeenCalledOnce();
  });

  // ⚠ The ⌘K palette passes NO onEscape and its Dialog owns the key. If this component ever bound
  // Escape unconditionally, the palette would close twice — which is the whole reason the behaviour
  // is opt-in rather than a default. This is the control case: it fails if someone "fixes" the
  // missing default.
  // The palette's real home: inside the design-system Dialog, which owns Escape. The inline list
  // must neither eat the key (the dialog would never close) nor lose typing and Enter to the dialog.
  it("lets the Dialog around it close on Escape, and still takes typing and Enter", async () => {
    const onOpenChange = vi.fn();
    const onSelect = vi.fn();
    // The caller owns the text, as the palette does.
    const Palette = () => {
      const [query, setQuery] = useState("a");
      return (
        <Dialog open onOpenChange={onOpenChange}>
          <DialogContent accessibilityLabel="Search Loomarr" placement="top">
            <SearchCommand query={query} onQueryChange={setQuery} results={results} onSelect={onSelect} />
          </DialogContent>
        </Dialog>
      );
    };
    render(<Palette />);
    const input = await screen.findByRole<HTMLInputElement>("combobox", { name: "Search" });
    input.focus();

    await userEvent.keyboard("b");
    expect(input.value).toBe("ab");
    await userEvent.keyboard("{ArrowDown}{Enter}");
    expect(onSelect).toHaveBeenCalledWith(results[1]);

    await userEvent.keyboard("{Escape}");
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

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
