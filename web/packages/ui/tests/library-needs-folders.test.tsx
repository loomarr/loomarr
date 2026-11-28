// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it } from "vitest";

import { type LibraryNeed, LibraryNeedsCards } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const need = (id: string, folder: string, files = [`/media/${folder}/${id}.mkv`]): LibraryNeed => ({
  id,
  kind: "movie",
  folder,
  files,
  others: [],
});

const answered = () => Promise.resolve();

// This package compiles without the DOM lib, so the test names the little it uses.
type El = {
  tagName: string;
  textContent: string | null;
  click(): void;
  getAttribute(name: string): string | null;
};
type Host = Parameters<typeof createRoot>[0] & {
  textContent: string | null;
  querySelector(selector: string): El | null;
  querySelectorAll(selector: string): ArrayLike<El>;
  remove(): void;
};
const doc = (
  globalThis as unknown as {
    document: {
      activeElement: El | null;
      body: { append(n: unknown): void };
      createElement(t: string): Host;
    };
  }
).document;

type Rest = Partial<Pick<Parameters<typeof LibraryNeedsCards>[0], "orders" | "problems">>;

const mount = (needs: LibraryNeed[], rest: Rest = {}) => {
  const host = doc.createElement("div");
  doc.body.append(host);
  const root = createRoot(host);
  act(() =>
    root.render(
      <LoomarrProvider theme="dark">
        <LibraryNeedsCards
          needs={needs}
          onChooseOrder={answered}
          onIgnore={answered}
          onLink={answered}
          onOpenTitle={() => {}}
          onSearch={() => Promise.resolve([])}
          orders={rest.orders ?? []}
          problems={rest.problems ?? []}
        />
      </LoomarrProvider>,
    ),
  );
  const cleanup = () => {
    act(() => root.unmount());
    host.remove();
  };
  return { host, cleanup };
};

describe("Needs you by folder", () => {
  it("heads the order questions and the titles to look at, after the folders", () => {
    const { host, cleanup } = mount([need("one", "Films")], {
      orders: [{ id: "show", name: "Kestrel Bay", disk: [], files: [], choices: [] }],
      problems: [
        { id: "bad", kind: "movie", name: "Low Tide", unplayable: 1, extraCopies: 0, extraBytes: 0 },
      ],
    });
    const groups = Array.from(host.querySelectorAll('[role="region"] [role="region"]'));
    expect(groups.map((g) => g.getAttribute("aria-label"))).toEqual([
      "Folder Films",
      "Episode order questions",
      "Needs a look",
    ]);
    cleanup();
  });

  it("puts each folder's cards together under its heading", () => {
    const { host, cleanup } = mount([need("one", "Films"), need("two", "Tapes"), need("three", "Films")]);
    const sections = Array.from(host.querySelectorAll('[role="region"][aria-label^="Folder "]'));
    expect(sections.map((s) => s.getAttribute("aria-label"))).toEqual(["Folder Films", "Folder Tapes"]);
    expect(sections[0]?.textContent).toContain("one.mkv");
    expect(sections[0]?.textContent).toContain("three.mkv");
    expect(sections[0]?.textContent).not.toContain("two.mkv");
    cleanup();
  });

  it("leads a card with its file name and says how many more it has", () => {
    const { host, cleanup } = mount([need("disc", "Films", ["/media/Films/a.mkv", "/media/Films/b.mkv"])]);
    expect(host.textContent).toContain("a.mkv");
    expect(host.textContent).toContain("With 1 more file");
    cleanup();
  });

  it("moves focus into Find it's field and gives it back on Cancel", () => {
    const { host, cleanup } = mount([need("one", "Films")]);
    const button = (name: string) => {
      const hit = Array.from(host.querySelectorAll('[role="button"], button')).find(
        (n) => n.textContent?.trim() === name,
      );
      if (!hit) throw new Error(`missing button: ${name}`);
      return hit;
    };
    act(() => button("Find it").click());
    expect(doc.activeElement?.tagName).toBe("INPUT");
    act(() => button("Cancel").click());
    expect(host.querySelector("input")).toBeNull();
    expect(doc.activeElement).toBe(button("Find it"));
    cleanup();
  });

  it("closes Find it on Escape", () => {
    const { host, cleanup } = mount([need("one", "Films")]);
    const find = Array.from(host.querySelectorAll('[role="button"], button')).find(
      (n) => n.textContent?.trim() === "Find it",
    );
    act(() => find?.click());
    const { KeyboardEvent } = globalThis as unknown as {
      KeyboardEvent: new (type: string, init: { key: string; bubbles: boolean }) => object;
    };
    const input = host.querySelector("input") as unknown as { dispatchEvent(event: object): void };
    act(() => input.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true })));
    expect(host.querySelector("input")).toBeNull();
    cleanup();
  });

  it("names a file nothing was guessed for once", () => {
    const { host, cleanup } = mount([need("one", "Films")]);
    expect(host.textContent?.split("one.mkv")).toHaveLength(2);
    expect(host.textContent).not.toMatch(/one(?!\.mkv)/);
    cleanup();
  });
});
