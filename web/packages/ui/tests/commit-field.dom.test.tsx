// @vitest-environment jsdom

import { YearField } from "@loomarr/design-system/fields";
import { act, cleanup, renderUi } from "@loomarr/test-support";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { CommitField } from "../index";

const mount = async (node: ReactNode) => {
  await act(async () => {
    renderUi(node);
  });
};
afterEach(cleanup);

// React tracks an input's value, so a test sets it through the native setter before the event.
const typeInto = async (input: HTMLInputElement, text: string) => {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
  await act(async () => {
    setter?.call(input, text);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
};

describe("CommitField", () => {
  it("hands the draft over on blur, not on each keystroke", async () => {
    const commits: string[] = [];
    await mount(<CommitField initial="4h" label="Repeat spacing" onCommit={(text) => commits.push(text)} />);

    const input = document.body.querySelector("input");
    if (!input) throw new Error("no input");
    expect(input.value).toBe("4h");
    await typeInto(input, "6h");
    expect(commits).toEqual([]);
    await act(async () => {
      input.dispatchEvent(new FocusEvent("focusout", { bubbles: true }));
      input.dispatchEvent(new FocusEvent("blur"));
    });
    expect(commits).toEqual(["6h"]);
  });

  it("hands the text over on every blur, even when nothing was typed", async () => {
    const commits: string[] = [];
    await mount(<CommitField initial="4h" label="Repeat spacing" onCommit={(text) => commits.push(text)} />);

    const input = document.body.querySelector("input");
    if (!input) throw new Error("no input");
    await act(async () => {
      input.dispatchEvent(new FocusEvent("focusout", { bubbles: true }));
      input.dispatchEvent(new FocusEvent("blur"));
    });
    expect(commits).toEqual(["4h"]);
  });

  it("holds back a draft its check refuses, saying why until the next edit", async () => {
    const commits: string[] = [];
    await mount(
      <CommitField
        initial=""
        label="Max from one series"
        onCommit={(text) => commits.push(text)}
        validate={(text) => (/^\d*$/.test(text) ? undefined : "Enter a whole number")}
      />,
    );
    const input = document.body.querySelector("input");
    if (!input) throw new Error("no input");
    const blur = () =>
      act(async () => {
        input.dispatchEvent(new FocusEvent("focusout", { bubbles: true }));
        input.dispatchEvent(new FocusEvent("blur"));
      });

    await typeInto(input, "three");
    await blur();
    expect(commits).toEqual([]);
    expect(input.getAttribute("aria-invalid")).toBe("true");
    expect(document.body.querySelector("input")?.getAttribute("aria-describedby")).toMatch(/error/);

    await typeInto(input, "3");
    expect(input.getAttribute("aria-invalid")).toBeNull();
    await blur();
    expect(commits).toEqual(["3"]);
  });

  it("keeps the draft when the stored text changes, until the box is keyed afresh", async () => {
    const box = (initial: string, key: string) => (
      <CommitField initial={initial} key={key} label="Repeat spacing" onCommit={() => undefined} />
    );
    const { rerender } = renderUi(box("4h", "a"));
    const input = () => {
      const found = document.body.querySelector("input");
      if (!found) throw new Error("no input");
      return found;
    };
    await typeInto(input(), "6h");
    rerender(box("8h", "a"));
    expect(input().value).toBe("6h");
    rerender(box("8h", "b"));
    expect(input().value).toBe("8h");
  });
});

describe("YearField", () => {
  it("is a box named by its own accessible name", async () => {
    await mount(
      <YearField
        accessibilityLabel="From year (Movie release, range 1)"
        label="From year"
        onChangeText={() => undefined}
        value="1990"
      />,
    );

    const input = document.body.querySelector("input");
    expect(input?.getAttribute("aria-label")).toBe("From year (Movie release, range 1)");
    expect(input?.value).toBe("1990");
  });
});
