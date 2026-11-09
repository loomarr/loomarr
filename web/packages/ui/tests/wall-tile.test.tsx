// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import { WallTile } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// This package has no DOM lib (it targets React Native too), so the slice of the jsdom document
// the test touches is typed here.
type Node = {
  click(): void;
  getAttribute(name: string): string | null;
  querySelector(s: string): Node | null;
  tagName: string;
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

describe("WallTile", () => {
  it("is one control holding the number, name, programme and detail", async () => {
    const onClick = vi.fn();
    await mount(
      <WallTile
        art={null}
        channelName="Coast 90s"
        detail="S2 E4 · 13 min left"
        number="07"
        onClick={onClick}
        progressPercent={56}
        title="Coastline Mysteries"
      />,
    );
    const tile = doc.body.querySelector("button") as Node;
    expect(tile.textContent).toContain("07");
    expect(tile.textContent).toContain("Coast 90s");
    expect(tile.textContent).toContain("Coastline Mysteries");
    expect(tile.textContent).toContain("S2 E4 · 13 min left");
    await act(async () => tile.click());
    expect(onClick).toHaveBeenCalledOnce();
  });

  it("renders as the given link", async () => {
    await mount(
      <WallTile
        art={null}
        channelName="Coast 90s"
        number="07"
        render={<a href="/watch/c1">x</a>}
        title="Coastline Mysteries"
      />,
    );
    expect(doc.body.querySelector("a")?.getAttribute("href")).toBe("/watch/c1");
    expect(doc.body.querySelector("button")).toBeNull();
  });

  it("marks the last channel, shorter when compact", async () => {
    await mount(<WallTile art={null} channelName="A" last number="03" title="T" />);
    expect(doc.body.textContent).toContain("You were here");
    doc.body.replaceChildren();
    await mount(<WallTile art={null} channelName="A" compact last number="03" title="T" />);
    expect(doc.body.textContent).toContain("Last");
    expect(doc.body.textContent).not.toContain("You were here");
  });
});
