// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import { HighlightRow, ListGroup, ListRow } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// This package has no DOM lib (it targets React Native too), so the slice of the jsdom document
// the test touches is typed here.
type Node = {
  click(): void;
  closest(s: string): Node | null;
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

describe("ListRow", () => {
  it("is announced as a list of rows", async () => {
    await mount(
      <ListGroup aria-label="On the way">
        <ListRow sub="For the late-night channel" title="A sci-fi anthology" tone="progress" />
        <ListRow title="A horror anthology" tone="progress" />
      </ListGroup>,
    );

    const list = doc.body.querySelector('[role="list"]');
    expect(list?.getAttribute("aria-label")).toBe("On the way");
    expect(list?.querySelectorAll('[role="listitem"]').length).toBe(2);
  });

  it("names its progress bar and shows the eta beside it", async () => {
    await mount(
      <ListGroup>
        <ListRow
          progress={{ eta: "about 40 min", label: "Downloading", value: 22 }}
          title="A sci-fi anthology"
          tone="progress"
        />
      </ListGroup>,
    );

    const bar = doc.body.querySelector('[role="progressbar"]');
    expect(bar?.getAttribute("aria-label")).toBe("Downloading");
    expect(bar?.getAttribute("aria-valuenow")).toBe("22");
    expect(doc.body.textContent).toContain("about 40 min");
  });

  // axe `nested-interactive` failed 45 nodes in the mock (#1659): cards and rows that were buttons
  // holding buttons. The row is never interactive, so its action is the only control in it.
  it("keeps its action as the row's only control", async () => {
    await mount(
      <ListGroup>
        <ListRow
          action={<button type="button">Edit and retry</button>}
          sub="Couldn't be built: only 2 titles found"
          title="A game-show request"
          tone="error"
        />
      </ListGroup>,
    );

    const row = doc.body.querySelector('[role="listitem"]');
    expect(row?.querySelectorAll("button").length).toBe(1);
    expect(row?.closest("button")).toBeNull();
  });

  it("hides the dot unless it adds something the title does not say", async () => {
    await mount(
      <ListGroup>
        <ListRow title="A mystery channel" tone="attention" />
        <ListRow title="A space channel" tone="attention" toneLabel="Waiting for an admin" />
      </ListGroup>,
    );

    const dots = doc.body.querySelectorAll('[role="img"]');
    expect(dots.length).toBe(1);
    expect(dots[0]?.getAttribute("aria-label")).toBe("Waiting for an admin");
  });
});

describe("HighlightRow (web)", () => {
  it("is one control per list item that shows everything and runs its action", async () => {
    const onClick = vi.fn();
    await mount(
      <ListGroup aria-label="Tonight">
        <HighlightRow
          channelNumber="07"
          detail=" · Pilot"
          onClick={onClick}
          reason="Season 4 premiere"
          time="9:04 PM"
          title="A sci-fi drama"
        />
      </ListGroup>,
    );

    const item = doc.body.querySelector('[role="listitem"]');
    const controls = item?.querySelectorAll("button");
    expect(controls?.length).toBe(1);
    expect(controls?.[0]?.textContent).toBe("9:04 PM07A sci-fi drama · PilotSeason 4 premiere");
    await act(async () => controls?.[0]?.click());
    expect(onClick).toHaveBeenCalledOnce();
  });

  // Tonight's rows open a channel: a real link, so middle-click and "open in new tab" work.
  it("renders as the link it is given", async () => {
    await mount(
      <ListGroup>
        <HighlightRow
          channelNumber="07"
          render={<a href="/watch/7">placeholder</a>}
          time="10:04 PM"
          title="A space horror film"
        />
      </ListGroup>,
    );

    expect(doc.body.querySelector("a")?.getAttribute("href")).toBe("/watch/7");
    expect(doc.body.querySelector("button")).toBeNull();
  });
});
