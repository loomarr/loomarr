// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

import { LibraryCollections, type LibraryCollectionsProps } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

type El = {
  click(): void;
  getAttribute(n: string): string | null;
  hasAttribute(n: string): boolean;
  textContent: string | null;
};
type Host = Parameters<typeof createRoot>[0] & {
  querySelector(s: string): El | null;
  querySelectorAll(s: string): ArrayLike<El>;
  remove(): void;
  textContent: string | null;
};
const doc = (
  globalThis as unknown as {
    document: { body: { append(n: unknown): void }; createElement(t: string): Host };
  }
).document;

const render = (overrides: Partial<LibraryCollectionsProps>) => {
  const props: LibraryCollectionsProps = {
    collections: [
      {
        id: "c1",
        name: "Rainy Sundays",
        titles: 12,
        pile: [{ name: "Harbour Lights" }, { name: "Paper Kites" }],
        importedFrom: "Emby",
        usedBy: [{ name: "Saturday Morning", number: 4 }],
      },
      { id: "c2", name: "Just One", titles: 1, pile: [{ name: "Night Ferry" }], usedBy: [] },
    ],
    sets: [{ id: "90000611", name: "The Lantern Keeper Collection", have: 2, total: 3, pile: [] }],
    onNew: vi.fn(),
    onEdit: vi.fn(),
    onCopy: vi.fn(() => Promise.resolve()),
    ...overrides,
  };
  const host = doc.createElement("div");
  doc.body.append(host);
  const root = createRoot(host);
  act(() =>
    root.render(
      <LoomarrProvider theme="dark">
        <LibraryCollections {...props} />
      </LoomarrProvider>,
    ),
  );
  const button = (name: string) => {
    const hit = Array.from(host.querySelectorAll('[role="button"], button')).find(
      (n) => n.textContent?.trim() === name || n.getAttribute("aria-label") === name,
    );
    if (!hit) throw new Error(`missing button: ${name}`);
    return hit;
  };
  const cleanup = () => {
    act(() => root.unmount());
    host.remove();
  };
  return { host, props, button, cleanup };
};

const settle = () => act(() => Promise.resolve());

describe("LibraryCollections", () => {
  it("says what each collection holds, where it came from and which channel uses it", () => {
    const { host, cleanup } = render({});
    expect(host.textContent).toContain("12 titles");
    expect(host.textContent).toContain("1 title");
    expect(host.textContent).toContain("Brought over from Emby");
    expect(host.textContent).toContain("On Saturday Morning (channel 4)");
    cleanup();
  });

  it("opens a collection to edit, and starts a new one", () => {
    const { props, button, cleanup } = render({});
    act(() => button("Edit Rainy Sundays").click());
    expect(props.onEdit).toHaveBeenCalledWith("c1");
    act(() => button("New collection").click());
    expect(props.onNew).toHaveBeenCalled();
    cleanup();
  });

  it("says how much of a ready-made set the library has, and copies it", async () => {
    const { host, props, button, cleanup } = render({});
    expect(host.textContent).toContain("From TMDB — you have 2 of 3");
    act(() => button("Make my own copy").click());
    await settle();
    expect(props.onCopy).toHaveBeenCalledWith("90000611");
    expect(
      button("Copied — it's under Yours").hasAttribute("disabled") ||
        button("Copied — it's under Yours").getAttribute("aria-disabled") === "true",
    ).toBe(true);
    cleanup();
  });

  it("says when a copy didn't work, and lets it be tried again", async () => {
    const { host, button, cleanup } = render({ onCopy: vi.fn(() => Promise.reject(new Error("no"))) });
    act(() => button("Make my own copy").click());
    await settle();
    expect(host.textContent).toContain("Couldn't make a copy just now. Try again in a moment.");
    expect(button("Make my own copy").getAttribute("aria-disabled")).not.toBe("true");
    cleanup();
  });

  it("leaves out the sets when the library has a movie of none", () => {
    const { host, cleanup } = render({ sets: [] });
    expect(host.textContent).not.toContain("Ready-made sets");
    cleanup();
  });
});
