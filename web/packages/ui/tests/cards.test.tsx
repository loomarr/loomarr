// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import { CardAction, CardStatus, ChannelChip, ListGroup, PictureCard, RowCard } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// This package has no DOM lib (it targets React Native too), so the slice of the jsdom document
// the test touches is typed here.
type Node = {
  click(): void;
  getAttribute(name: string): string | null;
  querySelector(s: string): Node | null;
  querySelectorAll(s: string): ArrayLike<Node>;
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

describe("PictureCard", () => {
  it("shows the chip, the title, one metadata line and one action", async () => {
    const onPress = vi.fn();
    await mount(
      <PictureCard
        action={<CardAction onPress={onPress}>Watch live</CardAction>}
        chip={<ChannelChip name="Late Night Sci-Fi" number="03" />}
        corner="0:30"
        media={<img alt="" src="data:," />}
        meta={["Season 2, episode 5", "9:00 – 9:30 PM"]}
        orientation="top"
        ratio="16/9"
        title="The Lantern Station"
      />,
    );

    expect(doc.body.querySelector("h3")?.textContent).toBe("The Lantern Station");
    const text = doc.body.textContent ?? "";
    expect(text).toContain("03");
    expect(text).toContain("Late Night Sci-Fi");
    expect(text).toContain("Season 2, episode 5·9:00 – 9:30 PM");
    expect(text).toContain("0:30");
    expect(doc.body.querySelectorAll("button").length).toBe(1);

    await act(async () => {
      doc.body.querySelector("button")?.click();
    });
    expect(onPress).toHaveBeenCalledTimes(1);
  });

  it("is an article in a list when laid out on the left", async () => {
    await mount(
      <ListGroup aria-label="Requests">
        <PictureCard
          action={<CardAction attention>Review</CardAction>}
          media={<img alt="" src="data:," />}
          meta={[
            <CardStatus key="s" label="Waiting for approval" tone="attention" />,
            "Requested 2 days ago",
          ]}
          orientation="left"
          ratio="2/3"
          title="The Copper Hills"
        />
      </ListGroup>,
    );

    const item = doc.body.querySelector('[role="listitem"]');
    expect(item?.querySelector('[role="article"]')).not.toBeNull();
    expect(item?.textContent).toContain("Waiting for approval·Requested 2 days ago");
  });
});

describe("RowCard", () => {
  it("shows the lead, the title, one metadata line and one action", async () => {
    await mount(
      <ListGroup aria-label="People">
        <RowCard
          action={<CardAction>Manage</CardAction>}
          lead={<span>A</span>}
          meta={["Admin", "seen today"]}
          title="Ada Marlowe"
        />
      </ListGroup>,
    );

    const text = doc.body.textContent ?? "";
    expect(text).toContain("Ada Marlowe");
    expect(text).toContain("Admin·seen today");
    expect(doc.body.querySelectorAll("button").length).toBe(1);
  });
});
