// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act, type ReactNode, useState } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import { NumberField } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// This package has no DOM lib (it targets React Native too), so the slice of the jsdom document
// the test touches is typed here.
type Input = { getAttribute(name: string): string | null; value: string };
type Doc = {
  body: { append(node: unknown): void; querySelector(s: string): Input | null; replaceChildren(): void };
  createElement(tag: string): Parameters<typeof createRoot>[0];
};
const doc = (globalThis as unknown as { document: Doc }).document;

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

const win = globalThis as unknown as {
  Event: new (type: string, init?: { bubbles?: boolean }) => unknown;
  FocusEvent: new (type: string, init?: { bubbles?: boolean }) => unknown;
  HTMLInputElement: { prototype: object };
};
type Typeable = Input & { dispatchEvent(event: unknown): boolean };
const field = () => {
  const found = doc.body.querySelector("input") as Typeable | null;
  if (!found) throw new Error("no input");
  return found;
};
// React tracks an input's value, so a test sets it through the native setter before the event.
const typeInto = async (text: string) => {
  const input = field();
  const setter = Object.getOwnPropertyDescriptor(win.HTMLInputElement.prototype, "value")?.set;
  await act(async () => {
    setter?.call(input, text);
    input.dispatchEvent(new win.Event("input", { bubbles: true }));
  });
};
const blur = async () => {
  const input = field();
  await act(async () => {
    input.dispatchEvent(new win.FocusEvent("focusout", { bubbles: true }));
    input.dispatchEvent(new win.FocusEvent("blur"));
  });
};

describe("NumberField", () => {
  // The parent owns the amount, so a harness holds it as the app does.
  const Harness = ({ onBlur, reexpress }: { onBlur?: () => void; reexpress?: (n: number) => number }) => {
    const [amount, setAmount] = useState(2);
    return (
      <NumberField
        amount={amount}
        label="Hours"
        onAmount={(next) => setAmount(reexpress ? reexpress(next) : next)}
        onBlur={onBlur}
      />
    );
  };

  it("keeps the text being typed, and reports the number, while it still reads as the amount", async () => {
    await mount(<Harness />);
    await typeInto("1.");
    expect(field().value).toBe("1.");
    await typeInto("1.5");
    expect(field().value).toBe("1.5");
  });

  it("goes back to the plain amount on blur, and tells the caller", async () => {
    const onBlur = vi.fn();
    await mount(<Harness onBlur={onBlur} />);
    await typeInto("1.");
    await blur();
    expect(field().value).toBe("1");
    expect(onBlur).toHaveBeenCalledTimes(1);
  });

  it("shows the amount the caller re-expressed, not the typed text", async () => {
    await mount(<Harness reexpress={(n) => n * 24} />);
    await typeInto("1.5");
    expect(field().value).toBe("36");
  });

  it("reports NaN for text that is not a number", async () => {
    const onAmount = vi.fn();
    await mount(<NumberField amount={2} label="Hours" onAmount={onAmount} />);
    await typeInto("abc");
    expect(onAmount).toHaveBeenLastCalledWith(Number.NaN);
    expect(field().value).toBe("2");
  });

  it("shows the amount as text in a box named by its label", async () => {
    await mount(<NumberField amount={1.5} label="Hours between checks" onAmount={() => undefined} />);

    const input = doc.body.querySelector("input");
    expect(input?.getAttribute("aria-label")).toBe("Hours between checks");
    expect(input?.value).toBe("1.5");
  });
});
