// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it } from "vitest";

import { CommitField, YearField } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// This package has no DOM lib (it targets React Native too), so the slice of the jsdom document
// the test touches is typed here.
type Input = {
  dispatchEvent(event: unknown): boolean;
  focus(): void;
  getAttribute(name: string): string | null;
  value: string;
};
type Doc = {
  body: { append(node: unknown): void; querySelector(s: string): Input | null; replaceChildren(): void };
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

// React tracks an input's value, so a test sets it through the native setter before the event.
const typeInto = async (input: Input, text: string) => {
  const setter = Object.getOwnPropertyDescriptor(win.HTMLInputElement.prototype, "value")?.set;
  await act(async () => {
    setter?.call(input, text);
    input.dispatchEvent(new win.Event("input", { bubbles: true }));
  });
};

describe("CommitField", () => {
  it("hands the draft over on blur, not on each keystroke", async () => {
    const commits: string[] = [];
    await mount(<CommitField initial="4h" label="Repeat spacing" onCommit={(text) => commits.push(text)} />);

    const input = doc.body.querySelector("input");
    if (!input) throw new Error("no input");
    expect(input.value).toBe("4h");
    await typeInto(input, "6h");
    expect(commits).toEqual([]);
    await act(async () => {
      input.dispatchEvent(new win.FocusEvent("focusout", { bubbles: true }));
      input.dispatchEvent(new win.FocusEvent("blur"));
    });
    expect(commits).toEqual(["6h"]);
  });

  it("hands the text over on every blur, even when nothing was typed", async () => {
    const commits: string[] = [];
    await mount(<CommitField initial="4h" label="Repeat spacing" onCommit={(text) => commits.push(text)} />);

    const input = doc.body.querySelector("input");
    if (!input) throw new Error("no input");
    await act(async () => {
      input.dispatchEvent(new win.FocusEvent("focusout", { bubbles: true }));
      input.dispatchEvent(new win.FocusEvent("blur"));
    });
    expect(commits).toEqual(["4h"]);
  });

  it("keeps the draft when the stored text changes, until the box is keyed afresh", async () => {
    const container = doc.createElement("div");
    doc.body.append(container);
    const root = createRoot(container);
    const render = (initial: string, key: string) =>
      act(async () => {
        root.render(
          <LoomarrProvider theme="dark">
            <CommitField initial={initial} key={key} label="Repeat spacing" onCommit={() => undefined} />
          </LoomarrProvider>,
        );
      });
    await render("4h", "a");
    const input = () => {
      const found = doc.body.querySelector("input");
      if (!found) throw new Error("no input");
      return found;
    };
    await typeInto(input(), "6h");
    await render("8h", "a");
    expect(input().value).toBe("6h");
    await render("8h", "b");
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

    const input = doc.body.querySelector("input");
    expect(input?.getAttribute("aria-label")).toBe("From year (Movie release, range 1)");
    expect(input?.value).toBe("1990");
  });
});
