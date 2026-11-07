// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import { DemoSignIn } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// This package has no DOM lib (it targets React Native too), so the slice of the jsdom document
// the test touches is typed here.
type Node = {
  click(): void;
  getAttribute(name: string): string | null;
  textContent: string | null;
};
type Doc = {
  body: {
    append(node: unknown): void;
    replaceChildren(): void;
    textContent: string | null;
    querySelector(s: string): Node | null;
    querySelectorAll(s: string): ArrayLike<Node>;
  };
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

const button = (name: string) => {
  const found = Array.from(doc.body.querySelectorAll('[role="button"]')).find(
    (node) => node.textContent?.trim() === name,
  );
  if (!found) throw new Error(`missing button: ${name}`);
  return found;
};

describe("DemoSignIn", () => {
  it("says it's the demo and signs in as the server's account in one press", async () => {
    const onSignIn = vi.fn();
    await mount(<DemoSignIn onSignIn={onSignIn} username="guest" />);

    expect(doc.body.querySelector('[role="region"]')?.getAttribute("aria-label")).toBe("Loomarr demo");
    expect(doc.body.textContent).toContain("This is the Loomarr demo");
    await act(async () => button("Sign in as guest").click());

    expect(onSignIn).toHaveBeenCalledOnce();
  });

  it("holds the button while signing in", async () => {
    const onSignIn = vi.fn();
    await mount(<DemoSignIn busy onSignIn={onSignIn} username="guest" />);
    const pending = button("Signing in…");

    await act(async () => pending.click());

    expect(pending.getAttribute("aria-disabled")).toBe("true");
    expect(onSignIn).not.toHaveBeenCalled();
  });
});
