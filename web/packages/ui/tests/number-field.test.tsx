// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it } from "vitest";

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

describe("NumberField", () => {
  it("shows the amount as text in a box named by its label", async () => {
    await mount(<NumberField amount={1.5} label="Hours between checks" onAmount={() => undefined} />);

    const input = doc.body.querySelector("input");
    expect(input?.getAttribute("aria-label")).toBe("Hours between checks");
    expect(input?.value).toBe("1.5");
  });
});
