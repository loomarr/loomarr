import { render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ListGroup } from "../list-row";
import { HighlightRow } from "./highlight-row";

describe("HighlightRow", () => {
  it("is one control per list item, named by everything it shows", () => {
    const onClick = vi.fn();
    render(
      <ListGroup aria-label="Tonight">
        <HighlightRow
          time="9:04 PM"
          channelNumber="07"
          title="A sci-fi drama"
          detail=" · Pilot"
          reason="Season 4 premiere"
          onClick={onClick}
        />
      </ListGroup>,
    );
    const item = within(screen.getByRole("list", { name: "Tonight" })).getByRole("listitem");
    const control = within(item).getByRole("button");
    expect(control).toHaveAccessibleName(/9:04 PM.*07.*A sci-fi drama ?· Pilot.*Season 4 premiere/);
    control.click();
    expect(onClick).toHaveBeenCalledOnce();
  });

  // Tonight's rows open a channel: a real link, so middle-click and "open in new tab" work.
  it("renders as the link it is given", () => {
    render(
      <ListGroup>
        <HighlightRow
          time="10:04 PM"
          channelNumber="07"
          title="A space horror film"
          render={<a href="/watch/7" />}
        />
      </ListGroup>,
    );
    expect(screen.getByRole("link")).toHaveAttribute("href", "/watch/7");
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
});
