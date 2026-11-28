// @vitest-environment jsdom

import { act, cleanup, renderUi } from "@loomarr/test-support";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { normalizeYearRanges, type YearRange, YearRangeListField } from "../index";

const doc = document;
const win = window;

const mount = async (node: ReactNode) => {
  await act(async () => {
    renderUi(node);
  });
};
afterEach(cleanup);

const named = (name: string) => {
  const found = doc.body.querySelector<HTMLInputElement>(`[aria-label="${name}"]`);
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

  it("keeps an invalid endpoint on screen, then reseeds the row when the ranges are replaced", async () => {
    const view = renderUi(null);
    const render = (next: YearRange[]) =>
      act(async () => {
        view.rerender(<YearRangeListField label="Movie release" onChange={vi.fn()} ranges={next} />);
      });
    await render(ranges);
    await typeInto("From year (Movie release, range 1)", "2050");
    await blur("From year (Movie release, range 1)");
    expect(named("From year (Movie release, range 1)").value).toBe("2050");
    expect(named("To year (Movie release, range 1)").value).toBe("1989");

    await render([{ from: 1970, to: 1979 }]);
    expect(named("From year (Movie release, range 1)").value).toBe("1970");
    expect(named("To year (Movie release, range 1)").value).toBe("1979");
  });

  it("clears the message once an edited row commits", async () => {
    const onChange = vi.fn();
    await mount(<YearRangeListField label="Movie release" onChange={onChange} ranges={ranges} />);

    await typeInto("To year (Movie release, range 1)", "1970");
    await blur("To year (Movie release, range 1)");
    expect(alertText()).toContain("Enter whole years");
    await typeInto("To year (Movie release, range 1)", "1985");
    await blur("To year (Movie release, range 1)");
    expect(alertText()).toBeUndefined();
    expect(onChange).toHaveBeenLastCalledWith([
      { from: 1980, to: 1985 },
      { from: 1995, to: 1999 },
    ]);
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
