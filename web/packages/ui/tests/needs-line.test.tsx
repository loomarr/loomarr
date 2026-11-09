// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it } from "vitest";

import { NeedsLine, type NeedsLineItem } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// This package has no DOM lib (it targets React Native too), so the slice of the jsdom document
// the test touches is typed here.
type Node = {
  getAttribute(name: string): string | null;
  querySelector(s: string): Node | null;
  querySelectorAll(s: string): ArrayLike<Node>;
  textContent: string | null;
};
type Doc = {
  body: Node & { append(node: unknown): void; replaceChildren(): void };
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

const items: NeedsLineItem[] = [
  {
    id: "approvals",
    value: "3",
    label: "requests to approve",
    short: "to approve",
    link: <a href="/requests/needs-you">x</a>,
  },
  {
    id: "quota",
    value: "91%",
    label: "of filler space used",
    short: "filler space",
    warn: true,
    link: <a href="/filler/settings">x</a>,
  },
];

const links = () => Array.from(doc.body.querySelectorAll("a"));

describe("NeedsLine", () => {
  it("is one navigation line of counts, each a link to its page", async () => {
    await mount(<NeedsLine items={items} />);

    const line = doc.body.querySelector('[role="navigation"]');
    expect(line?.getAttribute("aria-label")).toBe("Needs you");
    expect(links().map((a) => [a.textContent, a.getAttribute("href")])).toEqual([
      ["3 requests to approve", "/requests/needs-you"],
      ["91% of filler space used", "/filler/settings"],
    ]);
  });

  it("marks filler space running short with a named dot", async () => {
    await mount(<NeedsLine items={items} />);

    const quota = links()[1];
    expect(quota?.querySelector('[aria-label="Running short"]')).not.toBeNull();
    expect(links()[0]?.querySelector('[aria-label="Running short"]')).toBeNull();
  });

  it("uses the short words on a phone", async () => {
    await mount(<NeedsLine compact items={items} />);

    expect(links().map((a) => a.textContent)).toEqual(["3 to approve", "91% filler space"]);
  });

  it("renders nothing when nothing needs a look", async () => {
    await mount(<NeedsLine items={[]} />);

    expect(doc.body.querySelector('[role="navigation"]')).toBeNull();
  });
});
