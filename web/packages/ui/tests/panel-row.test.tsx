// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it } from "vitest";

import { ListGroup, PanelRow } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// This package has no DOM lib (it targets React Native too), so the slice of the jsdom document
// the test touches is typed here.
type Node = {
  children: ArrayLike<Node>;
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

// The wrap-not-clip behaviour is layout, which jsdom does not compute; the visual stories cover
// it. What is testable is that each row is one list item holding the identity and its cluster.
describe("PanelRow", () => {
  it("is one list item per row, identity before its cluster", async () => {
    await mount(
      <ListGroup aria-label="Playout" framed={false}>
        <PanelRow key="a" meta={<span>ok</span>}>
          <span>#3 · Cartoon Classics</span>
        </PanelRow>
        <PanelRow key="b">
          <span>#4 · Late Movies</span>
        </PanelRow>
      </ListGroup>,
    );

    const items = doc.body.querySelectorAll('[role="listitem"]');
    expect(items.length).toBe(2);
    expect(items[0]?.textContent).toBe("#3 · Cartoon Classicsok");
    expect(items[1]?.textContent).toBe("#4 · Late Movies");
  });

  it("draws no cluster when there is no meta", async () => {
    await mount(
      <ListGroup>
        <PanelRow>
          <span>Approved a channel</span>
        </PanelRow>
      </ListGroup>,
    );

    const row = doc.body.querySelector('[role="listitem"]')?.children[0];
    expect(row?.children.length).toBe(1);
  });
});
