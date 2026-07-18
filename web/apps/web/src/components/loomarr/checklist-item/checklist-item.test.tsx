import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ChecklistItem } from "./checklist-item";

describe("ChecklistItem", () => {
  it("shows the hint and a doc deep-link only on failure", () => {
    render(
      <ChecklistItem name="Tunarr" status="fail" hint="Could not reach Tunarr" docHref="/help#tunarr" />,
    );
    expect(screen.getByText("Could not reach Tunarr")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /fix/i })).toHaveAttribute("href", "/help#tunarr");
  });

  it("hides the hint on pass and exposes an accessible status", () => {
    render(<ChecklistItem name="TMDB" status="pass" hint="unused" docHref="/help#tmdb" />);
    expect(screen.queryByText("unused")).not.toBeInTheDocument();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    expect(screen.getByRole("img", { name: "Passed" })).toBeInTheDocument();
  });
});
