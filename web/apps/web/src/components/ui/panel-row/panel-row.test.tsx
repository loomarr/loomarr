import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { PanelRow } from "./panel-row";

// PanelRow's job is a wrap-not-truncate row. jsdom has no layout, so what is testable is the
// CONTRACT that produces the behaviour: the row carries min-w-0 + flex-wrap (so a flex child can
// shrink and the cluster wraps instead of overflowing), and Main carries min-w-0 + flex-1 (so it
// yields first). Pinning the classes is what stops a refactor from silently reintroducing the
// Firefox clipping this component exists to prevent.
describe("PanelRow", () => {
  it("renders its content", () => {
    render(
      <ul>
        <PanelRow>
          <PanelRow.Main>
            <span>Springfield</span>
          </PanelRow.Main>
          <PanelRow.Meta>
            <span>ok</span>
          </PanelRow.Meta>
        </PanelRow>
      </ul>,
    );
    expect(screen.getByText("Springfield")).toBeInTheDocument();
    expect(screen.getByText("ok")).toBeInTheDocument();
  });

  it("keeps the wrap-not-truncate contract on the row", () => {
    render(
      <ul>
        <PanelRow>
          <span>x</span>
        </PanelRow>
      </ul>,
    );
    const row = screen.getByRole("listitem");
    // The three classes that make content wrap rather than overflow-and-clip.
    expect(row).toHaveClass("min-w-0", "flex-wrap");
  });

  it("lets Main shrink and grow (so it yields before the meta cluster is clipped)", () => {
    render(
      <ul>
        <PanelRow>
          <PanelRow.Main>
            <span>label</span>
          </PanelRow.Main>
        </PanelRow>
      </ul>,
    );
    const main = screen.getByText("label").parentElement;
    expect(main).toHaveClass("min-w-0", "flex-1");
  });
});
