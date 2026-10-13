// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AiReconnectBanner } from "../index";

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

const text = () => doc.body.textContent ?? "";
const button = (name: string) => {
  const found = Array.from(doc.body.querySelectorAll('[role="button"]')).find(
    (node) => node.textContent?.trim() === name,
  );
  if (!found) throw new Error(`missing button: ${name}`);
  return found;
};

describe("AiReconnectBanner", () => {
  it("says the mock's copy, with the picks counted, as a polite status", async () => {
    await mount(<AiReconnectBanner pickCount={1} onKeepManual={vi.fn()} onTryAi={vi.fn()} />);

    expect(doc.body.querySelector('[role="status"]')?.getAttribute("aria-live")).toBe("polite");
    expect(text()).toContain("AI is connected now");
    expect(text()).toContain(
      "Your manual lineup (1 title) is still here. Keep building it by hand, or let Loomarr suggest titles for this description — your picks stay in the lineup either way.",
    );
  });

  it("offers both actions", async () => {
    const onTryAi = vi.fn();
    const onKeepManual = vi.fn();
    await mount(<AiReconnectBanner pickCount={3} onKeepManual={onKeepManual} onTryAi={onTryAi} />);

    await act(async () => button("Try AI suggestions").click());
    await act(async () => button("Keep building manually").click());

    expect(onTryAi).toHaveBeenCalledOnce();
    expect(onKeepManual).toHaveBeenCalledOnce();
  });

  it("holds Try AI suggestions and says why when it can't run yet", async () => {
    const onTryAi = vi.fn();
    await mount(
      <AiReconnectBanner
        pickCount={2}
        onKeepManual={vi.fn()}
        onTryAi={onTryAi}
        tryAiBlockedReason="Name this channel to try AI suggestions."
      />,
    );
    const tryAi = button("Try AI suggestions");

    await act(async () => tryAi.click());

    expect(tryAi.getAttribute("aria-disabled")).toBe("true");
    expect(onTryAi).not.toHaveBeenCalled();
    expect(text()).toContain("Name this channel to try AI suggestions.");
  });
});
