// @vitest-environment jsdom

import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { LoomarrProvider } from "../index";
import { ListGroup, PanelRow } from "../rows";

afterEach(cleanup);

const renderUi = (node: ReactNode) => render(<LoomarrProvider theme="dark">{node}</LoomarrProvider>);

// The wrap-not-clip behaviour is layout, which jsdom does not compute; the visual stories cover
// it. What is testable is that each row is one list item holding the identity and its cluster.
describe("PanelRow", () => {
  it("is one list item per row, identity before its cluster", () => {
    renderUi(
      <ListGroup aria-label="Playout" framed={false}>
        <PanelRow key="a" meta={<span>ok</span>}>
          <span>#3 · Cartoon Classics</span>
        </PanelRow>
        <PanelRow key="b">
          <span>#4 · Late Movies</span>
        </PanelRow>
      </ListGroup>,
    );

    const items = screen.getAllByRole("listitem");
    expect(items.length).toBe(2);
    expect(items[0]?.textContent).toBe("#3 · Cartoon Classicsok");
    expect(items[1]?.textContent).toBe("#4 · Late Movies");
  });

  it("draws no cluster when there is no meta", () => {
    renderUi(
      <ListGroup>
        <PanelRow>
          <span>Approved a channel</span>
        </PanelRow>
      </ListGroup>,
    );

    const row = screen.getByRole("listitem").children[0];
    expect(row?.children.length).toBe(1);
  });
});
