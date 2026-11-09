// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import { normalizeYearRanges, type YearRange, YearRangeListField } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// This package has no DOM lib (it targets React Native too), so the slice of the jsdom document
// the test touches is typed here.
type Node_ = {
  click(): void;
  dispatchEvent(event: unknown): boolean;
  textContent: string | null;
  value: string;
};
type Doc = {
  body: {
    append(node: unknown): void;
    querySelector(s: string): Node_ | null;
    querySelectorAll(s: string): ArrayLike<Node_>;
    replaceChildren(): void;
    textContent: string | null;
  };
  createElement(tag: string): Parameters<typeof createRoot>[0];
};
const doc = (globalThis as unknown as { document: Doc }).document;
const win = globalThis as unknown as {
  Event: new (type: string, init?: { bubbles?: boolean }) => unknown;
  FocusEvent: new (type: string, init?: { bubbles?: boolean }) => unknown;
  HTMLInputElement: { prototype: object };
};

const mount = async (node: ReactNode) => {
  const container = doc.createElement("div");
  doc.body.append(container);
  const root = createRoot(container);
  await act(async () => {
    root.render(<LoomarrProvider theme="dark">{node}</LoomarrProvider>);
  });
};
afterEach(() => {
  doc.body.replaceChildren();
});

const named = (name: string) => {
  const found = doc.body.querySelector(`[aria-label="${name}"]`);
  if (!found) throw new Error(`nothing named: ${name}`);
  return found;
};

// React tracks an input's value, so a test sets it through the native setter before the event.
const typeInto = async (name: string, text: string) => {
  const input = named(name);
  const setter = Object.getOwnPropertyDescriptor(win.HTMLInputElement.prototype, "value")?.set;
  await act(async () => {
    setter?.call(input, text);
    input.dispatchEvent(new win.Event("input", { bubbles: true }));
  });
};
const blur = async (name: string) => {
  const input = named(name);
  await act(async () => {
    input.dispatchEvent(new win.FocusEvent("focusout", { bubbles: true }));
    input.dispatchEvent(new win.FocusEvent("blur"));
  });
};
const press = async (name: string) => {
  const button = named(name);
  await act(async () => button.click());
};
const alertText = () => doc.body.querySelector('[role="alert"]')?.textContent ?? undefined;

describe("normalizeYearRanges", () => {
  it("drops incomplete and out-of-bounds ranges, sorts, and merges touching ones", () => {
    expect(
      normalizeYearRanges([
        { from: 2000, to: 2005 },
        { from: 1990, to: 1999 },
        { from: 1800, to: 1850 },
        { from: 2010 },
        { from: 2004, to: 2008 },
        { from: 2020, to: 2010 },
      ]),
    ).toEqual([{ from: 1990, to: 2008 }]);
    expect(normalizeYearRanges(undefined)).toEqual([]);
  });
});

describe("YearRangeListField", () => {
  const ranges: YearRange[] = [
    { from: 1980, to: 1989 },
    { from: 1995, to: 1999 },
  ];

  it("names every box and button with the list and the row", async () => {
    await mount(<YearRangeListField label="Movie release" onChange={vi.fn()} ranges={ranges} />);

    expect(named("From year (Movie release, range 1)").value).toBe("1980");
    expect(named("To year (Movie release, range 2)").value).toBe("1999");
    expect(named("Remove (Movie release, range 2)").textContent).toBe("Remove");
    expect(named("Movie release new range from").value).toBe("");
    expect(named("Add range for Movie release").textContent).toBe("Add range");
  });

  it("waits for a valid pair before committing an edited row, then merges it", async () => {
    const onChange = vi.fn();
    await mount(<YearRangeListField label="Movie release" onChange={onChange} ranges={ranges} />);

    await typeInto("From year (Movie release, range 2)", "2005");
    await blur("From year (Movie release, range 2)");
    expect(alertText()).toContain("From no later than To");
    expect(onChange).not.toHaveBeenCalled();

    await typeInto("To year (Movie release, range 2)", "2009");
    await blur("To year (Movie release, range 2)");
    expect(onChange).toHaveBeenLastCalledWith([
      { from: 1980, to: 1989 },
      { from: 2005, to: 2009 },
    ]);
    expect(alertText()).toBeUndefined();
  });

  it("adds a range, and rejects an incomplete one", async () => {
    const onChange = vi.fn();
    await mount(<YearRangeListField label="Movie release" onChange={onChange} ranges={ranges} />);

    await typeInto("Movie release new range from", "1991");
    await press("Add range for Movie release");
    expect(alertText()).toContain("Enter whole years");
    expect(onChange).not.toHaveBeenCalled();

    await typeInto("Movie release new range to", "1992");
    await press("Add range for Movie release");
    expect(onChange).toHaveBeenLastCalledWith([
      { from: 1980, to: 1989 },
      { from: 1991, to: 1992 },
      { from: 1995, to: 1999 },
    ]);
    expect(named("Movie release new range from").value).toBe("");
  });

  it("removes the row it names", async () => {
    const onChange = vi.fn();
    await mount(<YearRangeListField label="Movie release" onChange={onChange} ranges={ranges} />);

    await press("Remove (Movie release, range 1)");
    expect(onChange).toHaveBeenLastCalledWith([{ from: 1995, to: 1999 }]);
  });

  it("stops offering a new row at the limit and says when the list is empty", async () => {
    await mount(<YearRangeListField label="Era" maxRanges={2} onChange={vi.fn()} ranges={ranges} />);
    expect(doc.body.querySelector('[aria-label="Era new range from"]')).toBeNull();

    doc.body.replaceChildren();
    await mount(
      <YearRangeListField emptyText="No date requirement." label="Era" onChange={vi.fn()} ranges={[]} />,
    );
    expect(doc.body.textContent).toContain("No date requirement.");
  });
});
