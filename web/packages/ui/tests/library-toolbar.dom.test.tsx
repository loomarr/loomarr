// @vitest-environment jsdom

import { act, cleanup, renderUi } from "@loomarr/test-support";
import { afterEach, describe, expect, it, vi } from "vitest";

import { LibraryShelf, type LibraryShelfProps } from "../index";

const screen = vi.hoisted(() => ({ width: 1440 }));
vi.mock("react-native", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-native")>();
  return { ...actual, useWindowDimensions: () => ({ ...actual.useWindowDimensions(), width: screen.width }) };
});

afterEach(cleanup);

const shelf = (overrides: Partial<LibraryShelfProps> = {}): LibraryShelfProps => ({
  filter: "all",
  onFilterChange: vi.fn(),
  counts: { all: 1284, movie: 900, series: 384 },
  sort: "name",
  onSortChange: vi.fn(),
  genres: [{ name: "Drama", titles: 12 }],
  onGenreChange: vi.fn(),
  search: "",
  onSearchChange: vi.fn(),
  titles: [],
  loading: true,
  onOpenTitle: vi.fn(),
  ...overrides,
});

const button = (name: string) => {
  const found = [...document.querySelectorAll<HTMLElement>("button, [role=button]")].find(
    (node) => (node.getAttribute("aria-label") ?? node.textContent) === name,
  );
  if (!found) throw new Error(`no button named ${name}`);
  return found;
};

const choose = async (phrase: string, item: string) => {
  await act(async () => button(phrase).click());
  const found = [...document.querySelectorAll<HTMLElement>("[role=menuitem]")].find(
    (node) => node.textContent === item,
  );
  if (!found) throw new Error(`no menu item ${item}`);
  await act(async () => found.click());
};

describe("the Library toolbar from a tablet up", () => {
  it("draws the four words as tabs, without counts, and underlines the current one", () => {
    const props = shelf();
    renderUi(<LibraryShelf {...props} />);
    const tabs = [...document.querySelectorAll<HTMLElement>("[role=tab]")];
    expect(tabs.map((tab) => tab.textContent)).toEqual(["All", "Movies", "Shows", "Collections"]);
    expect(tabs.map((tab) => tab.getAttribute("aria-selected"))).toEqual(["true", "false", "false", "false"]);
    act(() => tabs[2]?.click());
    expect(props.onFilterChange).toHaveBeenCalledWith("series");
  });

  it("reads count, order and genre in order, after the tabs and the search", () => {
    renderUi(<LibraryShelf {...shelf()} />);
    expect(document.body.textContent).toMatch(/1,284 titles·A to Z.*·Every genre/);
    const stops = [
      ...document.querySelectorAll<HTMLElement>('[role=tab][tabindex="0"], input, button[aria-label]'),
    ].map((node) => node.getAttribute("aria-label") ?? node.textContent);
    expect(stops).toEqual(["All", "Search titles", "A to Z, order", "Every genre, genre"]);
    // The count is words, not a control.
    expect(() => button("1,284 titles")).toThrow();
  });

  it("opens each phrase's menu and applies the choice", async () => {
    const props = shelf();
    renderUi(<LibraryShelf {...props} />);
    await choose("A to Z, order", "Recently added");
    expect(props.onSortChange).toHaveBeenCalledWith("recent");
    await choose("Every genre, genre", "Drama · 12");
    expect(props.onGenreChange).toHaveBeenCalledWith("Drama");
  });

  it("ends the sentence on More filters, which opens the panel and keeps its place in the order", async () => {
    const filtering = {
      filters: {},
      onChange: vi.fn(),
      onClear: vi.fn(),
      personSearch: { value: "", onChange: vi.fn(), options: [], loading: false, failed: false },
      total: 1284,
    } as unknown as NonNullable<LibraryShelfProps["filtering"]>;
    renderUi(<LibraryShelf {...shelf({ filtering })} />);
    const more = button("More filters");
    expect(more.getAttribute("aria-expanded")).toBe("false");
    const phrases = [button("A to Z, order"), button("Every genre, genre"), more];
    expect(phrases[1]?.compareDocumentPosition(more)).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
    await act(async () => more.click());
    expect(button("More filters").getAttribute("aria-expanded")).toBe("true");
  });

  it("underlines only the current word", () => {
    renderUi(<LibraryShelf {...shelf({ filter: "movie" })} />);
    const [all, movies, shows, collections] = [...document.querySelectorAll("[role=tab]")].map((tab) =>
      (tab.firstElementChild?.firstElementChild?.className ?? "").split(" "),
    );
    expect(shows).toEqual(all);
    expect(collections).toEqual(all);
    const own = movies?.filter((name) => !all?.includes(name));
    // One class apart, the border's: the rule is the only box difference between the words.
    expect(own).toHaveLength(1);
    expect(own?.[0]).toMatch(/^_b/);
  });

  it("counts what the choices leave, and goes back to every genre", async () => {
    const props = shelf({ genre: "drama" });
    renderUi(<LibraryShelf {...props} />);
    expect(document.body.textContent).toContain("12 titles·A to Z");
    await choose("Drama, genre", "Every genre");
    expect(props.onGenreChange).toHaveBeenCalledWith(undefined);
    cleanup();
    renderUi(<LibraryShelf {...shelf({ filter: "movie" })} />);
    expect(document.body.textContent).toContain("900 titles");
  });
});
