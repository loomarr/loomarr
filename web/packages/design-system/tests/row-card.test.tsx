// @vitest-environment jsdom

import { act, cleanup, render } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { LoomarrProvider } from "../index";
import { CardAction, ListGroup, RowCard } from "../rows";

const mount = async (node: ReactNode) => {
  await act(async () => {
    render(<LoomarrProvider theme="dark">{node}</LoomarrProvider>);
  });
};
afterEach(cleanup);

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
