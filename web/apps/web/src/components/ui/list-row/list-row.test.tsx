import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Button } from "../button";
import { ListGroup, ListRow } from "./list-row";

describe("ListRow", () => {
  it("is announced as a list of rows", () => {
    render(
      <ListGroup aria-label="On the way">
        <ListRow tone="progress" title="A sci-fi anthology" sub="For the late-night channel" />
        <ListRow tone="progress" title="A horror anthology" />
      </ListGroup>,
    );
    const list = screen.getByRole("list", { name: "On the way" });
    expect(within(list).getAllByRole("listitem")).toHaveLength(2);
  });

  it("names its progress bar and shows the eta beside it", () => {
    render(
      <ListGroup>
        <ListRow
          tone="progress"
          title="A sci-fi anthology"
          progress={{ value: 22, label: "Downloading", eta: "about 40 min" }}
        />
      </ListGroup>,
    );
    expect(screen.getByRole("progressbar", { name: "Downloading" })).toHaveAttribute("aria-valuenow", "22");
    expect(screen.getByText("about 40 min")).toBeInTheDocument();
  });

  // axe `nested-interactive` failed 45 nodes in the mock (#1659): cards and rows that were buttons
  // holding buttons. The row is never interactive, so its action is the only control in it.
  it("keeps its action as the row's only control", () => {
    render(
      <ListGroup>
        <ListRow
          tone="error"
          title="A game-show request"
          sub="Couldn't be built: only 2 titles found"
          action={
            <Button size="sm" variant="outline">
              Edit and retry
            </Button>
          }
        />
      </ListGroup>,
    );
    const row = screen.getByRole("listitem");
    expect(within(row).getAllByRole("button")).toHaveLength(1);
    expect(row.closest("button")).toBeNull();
  });

  it("hides the dot unless it adds something the title does not say", () => {
    render(
      <ListGroup>
        <ListRow tone="attention" title="A mystery channel" />
        <ListRow tone="attention" toneLabel="Waiting for an admin" title="A space channel" />
      </ListGroup>,
    );
    expect(screen.getAllByRole("img")).toHaveLength(1);
    expect(screen.getByRole("img", { name: "Waiting for an admin" })).toBeInTheDocument();
  });
});
