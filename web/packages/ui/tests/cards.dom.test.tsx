// @vitest-environment jsdom

import { act, cleanup, renderUi } from "@loomarr/test-support";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { CardAction, CardStatus, ChannelChip, ListGroup, PictureCard, RowCard } from "../index";

const mount = async (node: ReactNode) => {
  await act(async () => {
    renderUi(node);
  });
};
afterEach(cleanup);

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

    expect(document.body.querySelector("h3")?.textContent).toBe("The Lantern Station");
    const text = document.body.textContent ?? "";
    expect(text).toContain("03");
    expect(text).toContain("Late Night Sci-Fi");
    expect(text).toContain("Season 2, episode 5·9:00 – 9:30 PM");
    expect(text).toContain("0:30");
    expect(document.body.querySelectorAll("button").length).toBe(1);

    await act(async () => {
      document.body.querySelector("button")?.click();
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

    const item = document.body.querySelector('[role="listitem"]');
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

    const text = document.body.textContent ?? "";
    expect(text).toContain("Ada Marlowe");
    expect(text).toContain("Admin·seen today");
    expect(document.body.querySelectorAll("button").length).toBe(1);
  });
});
