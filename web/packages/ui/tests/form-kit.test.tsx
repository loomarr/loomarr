// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import { RequestChannel } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// This package has no DOM lib (it targets React Native too), so the slice of the jsdom document
// the test touches is typed here.
type Node = {
  click(): void;
  dispatchEvent(event: unknown): boolean;
  textContent: string | null;
  value: string;
};
type Doc = {
  body: {
    append(node: unknown): void;
    querySelector(s: string): Node | null;
    querySelectorAll(s: string): ArrayLike<Node>;
    replaceChildren(): void;
    textContent: string | null;
  };
  createElement(tag: string): Parameters<typeof createRoot>[0];
};
const doc = (globalThis as unknown as { document: Doc }).document;
const win = globalThis as unknown as {
  Event: new (type: string, init?: { bubbles?: boolean }) => unknown;
  HTMLTextAreaElement: { prototype: object };
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

const typeInto = async (input: Node, text: string) => {
  const setter = Object.getOwnPropertyDescriptor(win.HTMLTextAreaElement.prototype, "value")?.set;
  await act(async () => {
    setter?.call(input, text);
    input.dispatchEvent(new win.Event("input", { bubbles: true }));
  });
};
const press = async (name: string) => {
  const found = Array.from(doc.body.querySelectorAll('[role="button"]')).find(
    (node) => node.textContent?.trim() === name,
  );
  if (!found) throw new Error(`missing button: ${name}`);
  await act(async () => {
    found.click();
  });
};

const request = (onSubmitBrief: () => void) => (
  <RequestChannel
    brief={{ kind: "idle" }}
    ideas={[]}
    ideasStatus="ready"
    nowMs={0}
    onBack={vi.fn()}
    onHideIdea={vi.fn()}
    onRequestIdea={vi.fn()}
    onRetryIdeas={vi.fn()}
    onSubmitBrief={onSubmitBrief}
    onUndoHide={vi.fn()}
    viewer="member"
  />
);

describe("a shared form (useUiForm)", () => {
  it("says what the schema refuses on submit, then lets the viewer fix it and send", async () => {
    const onSubmitBrief = vi.fn();
    await mount(request(onSubmitBrief));

    await press("Suggest titles");
    expect(onSubmitBrief).not.toHaveBeenCalled();
    expect(doc.body.textContent).toContain("Describe the channel you want (a sentence is plenty).");

    const input = doc.body.querySelector("textarea");
    if (!input) throw new Error("no textarea");
    await typeInto(input, "Saturday-morning cartoons, bright and silly");
    expect(doc.body.textContent).not.toContain("a sentence is plenty");
    await press("Suggest titles");
    expect(onSubmitBrief).toHaveBeenCalledTimes(1);
  });
});
