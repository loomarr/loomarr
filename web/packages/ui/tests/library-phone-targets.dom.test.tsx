// @vitest-environment jsdom

import { semanticTargets } from "@loomarr/design-system";
import { act, cleanup, renderUi } from "@loomarr/test-support";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  LibraryCollections,
  type LibraryFolder,
  LibrarySettings,
  type LibrarySettingsProps,
  StatePanel,
} from "../index";

// The window the surface is drawn in, in points.
const screen = vi.hoisted(() => ({ width: 390 }));
vi.mock("react-native", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-native")>();
  return { ...actual, useWindowDimensions: () => ({ ...actual.useWindowDimensions(), width: screen.width }) };
});

type El = { click(): void; style: { minHeight: string }; textContent: string | null };
const doc = (globalThis as unknown as { document: { querySelectorAll(s: string): ArrayLike<El> } }).document;
const all = (selector: string) => Array.from(doc.querySelectorAll(selector));
const BUTTONS = 'button, [role="button"]';
const named = (name: string) => all(BUTTONS).filter((n) => n.textContent?.trim() === name);
const press = async (name: string) => {
  const [hit] = named(name);
  if (!hit) throw new Error(`missing button: ${name}`);
  await act(async () => hit.click());
};

// Every one of these buttons is on screen, and at least the touch target tall.
const TOUCH = `${semanticTargets.touch}px`;
const expectTouch = (...names: string[]) => {
  for (const name of names) {
    const heights = named(name).map((n) => n.style.minHeight);
    expect({ name, heights: [...new Set(heights)] }).toEqual({ name, heights: [TOUCH] });
  }
};

const mount = async (node: ReactNode) => {
  await act(async () => {
    renderUi(node);
  });
};
afterEach(() => {
  cleanup();
  screen.width = 390;
});

const TV: LibraryFolder = {
  path: "/media/tv",
  inUse: false,
  readable: true,
  addedByHand: false,
  foundIn: [{ authority: "sonarr", path: "/tv" }],
};
const MOVIES: LibraryFolder = { ...TV, path: "/media/movies", inUse: true, readable: false };

const settings: LibrarySettingsProps = {
  health: "folder_offline",
  folders: [TV, MOVIES],
  pathMap: [{ authority: "sonarr", authorityPath: "/tv", localPath: "/media/tv", byHand: false }],
  scan: { enabled: true, step: "idle", stepNumber: 0, done: 0, total: 0, paused: false, quiet: false },
  held: [],
  hooks: [],
  checking: false,
  onCheckAgain: vi.fn(),
  onRescan: vi.fn(),
  onRefreshEverything: vi.fn(),
  onConfirmRemoval: vi.fn(),
  onUseFolder: vi.fn(),
  onAddFolder: vi.fn(async () => undefined),
  onMapFolder: vi.fn(),
};

describe("Settings, Library on a phone", () => {
  it("gives every action, the folder fields and the folder names row a touch target", async () => {
    await mount(<LibrarySettings {...settings} />);
    expectTouch("Check again", "Check files again", "Update details", "Use this folder", "Stop using");
    expectTouch("Add a folder");
    const disclosure = all(BUTTONS).find((n) => n.textContent?.includes("different names"));
    expect(disclosure?.style.minHeight).toBe(TOUCH);
    await act(async () => disclosure?.click());
    expect(all("input").map((n) => n.style.minHeight)).toEqual([TOUCH]);
    await press("Add a folder");
    expectTouch("Add folder", "Cancel");
  });

  it("gives the questions it asks touch targets", async () => {
    await mount(<LibrarySettings {...settings} />);
    await press("Stop using");
    // The page's own button and the question's answer share the name.
    expect(named("Stop using")).toHaveLength(2);
    expectTouch("Stop using", "Cancel");
  });

  it("keeps the pointer's size from a tablet up", async () => {
    screen.width = 1440;
    await mount(<LibrarySettings {...settings} />);
    expect(named("Stop using")[0]?.style.minHeight).toBe(`${semanticTargets.pointer}px`);
    expect(named("Add a folder")[0]?.style.minHeight).toBe(`${semanticTargets.pointer}px`);
  });
});

describe("Collections on a phone", () => {
  const collections = [{ id: "c1", name: "Rainy Days", titles: 2, pile: [], usedBy: [] }];
  const manage = { onNew: vi.fn(), onEdit: vi.fn(), onCopy: vi.fn(async () => {}) };
  const sets = [{ id: "90000611", name: "The Lantern Keeper Collection", have: 2, total: 3, pile: [] }];

  it("gives Edit, New collection and Make my own copy a touch target", async () => {
    await mount(<LibraryCollections collections={collections} manage={manage} sets={sets} />);
    // Edit is named for its collection to a screen reader; its words are Edit.
    expectTouch("Edit", "New collection", "Make my own copy");
  });

  it("gives New collection a touch target when none is made", async () => {
    await mount(<LibraryCollections collections={[]} manage={manage} sets={[]} />);
    expectTouch("New collection");
  });
});

describe("StatePanel", () => {
  const panel = (density?: "pointer") => (
    <StatePanel
      action={{ label: "Show everything", onPress: vi.fn() }}
      density={density}
      kind="empty"
      title="No movies yet"
    />
  );

  it("gives its action a touch target below a tablet's width, and the pointer's from there up", async () => {
    await mount(panel());
    expectTouch("Show everything");
    cleanup();
    screen.width = 768;
    await mount(panel());
    expect(named("Show everything")[0]?.style.minHeight).toBe(`${semanticTargets.pointer}px`);
  });

  it("keeps the density it is asked for", async () => {
    await mount(panel("pointer"));
    expect(named("Show everything")[0]?.style.minHeight).toBe(`${semanticTargets.pointer}px`);
  });
});
