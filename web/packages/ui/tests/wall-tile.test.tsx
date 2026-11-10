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

  it("names who is watching in a pill, and reads the full words on the compact tile", async () => {
    await mount(
      <WallTile art={null} channelName="Coast 90s" number="07" title="Coastline" watchers={["Sam", "Jo"]} />,
    );
    expect(doc.body.textContent).toContain("Sam, Jo watching");
    doc.body.replaceChildren();
    await mount(
      <WallTile
        art={null}
        channelName="Coast 90s"
        compact
        number="07"
        title="Coastline"
        watchers={["Sam", "Jo"]}
      />,
    );
    expect(doc.body.textContent).toContain("S J");
    expect(doc.body.textContent).toContain("Sam, Jo watching");
  });

  it("shows no pill without watchers, and the last-tuned tile keeps its own pill", async () => {
    await mount(<WallTile art={null} channelName="Coast 90s" number="07" title="Coastline" />);
    expect(doc.body.textContent).not.toContain("watching");
    doc.body.replaceChildren();
    await mount(
      <WallTile art={null} channelName="Coast 90s" last number="07" title="Coastline" watchers={["Sam"]} />,
    );
    expect(doc.body.textContent).toContain("You were here");
    expect(doc.body.textContent).not.toContain("watching");
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

const oneLine = /(?=.*\b_to-)(?=.*\b_ws-)/;

describe("WallTile over a picture (#3407)", () => {
  const Still = () => <div data-testid="still" />;
  const nameOf = (text: string) =>
    Array.from(doc.body.querySelectorAll("*")).find(
      (n) => n.textContent === text && n.getAttribute("data-testid") === null && !n.querySelector("*"),
    ) as Node;

  it("draws a top scrim behind the number and name whenever it has a picture", async () => {
    await mount(<WallTile art={<Still />} channelName="Coast 90s" number="7" title="Coastline" />);
    expect(doc.body.querySelector('[data-testid="wall-tile-top-scrim"]')).not.toBeNull();
    doc.body.replaceChildren();
    await mount(<WallTile art={null} channelName="Coast 90s" number="7" title="Coastline" />);
    expect(doc.body.querySelector('[data-testid="wall-tile-top-scrim"]')).toBeNull();
  });

  it("sets the channel name in the light over-picture tone, not the muted one", async () => {
    await mount(<WallTile art={<Still />} channelName="Coast 90s" number="7" title="Coastline" />);
    expect(computed(nameOf("Coast 90s")).color.toLowerCase()).toBe("var(--contentprimary)");
  });

  it("keeps the name on one truncating line so the last-watched pill never covers it", async () => {
    await mount(
      <WallTile art={<Still />} channelName="A very long channel name indeed" last number="7" title="T" />,
    );
    const name = nameOf("A very long channel name indeed");
    // numberOfLines={1} is Tamagui's text-overflow ellipsis (`_to-`) on a no-wrap line (`_ws-`).
    expect(name.getAttribute("class") ?? "").toMatch(oneLine);
    // The pill is a sibling of the name in the same row, so a long name shrinks rather than runs under it.
    expect((nameOf("You were here").parentElement as Node).parentElement).toBe(name.parentElement);
  });
});

describe("WallTile compact (#3407)", () => {
  it("follows the mock: the number alone on the top row, the title on one line, one caption line with the name first", async () => {
    await mount(
      <WallTile
        art={<div data-testid="still" />}
        channelName="Nature Hour"
        compact
        detail="13 min left"
        last
        number="12"
        title="Heron Country"
      />,
    );
    const tile = doc.body.querySelector("button") as Node;
    const text = tile.textContent ?? "";
    const number = Array.from(doc.body.querySelectorAll("span")).find((n) => n.textContent === "12") as Node;
    const row = (number.parentElement as Node).parentElement as Node;
    // The top row holds the number block and the "Last" pill, and no channel name or title.
    expect(row.textContent).toContain("12");
    expect(row.textContent).toContain("Last");
    expect(row.textContent).not.toContain("Nature Hour");
    expect(row.textContent).not.toContain("Heron Country");
    // The title is one clipped line (an h3, so the clip is inline style).
    const title = doc.body.querySelector("h3") as Node;
    expect(title.getAttribute("style") ?? "").toMatch(/white-space:\s*nowrap/);
    expect(title.getAttribute("style") ?? "").toMatch(/text-overflow:\s*ellipsis/);
    // Exactly one caption line, truncating, carrying the name first and then the time left.
    const caption = Array.from(doc.body.querySelectorAll("*")).filter(
      (n) => !n.querySelector("*") && n.textContent === "Nature Hour · 13 min left",
    );
    expect(caption).toHaveLength(1);
    expect(caption[0]?.getAttribute("class") ?? "").toMatch(oneLine);
    expect(text.split("Nature Hour").length - 1).toBe(1);
    expect(text.indexOf("Heron Country")).toBeLessThan(text.indexOf("Nature Hour"));
    // WallTile never draws a monogram itself: the picture slot is the only thing behind the words.
    expect(text).not.toMatch(/\bNH\b/);
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
