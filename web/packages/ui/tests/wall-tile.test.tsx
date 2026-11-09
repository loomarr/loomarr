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
  parentElement: Node | null;
  querySelector(s: string): Node | null;
  tagName: string;
  textContent: string | null;
};
type Doc = {
  body: Node & {
    append(node: unknown): void;
    querySelectorAll(s: string): ArrayLike<Node>;
    replaceChildren(): void;
  };
  createElement(tag: string): Parameters<typeof createRoot>[0];
};
const doc = (globalThis as unknown as { document: Doc }).document;
const computed = (node: Node) =>
  (
    globalThis as unknown as {
      getComputedStyle(n: Node): { backgroundColor: string; color: string };
    }
  ).getComputedStyle(node);

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

  // jsdom reports a theme colour as the CSS variable it reads (and lower-cases the name).
  const numberColours = () => {
    const number = Array.from(doc.body.querySelectorAll("span")).find((n) => n.textContent === "07") as Node;
    return {
      fill: computed(number.parentElement as Node).backgroundColor.toLowerCase(),
      ink: computed(number).color.toLowerCase(),
    };
  };

  it("fills the number block with the channel's decade colour, the number in the decade ink", async () => {
    await mount(<WallTile art={null} channelName="A" decade={1980} number="07" title="T" />);
    expect(numberColours()).toEqual({ fill: "var(--decade1980)", ink: "var(--decadeink)" });
  });

  it("keeps the number block neutral without a decade, or for a decade with no colour", async () => {
    for (const decade of [undefined, 2000]) {
      doc.body.replaceChildren();
      await mount(
        <WallTile
          art={null}
          channelName="A"
          number="07"
          title="T"
          {...(decade === undefined ? {} : { decade })}
        />,
      );
      expect(numberColours()).toEqual({ fill: "var(--artworkscrimstrong)", ink: "var(--contentprimary)" });
    }
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

describe("WallTile in the light theme", () => {
  it("keeps its words in the dark scope, light on the scrim, as the player does", async () => {
    const container = doc.createElement("div");
    doc.body.append(container);
    const root = createRoot(container);
    await act(async () => {
      root.render(
        <LoomarrProvider theme="light">
          <WallTile art={null} channelName="Coast 90s" number="07" title="Coastline Mysteries" />
        </LoomarrProvider>,
      );
    });
    const title = doc.body.querySelector("h3") as Node;
    let scope: Node | null = title;
    while (scope && !(scope.getAttribute("class") ?? "").includes("t_dark")) scope = scope.parentElement;
    expect(scope).not.toBeNull();
  });
});
