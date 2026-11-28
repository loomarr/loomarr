// @vitest-environment jsdom

import { Action, CompactSelect } from "@loomarr/design-system";
import { act, cleanup, renderUi } from "@loomarr/test-support";
import { afterEach, describe, expect, it } from "vitest";

import { type LibraryNeed, LibraryNeedsCards } from "../index";

afterEach(cleanup);

// This package compiles without the DOM lib, so the test names the little it uses.
type El = {
  focus(): void;
  blur(): void;
  click(): void;
  getAttribute(name: string): string | null;
  style: {
    outline: string;
    outlineStyle: string;
    outlineColor: string;
    outlineWidth: string;
    borderColor: string;
    borderTopColor: string;
  };
};
type Host = { querySelector(selector: string): El | null };
const doc = (globalThis as unknown as { document: { activeElement: El | null } }).document;

const find = (host: unknown, selector: string) => {
  const hit = (host as Host).querySelector(selector);
  if (!hit) throw new Error(`missing: ${selector}`);
  return hit;
};

const AMBER = "rgb(255, 176, 32)";

describe("one focus ring", () => {
  it("shows an unfilled Action's amber border, with its outline there but unseen", () => {
    const { container } = renderUi(<Action tone="secondary">Cancel</Action>);
    const button = find(container, "button");
    act(() => button.focus());

    expect(button.style.borderTopColor).toBe(AMBER);
    // Forced colours repaint a transparent solid outline, so focus still shows there, and an
    // outline that is spoken for leaves the browser none of its own to add.
    expect(button.style.outlineStyle).toBe("solid");
    expect(button.style.outlineWidth).toBe("2px");
    expect(button.style.outlineColor).toBe("transparent");
  });

  it("gives a select the amber border on focus, and takes it away on blur", () => {
    const { container } = renderUi(
      <CompactSelect
        label="Order"
        onValueChange={() => {}}
        options={[{ label: "A", value: "a" }]}
        value="a"
      />,
    );
    const select = find(container, "select");
    act(() => select.focus());
    expect(select.style.borderColor).toBe("var(--actionFocus)");
    expect(select.style.outline).toBe("2px solid transparent");

    act(() => select.blur());
    expect(select.style.borderColor).toBe("var(--borderControl)");
  });
});

const need = (id: string): LibraryNeed => ({
  id,
  kind: "movie",
  folder: "Films",
  files: [`/media/Films/${id}.mkv`],
  lengthMillis: 0,
  bytes: 0,
  others: [],
});

const answered = () => Promise.resolve();

const cards = (restored?: Record<string, number>) => (
  <LibraryNeedsCards
    needs={[need("one"), need("two")]}
    onChooseOrder={answered}
    onIgnore={answered}
    onLink={answered}
    onOpenTitle={() => {}}
    onSearch={() => Promise.resolve([])}
    orders={[]}
    problems={[{ id: "bad", kind: "movie", name: "Low Tide", unplayable: 1, extraCopies: 0, extraBytes: 0 }]}
    restored={restored}
  />
);

describe("Needs you focus and names", () => {
  it("puts focus on the restored card after an Undo, not on the next one", async () => {
    const { container, rerender } = renderUi(cards());
    await act(async () => find(container, '[aria-label="Ignore this: one.mkv"]').click());
    expect(doc.activeElement?.getAttribute("aria-label")).toBe("Ignore this: two.mkv");

    // The app counts a restore once the server has it back.
    await act(async () => rerender(cards({ one: 1 })));
    expect(doc.activeElement?.getAttribute("aria-label")).toBe("Ignore this: one.mkv");
  });

  it("names See why for its title", () => {
    const { container } = renderUi(cards());
    expect(find(container, '[aria-label="See why: Low Tide"]')).toBeDefined();
  });
});
