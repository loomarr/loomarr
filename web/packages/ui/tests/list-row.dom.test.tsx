// @vitest-environment jsdom

import { ListGroup } from "@loomarr/design-system/rows";
import { act, cleanup, renderUi } from "@loomarr/test-support";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { HighlightRow } from "../index";

const doc = document;

const mount = async (node: ReactNode) => {
  await act(async () => {
    renderUi(node);
  });
};
afterEach(cleanup);

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
